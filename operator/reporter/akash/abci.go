package akash

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// abciConn follows SDK client.Context's generated-client adapter, but keeps the
// caller's context, reporter request limits and height checks on error responses.
type abciConn struct {
	client *Client
}

var _ grpc.ClientConnInterface = (*abciConn)(nil)

func (a *abciConn) Invoke(ctx context.Context, method string, req, reply any, opts ...grpc.CallOption) error {
	return a.client.intercept(ctx, method, req, reply, nil, func(ctx context.Context, method string, req, reply any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
		return a.query(ctx, method, req, reply, opts...)
	}, opts...)
}

func (*abciConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, status.Error(codes.Unimplemented, "ABCI does not support streams")
}

func (a *abciConn) query(ctx context.Context, method string, req, reply any, opts ...grpc.CallOption) error {
	if method == "/cosmos.tx.v1beta1.Service/Simulate" {
		return a.simulate(ctx, req, reply, opts...)
	}

	var height int64
	md, _ := metadata.FromOutgoingContext(ctx)
	if values := md.Get(grpctypes.GRPCBlockHeightHeader); len(values) > 0 {
		var err error
		if len(values) != 1 {
			return errors.New("ABCI requires one query height")
		}

		height, err = strconv.ParseInt(values[0], 10, 64)
		if err != nil || height <= 0 {
			return errors.New("ABCI query height must be positive")
		}
	}

	cdc := codec.NewProtoCodec(a.client.registry).GRPCCodec()
	data, err := cdc.Marshal(req)
	if err != nil {
		return errors.New("cannot encode ABCI query")
	}

	if len(data) > maxRecvBytes {
		return status.Error(codes.ResourceExhausted, "ABCI request exceeds size limit")
	}

	result, err := a.client.rpc.ABCIQueryWithOptions(ctx, method, data, rpcclient.ABCIQueryOptions{
		Height: height,
		Prove:  false,
	})
	if err != nil {
		return err
	}

	response := result.Response
	if response.Height == 0 && response.Codespace == sdkerrors.RootCodespace && response.Code == sdkerrors.ErrUnknownRequest.ABCICode() {
		return fmt.Errorf("ABCI route %s is unavailable; the native node must register this service", method)
	}
	if response.Height <= 0 || (height > 0 && response.Height != height) {
		return errors.New("ABCI query height mismatch")
	}

	header := metadata.Pairs(grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(response.Height, 10))
	for _, opt := range opts {
		if h, ok := opt.(grpc.HeaderCallOption); ok && h.HeaderAddr != nil {
			*h.HeaderAddr = header
		}
	}

	if response.Code != 0 {
		// BaseApp maps gRPC errors into SDK query codes. Match the codespace too:
		// an unrelated application's numeric code must never become absence.
		if response.Codespace != sdkerrors.RootCodespace {
			return errors.New("ABCI query failed with an unrecognized codespace")
		}

		code := codes.Unknown
		switch response.Code {
		case sdkerrors.ErrInvalidRequest.ABCICode():
			code = codes.InvalidArgument
		case sdkerrors.ErrUnauthorized.ABCICode():
			code = codes.Unauthenticated
		case sdkerrors.ErrKeyNotFound.ABCICode():
			code = codes.NotFound
		}

		return status.Error(code, response.Log)
	}

	if len(response.Value) > maxRecvBytes {
		return status.Error(codes.ResourceExhausted, "ABCI response exceeds size limit")
	}

	if err := cdc.Unmarshal(response.Value, reply); err != nil {
		return errors.New("invalid ABCI protobuf response")
	}

	return codectypes.UnpackInterfaces(reply, a.client.registry)
}

// simulate uses BaseApp's raw route, which is available without the optional tx service.
// Both routes execute current state; echoed query heights do not pin simulation state.
func (a *abciConn) simulate(ctx context.Context, req, reply any, opts ...grpc.CallOption) error {
	md, _ := metadata.FromOutgoingContext(ctx)
	if _, supplied := md[grpctypes.GRPCBlockHeightHeader]; supplied {
		return errors.New("ABCI simulation is latest-state only; query height metadata is unsupported")
	}

	request, ok := req.(*txtypes.SimulateRequest)
	if !ok || request == nil || request.Tx != nil || request.TxBytes == nil {
		return errors.New("ABCI simulation requires transaction bytes")
	}

	out, ok := reply.(*txtypes.SimulateResponse)
	if !ok || out == nil {
		return errors.New("invalid ABCI simulation response type")
	}

	if len(request.TxBytes) > maxRecvBytes {
		return status.Error(codes.ResourceExhausted, "ABCI request exceeds size limit")
	}

	result, err := a.client.rpc.ABCIQueryWithOptions(ctx, "/app/simulate", request.TxBytes, rpcclient.ABCIQueryOptions{
		Height: 0,
		Prove:  false,
	})
	if err != nil {
		return err
	}

	response := result.Response
	// Raw execution failures have no response height or gas info. Only a valid
	// native answer is deterministic for Prune; malformed replies remain retryable.
	if response.Code != 0 {
		if response.Codespace == "" || response.Log == "" || len(response.Value) != 0 {
			return errors.New("invalid ABCI simulation error response")
		}

		return status.Errorf(codes.Unknown, "ABCI simulation failed (%s/%d): %s", response.Codespace, response.Code, response.Log)
	}

	if response.Log != "" {
		return errors.New("invalid ABCI simulation success response")
	}
	if response.Height <= 0 {
		return errors.New("ABCI query height mismatch")
	}
	if len(response.Value) > maxRecvBytes {
		return status.Error(codes.ResourceExhausted, "ABCI response exceeds size limit")
	}

	simulation, err := decodeSimulationJSON(response.Value)
	if err != nil {
		return err
	}

	*out = txtypes.SimulateResponse{
		GasInfo: &simulation.GasInfo,
		Result:  simulation.Result,
	}

	header := metadata.Pairs(grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(response.Height, 10))
	for _, opt := range opts {
		if h, ok := opt.(grpc.HeaderCallOption); ok && h.HeaderAddr != nil {
			*h.HeaderAddr = header
		}
	}

	return nil
}

// decodeSimulationJSON validates gas presence before the SDK decodes its non-pointer fields.
func decodeSimulationJSON(data []byte) (simulation sdk.SimulationResponse, err error) {
	var fields map[string]*json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation JSON response")
	}
	gasInfo := simulationJSONField(fields, "gas_info", "gasInfo")
	if gasInfo == nil {
		return sdk.SimulationResponse{}, errors.New("simulation returned incomplete gas info")
	}

	var gasFields map[string]*json.RawMessage
	if err := json.Unmarshal(*gasInfo, &gasFields); err != nil {
		return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation JSON response")
	}
	for _, names := range [][2]string{{"gas_wanted", "gasWanted"}, {"gas_used", "gasUsed"}} {
		value := simulationJSONField(gasFields, names[0], names[1])
		if value == nil {
			return sdk.SimulationResponse{}, errors.New("simulation returned incomplete gas info")
		}

		// jsonpb strips quotes from numeric strings before decoding. A pointer
		// also rejects quoted null, which otherwise silently leaves uint64 zero.
		raw := *value
		if len(raw) >= 2 && raw[0] == '"' {
			raw = raw[1 : len(raw)-1]
		}
		var amount *uint64
		if err := json.Unmarshal(raw, &amount); err != nil || amount == nil {
			return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation gas info")
		}
	}

	// Pass the same effective gas object to jsonpb, retaining Result and unknown
	// fields so the SDK still decodes Any values and rejects invalid fields.
	*gasInfo, err = json.Marshal(gasFields)
	if err != nil {
		return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation JSON response")
	}
	data, err = json.Marshal(fields)
	if err != nil {
		return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation JSON response")
	}

	// The pinned jsonpb Any decoder dereferences null well-known values. Contain
	// panics only at this untrusted parser call, never around client execution.
	defer func() {
		if recover() != nil {
			simulation = sdk.SimulationResponse{}
			err = errors.New("invalid ABCI simulation JSON response")
		}
	}()
	simulation, err = authclient.ParseQueryResponse(data)
	if err != nil {
		return sdk.SimulationResponse{}, errors.New("invalid ABCI simulation JSON response")
	}
	return simulation, nil
}

// simulationJSONField normalizes jsonpb's exact field names: the last duplicate
// wins in the map, and a camelCase alias takes precedence even when it is null.
func simulationJSONField(fields map[string]*json.RawMessage, name, alias string) *json.RawMessage {
	if value, ok := fields[alias]; ok {
		fields[name] = value
		delete(fields, alias)
	}
	return fields[name]
}

// legacyComet retains the configured target: Comet's GetTrimmedURL omits RawQuery.
// Restore only the initial request target; subsequent requests follow the redirect location.
func legacyComet(raw string, transport *http.Transport) (*rpchttp.HTTP, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Host == "" || target.User != nil || target.Fragment != "" || (target.Scheme != "http" && target.Scheme != "https") {
		return nil, errors.New("invalid legacy RPC URL; use http/https without credentials or fragment")
	}

	client := &http.Client{
		Transport: &legacyTarget{
			target:    *target,
			transport: transport,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Del("Referer")
			if len(via) > 20 {
				return errors.New("stopped after 20 redirects")
			}

			if len(via) == 1 {
				location, err := url.Parse(req.Response.Header.Get("Location"))
				if err != nil {
					return errors.New("invalid RPC redirect")
				}

				req.URL = target.ResolveReference(location)
			}

			if req.URL.User != nil {
				return errors.New("redirect URL credentials are not supported")
			}

			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("invalid redirect scheme")
			}

			return nil
		},
	}

	rpc, err := rpchttp.NewWithClient(raw, "/websocket", client)
	if err != nil {
		return nil, errors.New("cannot create legacy Comet client")
	}

	return rpc, nil
}

type legacyTarget struct {
	target    url.URL
	transport http.RoundTripper
}

// JSON encodes protobuf response values as base64. Bound the envelope as well as
// the decoded message, allowing base64 expansion and at most 1 MiB of metadata.
const maxABCIEnvelope = (maxRecvBytes+2)/3*4 + (1 << 20)

func (t *legacyTarget) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if req.Response == nil {
		target := t.target
		clone.URL = &target
		clone.Host = target.Host
	}

	response, err := t.transport.RoundTrip(clone)
	if err != nil {
		if req.Context().Err() != nil {
			return nil, req.Context().Err()
		}

		return nil, errors.New("legacy RPC transport failed")
	}

	if response.StatusCode >= http.StatusBadRequest {
		response.Body.Close()
		return nil, errors.New("legacy RPC HTTP status indicates failure")
	}

	if response.ContentLength > maxABCIEnvelope {
		response.Body.Close()
		return nil, status.Error(codes.ResourceExhausted, "RPC response exceeds size limit")
	}

	response.Body = &boundedRPCBody{
		ReadCloser: response.Body,
		remaining:  maxABCIEnvelope,
	}

	return response, nil
}

type boundedRPCBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedRPCBody) Read(p []byte) (int, error) {
	if b.remaining < 0 {
		return 0, status.Error(codes.ResourceExhausted, "RPC response exceeds size limit")
	}

	if int64(len(p)) > b.remaining+1 {
		p = p[:b.remaining+1]
	}

	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	if b.remaining < 0 {
		return 0, status.Error(codes.ResourceExhausted, "RPC response exceeds size limit")
	}

	return n, err
}

// HTTP errors may include a redirect Location or endpoint query credentials.
// Preserve cancellation for callers without retaining a credential-bearing cause.
func sanitizeLegacyHTTP(err error) error {
	if err == nil {
		return nil
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}

		if errors.Is(err, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}

		return errors.New("legacy RPC HTTP request failed")
	}

	return err
}

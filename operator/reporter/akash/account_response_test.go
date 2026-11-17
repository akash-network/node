package akash

import (
	"context"
	"math"
	"net"
	"net/http"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type accountResponseClient struct {
	authtypes.QueryClient
	response *authtypes.QueryAccountResponse
	err      error
}

func (c accountResponseClient) Account(context.Context, *authtypes.QueryAccountRequest, ...grpc.CallOption) (*authtypes.QueryAccountResponse, error) {
	return c.response, c.err
}

type accountResponseServer struct {
	authtypes.UnimplementedQueryServer
	response *authtypes.QueryAccountResponse
	err      error
}

func (s accountResponseServer) Account(context.Context, *authtypes.QueryAccountRequest) (*authtypes.QueryAccountResponse, error) {
	return s.response, s.err
}

func grpcAccountResponse(t *testing.T, response *authtypes.QueryAccountResponse, queryErr error) *Client {
	t.Helper()
	c := &Client{
		registry: interfaceRegistry(),
		reads:    semaphore.NewWeighted(1),
	}
	var lc net.ListenConfig
	listener, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.ForceServerCodec(codec.NewProtoCodec(c.registry).GRPCCodec()))
	authtypes.RegisterQueryServer(server, &accountResponseServer{
		response: response,
		err:      queryErr,
	})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(codec.NewProtoCodec(c.registry).GRPCCodec())),
		grpc.WithUnaryInterceptor(c.intercept),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	c.conn, c.queries = conn, conn
	c.auth = authtypes.NewQueryClient(conn)
	return c
}

func TestAccountNumberResponseBoundary(t *testing.T) {
	for _, transport := range []string{"stub", "grpc", "abci"} {
		for _, name := range []string{"nil-response", "missing-account", "empty-any", "unknown-any", "malformed-any", "base-zero", "base-max", "vesting", "not-found", "invalid-request", "unavailable"} {
			t.Run(transport+"/"+name, func(t *testing.T) {
				response := &authtypes.QueryAccountResponse{}
				var queryErr error
				var want uint64
				var valid bool
				switch name {
				case "nil-response":
					response = nil
				case "empty-any":
					response.Account = &codectypes.Any{}
				case "unknown-any":
					response.Account = &codectypes.Any{TypeUrl: "/unknown.Account"}
				case "malformed-any":
					response.Account = &codectypes.Any{
						TypeUrl: "/cosmos.auth.v1beta1.BaseAccount",
						Value:   []byte{0xff},
					}
				case "base-zero", "base-max", "vesting":
					valid = true
					if name == "base-max" {
						want = math.MaxUint64
					} else if name == "vesting" {
						want = 190
					}

					var account proto.Message = &authtypes.BaseAccount{AccountNumber: want}
					if name == "vesting" {
						account = &vestingtypes.BaseVestingAccount{
							BaseAccount: &authtypes.BaseAccount{AccountNumber: want},
						}
					}
					value, err := codectypes.NewAnyWithValue(account)
					require.NoError(t, err)
					// Avoid the constructor's cached value, exercising actual Any decoding.
					response.Account = &codectypes.Any{
						TypeUrl: value.TypeUrl,
						Value:   value.Value,
					}
				case "not-found":
					queryErr = status.Error(codes.NotFound, "account not found")
				case "invalid-request":
					queryErr = status.Error(codes.InvalidArgument, "invalid account request")
				case "unavailable":
					queryErr = status.Error(codes.Unavailable, "account transport unavailable")
				}

				var c *Client
				switch transport {
				case "stub":
					c = &Client{
						auth: accountResponseClient{
							response: response,
							err:      queryErr,
						},
						registry: interfaceRegistry(),
					}
				case "grpc":
					c = grpcAccountResponse(t, response, queryErr)
				case "abci":
					var value []byte
					if response != nil {
						var err error
						value, err = proto.Marshal(response)
						require.NoError(t, err)
					}
					c = mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
						require.Equal(t, "/cosmos.auth.v1beta1.Query/Account", wire.Path)
						result := abci.ResponseQuery{
							Height: 42,
							Value:  value,
						}
						if queryErr != nil {
							result.Value = nil
							result.Codespace = sdkerrors.RootCodespace
							result.Code = sdkerrors.ErrUnknownRequest.ABCICode()
							result.Log = queryErr.Error()
							if name == "not-found" {
								result.Code = sdkerrors.ErrKeyNotFound.ABCICode()
							} else if name == "invalid-request" {
								result.Code = sdkerrors.ErrInvalidRequest.ABCICode()
							}
						}
						return result
					})
				}

				var got uint64
				var err error
				require.NotPanics(t, func() {
					got, err = c.AccountNumber(t.Context(), sdk.AccAddress(make([]byte, 20)))
				})
				if valid {
					require.NoError(t, err)
					require.Equal(t, want, got)
					return
				}
				require.Error(t, err)
				require.Zero(t, got)
				if name == "not-found" {
					require.ErrorIs(t, err, ErrAccountNotFound)
					return
				}
				require.NotErrorIs(t, err, ErrAccountNotFound)
				require.NotEqual(t, codes.NotFound, status.Code(err))
				if name == "nil-response" || name == "missing-account" || name == "empty-any" {
					require.EqualError(t, err, "account query returned no account")
				}
				if name == "invalid-request" {
					require.Equal(t, codes.InvalidArgument, status.Code(err))
				}
				if transport == "stub" && queryErr != nil {
					require.Same(t, queryErr, err)
				}
			})
		}
	}
}

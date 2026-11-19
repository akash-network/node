package akash

import (
	"net/http"
	"sync/atomic"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authclient "github.com/cosmos/cosmos-sdk/x/auth/client"
	"github.com/cosmos/gogoproto/proto"
	gogotypes "github.com/cosmos/gogoproto/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/status"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
)

func TestLegacySimulationEffectiveGas(t *testing.T) {
	for _, tc := range []struct {
		name  string
		body  string
		valid bool
		used  uint64
	}{
		{
			name:  "camel names only",
			body:  `{"gasInfo":{"gasWanted":"10","gasUsed":"7"}}`,
			valid: true,
			used:  7,
		},
		{
			name:  "camel container wins before snake",
			body:  `{"gasInfo":{"gasWanted":"10","gasUsed":"7"},"gas_info":null}`,
			valid: true,
			used:  7,
		},
		{
			name:  "camel container wins after snake",
			body:  `{"gas_info":{"gas_wanted":"10"},"gasInfo":{"gasWanted":"10","gasUsed":"7"}}`,
			valid: true,
			used:  7,
		},
		{
			name: "null camel container before complete snake",
			body: `{"gasInfo":null,"gas_info":{"gas_wanted":"10","gas_used":"7"}}`,
		},
		{
			name: "missing effective camel gas",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gasInfo":{"gasWanted":"10"}}`,
		},
		{
			name: "wrong case container",
			body: `{"GAS_INFO":{"gas_wanted":"10","gas_used":"7"}}`,
		},
		{
			name: "wrong case container alongside valid one",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"GasInfo":null}`,
		},
		{
			name: "wrong case scalar",
			body: `{"gas_info":{"gas_wanted":"10","GAS_USED":"7"}}`,
		},
		{
			name: "unknown scalar alongside valid one",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7","GasUsed":null}}`,
		},
		{
			name:  "duplicate container final complete",
			body:  `{"gas_info":{"gas_wanted":"10"},"gas_info":{"gas_wanted":"10","gas_used":"7"}}`,
			valid: true,
			used:  7,
		},
		{
			name:  "duplicate container final complete after null",
			body:  `{"gas_info":null,"gas_info":{"gas_wanted":"10","gas_used":"7"}}`,
			valid: true,
			used:  7,
		},
		{
			name: "duplicate container final null",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gas_info":null}`,
		},
		{
			name: "escaped duplicate container final incomplete",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gas_\u0069nfo":{}}`,
		},
		{
			name: "duplicate camel container final missing wanted",
			body: `{"gasInfo":{"gasWanted":"10","gasUsed":"7"},"gasInfo":{"gasUsed":"7"}}`,
		},
		{
			name: "duplicate scalar final null",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7","gas_used":null}}`,
		},
		{
			name:  "duplicate scalar final zero",
			body:  `{"gas_info":{"gas_wanted":"10","gas_used":null,"gas_used":"0"}}`,
			valid: true,
		},
		{
			name: "duplicate wanted final null",
			body: `{"gas_info":{"gas_wanted":"10","gas_wanted":null,"gas_used":"7"}}`,
		},
		{
			name: "camel wanted null overrides snake",
			body: `{"gas_info":{"gas_wanted":"10","gasWanted":null,"gas_used":"7"}}`,
		},
		{
			name: "camel used null before snake",
			body: `{"gas_info":{"gas_wanted":"10","gasUsed":null,"gas_used":"7"}}`,
		},
		{
			name:  "camel used zero before snake",
			body:  `{"gas_info":{"gas_wanted":"10","gasUsed":"0","gas_used":"7"}}`,
			valid: true,
		},
		{
			name:  "camel used zero after snake",
			body:  `{"gas_info":{"gas_wanted":"10","gas_used":"7","gasUsed":0}}`,
			valid: true,
		},
		{
			name:  "duplicate camel used final nonzero",
			body:  `{"gas_info":{"gas_wanted":"10","gasUsed":null,"gasUsed":"7"}}`,
			valid: true,
			used:  7,
		},
		{
			name: "duplicate camel used final null",
			body: `{"gas_info":{"gas_wanted":"10","gasUsed":"7","gasUsed":null}}`,
		},
		{
			name:  "zero used and max wanted",
			body:  `{"gasInfo":{"gasWanted":"18446744073709551615","gasUsed":"0"}}`,
			valid: true,
		},
		{
			name:  "max used",
			body:  `{"gas_info":{"gas_wanted":"18446744073709551615","gas_used":"18446744073709551615"}}`,
			valid: true,
			used:  ^uint64(0),
		},
		{
			name: "quoted null used",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"null"}}`,
		},
		{
			name: "quoted whitespace null wanted",
			body: `{"gas_info":{"gas_wanted":" null ","gas_used":"7"}}`,
		},
		{
			name: "whitespace null used",
			body: `{"gas_info":{"gas_wanted":"10","gas_used": null }}`,
		},
		{
			name: "escaped numeric string matches pinned rejection",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"\u0037"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			c := mockABCI(t, func(_ *http.Request, wire abciWireRequest) abci.ResponseQuery {
				calls.Add(1)
				require.Equal(t, "/app/simulate", wire.Path)
				return abci.ResponseQuery{
					Height: 42,
					Value:  []byte(tc.body),
				}
			})
			response, err := c.txs.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: []byte{1}})
			require.EqualValues(t, 1, calls.Load())
			if !tc.valid {
				require.Error(t, err)
				require.Nil(t, response)
				_, native := status.FromError(err)
				require.False(t, native)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.used, response.GasInfo.GasUsed)
			pinned, err := authclient.ParseQueryResponse([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, pinned.GasInfo, *response.GasInfo)
		})
	}
}

func TestLegacySimulationAnyValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		valid bool
	}{
		{
			name:  "uint64 max",
			value: `{"@type":"/google.protobuf.UInt64Value","value":"18446744073709551615"}`,
			valid: true,
		},
		{
			name:  "timestamp",
			value: `{"@type":"type.googleapis.com/google.protobuf.Timestamp","value":"2026-11-15T00:00:00Z"}`,
			valid: true,
		},
		{
			name:  "duration",
			value: `{"@type":"/google.protobuf.Duration","value":"1.250s"}`,
			valid: true,
		},
		{
			name:  "duplicate WKT value final valid",
			value: `{"@type":"/google.protobuf.UInt64Value","value":null,"value":"7"}`,
			valid: true,
		},
		{
			name:  "duplicate WKT value final null",
			value: `{"@type":"/google.protobuf.UInt64Value","value":"7","value":null}`,
		},
		{
			name:  "missing WKT value",
			value: `{"@type":"/google.protobuf.UInt64Value"}`,
		},
		{
			name:  "wrong case WKT value",
			value: `{"@type":"/google.protobuf.UInt64Value","Value":"7"}`,
		},
		{
			name:  "invalid uint64 WKT",
			value: `{"@type":"/google.protobuf.UInt64Value","value":{}}`,
		},
		{
			name:  "invalid timestamp WKT",
			value: `{"@type":"/google.protobuf.Timestamp","value":"yesterday"}`,
		},
		{
			name:  "invalid duration WKT",
			value: `{"@type":"/google.protobuf.Duration","value":[]}`,
		},
		{
			name:  "nested WKT null",
			value: `{"@type":"/google.protobuf.Any","value":{"@type":"/google.protobuf.Duration","value":null}}`,
		},
		{
			name:  "null type",
			value: `{"@type":null,"value":"7"}`,
		},
		{
			name:  "unknown type with fake pruning index",
			value: `{"@type":"/message index: 1: SECRET","value":"7"}`,
		},
		{
			name:  "struct null member is legitimate",
			value: `{"@type":"/google.protobuf.Struct","value":{"nullable":null}}`,
			valid: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"msg_responses":[` + tc.value + `]}}`
			c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
				return abci.ResponseQuery{
					Height: 42,
					Value:  []byte(body),
				}
			})
			require.NotPanics(t, func() {
				response, err := c.txs.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: []byte{1}})
				if !tc.valid {
					require.Error(t, err)
					require.Nil(t, response)
					require.NotContains(t, err.Error(), "SECRET")
					_, native := status.FromError(err)
					require.False(t, native)
					return
				}

				require.NoError(t, err)
				pinned, err := authclient.ParseQueryResponse([]byte(body))
				require.NoError(t, err)
				require.True(t, proto.Equal(pinned.Result, response.Result))
				require.Len(t, response.Result.MsgResponses, 1)
			})
		})
	}
}

func TestLegacySimulationCompleteResult(t *testing.T) {
	var responses []*codectypes.Any
	for _, message := range []proto.Message{
		&sinkv1.MsgReportOutcomeResponse{},
		&gogotypes.UInt64Value{Value: ^uint64(0)},
		&gogotypes.Timestamp{Seconds: 1794700800},
		&gogotypes.Duration{
			Seconds: 1,
			Nanos:   250000000,
		},
	} {
		packed, err := codectypes.NewAnyWithValue(message)
		require.NoError(t, err)
		responses = append(responses, packed)
	}
	for _, used := range []uint64{0, 75984} {
		original := &sdk.SimulationResponse{
			GasInfo: sdk.GasInfo{
				GasWanted: ^uint64(0),
				GasUsed:   used,
			},
			Result: &sdk.Result{
				Data: []byte{0, 255, 1},
				Log:  "complete response",
				Events: []abci.Event{
					{
						Type: "sink",
						Attributes: []abci.EventAttribute{
							{
								Key:   "outcome",
								Value: "minted",
								Index: true,
							},
						},
					},
				},
				MsgResponses: responses,
			},
		}
		// BaseApp uses this exact SDK marshaler with original names and defaults.
		body, err := codec.ProtoMarshalJSON(original, nil)
		require.NoError(t, err)
		c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
			return abci.ResponseQuery{
				Height: 42,
				Value:  body,
			}
		})
		response, err := c.txs.Simulate(t.Context(), &txtypes.SimulateRequest{TxBytes: []byte{1}})
		require.NoError(t, err)
		require.Equal(t, original.GasInfo, *response.GasInfo)
		expected, err := proto.Marshal(original.Result)
		require.NoError(t, err)
		actual, err := proto.Marshal(response.Result)
		require.NoError(t, err)
		require.Equal(t, expected, actual, "Result, events, data and Any payloads must survive")
		require.Equal(t, responses[0].TypeUrl, response.Result.MsgResponses[0].TypeUrl)
		var decoded gogotypes.UInt64Value
		require.NoError(t, proto.Unmarshal(response.Result.MsgResponses[1].Value, &decoded))
		require.Equal(t, ^uint64(0), decoded.Value)
	}
}

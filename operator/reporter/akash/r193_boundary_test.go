package akash

import (
	"net/http"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/status"
)

func TestR193GasPresenceMatchesDecodedValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "camel container overrides checked snake container with null",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gasInfo":null}`,
		},
		{
			name: "camel scalar overrides checked snake scalar with null",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7","gasUsed":null}}`,
		},
		{
			name: "duplicate container loses checked gas used",
			body: `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"gas_info":{"gas_wanted":"10"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
				return abci.ResponseQuery{
					Height: 42,
					Value:  []byte(tc.body),
				}
			})
			used, err := c.Simulate(t.Context(), []byte{1})
			t.Logf("body=%s used=%d err=%v", tc.body, used, err)
			require.Error(t, err, "a missing/null effective gas field must remain retryable")
			_, native := status.FromError(err)
			require.False(t, native)
		})
	}
}

func TestR193MalformedAnyDoesNotPanic(t *testing.T) {
	for _, kind := range []string{"google.protobuf.UInt64Value", "google.protobuf.Timestamp", "google.protobuf.Duration"} {
		t.Run(kind, func(t *testing.T) {
			body := `{"gas_info":{"gas_wanted":"10","gas_used":"7"},"result":{"msg_responses":[{"@type":"/` + kind + `","value":null}]}}`
			c := mockABCI(t, func(_ *http.Request, _ abciWireRequest) abci.ResponseQuery {
				return abci.ResponseQuery{
					Height: 42,
					Value:  []byte(body),
				}
			})
			assert.NotPanics(t, func() {
				used, err := c.Simulate(t.Context(), []byte{1})
				t.Logf("body=%s used=%d err=%v", body, used, err)
				require.Error(t, err)
				_, native := status.FromError(err)
				require.False(t, native)
			}, "malformed RPC data must return an error without terminating the reporter")
		})
	}
}

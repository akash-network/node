package relayer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func TestCoreAccountProbesRejectMalformedAbsence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var call struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&call))
		require.Contains(t, []string{"getAccountInfo", "getMultipleAccounts"}, call.Method)
		var value any
		if call.Method == "getMultipleAccounts" {
			value = []any{nil}
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      call.ID,
			"result":  map[string]any{"value": value},
		}))
	}))
	defer server.Close()
	client := rpc.New(server.URL)
	defer client.Close()
	s := &service{
		reader: bridge.NewReader(client, solana.SystemProgramID, "test-1", nil),
	}
	for name, read := range map[string]func(context.Context, solana.PublicKey, rpc.CommitmentType) (*rpc.Account, error){
		"header":     s.headerAccount,
		"resolution": s.resolutionAccount,
	} {
		t.Run(name, func(t *testing.T) {
			account, err := read(context.Background(), solana.SystemProgramID, rpc.CommitmentFinalized)
			require.ErrorContains(t, err, "incomplete account response")
			require.Nil(t, account)
		})
	}
	t.Run("header-batch", func(t *testing.T) {
		accounts, err := s.headerAccounts(context.Background(), []solana.PublicKey{solana.SystemProgramID}, rpc.CommitmentFinalized)
		require.ErrorContains(t, err, "incomplete account response")
		require.Nil(t, accounts)
	})
}

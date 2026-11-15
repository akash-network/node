package bridge

import (
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
)

func TestR156HeaderTemporaryInitializable(t *testing.T) {
	require.False(t, HeaderTemporaryInitializable(nil), "absence stays distinct")
	account := accountFixture(solana.SystemProgramID, []byte{})
	require.True(t, HeaderTemporaryInitializable(account))
	account.Data = nil
	require.False(t, HeaderTemporaryInitializable(account))
	account.Data = new(rpc.DataBytesOrJSON)
	require.False(t, HeaderTemporaryInitializable(account), "undecoded zero value is not empty data")
	require.NoError(t, json.Unmarshal([]byte(`{"parsed":{}}`), account.Data))
	require.False(t, HeaderTemporaryInitializable(account), "JSON data is not decoded binary")
	account.Data = rpc.DataBytesOrJSONFromBytes(nil)
	require.False(t, HeaderTemporaryInitializable(account), "nil binary is not present decoded data")
	account.Data, _ = rpc.DataBytesOrJSONFromBase64("")
	require.True(t, HeaderTemporaryInitializable(account), "explicit base64 empty data is present")
	account.Executable = true
	require.False(t, HeaderTemporaryInitializable(account))
	account.Executable = false
	account.Owner = keyByte(99)
	require.False(t, HeaderTemporaryInitializable(account))
	account.Owner = solana.SystemProgramID
	account.Data = rpc.DataBytesOrJSONFromBytes([]byte{1})
	require.False(t, HeaderTemporaryInitializable(account))
}

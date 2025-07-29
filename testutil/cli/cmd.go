package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/gogoproto/jsonpb"
)

// ExecTestCLICmd builds the client context, mocks the output and executes the command.
func ExecTestCLICmd(ctx context.Context, clientCtx client.Context, cmd *cobra.Command, extraArgs ...string) (testutil.BufferWriter, error) {
	cmd.SetArgs(extraArgs)

	_, out := testutil.ApplyMockIO(cmd)
	clientCtx = clientCtx.WithOutput(out)

	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, client.ClientContextKey, &clientCtx)
	ctx = context.WithValue(ctx, server.ServerContextKey, server.NewDefaultContext())

	if err := cmd.ExecuteContext(ctx); err != nil {
		return out, err
	}

	return out, nil
}

// ValidateTxSuccessful is a gentle response to inappropriate approach of cli test utils
// send transaction may fail and calling cli routine won't know about it
func ValidateTxSuccessful(t testing.TB, cctx client.Context, data []byte) (*sdk.TxResponse, sdk.Tx) {
	t.Helper()

	res := getTxResponse(t, cctx, data)
	require.Zero(t, res.Code, res)

	var tx sdk.Tx
	err := cctx.Codec.UnpackAny(res.Tx, &tx)
	require.NoError(t, err)

	return res, tx
}

func ValidateTxUnSuccessful(t testing.TB, cctx client.Context, data []byte) {
	t.Helper()

	res := getTxResponse(t, cctx, data)
	require.NotZero(t, res.Code, res)
}

func getTxResponse(t testing.TB, cctx client.Context, data []byte) *sdk.TxResponse {
	var resp sdk.TxResponse

	err := jsonpb.Unmarshal(bytes.NewBuffer(data), &resp)
	require.NoError(t, err)

	res, err := tx.QueryTx(cctx, resp.TxHash)
	require.NoError(t, err)

	return res
}

// GetTxFees is a gentle response to inappropriate approach of cli test utils
// send transaction may fail and calling cli routine won't know about it
func GetTxFees(t testing.TB, cctx client.Context, data []byte) sdk.FeeTx {
	t.Helper()

	res := getTxResponse(t, cctx, data)
	require.Zero(t, res.Code, res)

	var fees sdk.FeeTx
	err := cctx.Codec.UnpackAny(res.Tx, &fees)
	require.NoError(t, err)

	return fees
}

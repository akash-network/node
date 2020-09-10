package cmd

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// TestSendManifest for integration testing
func TestSendManifest(clientCtx client.Context, id mtypes.BidID, provider, sdlPath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner.String()),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
		fmt.Sprintf("--oseq=%v", id.OSeq),
		fmt.Sprintf("--provider=%v", provider),
	}
	args = append(args, extraArgs...)
	return clitestutil.ExecTestCLICmd(clientCtx, sendManifestCmd(), args)
}

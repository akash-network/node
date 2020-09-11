package cmd

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

// TestSendManifest for integration testing
func TestSendManifest(clientCtx client.Context, id mtypes.BidID, sdlPath string, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", id.Owner),
		fmt.Sprintf("--dseq=%v", id.DSeq),
		fmt.Sprintf("--gseq=%v", id.GSeq),
		fmt.Sprintf("--oseq=%v", id.OSeq),
		fmt.Sprintf("--provider=%s", id.Provider),
	}
	args = append(args, sdlPath)
	args = append(args, extraArgs...)
	fmt.Printf("%v\n", args)
	return clitestutil.ExecTestCLICmd(clientCtx, sendManifestCmd(), args)
}

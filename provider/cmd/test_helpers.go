package cmd

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	testutilcli "github.com/ovrclk/akash/testutil/cli"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

/*
TestSendManifest for integration testing
this is similar to cli command exampled below
akash provider send-manifest --owner <address> \
	--dseq 7 --gseq 1 --oseq 1 \
	--provider <address> ./../_run/kube/deployment.yaml \
	--home=/tmp/akash_integration_TestE2EApp_324892307/.akashctl --node=tcp://0.0.0.0:41863
*/
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
	return testutilcli.ExecTestCLICmd(clientCtx, sendManifestCmd(), args...)
}

//go:build e2e.integration

package e2e

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"

	"pkg.akt.dev/go/cli"
	"pkg.akt.dev/go/cli/testutil"
)

// ExecGroupClose executes the group close command
func ExecGroupClose(ctx context.Context, cctx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return testutil.ExecTestCLICmd(ctx, cctx, cli.GetTxDeploymentGroupCloseCmd(), args...)
}



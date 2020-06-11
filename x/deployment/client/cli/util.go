package cli

import "github.com/cosmos/cosmos-sdk/client/context"

func currentBlockHeight(ctx context.CLIContext) (uint64, error) {
	client, err := ctx.GetNode()
	if err != nil {
		return 0, err
	}
	status, err := client.Status()
	if err != nil {
		return 0, err
	}
	return uint64(status.SyncInfo.LatestBlockHeight), nil
}

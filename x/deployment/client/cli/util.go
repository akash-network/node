package cli

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
)

func currentBlockHeight(ctx client.Context) (uint64, error) {
	client, err := ctx.GetNode()
	if err != nil {
		return 0, err
	}
	status, err := client.Status(context.Background())
	if err != nil {
		return 0, err
	}
	return uint64(status.SyncInfo.LatestBlockHeight), nil
}

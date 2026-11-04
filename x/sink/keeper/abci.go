package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/telemetry"

	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

// BeginBlocker is called at the beginning of each block
func (k *keeper) BeginBlocker(ctx context.Context) error {
	startTm := telemetry.Now()
	defer telemetry.ModuleMeasureSince(types.ModuleName, startTm, telemetry.MetricKeyBeginBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)

	// The allocator stores the last issued sequence and nextID issues the
	// stored value plus one, so a reset to zero makes each block start at 1.
	err := k.sequence.Set(ctx, 0)
	if err != nil {
		sctx.Logger().Error("failed to reset ledger sequence", "err", err)
	}

	return nil
}

func (k *keeper) EndBlocker(_ context.Context) error {
	return nil
}

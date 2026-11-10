package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

const quotaEpochIdentifier = "day"

func (k *keeper) getEpochID(ctx sdk.Context) (uint64, error) {
	epoch, err := k.epochs.GetEpoch(ctx, quotaEpochIdentifier)
	if err != nil {
		return 0, err
	}

	if epoch.ID != quotaEpochIdentifier || epoch.Duration <= 0 || epoch.CurrentEpoch < 0 ||
		epoch.EpochCountingStarted != (epoch.CurrentEpoch > 0) {
		return 0, types.ErrInvalidClock.Wrap("invalid quota epoch")
	}

	return uint64(epoch.CurrentEpoch), nil
}

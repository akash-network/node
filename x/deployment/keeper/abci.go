package keeper

import (
	"context"
)

func (k Keeper) EndBlocker(_ context.Context) error {
	return nil
}

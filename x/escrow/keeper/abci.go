package keeper

import (
	"context"
)

// EndBlocker is called at the end of each block to manage settlement on regular intervals
func (k *keeper) EndBlocker(_ context.Context) error {
	return nil
}

package app

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// chainAnteHandlers point of this function is to sequentially run multiple chains
// like it would be one single chain. It becomes useful in case app implements own
// AnteHandlers and app wants run them with all default handlers from the sdk
// Purpose of the function came from an issue that sdk.AnteHandler does not have next parameter
// and default chain can be accessed only from ante.NewAnteHandler which returns sdk.AnteHandler.
// It eliminates possibility for apps to chain own Ante
func chainAnteHandlers(handlers ...sdk.AnteHandler) sdk.AnteHandler {
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		for _, handler := range handlers {
			var err error
			if ctx, err = handler(ctx, tx, simulate); err != nil {
				return ctx, err
			}
		}

		return ctx, nil
	}
}

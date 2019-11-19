package keeper

import sdk "github.com/cosmos/cosmos-sdk/types"

func providerKey(id sdk.Address) []byte {
	return id.Bytes()
}

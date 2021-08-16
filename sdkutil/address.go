package sdkutil

import sdk "github.com/cosmos/cosmos-sdk/types"

// MustAccAddressFromBech32 creates an AccAddress from a Bech32 string.
// It panics if there is an error.
func MustAccAddressFromBech32(address string) sdk.AccAddress {
	addr, err := sdk.AccAddressFromBech32(address)
	if err != nil {
		panic(err)
	}
	return addr
}

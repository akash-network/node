package sdkutil

import sdk "github.com/cosmos/cosmos-sdk/types"

// GetAccAddressFromBech32 creates an AccAddress from a Bech32 string.
// It internally calls `sdk.AccAddressFromBech32` and ignores the error.
func GetAccAddressFromBech32(address string) sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(address)
	return addr
}

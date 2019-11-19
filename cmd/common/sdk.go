package common

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	bech32PrefixAccAddr = "akash"
	bech32PrefixAccPub  = "akashpub"

	bech32PrefixValAddr = "akashvaloper"
	bech32PrefixValPub  = "akashvaloperpub"

	bech32PrefixConsAddr = "akashvalcons"
	bech32PrefixConsPub  = "akashvalconspub"
)

func InitSDKConfig() {
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(bech32PrefixAccAddr, bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(bech32PrefixValAddr, bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(bech32PrefixConsAddr, bech32PrefixConsPub)

	// config.SetBech32PrefixForAccount(sdk.Bech32PrefixAccAddr, sdk.Bech32PrefixAccPub)
	// config.SetBech32PrefixForValidator(sdk.Bech32PrefixValAddr, sdk.Bech32PrefixValPub)
	// config.SetBech32PrefixForConsensusNode(sdk.Bech32PrefixConsAddr, sdk.Bech32PrefixConsPub)

	// config.SetCoinType(yourCoinType)
	// config.SetFullFundraiserPath(yourFullFundraiserPath)

	config.Seal()
}

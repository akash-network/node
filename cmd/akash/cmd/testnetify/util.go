package testnetify

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	ibchost "github.com/cosmos/ibc-go/v4/modules/core/24-host"
	ibccoretypes "github.com/cosmos/ibc-go/v4/modules/core/types"
)

func GetIBCGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *ibccoretypes.GenesisState {
	var genesisState ibccoretypes.GenesisState

	if appState[ibchost.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[ibchost.ModuleName], &genesisState)
	}

	return &genesisState
}

func GetGovGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *govtypes.GenesisState {
	var genesisState govtypes.GenesisState

	if appState[govtypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[govtypes.ModuleName], &genesisState)
	}

	return &genesisState
}

func GetSlashingGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *slashingtypes.GenesisState {
	var genesisState slashingtypes.GenesisState

	if appState[slashingtypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[slashingtypes.ModuleName], &genesisState)
	}

	return &genesisState
}

func GetDistributionGenesisStateFromAppState(cdc codec.JSONCodec, appState map[string]json.RawMessage) *distributiontypes.GenesisState {
	var genesisState distributiontypes.GenesisState

	if appState[distributiontypes.ModuleName] != nil {
		cdc.MustUnmarshalJSON(appState[distributiontypes.ModuleName], &genesisState)
	}

	return &genesisState
}

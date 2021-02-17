package app

import (
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	ibctransfertypes "github.com/cosmos/cosmos-sdk/x/ibc/applications/transfer/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	escrowtypes "github.com/ovrclk/akash/x/escrow/types"
)

func MacPerms() map[string][]string {
	return map[string][]string{
		authtypes.FeeCollectorName:     nil,
		escrowtypes.ModuleName:         nil,
		distrtypes.ModuleName:          nil,
		minttypes.ModuleName:           {authtypes.Minter},
		stakingtypes.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
		stakingtypes.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
		govtypes.ModuleName:            {authtypes.Burner},
		ibctransfertypes.ModuleName:    {authtypes.Minter, authtypes.Burner},
	}
}

func MacAddrs() map[string]bool {
	perms := MacPerms()
	addrs := make(map[string]bool, len(perms))
	for k := range perms {
		addrs[authtypes.NewModuleAddress(k).String()] = true
	}
	return addrs
}

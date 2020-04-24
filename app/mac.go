package app

import (
	"github.com/cosmos/cosmos-sdk/x/auth"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/gov"
	transfer "github.com/cosmos/cosmos-sdk/x/ibc/20-transfer"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/cosmos-sdk/x/supply"
)

func macPerms() map[string][]string {
	return map[string][]string{
		auth.FeeCollectorName:           nil,
		distr.ModuleName:                nil,
		mint.ModuleName:                 {supply.Minter},
		staking.BondedPoolName:          {supply.Burner, supply.Staking},
		staking.NotBondedPoolName:       {supply.Burner, supply.Staking},
		gov.ModuleName:                  {supply.Burner},
		transfer.GetModuleAccountName(): {auth.Minter, auth.Burner},
	}
}

func macAddrs() map[string]bool {
	perms := macPerms()
	addrs := make(map[string]bool, len(perms))
	for k := range perms {
		addrs[supply.NewModuleAddress(k).String()] = true
	}
	return addrs
}

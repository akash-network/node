package simulation

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	types "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

// RandomizedGenState enables Sink with ordinary simulation accounts as reporters.
// Auth and bank's simulation genesis provide their accounts and initial funds.
func RandomizedGenState(simState *module.SimulationState) {
	genesis := types.DefaultGenesis()
	params := &genesis.Params
	if len(simState.Accounts) > 0 {
		params.Enabled = true
		params.RefundsEnabled = true
		params.ReportersThreshold = sdkmath.LegacyMustNewDecFromStr("0.75")

		for _, index := range simState.Rand.Perm(len(simState.Accounts))[:min(3, len(simState.Accounts))] {
			params.Reporters = append(params.Reporters, simState.Accounts[index].Address.String())
		}
	}

	minimum := int64(1 + simState.Rand.Intn(100))
	maximum := minimum * int64(2+simState.Rand.Intn(9))
	params.MinAmountPerTransfer = sdk.NewInt64Coin(sdkutil.DenomUakt, minimum)
	params.MaxAmountPerTransfer = sdk.NewInt64Coin(sdkutil.DenomUakt, maximum)
	params.MaxEpochAmountPerAccount = sdk.NewInt64Coin(sdkutil.DenomUakt, maximum*4)
	params.MaxTransferAmountPerEpoch = sdk.NewInt64Coin(sdkutil.DenomUakt, maximum*20)
	params.MaxEpochRefund = sdk.NewInt64Coin(sdkutil.DenomUakt, maximum*4)

	genesis.PolicySequence = 1
	genesis.Policies = []types.ReporterPolicy{
		{
			Version:   genesis.PolicySequence,
			Members:   params.Reporters,
			Threshold: params.ReportersThreshold,
		},
	}

	simState.GenState[types.ModuleName] = simState.Cdc.MustMarshalJSON(genesis)
}

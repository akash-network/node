package app

import (
	"encoding/json"
	"math/rand"

	errorsmod "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/spf13/viper"
	"pkg.akt.dev/go/sdkutil"

	abci "github.com/cometbft/cometbft/abci/types"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/log"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"

	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/simulation"
	"github.com/cosmos/cosmos-sdk/x/staking"
)

// ExportAppStateAndValidators exports the state of the application for a genesis
// file.
func (app *AkashApp) ExportAppStateAndValidators(
	forZeroHeight bool,
	jailAllowedAddrs []string,
	modulesToExport []string,
) (servertypes.ExportedApp, error) {
	// as if they could withdraw from the start of the next block
	ctx := app.NewContextLegacy(true, cmproto.Header{Height: app.LastBlockHeight()})

	// We export at last height + 1, because that's the height at which
	// Tendermint will start InitChain.
	height := app.LastBlockHeight() + 1

	if forZeroHeight {
		height = 0
		app.prepForZeroHeightGenesis(ctx, jailAllowedAddrs)
	}

	genState, err := app.MM.ExportGenesisForModules(ctx, app.cdc, modulesToExport)
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	appState, err := json.MarshalIndent(genState, "", "  ")
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	validators, err := staking.WriteValidators(ctx, app.Keepers.Cosmos.Staking)
	if err != nil {
		return servertypes.ExportedApp{}, err
	}

	return servertypes.ExportedApp{
		AppState:        appState,
		Validators:      validators,
		Height:          height,
		ConsensusParams: app.GetConsensusParams(ctx),
	}, nil
}

// prepForZeroHeightGenesis prepare for fresh start at zero height
// NOTE zero height genesis is a temporary feature that will be deprecated
//
//	in favour of export at a block height
func (app *AkashApp) prepForZeroHeightGenesis(ctx sdk.Context, jailAllowedAddrs []string) {
	// Check if there is an allowed address list
	applyAllowedAddrs := len(jailAllowedAddrs) > 0

	allowedAddrsMap := make(map[string]bool)

	for _, addr := range jailAllowedAddrs {
		_, err := sdk.ValAddressFromBech32(addr)
		if err != nil {
			panic(err)
		}
		allowedAddrsMap[addr] = true
	}

	/* Just to be safe, assert the invariants on current state. */
	app.Keepers.Cosmos.Crisis.AssertInvariants(ctx)

	/* Handle fee distribution state. */

	// withdraw all validator commission
	err := app.Keepers.Cosmos.Staking.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) (stop bool) {
		valBz, err := app.Keepers.Cosmos.Staking.ValidatorAddressCodec().StringToBytes(val.GetOperator())
		if err != nil {
			panic(err)
		}

		_, err = app.Keepers.Cosmos.Distr.WithdrawValidatorCommission(ctx, valBz)
		if err != nil {
			if !errorsmod.IsOf(err, distrtypes.ErrNoValidatorCommission) {
				panic(err)
			}
		}
		return false
	})
	if err != nil {
		panic(err)
	}

	// withdraw all delegator rewards
	dels, err := app.Keepers.Cosmos.Staking.GetAllDelegations(ctx)
	if err != nil {
		panic(err)
	}

	for _, delegation := range dels {
		valAddr, err := sdk.ValAddressFromBech32(delegation.ValidatorAddress)
		if err != nil {
			panic(err)
		}

		delAddr, err := sdk.AccAddressFromBech32(delegation.DelegatorAddress)
		if err != nil {
			panic(err)
		}
		_, _ = app.Keepers.Cosmos.Distr.WithdrawDelegationRewards(ctx, delAddr, valAddr)
	}

	// clear validator slash events
	app.Keepers.Cosmos.Distr.DeleteAllValidatorSlashEvents(ctx)

	// clear validator historical rewards
	app.Keepers.Cosmos.Distr.DeleteAllValidatorHistoricalRewards(ctx)

	// set context height to zero
	height := ctx.BlockHeight()
	ctx = ctx.WithBlockHeight(0)

	// reinitialize all validators
	err = app.Keepers.Cosmos.Staking.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) (stop bool) {
		valBz, err := sdk.ValAddressFromBech32(val.GetOperator())
		if err != nil {
			panic(err)
		}

		// donate any unwithdrawn outstanding reward fraction tokens to the community pool
		scraps, err := app.Keepers.Cosmos.Distr.GetValidatorOutstandingRewardsCoins(ctx, valBz)
		if err != nil {
			panic(err)
		}

		feePool, err := app.Keepers.Cosmos.Distr.FeePool.Get(ctx)
		if err != nil {
			panic(err)
		}

		feePool.CommunityPool = feePool.CommunityPool.Add(scraps...)
		err = app.Keepers.Cosmos.Distr.FeePool.Set(ctx, feePool)
		if err != nil {
			panic(err)
		}

		if err := app.Keepers.Cosmos.Distr.Hooks().AfterValidatorCreated(ctx, valBz); err != nil {
			panic(err)
		}
		return false
	})
	if err != nil {
		panic(err)
	}

	// reinitialize all delegations
	for _, del := range dels {
		valAddr, err := sdk.ValAddressFromBech32(del.ValidatorAddress)
		if err != nil {
			panic(err)
		}

		delAddr, err := sdk.AccAddressFromBech32(del.DelegatorAddress)
		if err != nil {
			panic(err)
		}
		err = app.Keepers.Cosmos.Distr.Hooks().BeforeDelegationCreated(ctx, delAddr, valAddr)
		if err != nil {
			panic(err)
		}

		err = app.Keepers.Cosmos.Distr.Hooks().AfterDelegationModified(ctx, delAddr, valAddr)
		if err != nil {
			panic(err)
		}
	}

	// reset context height
	ctx = ctx.WithBlockHeight(height)

	/* Handle staking state. */

	// iterate through redelegations, reset creation height
	err = app.Keepers.Cosmos.Staking.IterateRedelegations(ctx, func(_ int64, red stakingtypes.Redelegation) (stop bool) {
		for i := range red.Entries {
			red.Entries[i].CreationHeight = 0
		}
		err = app.Keepers.Cosmos.Staking.SetRedelegation(ctx, red)
		if err != nil {
			panic(err)
		}
		return false
	})
	if err != nil {
		panic(err)
	}

	// iterate through unbonding delegations, reset creation height
	err = app.Keepers.Cosmos.Staking.IterateUnbondingDelegations(ctx, func(_ int64, ubd stakingtypes.UnbondingDelegation) (stop bool) {
		for i := range ubd.Entries {
			ubd.Entries[i].CreationHeight = 0
		}
		err = app.Keepers.Cosmos.Staking.SetUnbondingDelegation(ctx, ubd)
		if err != nil {
			panic(err)
		}
		return false
	})
	if err != nil {
		panic(err)
	}

	// Iterate through validators by power descending, reset bond heights, and
	// update bond intra-tx counters.

	store := ctx.KVStore(app.GetKey(stakingtypes.StoreKey))
	iter := storetypes.KVStoreReversePrefixIterator(store, stakingtypes.ValidatorsKey)

	counter := int16(0)

	for ; iter.Valid(); iter.Next() {
		addr := sdk.ValAddress(stakingtypes.AddressFromValidatorsKey(iter.Key()))
		validator, err := app.Keepers.Cosmos.Staking.GetValidator(ctx, addr)
		if err != nil {
			panic(err)
		}

		validator.UnbondingHeight = 0
		if applyAllowedAddrs && !allowedAddrsMap[addr.String()] {
			validator.Jailed = true
		}

		err = app.Keepers.Cosmos.Staking.SetValidator(ctx, validator)
		if err != nil {
			panic(err)
		}
		counter++
	}

	_ = iter.Close()

	_, _ = app.Keepers.Cosmos.Staking.ApplyAndReturnValidatorSetUpdates(ctx)

	/* Handle slashing state. */

	// reset start height on signing infos
	err = app.Keepers.Cosmos.Slashing.IterateValidatorSigningInfos(
		ctx,
		func(addr sdk.ConsAddress, info slashingtypes.ValidatorSigningInfo) (stop bool) {
			info.StartHeight = 0
			err = app.Keepers.Cosmos.Slashing.SetValidatorSigningInfo(ctx, addr, info)
			if err != nil {
				panic(err)
			}
			return false
		},
	)
	if err != nil {
		panic(err)
	}
}

// Setup initializes a new AkashApp. A Nop logger is set in AkashApp.
func Setup(opts ...SetupAppOption) *AkashApp {
	cfg := &setupAppOptions{
		encCfg:  sdkutil.MakeEncodingConfig(),
		home:    DefaultHome,
		checkTx: false,
		chainID: "akash-1",
	}

	ModuleBasics().RegisterInterfaces(cfg.encCfg.InterfaceRegistry)

	for _, opt := range opts {
		opt(cfg)
	}

	db := dbm.NewMemDB()

	appOpts := viper.New()

	appOpts.Set("home", cfg.home)

	r := rand.New(rand.NewSource(0)) // nolint: gosec
	genTime := simulation.RandTimestamp(r)

	appOpts.Set("GenesisTime", genTime)

	app := NewApp(
		log.NewNopLogger(),
		db,
		nil,
		true,
		5,
		map[int64]bool{},
		cfg.encCfg,
		appOpts,
		baseapp.SetChainID(cfg.chainID),
	)

	if !cfg.checkTx {
		var state GenesisState
		if cfg.genesisFn == nil {
			// init chain must be called to stop deliverState from being nil
			state = NewDefaultGenesisState(app.AppCodec())
		} else {
			state = cfg.genesisFn(app.cdc)
		}

		stateBytes, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			panic(err)
		}

		// Initialize the chain
		_, err = app.InitChain(
			&abci.RequestInitChain{
				Validators:    []abci.ValidatorUpdate{},
				AppStateBytes: stateBytes,
				ChainId:       cfg.chainID,
			},
		)
		if err != nil {
			panic(err)
		}
	}

	return app
}

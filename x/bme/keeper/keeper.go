package keeper

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bmetypes "pkg.akt.dev/go/node/bme/v1"
	"pkg.akt.dev/go/sdkutil"

	bmeimports "pkg.akt.dev/node/v2/x/bme/imports"
)

const (
	secondsPerDay = (24 * time.Hour) / time.Second
)

type Keeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetParams(sdk.Context) (bmetypes.Params, error)
	SetParams(sdk.Context, bmetypes.Params) error

	GetVaultState(sdk.Context) (bmetypes.State, error)

	GetCircuitBreakerStatus(sdk.Context) (bmetypes.CircuitBreakerStatus, error)
	GetCollateralRatio(sdk.Context) (sdkmath.LegacyDec, error)

	BeginBlocker(_ context.Context) error
	EndBlocker(context.Context) error

	BurnMintFromAddressToModuleAccount(sdk.Context, sdk.AccAddress, string, sdk.Coin, string) (sdk.DecCoin, error)
	BurnMintFromModuleAccountToAddress(sdk.Context, string, sdk.AccAddress, sdk.Coin, string) (sdk.DecCoin, error)
	BurnMintOnAccount(sdk.Context, sdk.AccAddress, sdk.Coin, string) (sdk.DecCoin, error)

	NewQuerier() Querier
	GetAuthority() string
}

// keeper
//
//	the vault contains "balances/credits" for certain tokens
//	   AKT - this implementation uses true burn/mint instead of remint credits due to cosmos-sdk complexities with a latter option when
//	         when trying to remove remint credit for total supply.
//	         the BME, however, needs to track how much has been burned
//	   ACT - is not tracked here, rather via bank.TotalSupply() call and result is equivalent to OutstandingACT.
//	         If the total ACT supply is less than akt_to_mint * akt_price, then AKT cannot be minted and ErrInsufficientACT is returned
//	         to the caller
type keeper struct {
	cdc  codec.BinaryCodec
	skey *storetypes.KVStoreKey
	ssvc store.KVStoreService

	authority string

	Schema collections.Schema
	Params collections.Item[bmetypes.Params]
	//remintCredits  collections.Map[string, sdkmath.Int]
	totalBurned    collections.Map[string, sdkmath.Int]
	totalMinted    collections.Map[string, sdkmath.Int]
	ledger         collections.Map[collections.Pair[int64, int64], bmetypes.BMRecord]
	ledgerSequence int64

	accKeeper    bmeimports.AccountKeeper
	bankKeeper   bmeimports.BankKeeper
	oracleKeeper bmeimports.OracleKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	skey *storetypes.KVStoreKey,
	authority string,
	accKeeper bmeimports.AccountKeeper,
	bankKeeper bmeimports.BankKeeper,
	oracleKeeper bmeimports.OracleKeeper,
) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	k := &keeper{
		cdc:          cdc,
		skey:         skey,
		ssvc:         runtime.NewKVStoreService(skey),
		authority:    authority,
		accKeeper:    accKeeper,
		bankKeeper:   bankKeeper,
		oracleKeeper: oracleKeeper,
		Params:       collections.NewItem(sb, ParamsKey, "params", codec.CollValue[bmetypes.Params](cdc)),
		//remintCredits: collections.NewMap(sb, RemintCreditsKey, "remint_credits", collections.StringKey, sdk.IntValue),
		totalBurned: collections.NewMap(sb, TotalBurnedKey, "total_burned", collections.StringKey, sdk.IntValue),
		totalMinted: collections.NewMap(sb, TotalMintedKey, "total_minted", collections.StringKey, sdk.IntValue),
		ledger:      collections.NewMap(sb, LedgerKey, "ledger", collections.PairKeyCodec(collections.Int64Key, collections.Int64Key), codec.CollValue[bmetypes.BMRecord](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// Codec returns keeper codec
func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) NewQuerier() Querier {
	return Querier{k}
}

func (k *keeper) GetAuthority() string {
	return k.authority
}

func (k *keeper) Logger(sctx sdk.Context) log.Logger {
	return sctx.Logger().With("module", "x/"+bmetypes.ModuleName)
}

func (k *keeper) GetParams(ctx sdk.Context) (bmetypes.Params, error) {
	return k.Params.Get(ctx)
}

func (k *keeper) SetParams(ctx sdk.Context, params bmetypes.Params) error {
	return k.Params.Set(ctx, params)
}

func (k *keeper) GetVaultState(ctx sdk.Context) (bmetypes.State, error) {
	addr := k.accKeeper.GetModuleAddress(bmetypes.ModuleName)

	res := bmetypes.State{
		Balances: k.bankKeeper.GetAllBalances(ctx, addr),
	}

	err := k.totalBurned.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
		res.Burned = append(res.Burned, sdk.NewCoin(denom, value))

		return false, nil
	})
	if err != nil {
		return res, err
	}

	err = k.totalMinted.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
		res.Minted = append(res.Minted, sdk.NewCoin(denom, value))

		return false, nil
	})
	if err != nil {
		return res, err
	}

	//err = k.remintCredits.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
	//	res.RemintCredits = append(res.RemintCredits, sdk.NewCoin(denom, value))
	//
	//	return false, nil
	//})
	//if err != nil {
	//	return res, err
	//}

	return res, nil
}

// BurnMintFromAddressToModuleAccount collateralizes coins from source address into module account, mints new coins with price fetched from oracle,
// and sends minted coins to a module account
func (k *keeper) BurnMintFromAddressToModuleAccount(
	sctx sdk.Context,
	srcAddr sdk.AccAddress,
	moduleAccount string,
	burnCoin sdk.Coin,
	toDenom string,
) (sdk.DecCoin, error) {
	burn, mint, err := k.prepareToBM(sctx, burnCoin, toDenom)
	if err != nil {
		return sdk.DecCoin{}, err
	}

	mAddr := k.accKeeper.GetModuleAddress(moduleAccount)
	preRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromAccountToModule(sctx, srcAddr, bmetypes.ModuleName, sdk.NewCoins(burnCoin))
	}

	postRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromModuleToModule(sctx, bmetypes.ModuleName, moduleAccount, sdk.NewCoins(mint.Coin))
	}

	if burn.Coin.Denom == sdkutil.DenomUact {
		err = k.mintACT(sctx, burn, mint, srcAddr, mAddr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	} else {
		err = k.burnACT(sctx, burn, mint, srcAddr, mAddr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	}

	return sdk.NewDecCoin(mint.Coin.Denom, mint.Coin.Amount), nil
}

// BurnMintFromModuleAccountToAddress burns coins from a module account, mints new coins with price fetched from oracle,
// and sends minted coins to an account
func (k *keeper) BurnMintFromModuleAccountToAddress(
	sctx sdk.Context,
	moduleAccount string,
	dstAddr sdk.AccAddress,
	burnCoin sdk.Coin,
	toDenom string,
) (sdk.DecCoin, error) {
	burn, mint, err := k.prepareToBM(sctx, burnCoin, toDenom)
	if err != nil {
		return sdk.DecCoin{}, err
	}

	mAddr := k.accKeeper.GetModuleAddress(moduleAccount)
	preRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromModuleToModule(sctx, moduleAccount, bmetypes.ModuleName, sdk.NewCoins(burnCoin))
	}

	postRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromModuleToAccount(sctx, bmetypes.ModuleName, dstAddr, sdk.NewCoins(mint.Coin))
	}

	if burn.Coin.Denom == sdkutil.DenomUact {
		err = k.mintACT(sctx, burn, mint, mAddr, dstAddr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	} else {
		err = k.burnACT(sctx, burn, mint, mAddr, dstAddr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	}

	return sdk.NewDecCoin(mint.Coin.Denom, mint.Coin.Amount), nil
}

// BurnMintOnAccount burns coins from an account, mints new coins with price fetched from oracle and
// sends minted coins to the same account
func (k *keeper) BurnMintOnAccount(sctx sdk.Context, addr sdk.AccAddress, burnCoin sdk.Coin, toDenom string) (sdk.DecCoin, error) {
	burn, mint, err := k.prepareToBM(sctx, burnCoin, toDenom)
	if err != nil {
		return sdk.DecCoin{}, err
	}

	preRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromAccountToModule(sctx, addr, bmetypes.ModuleName, sdk.NewCoins(burnCoin))
	}

	postRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromModuleToAccount(sctx, bmetypes.ModuleName, addr, sdk.NewCoins(mint.Coin))
	}

	if burn.Coin.Denom == sdkutil.DenomUact {
		err = k.mintACT(sctx, burn, mint, addr, addr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	} else {
		err = k.burnACT(sctx, burn, mint, addr, addr, preRun, postRun)
		if err != nil {
			return sdk.DecCoin{}, err
		}
	}

	return sdk.NewDecCoin(mint.Coin.Denom, mint.Coin.Amount), nil
}

// prepareToBM validate fetch prices and calculate amount to be minted
// check if there is enough balance to burn happens in burnMint function after preRun call
// which sends funds from source account/module to the bme module
func (k *keeper) prepareToBM(sctx sdk.Context, burnCoin sdk.Coin, toDenom string) (bmetypes.CoinPrice, bmetypes.CoinPrice, error) {
	params, err := k.GetParams(sctx)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
	}

	//if !params.Enabled {
	//	return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, bmetypes.ErrModuleDisabled
	//}

	priceFrom, err := k.oracleKeeper.GetAggregatedPrice(sctx, burnCoin.Denom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
	}

	priceTo, err := k.oracleKeeper.GetAggregatedPrice(sctx, toDenom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
	}

	if !((burnCoin.Denom == sdkutil.DenomUakt) && (toDenom == sdkutil.DenomUact)) &&
		!((burnCoin.Denom == sdkutil.DenomUact) && (toDenom == sdkutil.DenomUakt)) {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, bmetypes.ErrInvalidDenom.Wrapf("invalid swap route %s -> %s", burnCoin.Denom, toDenom)
	}

	// calculate a swap ratio
	// 1. ACT price is always $1.00
	// 2. AKT price from oracle is $1.14
	// burn 100ACT to mint AKT
	//  swap rate = ($1.00 / $1.14) == 0.87719298
	//  akt to mint = ACT * swap_rate
	//    akt = (100 * 0.87719298) == 87.719298AKT
	swapRate := priceFrom.Quo(priceTo)

	// if burned token is ACT then check it's total supply
	// and return error when there is not enough ACT to burn
	if burnCoin.Denom == sdkutil.DenomUact {
		totalSupply := k.bankKeeper.GetSupply(sctx, burnCoin.Denom)
		if totalSupply.IsLT(burnCoin) {
			return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, bmetypes.ErrInsufficientVaultFunds.Wrapf("requested burn amount: %s (requested to burn) > %s (total supply)", burnCoin, totalSupply)
		}
	} else {
		totalSupply := k.bankKeeper.GetSupply(sctx, toDenom)

		// any other token (at this moment AKT only) must be checked against CR
		crStatus, err := k.getCircuitBreakerStatus(sctx, params, toDenom, swapRate, totalSupply)
		if err != nil {
			return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
		}

		if crStatus == bmetypes.CircuitBreakerStatusHalt {
			return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, bmetypes.ErrCircuitBreakerActive
		}
	}

	mintAmount := sdkmath.LegacyNewDecFromInt(burnCoin.Amount).Mul(swapRate).TruncateInt()
	mintCoin := sdk.NewCoin(toDenom, mintAmount)

	toBurn := bmetypes.CoinPrice{
		Coin:  burnCoin,
		Price: priceFrom,
	}

	toMint := bmetypes.CoinPrice{
		Coin:  mintCoin,
		Price: priceTo,
	}

	return toBurn, toMint, nil
}

// mintACT performs actual ACT mint
// it does not check if CR is active, so it is caller's responsibility to ensure burn/mint
// can actually be performed.
func (k *keeper) mintACT(
	sctx sdk.Context,
	burn bmetypes.CoinPrice,
	mint bmetypes.CoinPrice,
	srcAddr sdk.Address,
	dstAddr sdk.Address,
	preRun func(sdk.Context) error,
	postRun func(sdk.Context) error,
) error {
	// preRun sends coins to be burned from source (either address or another module) to this module
	if err := preRun(sctx); err != nil {
		return err
	}

	if err := k.bankKeeper.MintCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(mint.Coin)); err != nil {
		return bmetypes.ErrMintFailed.Wrapf("failed to mint %s: %s", mint.Coin.Denom, err)
	}

	if err := postRun(sctx); err != nil {
		return err
	}

	if err := k.recordState(sctx, srcAddr, dstAddr, burn, mint); err != nil {
		return err
	}

	return nil
}

// burnMint performs actual ACT burn
// it does not check if CR is active, so it is caller's responsibility to ensure burn/mint
// can actually be performed.
func (k *keeper) burnACT(
	sctx sdk.Context,
	burn bmetypes.CoinPrice,
	mint bmetypes.CoinPrice,
	srcAddr sdk.Address,
	dstAddr sdk.Address,
	preRun func(sdk.Context) error,
	postRun func(sdk.Context) error,
) error {
	// preRun sends coins to be burned from source (either address or another module) to this module
	if err := preRun(sctx); err != nil {
		return err
	}

	if err := k.bankKeeper.BurnCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(burn.Coin)); err != nil {
		return bmetypes.ErrBurnFailed.Wrapf("failed to burn %s: %s", burn.Coin.Denom, err)
	}

	if err := postRun(sctx); err != nil {
		return err
	}

	if err := k.recordState(sctx, srcAddr, dstAddr, burn, mint); err != nil {
		return err
	}

	return nil
}

//// burnMint performs actual burn/mint of the tokens
//// it does not check if CR is active, so it is caller's responsibility to ensure burn/mint
//// can actually be performed.
//func (k *keeper) burnMint(
//	sctx sdk.Context,
//	burn bmetypes.CoinPrice,
//	mint bmetypes.CoinPrice,
//	srcAddr sdk.Address,
//	dstAddr sdk.Address,
//	preRun func(sdk.Context) error,
//	postRun func(sdk.Context) error,
//) error {
//	// preRun sends coins to be burned from source (either address or another module) to this module
//	if err := preRun(sctx); err != nil {
//		return err
//	}
//
//	if err := k.bankKeeper.BurnCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(burn.Coin)); err != nil {
//		return bmetypes.ErrBurnFailed.Wrapf("failed to burn %s: %s", burn.Coin.Denom, err)
//	}
//
//	if err := k.bankKeeper.MintCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(mint.Coin)); err != nil {
//		return bmetypes.ErrMintFailed.Wrapf("failed to mint %s: %s", mint.Coin.Denom, err)
//	}
//
//	if err := postRun(sctx); err != nil {
//		return err
//	}
//
//	if err := k.recordState(sctx, srcAddr, dstAddr, burn, mint); err != nil {
//		return err
//	}
//
//	return nil
//}

func (k *keeper) recordState(sctx sdk.Context, srcAddr sdk.Address, dstAddr sdk.Address, burned bmetypes.CoinPrice, minted bmetypes.CoinPrice) error {
	// sanity checks,
	// burned/minted must not represent the same denom
	if burned.Coin.Denom == minted.Coin.Denom {
		return bmetypes.ErrInvalidDenom.Wrapf("burned minted coins must not be of same denom (%s != %s)", burned.Coin.Denom, minted.Coin.Denom)
	}

	key := collections.Join(sctx.BlockHeight(), k.ledgerSequence)
	exists, err := k.ledger.Has(sctx, key)
	if err != nil {
		return err
	}

	// this should not happen if the following case returns,
	// something went horribly wrong with the sequencer and BeginBlocker
	if exists {
		return bmetypes.ErrRecordExists
	}

	// track remint credits for non-ACT tokens only
	//remintCoin := burned.Coin
	//if burned.Coin.Denom == sdkutil.DenomUact {
	//	remintCoin = minted.Coin
	//}

	//credit, err := k.remintCredits.Get(sctx, remintCoin.Denom)
	//if err != nil {
	//	return err
	//}
	//
	//if burned.Coin.Denom == sdkutil.DenomUact {
	//	credit = credit.Sub(remintCoin.Amount)
	//} else {
	//	credit = credit.Add(remintCoin.Amount)
	//}
	//
	//err = k.remintCredits.Set(sctx, remintCoin.Denom, remintCoin.Amount)
	//if err != nil {
	//	return err
	//}

	record := bmetypes.BMRecord{
		BurnedFrom: srcAddr.String(),
		MintedTo:   dstAddr.String(),
		Burner:     bmetypes.ModuleName,
		Minter:     bmetypes.ModuleName,
		Burned:     burned,
		Minted:     minted,
	}

	err = k.ledger.Set(sctx, key, record)
	if err != nil {
		return err
	}

	err = sctx.EventManager().EmitTypedEvent(record.ToEvent())
	if err != nil {
		return err
	}

	k.ledgerSequence++

	return nil
}

func (k *keeper) GetCircuitBreakerStatus(ctx sdk.Context) (bmetypes.CircuitBreakerStatus, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return bmetypes.CircuitBreakerStatusUnspecified, err
	}

	priceA, err := k.oracleKeeper.GetAggregatedPrice(ctx, sdkutil.DenomUakt)
	if err != nil {
		return bmetypes.CircuitBreakerStatusUnspecified, err
	}

	priceB, err := k.oracleKeeper.GetAggregatedPrice(ctx, sdkutil.DenomUact)
	if err != nil {
		return bmetypes.CircuitBreakerStatusUnspecified, err
	}

	// calculate a swap ratio
	// 1. ACT price is always $1.00
	// 2. AKT price from oracle is $1.14
	// burn 100ACT to mint AKT
	//  swap rate = ($1.00 / $1.14) == 0.87719298
	//  akt to mint = ACT * swap_rate
	//    akt = (100 * 0.87719298) == 87.719298AKT
	swapRate := priceA.Quo(priceB)

	totalSupply := k.bankKeeper.GetSupply(ctx, sdkutil.DenomUact)

	return k.getCircuitBreakerStatus(ctx, params, sdkutil.DenomUakt, swapRate, totalSupply)
}

// getCircuitBreakerStatus returns the current circuit breaker status
func (k *keeper) getCircuitBreakerStatus(
	ctx sdk.Context,
	params bmetypes.Params,
	denomA string,
	swapRate sdkmath.LegacyDec,
	coinB sdk.Coin,
) (bmetypes.CircuitBreakerStatus, error) {
	cr, err := k.getCollateralRatio(ctx, denomA, swapRate, coinB)
	if err != nil {
		return bmetypes.CircuitBreakerStatusUnspecified, err
	}

	warnThreshold := sdkmath.LegacyNewDec(int64(params.CircuitBreakerWarnThreshold)).Quo(sdkmath.LegacyNewDec(10000))
	haltThreshold := sdkmath.LegacyNewDec(int64(params.CircuitBreakerHaltThreshold)).Quo(sdkmath.LegacyNewDec(10000))

	if cr.LT(haltThreshold) {
		return bmetypes.CircuitBreakerStatusHalt, nil
	}

	if cr.LT(warnThreshold) {
		return bmetypes.CircuitBreakerStatusWarning, nil
	}

	return bmetypes.CircuitBreakerStatusHealthy, nil
}

// GetCollateralRatio calculates CR,
// for example, CR = (bme balance of AKT * price in USD) / bme balance of ACT
func (k *keeper) GetCollateralRatio(sctx sdk.Context) (sdkmath.LegacyDec, error) {
	priceA, err := k.oracleKeeper.GetAggregatedPrice(sctx, sdkutil.DenomUakt)
	if err != nil {
		return sdkmath.LegacyZeroDec(), err
	}

	priceB, err := k.oracleKeeper.GetAggregatedPrice(sctx, sdkutil.DenomUact)
	if err != nil {
		return sdkmath.LegacyZeroDec(), err
	}

	// calculate a swap ratio
	// 1. ACT price is always $1.00
	// 2. AKT price from oracle is $1.14
	// burn 100ACT to mint AKT
	//  swap rate = ($1.00 / $1.14) == 0.87719298
	//  akt to mint = ACT * swap_rate
	//    akt = (100 * 0.87719298) == 87.719298AKT
	swapRate := priceA.Quo(priceB)

	totalSupply := k.bankKeeper.GetSupply(sctx, sdkutil.DenomUact)

	return k.getCollateralRatio(sctx, sdkutil.DenomUakt, swapRate, totalSupply)
}

func (k *keeper) getCollateralRatio(sctx sdk.Context, denomA string, swapRate sdkmath.LegacyDec, coinB sdk.Coin) (sdkmath.LegacyDec, error) {
	if coinB.Denom != sdkutil.DenomUact {
		return sdkmath.LegacyDec{}, bmetypes.ErrInvalidDenom.Wrapf("unsupported CR denom %s", coinB.Denom)
	}

	macc := k.accKeeper.GetModuleAddress(bmetypes.ModuleName)
	balanceA := k.bankKeeper.GetBalance(sctx, macc, denomA)

	cr := sdkmath.LegacyNewDecFromInt(balanceA.Amount).Mul(swapRate).Quo(coinB.Amount.ToLegacyDec())

	return cr, nil
}

// BeginBlocker is called at the beginning of each block
func (k *keeper) BeginBlocker(_ context.Context) error {
	// reset the ledger sequence on each new block
	k.ledgerSequence = 0

	return nil
}

// EndBlocker is called at the end of each block to manage snapshots.
// It records periodic snapshots and prunes old ones.
func (k *keeper) EndBlocker(_ context.Context) error {
	return nil
}

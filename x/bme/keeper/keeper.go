package keeper

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
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
	Schema() collections.Schema
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetParams(sdk.Context) (bmetypes.Params, error)
	SetParams(sdk.Context, bmetypes.Params) error

	AddLedgerRecord(sdk.Context, bmetypes.LedgerRecordID, bmetypes.LedgerRecord) error
	AddLedgerPendingRecord(sdk.Context, bmetypes.LedgerRecordID, bmetypes.LedgerPendingRecord) error

	IterateLedgerRecords(sctx sdk.Context, f func(bmetypes.LedgerRecordID, bmetypes.LedgerRecord) (bool, error)) error
	IterateLedgerPendingRecords(sdk.Context, func(bmetypes.LedgerRecordID, bmetypes.LedgerPendingRecord) (bool, error)) error

	GetState(sdk.Context) (bmetypes.State, error)

	GetMintStatus(sdk.Context) (bmetypes.MintStatus, error)
	GetCollateralRatio(sdk.Context) (sdkmath.LegacyDec, error)

	BeginBlocker(_ context.Context) error
	EndBlocker(context.Context) error

	RequestBurnMint(ctx context.Context, srcAddr sdk.AccAddress, dstAddr sdk.AccAddress, burnCoin sdk.Coin, toDenom string) (bmetypes.LedgerRecordID, error)

	InitGenesis(ctx sdk.Context, data *bmetypes.GenesisState)
	ExportGenesis(ctx sdk.Context) *bmetypes.GenesisState

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
	cdc       codec.BinaryCodec
	skey      *storetypes.KVStoreKey
	ssvc      store.KVStoreService
	ac        address.Codec
	authority string

	schema    collections.Schema
	Params    collections.Item[bmetypes.Params]
	status    collections.Item[bmetypes.Status]
	mintEpoch collections.Item[bmetypes.MintEpoch]
	//mintStatusRecords collections.Map[int64, bmetypes.CircuitBreaker]
	totalBurned    collections.Map[string, sdkmath.Int]
	totalMinted    collections.Map[string, sdkmath.Int]
	remintCredits  collections.Map[string, sdkmath.Int]
	ledgerPending  collections.Map[bmetypes.LedgerRecordID, bmetypes.LedgerPendingRecord]
	ledger         collections.Map[bmetypes.LedgerRecordID, bmetypes.LedgerRecord]
	ledgerSequence int64

	accKeeper    bmeimports.AccountKeeper
	bankKeeper   bmeimports.BankKeeper
	oracleKeeper bmeimports.OracleKeeper
}

func NewKeeper(
	cdc codec.BinaryCodec,
	skey *storetypes.KVStoreKey,
	ac address.Codec,
	authority string,
	accKeeper bmeimports.AccountKeeper,
	bankKeeper bmeimports.BankKeeper,
	oracleKeeper bmeimports.OracleKeeper,
) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	k := &keeper{
		cdc:           cdc,
		skey:          skey,
		ac:            ac,
		ssvc:          ssvc,
		authority:     authority,
		accKeeper:     accKeeper,
		bankKeeper:    bankKeeper,
		oracleKeeper:  oracleKeeper,
		Params:        collections.NewItem(sb, ParamsKey, "params", codec.CollValue[bmetypes.Params](cdc)),
		status:        collections.NewItem(sb, MintStatusKey, "mint_status", codec.CollValue[bmetypes.Status](cdc)),
		mintEpoch:     collections.NewItem(sb, MintEpochKey, "mint_epoch", codec.CollValue[bmetypes.MintEpoch](cdc)),
		remintCredits: collections.NewMap(sb, RemintCreditsKey, "remint_credits", collections.StringKey, sdk.IntValue),
		totalBurned:   collections.NewMap(sb, TotalBurnedKey, "total_burned", collections.StringKey, sdk.IntValue),
		totalMinted:   collections.NewMap(sb, TotalMintedKey, "total_minted", collections.StringKey, sdk.IntValue),
		ledgerPending: collections.NewMap(sb, LedgerPendingKey, "ledger_pending", ledgerRecordIDCodec{}, codec.CollValue[bmetypes.LedgerPendingRecord](cdc)),
		ledger:        collections.NewMap(sb, LedgerKey, "ledger", ledgerRecordIDCodec{}, codec.CollValue[bmetypes.LedgerRecord](cdc)),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.schema = schema

	return k
}

func (k *keeper) Schema() collections.Schema {
	return k.schema
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

func (k *keeper) AddLedgerRecord(sctx sdk.Context, id bmetypes.LedgerRecordID, record bmetypes.LedgerRecord) error {
	return k.ledger.Set(sctx, id, record)
}

func (k *keeper) AddLedgerPendingRecord(sctx sdk.Context, id bmetypes.LedgerRecordID, record bmetypes.LedgerPendingRecord) error {
	return k.ledgerPending.Set(sctx, id, record)
}

func (k *keeper) IterateLedgerRecords(sctx sdk.Context, f func(bmetypes.LedgerRecordID, bmetypes.LedgerRecord) (bool, error)) error {
	return k.ledger.Walk(sctx, nil, f)
}

func (k *keeper) IterateLedgerPendingRecords(sctx sdk.Context, f func(bmetypes.LedgerRecordID, bmetypes.LedgerPendingRecord) (bool, error)) error {
	return k.ledgerPending.Walk(sctx, nil, f)
}

func (k *keeper) GetState(ctx sdk.Context) (bmetypes.State, error) {
	addr := k.accKeeper.GetModuleAddress(bmetypes.ModuleName)

	balances := k.bankKeeper.GetAllBalances(ctx, addr)

	actSupply := k.bankKeeper.GetSupply(ctx, sdkutil.DenomUact)
	actBalance := k.bankKeeper.GetBalance(ctx, addr, sdkutil.DenomUact)

	actBalance = actSupply.Sub(actBalance)
	balances = balances.Add(actBalance)

	res := bmetypes.State{
		Balances: balances,
	}

	err := k.totalBurned.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
		res.TotalBurned = append(res.TotalBurned, sdk.NewCoin(denom, value))

		return false, nil
	})
	if err != nil {
		return res, err
	}

	err = k.totalMinted.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
		res.TotalMinted = append(res.TotalMinted, sdk.NewCoin(denom, value))

		return false, nil
	})
	if err != nil {
		return res, err
	}

	err = k.remintCredits.Walk(ctx, nil, func(denom string, value sdkmath.Int) (stop bool, err error) {
		res.RemintCredits = append(res.RemintCredits, sdk.NewCoin(denom, value))

		return false, nil
	})

	if err != nil {
		return res, err
	}

	return res, nil
}

// BurnMintFromModuleAccountToAddress burns coins from a module account, mints new coins with price fetched from oracle,
// and sends minted coins to an account
func (k *keeper) executeBurnMint(
	sctx sdk.Context,
	id bmetypes.LedgerRecordID,
	srcAddr sdk.AccAddress,
	dstAddr sdk.AccAddress,
	burnCoin sdk.Coin,
	toDenom string,
) error {
	burn, mint, err := k.prepareToBM(sctx, burnCoin, toDenom)
	if err != nil {
		return err
	}

	postRun := func(sctx sdk.Context) error {
		return k.bankKeeper.SendCoinsFromModuleToAccount(sctx, bmetypes.ModuleName, dstAddr, sdk.NewCoins(mint.Coin))
	}

	if burn.Coin.Denom == sdkutil.DenomUakt {
		err = k.mintACT(sctx, id, burn, mint, srcAddr, dstAddr, postRun)
		if err != nil {
			return err
		}
	} else {
		err = k.burnACT(sctx, id, burn, mint, srcAddr, dstAddr, postRun)
		if err != nil {
			return err
		}
	}

	return nil
}

// prepareToBM validate fetch prices and calculate the amount to be minted
// check if there is enough balance to burn happens in burnMint function after preRun call
// which sends funds from source account/module to the bme module
func (k *keeper) prepareToBM(sctx sdk.Context, burnCoin sdk.Coin, toDenom string) (bmetypes.CoinPrice, bmetypes.CoinPrice, error) {
	priceFrom, err := k.oracleKeeper.GetAggregatedPrice(sctx, burnCoin.Denom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
	}

	priceTo, err := k.oracleKeeper.GetAggregatedPrice(sctx, toDenom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, err
	}

	//if !((burnCoin.Denom == sdkutil.DenomUakt) && (toDenom == sdkutil.DenomUact)) &&
	//	!((burnCoin.Denom == sdkutil.DenomUact) && (toDenom == sdkutil.DenomUakt)) {
	//	return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, bmetypes.ErrInvalidDenom.Wrapf("invalid swap route %s -> %s", burnCoin.Denom, toDenom)
	//}

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
	id bmetypes.LedgerRecordID,
	burn bmetypes.CoinPrice,
	mint bmetypes.CoinPrice,
	srcAddr sdk.Address,
	dstAddr sdk.Address,
	postRun func(sdk.Context) error,
) error {
	remintIssued := bmetypes.CoinPrice{
		Coin:  sdk.NewCoin(mint.Coin.Denom, sdkmath.ZeroInt()),
		Price: mint.Price,
	}

	if err := k.bankKeeper.MintCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(mint.Coin)); err != nil {
		return bmetypes.ErrMintFailed.Wrapf("failed to mint %s: %s", mint.Coin.Denom, err)
	}

	if err := postRun(sctx); err != nil {
		return err
	}

	if err := k.recordState(sctx, id, srcAddr, dstAddr, burn, mint, remintIssued); err != nil {
		return err
	}

	return nil
}

// burnMint performs actual ACT burn
// it does not check if CR is active, so it is caller's responsibility to ensure burn/mint
// can actually be performed.
func (k *keeper) burnACT(
	sctx sdk.Context,
	id bmetypes.LedgerRecordID,
	burn bmetypes.CoinPrice,
	mint bmetypes.CoinPrice,
	srcAddr sdk.Address,
	dstAddr sdk.Address,
	postRun func(sdk.Context) error,
) error {
	toMint := bmetypes.CoinPrice{
		Coin:  sdk.NewCoin(mint.Coin.Denom, sdkmath.ZeroInt()),
		Price: mint.Price,
	}

	remintIssued := bmetypes.CoinPrice{
		Coin:  sdk.NewCoin(mint.Coin.Denom, sdkmath.ZeroInt()),
		Price: mint.Price,
	}

	remintCredit, err := k.remintCredits.Get(sctx, sdkutil.DenomUakt)
	if err != nil {
		return err
	}

	// if there is enough remint credit, issue reminted coins only
	if remintCredit.GTE(mint.Coin.Amount) {
		remintIssued = bmetypes.CoinPrice{
			Coin:  mint.Coin,
			Price: mint.Price,
		}
	} else {
		// we're shortfall here, need to mint
		toMint = bmetypes.CoinPrice{
			Coin:  mint.Coin.Sub(sdk.NewCoin(mint.Coin.Denom, remintCredit)),
			Price: mint.Price,
		}

		if remintCredit.GT(sdkmath.ZeroInt()) {
			remintIssued = bmetypes.CoinPrice{
				Coin:  sdk.NewCoin(mint.Coin.Denom, remintCredit.Add(sdkmath.ZeroInt())),
				Price: mint.Price,
			}

			toMint.Coin = mint.Coin.Sub(sdk.NewCoin(mint.Coin.Denom, remintCredit))
		}
	}

	if err = k.bankKeeper.BurnCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(burn.Coin)); err != nil {
		return bmetypes.ErrBurnFailed.Wrapf("failed to burn %s: %s", burn.Coin.Denom, err)
	}

	if toMint.Coin.Amount.GT(sdkmath.ZeroInt()) {
		if err = k.bankKeeper.MintCoins(sctx, bmetypes.ModuleName, sdk.NewCoins(toMint.Coin)); err != nil {
			return bmetypes.ErrBurnFailed.Wrapf("failed to mint %s: %s", toMint.Coin.Denom, err)
		}
	}

	if err = postRun(sctx); err != nil {
		return err
	}

	if err = k.recordState(sctx, id, srcAddr, dstAddr, burn, toMint, remintIssued); err != nil {
		return err
	}

	return nil
}

func (k *keeper) recordState(
	sctx sdk.Context,
	id bmetypes.LedgerRecordID,
	srcAddr sdk.Address,
	dstAddr sdk.Address,
	burned bmetypes.CoinPrice,
	minted bmetypes.CoinPrice,
	remintIssued bmetypes.CoinPrice,
) error {
	// sanity checks,
	// burned/minted must not represent the same denom
	if burned.Coin.Denom == minted.Coin.Denom {
		return bmetypes.ErrInvalidDenom.Wrapf("burned/minted coins must not be of same denom (%s != %s)", burned.Coin.Denom, minted.Coin.Denom)
	}

	if minted.Coin.Amount.Equal(sdkmath.ZeroInt()) && remintIssued.Coin.Amount.Equal(sdkmath.ZeroInt()) {
		return bmetypes.ErrInvalidAmount.Wrapf("minted must not be 0 if remintIssued is 0")
	}

	exists, err := k.ledger.Has(sctx, id)
	if err != nil {
		return err
	}

	// this should not happen if the following case returns,
	// something went horribly wrong with the sequencer and BeginBlocker
	if exists {
		return bmetypes.ErrRecordExists
	}

	var rBurned *bmetypes.CoinPrice
	var rMinted *bmetypes.CoinPrice
	var remintCreditAccrued *bmetypes.CoinPrice
	var remintCreditIssued *bmetypes.CoinPrice

	// remint accruals are tracked for non-ACT tokens only
	if burned.Coin.Denom == sdkutil.DenomUakt {
		coin := burned.Coin
		remintCredit, err := k.remintCredits.Get(sctx, coin.Denom)
		if err != nil {
			return err
		}

		remintCredit = remintCredit.Add(coin.Amount)
		if err = k.remintCredits.Set(sctx, coin.Denom, remintCredit); err != nil {
			return err
		}

		remintCreditAccrued = &bmetypes.CoinPrice{
			Coin:  coin,
			Price: burned.Price,
		}
	} else {
		rBurned = &bmetypes.CoinPrice{
			Coin:  burned.Coin,
			Price: burned.Price,
		}
	}

	if remintIssued.Coin.Amount.GT(sdkmath.ZeroInt()) {
		coin := remintIssued.Coin
		remintCredit, err := k.remintCredits.Get(sctx, coin.Denom)
		if err != nil {
			return err
		}

		remintCredit = remintCredit.Sub(coin.Amount)
		if err = k.remintCredits.Set(sctx, coin.Denom, remintCredit); err != nil {
			return err
		}

		remintCreditIssued = &bmetypes.CoinPrice{
			Coin:  remintIssued.Coin,
			Price: remintIssued.Price,
		}
	}

	if minted.Coin.Amount.GT(sdkmath.ZeroInt()) {
		mint, err := k.totalMinted.Get(sctx, minted.Coin.Denom)
		if err != nil {
			return err
		}

		mint = mint.Add(minted.Coin.Amount)
		err = k.totalMinted.Set(sctx, minted.Coin.Denom, mint)
		if err != nil {
			return err
		}

		rMinted = &minted
	}

	if rBurned != nil && rBurned.Coin.Amount.GT(sdkmath.ZeroInt()) {
		burn, err := k.totalBurned.Get(sctx, rBurned.Coin.Denom)
		if err != nil {
			return err
		}

		burn = burn.Add(rBurned.Coin.Amount)
		err = k.totalBurned.Set(sctx, rBurned.Coin.Denom, burn)
		if err != nil {
			return err
		}
	}

	record := bmetypes.LedgerRecord{
		BurnedFrom:          srcAddr.String(),
		MintedTo:            dstAddr.String(),
		Burner:              bmetypes.ModuleName,
		Minter:              bmetypes.ModuleName,
		Burned:              rBurned,
		Minted:              rMinted,
		RemintCreditAccrued: remintCreditAccrued,
		RemintCreditIssued:  remintCreditIssued,
	}

	err = k.ledgerPending.Remove(sctx, id)
	if err != nil {
		return err
	}

	err = k.ledger.Set(sctx, id, record)
	if err != nil {
		return err
	}

	err = sctx.EventManager().EmitTypedEvent(&bmetypes.EventLedgerRecordExecuted{
		ID: id,
	})
	if err != nil {
		return err
	}

	k.ledgerSequence++

	return nil
}

func (k *keeper) GetMintStatus(sctx sdk.Context) (bmetypes.MintStatus, error) {
	cb, err := k.status.Get(sctx)
	if err != nil {
		return bmetypes.MintStatusUnspecified, err
	}

	return cb.Status, nil
}

// GetCollateralRatio calculates CR,
// for example, CR = (bme balance of AKT * price in USD) / bme balance of ACT
func (k *keeper) GetCollateralRatio(sctx sdk.Context) (sdkmath.LegacyDec, error) {
	return k.calculateCR(sctx)
}

func (k *keeper) calculateCR(sctx sdk.Context) (sdkmath.LegacyDec, error) {
	cr := sdkmath.LegacyZeroDec()

	priceA, err := k.oracleKeeper.GetAggregatedPrice(sctx, sdkutil.DenomAkt)
	if err != nil {
		return cr, err
	}

	priceB, err := k.oracleKeeper.GetAggregatedPrice(sctx, sdkutil.DenomAct)
	if err != nil {
		return cr, err
	}

	macc := k.accKeeper.GetModuleAddress(bmetypes.ModuleName)
	balanceA := k.bankKeeper.GetBalance(sctx, macc, sdkutil.DenomUakt)

	swapRate := priceA.Quo(priceB)

	cr.AddMut(balanceA.Amount.ToLegacyDec())
	cr.MulMut(swapRate)

	outstandingACT := k.bankKeeper.GetSupply(sctx, sdkutil.DenomUact)
	if outstandingACT.Amount.GT(sdkmath.ZeroInt()) {
		cr.QuoMut(outstandingACT.Amount.ToLegacyDec())
	}

	return cr, nil
}

func (k *keeper) RequestBurnMint(ctx context.Context, srcAddr sdk.AccAddress, dstAddr sdk.AccAddress, burnCoin sdk.Coin, toDenom string) (bmetypes.LedgerRecordID, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	if !((burnCoin.Denom == sdkutil.DenomUakt) && (toDenom == sdkutil.DenomUact)) &&
		!((burnCoin.Denom == sdkutil.DenomUact) && (toDenom == sdkutil.DenomUakt)) {
		return bmetypes.LedgerRecordID{}, bmetypes.ErrInvalidDenom.Wrapf("invalid swap route %s -> %s", burnCoin.Denom, toDenom)
	}

	// do not queue request if circuit breaker is tripper
	_, _, err := k.prepareToBM(sctx, burnCoin, toDenom)
	if err != nil {
		return bmetypes.LedgerRecordID{}, err
	}

	id := bmetypes.LedgerRecordID{
		Denom:    burnCoin.Denom,
		ToDenom:  toDenom,
		Source:   srcAddr.String(),
		Height:   sctx.BlockHeight(),
		Sequence: k.ledgerSequence,
	}

	err = k.bankKeeper.SendCoinsFromAccountToModule(sctx, srcAddr, bmetypes.ModuleName, sdk.NewCoins(burnCoin))
	if err != nil {
		return id, err
	}

	err = k.ledgerPending.Set(ctx, id, bmetypes.LedgerPendingRecord{
		Owner:       srcAddr.String(),
		To:          dstAddr.String(),
		CoinsToBurn: burnCoin,
		DenomToMint: toDenom,
	})

	if err != nil {
		return id, err
	}

	k.ledgerSequence++

	return id, nil
}

func (k *keeper) mintStatusUpdate(sctx sdk.Context) (bmetypes.Status, bool) {
	params, err := k.GetParams(sctx)
	if err != nil {
		// if unable to load params, something went horribly wrong
		panic(err)
	}

	cb, err := k.status.Get(sctx)
	if err != nil {
		// if unable to load circuit breaker state, something went horribly wrong
		panic(err)
	}
	pCb := cb

	cr, err := k.calculateCR(sctx)
	if err != nil {
		if cb.Status != bmetypes.MintStatusHaltCR {
			cb.Status = bmetypes.MintStatusHaltOracle
		}
	} else {
		crInt := uint32(cr.Mul(sdkmath.LegacyNewDec(10000)).TruncateInt64())
		if crInt > params.CircuitBreakerWarnThreshold {
			cb.Status = bmetypes.MintStatusHealthy
			cb.EpochHeightDiff = calculateBlocksDiff(params, crInt)
		} else if (crInt <= params.CircuitBreakerWarnThreshold) && (crInt > params.CircuitBreakerWarnThreshold) {
			cb.Status = bmetypes.MintStatusWarning
			cb.EpochHeightDiff = calculateBlocksDiff(params, crInt)
		} else {
			// halt ACT mint
			cb.Status = bmetypes.MintStatusHaltCR
		}
	}

	changed := !cb.Equal(pCb)

	if changed {
		cb.PreviousStatus = pCb.Status

		err = k.status.Set(sctx, cb)
		if err != nil {
			panic(err)
		}

		err = sctx.EventManager().EmitTypedEvent(&bmetypes.EventMintStatusChange{
			PreviousStatus:  pCb.PreviousStatus,
			NewStatus:       cb.Status,
			CollateralRatio: cr,
		})
		if err != nil {
			sctx.Logger().Error("failed to emit mint status change event", "error", err)
		}
	}

	return cb, changed
}

func calculateBlocksDiff(params bmetypes.Params, cr uint32) int64 {
	if cr >= params.CircuitBreakerWarnThreshold {
		return params.MinEpochBlocks
	}

	steps := int64((params.CircuitBreakerWarnThreshold - cr) / params.EpochBlocksBackoff)

	// Use scaled value to maintain precision
	// Scale by BPS^steps then divide at the end
	scale := int64(1)
	mult := int64(1)

	for i := int64(0); i < steps; i++ {
		mult *= 10000 + int64(params.EpochBlocksBackoff)
		scale *= 10000
	}

	res := (params.MinEpochBlocks * mult) / scale

	if res < params.MinEpochBlocks {
		panic("epoch blocks diff calculation resulted in negative value")
	}

	return res
}

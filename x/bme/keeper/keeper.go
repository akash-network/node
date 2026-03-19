package keeper

import (
	"context"
	"errors"
	"math"
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
	IterateLedgerCanceledRecords(sdk.Context, func(bmetypes.LedgerRecordID, bmetypes.LedgerCanceledRecord) (bool, error)) error

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

	schema                collections.Schema
	Params                collections.Item[bmetypes.Params]
	status                collections.Item[bmetypes.Status]
	mintEpoch             collections.Item[bmetypes.MintEpoch]
	totalBurned           collections.Map[string, sdkmath.Int]
	totalMinted           collections.Map[string, sdkmath.Int]
	remintCredits         collections.Map[string, sdkmath.Int]
	ledgerPendingBalances collections.Map[string, sdkmath.Int]
	ledgerPending         collections.Map[bmetypes.LedgerRecordID, bmetypes.LedgerPendingRecord]
	ledgerCanceled        collections.Map[bmetypes.LedgerRecordID, bmetypes.LedgerCanceledRecord]
	ledger                collections.Map[bmetypes.LedgerRecordID, bmetypes.LedgerRecord]
	ledgerSequence        int64

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
		cdc:                   cdc,
		skey:                  skey,
		ac:                    ac,
		ssvc:                  ssvc,
		authority:             authority,
		accKeeper:             accKeeper,
		bankKeeper:            bankKeeper,
		oracleKeeper:          oracleKeeper,
		Params:                collections.NewItem(sb, ParamsKey, "params", codec.CollValue[bmetypes.Params](cdc)),
		status:                collections.NewItem(sb, MintStatusKey, "mint_status", codec.CollValue[bmetypes.Status](cdc)),
		mintEpoch:             collections.NewItem(sb, MintEpochKey, "mint_epoch", codec.CollValue[bmetypes.MintEpoch](cdc)),
		remintCredits:         collections.NewMap(sb, RemintCreditsKey, "remint_credits", collections.StringKey, sdk.IntValue),
		totalBurned:           collections.NewMap(sb, TotalBurnedKey, "total_burned", collections.StringKey, sdk.IntValue),
		totalMinted:           collections.NewMap(sb, TotalMintedKey, "total_minted", collections.StringKey, sdk.IntValue),
		ledgerPending:         collections.NewMap(sb, LedgerPendingKey, "ledger_pending", ledgerRecordIDCodec{}, codec.CollValue[bmetypes.LedgerPendingRecord](cdc)),
		ledgerCanceled:        collections.NewMap(sb, LedgerFailedKey, "ledger_canceled", ledgerRecordIDCodec{}, codec.CollValue[bmetypes.LedgerCanceledRecord](cdc)),
		ledgerPendingBalances: collections.NewMap(sb, LedgerPendingBalancesKey, "ledger_pending_balances", collections.StringKey, sdk.IntValue),
		ledger:                collections.NewMap(sb, LedgerKey, "ledger", ledgerRecordIDCodec{}, codec.CollValue[bmetypes.LedgerRecord](cdc)),
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

func (k *keeper) IterateLedgerCanceledRecords(sctx sdk.Context, f func(bmetypes.LedgerRecordID, bmetypes.LedgerCanceledRecord) (bool, error)) error {
	return k.ledgerCanceled.Walk(sctx, nil, f)
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

func (k *keeper) executeBurnMint(
	sctx sdk.Context,
	params bmetypes.Params,
	id bmetypes.LedgerRecordID,
	srcAddr sdk.AccAddress,
	dstAddr sdk.AccAddress,
	burnCoin sdk.Coin,
	toDenom string,
) error {
	// sanity check
	if burnCoin.Amount.Equal(sdkmath.ZeroInt()) {
		return bmetypes.ErrInvalidAmount.Wrapf("zero burn amount")
	}

	err := func() error {
		burn, mint, spread, err := k.prepareToBM(sctx, params, burnCoin, toDenom)
		if err != nil {
			return err
		}

		// send user the full mint minus the spread; spread stays in the module account (vault)
		userCoin := mint.Coin.Sub(spread)

		postRun := func(sctx sdk.Context) error {
			return k.bankKeeper.SendCoinsFromModuleToAccount(sctx, bmetypes.ModuleName, dstAddr, sdk.NewCoins(userCoin))
		}

		if burn.Coin.Denom == sdkutil.DenomUakt {
			return k.mintACT(sctx, id, burn, mint, spread, srcAddr, dstAddr, postRun)
		}

		return k.burnACT(sctx, id, burn, mint, spread, srcAddr, dstAddr, postRun)
	}()

	if err != nil {
		return k.cancelBurnMint(sctx, id, srcAddr, dstAddr, burnCoin, toDenom, err)
	}

	return nil
}

// cancelBurnMint records a failed burn/mint operation, refunds coins to the owner,
// and cleans up the pending ledger state.
func (k *keeper) cancelBurnMint(
	sctx sdk.Context,
	id bmetypes.LedgerRecordID,
	srcAddr sdk.AccAddress,
	dstAddr sdk.AccAddress,
	burnCoin sdk.Coin,
	toDenom string,
	reason error,
) error {
	cancelReason := bmetypes.BMCancelReasonUnknown
	if errors.Is(reason, bmetypes.ErrEpsilon) {
		cancelReason = bmetypes.BMCancelReasonEpsilon
	}

	if err := k.ledgerCanceled.Set(sctx, id, bmetypes.LedgerCanceledRecord{
		Owner:        srcAddr.String(),
		To:           dstAddr.String(),
		CancelReason: cancelReason,
		CoinsToBurn:  burnCoin,
		DenomToMint:  toDenom,
	}); err != nil {
		return err
	}

	if err := k.ledgerPending.Remove(sctx, id); err != nil {
		return err
	}

	pendingBalance, err := k.ledgerPendingBalances.Get(sctx, burnCoin.Denom)
	if err != nil {
		return err
	}

	pendingBalance = pendingBalance.Sub(burnCoin.Amount)
	if err = k.ledgerPendingBalances.Set(sctx, burnCoin.Denom, pendingBalance); err != nil {
		return err
	}

	// refund coins to the original owner
	if err = k.bankKeeper.SendCoinsFromModuleToAccount(sctx, bmetypes.ModuleName, srcAddr, sdk.NewCoins(burnCoin)); err != nil {
		return err
	}

	err = sctx.EventManager().EmitTypedEvent(&bmetypes.EventLedgerRecordCanceled{
		ID:           id,
		CancelReason: cancelReason,
		Owner:        srcAddr.String(),
		To:           dstAddr.String(),
		CoinsToBurn:  burnCoin,
		DenomToMint:  toDenom,
	})
	if err != nil {
		return err
	}

	return nil
}

// prepareToBM validate fetch prices and calculate the amount to be minted
// check if there are enough balances to burn happens in burnMint function after preRun call
// which sends funds from the source account / module to the bme module
func (k *keeper) prepareToBM(sctx sdk.Context, params bmetypes.Params, burnCoin sdk.Coin, toDenom string) (bmetypes.CoinPrice, bmetypes.CoinPrice, sdk.Coin, error) {
	zeroSpread := sdk.NewCoin(toDenom, sdkmath.ZeroInt())

	priceFrom, err := k.oracleKeeper.GetAggregatedPrice(sctx, burnCoin.Denom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, err
	}

	priceTo, err := k.oracleKeeper.GetAggregatedPrice(sctx, toDenom)
	if err != nil {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, err
	}

	if priceFrom.IsZero() || priceTo.IsZero() {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, bmetypes.ErrZeroPrice.Wrapf("oracle prices must be non-zero (%s=%s, %s=%s)", burnCoin.Denom, priceFrom, toDenom, priceTo)
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
			return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, bmetypes.ErrInsufficientVaultFunds.Wrapf("requested burn amount: %s (requested to burn) > %s (total supply)", burnCoin, totalSupply)
		}
	}

	denomUnit, found := sdk.GetDenomUnit(toDenom)
	if !found {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, bmetypes.ErrInvalidDenom.Wrapf("denom %s is not registered", toDenom)
	}

	mintAmountDec := sdkmath.LegacyNewDecFromInt(burnCoin.Amount).Mul(swapRate)
	mintAmount := mintAmountDec.TruncateInt()

	// check if the result after truncation is zero (below the smallest unit).
	// note: we check after TruncateInt rather than using mintAmountDec.Mul(denomUnit).LT(denomUnit)
	// because sdk.Dec.Mul uses chopPrecisionAndRound which can round up edge cases like
	// 0.999999999999999999 * 10^-6 → 10^-6 (equals denomUnit), masking a zero truncation.
	if !mintAmount.IsPositive() {
		return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, bmetypes.ErrEpsilon.Wrapf("result %s truncates to zero for %s (denomUnit %s)", mintAmountDec, toDenom, denomUnit)
	}

	// enforce minimum mint amount per denomination
	for _, minCoin := range params.MinMint {
		if minCoin.Denom == toDenom && minCoin.Amount.IsPositive() && mintAmount.LT(minCoin.Amount) {
			return bmetypes.CoinPrice{}, bmetypes.CoinPrice{}, zeroSpread, bmetypes.ErrMinimumMint.Wrapf(
				"mint output %s is below minimum %s", sdk.NewCoin(toDenom, mintAmount), minCoin)
		}
	}

	// calculate spread: full amount is minted, spread portion stays in vault
	var spreadBps uint32
	if burnCoin.Denom == sdkutil.DenomUakt {
		spreadBps = params.MintSpreadBps // AKT->ACT mint spread
	} else {
		spreadBps = params.SettleSpreadBps // ACT->AKT settle spread
	}

	spreadCoin := zeroSpread
	if spreadBps > 0 {
		spreadAmountDec := mintAmountDec.Mul(sdkmath.LegacyNewDec(int64(spreadBps))).Quo(sdkmath.LegacyNewDec(10000))
		spreadAmount := spreadAmountDec.TruncateInt()
		if spreadAmount.IsPositive() {
			spreadCoin = sdk.NewCoin(toDenom, spreadAmount)
		}
	}

	mintCoin := sdk.NewCoin(toDenom, mintAmount)

	toBurn := bmetypes.CoinPrice{
		Coin:  burnCoin,
		Price: priceFrom,
	}

	toMint := bmetypes.CoinPrice{
		Coin:  mintCoin,
		Price: priceTo,
	}

	return toBurn, toMint, spreadCoin, nil
}

// mintACT performs actual ACT mint
// it does not check if CR is active, so it is caller's responsibility to ensure burn/mint
// can actually be performed.
func (k *keeper) mintACT(
	sctx sdk.Context,
	id bmetypes.LedgerRecordID,
	burn bmetypes.CoinPrice,
	mint bmetypes.CoinPrice,
	spread sdk.Coin,
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

	if err := k.recordState(sctx, id, srcAddr, dstAddr, burn, mint, spread, remintIssued); err != nil {
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
	spread sdk.Coin,
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

	if err = k.recordState(sctx, id, srcAddr, dstAddr, burn, toMint, spread, remintIssued); err != nil {
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
	spread sdk.Coin,
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

	pendingBalance, err := k.ledgerPendingBalances.Get(sctx, burned.Coin.Denom)
	if err != nil {
		return err
	}

	pendingBalance = pendingBalance.Sub(burned.Coin.Amount)

	err = k.ledgerPendingBalances.Set(sctx, burned.Coin.Denom, pendingBalance)
	if err != nil {
		return err
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
		Spread:              spread,
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
		ID:                  id,
		BurnedFrom:          srcAddr.String(),
		MintedTo:            dstAddr.String(),
		Burner:              bmetypes.ModuleName,
		Minter:              bmetypes.ModuleName,
		Burned:              rBurned,
		Minted:              rMinted,
		Spread:              spread,
		RemintCreditAccrued: remintCreditAccrued,
		RemintCreditIssued:  remintCreditIssued,
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

	if priceA.IsZero() || priceB.IsZero() {
		return cr, bmetypes.ErrZeroPrice.Wrapf("oracle prices must be non-zero (AKT=%s, ACT=%s)", priceA, priceB)
	}

	macc := k.accKeeper.GetModuleAddress(bmetypes.ModuleName)
	balanceA := k.bankKeeper.GetBalance(sctx, macc, sdkutil.DenomUakt)

	pendingBalance, err := k.ledgerPendingBalances.Get(sctx, sdkutil.DenomUakt)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return cr, err
		}

		pendingBalance = sdkmath.ZeroInt()
	}

	balanceA = balanceA.Sub(sdk.NewCoin(balanceA.Denom, pendingBalance))

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

	// do not queue request if oracle price is not healthy or circuit breaker is tripped
	_, err := k.oracleKeeper.GetAggregatedPrice(sctx, burnCoin.Denom)
	if err != nil {
		return bmetypes.LedgerRecordID{}, err
	}

	_, err = k.oracleKeeper.GetAggregatedPrice(sctx, toDenom)
	if err != nil {
		return bmetypes.LedgerRecordID{}, err
	}

	status, err := k.status.Get(sctx)
	if err != nil {
		return bmetypes.LedgerRecordID{}, err
	}

	if status.Status >= bmetypes.MintStatusHaltCR {
		// ACT refunds to AKT are not allowed only when a circuit breaker is tripped due to CR
		if (status.Status > bmetypes.MintStatusHaltCR) ||
			!((burnCoin.Denom == sdkutil.DenomUact) && (toDenom == sdkutil.DenomUakt)) {
			return bmetypes.LedgerRecordID{}, bmetypes.ErrCircuitBreakerActive
		}
	}

	id := bmetypes.LedgerRecordID{
		Denom:    burnCoin.Denom,
		ToDenom:  toDenom,
		Source:   srcAddr.String(),
		Height:   sctx.BlockHeight(),
		Sequence: k.ledgerSequence,
	}

	pendingAmount, err := k.ledgerPendingBalances.Get(ctx, burnCoin.Denom)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return id, err
		}

		pendingAmount = sdkmath.ZeroInt()
	}

	pendingAmount = pendingAmount.Add(burnCoin.Amount)
	err = k.ledgerPendingBalances.Set(ctx, burnCoin.Denom, pendingAmount)
	if err != nil {
		return id, err
	}

	err = k.bankKeeper.SendCoinsFromAccountToModule(sctx, srcAddr, bmetypes.ModuleName, sdk.Coins{burnCoin})
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
		// Clamp CR in BPS to math.MaxUint32 to prevent int64→uint32 overflow
		// when CR is extremely large (e.g., zero outstanding ACT supply).
		crInt := cr.Mul(sdkmath.LegacyNewDec(10000)).TruncateInt64()
		if crInt > math.MaxUint32 {
			crInt = math.MaxUint32
		}

		warnThreshold := int64(params.CircuitBreakerWarnThreshold)
		haltThreshold := int64(params.CircuitBreakerHaltThreshold)

		if crInt > warnThreshold {
			cb.Status = bmetypes.MintStatusHealthy
			cb.EpochHeightDiff = calculateBlocksDiff(params, crInt)
		} else if crInt <= warnThreshold && crInt > haltThreshold {
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
			PreviousStatus:  cb.PreviousStatus,
			NewStatus:       cb.Status,
			CollateralRatio: cr,
		})
		if err != nil {
			sctx.Logger().Error("failed to emit mint status change event", "error", err)
		}
	}

	return cb, changed
}

func calculateBlocksDiff(params bmetypes.Params, cr int64) int64 {
	warnThreshold := int64(params.CircuitBreakerWarnThreshold)

	if cr >= warnThreshold {
		return params.MinEpochBlocks
	}

	if params.EpochBlocksBackoffPercent == 0 {
		return params.MinEpochBlocks
	}

	// The number of steps CR has dropped below warn threshold.
	// Each step = 10 BPS = 0.001 in the stored format (10000 = 1.0).
	steps := uint64((warnThreshold - cr) / 10)

	if steps == 0 {
		return params.MinEpochBlocks
	}

	// MinEpochBlocks * (1 + EpochBlocksBackoff/100) ^ steps.
	// EpochBlocksBackoff is in percent (e.g., 10 = 10%).
	// Each step grows the backoff by EpochBlocksBackoff% of the current value.
	// Uses sdkmath.LegacyDec for deterministic arbitrary-precision arithmetic.
	base := sdkmath.LegacyNewDec(100 + int64(params.EpochBlocksBackoffPercent)).Quo(sdkmath.LegacyNewDec(100))

	// Cap at ~1 day of blocks (assuming 6s per block) to prevent excessively long epochs.
	// Cap the Dec before TruncateInt64 to avoid int64 overflow with aggressive params.
	maxEpochBlocks := sdkmath.LegacyNewDec(14400)

	resDec := sdkmath.LegacyNewDec(params.MinEpochBlocks).Mul(base.Power(steps))
	if resDec.GT(maxEpochBlocks) {
		resDec = maxEpochBlocks
	}

	res := resDec.TruncateInt64()
	if res < params.MinEpochBlocks {
		return params.MinEpochBlocks
	}

	return res
}

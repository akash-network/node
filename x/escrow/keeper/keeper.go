package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "github.com/akash-network/akash-api/go/node/escrow/v1beta3"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

type AccountHook func(sdk.Context, types.Account)
type PaymentHook func(sdk.Context, types.FractionalPayment)

type Keeper interface {
	Codec() codec.BinaryCodec
	StoreKey() sdk.StoreKey
	AccountCreate(ctx sdk.Context, id types.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id types.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error
	AccountSettle(ctx sdk.Context, id types.AccountID) (bool, error)
	AccountClose(ctx sdk.Context, id types.AccountID) error
	PaymentCreate(ctx sdk.Context, id types.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error
	PaymentWithdraw(ctx sdk.Context, id types.AccountID, pid string) error
	PaymentClose(ctx sdk.Context, id types.AccountID, pid string) error
	GetAccount(ctx sdk.Context, id types.AccountID) (types.Account, error)
	GetPayment(ctx sdk.Context, id types.AccountID, pid string) (types.FractionalPayment, error)
	AddOnAccountClosedHook(AccountHook) Keeper
	AddOnPaymentClosedHook(PaymentHook) Keeper

	// for genesis
	WithAccounts(sdk.Context, func(types.Account) bool)
	WithPayments(sdk.Context, func(types.FractionalPayment) bool)
	SaveAccount(sdk.Context, types.Account)
	SavePayment(sdk.Context, types.FractionalPayment)
}

func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey, bkeeper BankKeeper, tkeeper TakeKeeper, dkeeper DistrKeeper) Keeper {
	return &keeper{
		cdc:     cdc,
		skey:    skey,
		bkeeper: bkeeper,
		tkeeper: tkeeper,
		dkeeper: dkeeper,
	}
}

type keeper struct {
	cdc     codec.BinaryCodec
	skey    sdk.StoreKey
	bkeeper BankKeeper
	tkeeper TakeKeeper
	dkeeper DistrKeeper

	hooks struct {
		onAccountClosed []AccountHook
		onPaymentClosed []PaymentHook
	}
}

func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k *keeper) StoreKey() sdk.StoreKey {
	return k.skey
}

func (k *keeper) AccountCreate(ctx sdk.Context, id types.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	if store.Has(key) {
		return types.ErrAccountExists
	}

	obj := &types.Account{
		ID:          id,
		Owner:       owner.String(),
		State:       types.AccountOpen,
		Balance:     sdk.NewDecCoin(deposit.Denom, sdk.ZeroInt()),
		Transferred: sdk.NewDecCoin(deposit.Denom, sdk.ZeroInt()),
		SettledAt:   ctx.BlockHeight(),
		Depositor:   depositor.String(),
		Funds:       sdk.NewDecCoin(deposit.Denom, sdk.ZeroInt()),
	}

	if err := obj.ValidateBasic(); err != nil {
		return err
	}

	if err := k.fetchDepositToAccount(ctx, obj, owner, depositor, deposit); err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(obj))

	return nil
}

// fetchDepositToAccount fetches deposit amount from the depositor's account to the escrow
// account and accordingly updates the balance or funds.
func (k *keeper) fetchDepositToAccount(ctx sdk.Context, acc *types.Account, owner, depositor sdk.AccAddress, deposit sdk.Coin) error {
	if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, depositor, types.ModuleName, sdk.NewCoins(deposit)); err != nil {
		return err
	}
	if owner.Equals(depositor) {
		acc.Balance = acc.Balance.Add(sdk.NewDecCoinFromCoin(deposit))
	} else {
		acc.Funds = acc.Funds.Add(sdk.NewDecCoinFromCoin(deposit))
	}
	return nil
}

func (k *keeper) AccountDeposit(ctx sdk.Context, id types.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	obj, err := k.GetAccount(ctx, id)
	if err != nil {
		return err
	}

	if obj.State != types.AccountOpen {
		return types.ErrAccountClosed
	}

	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	if err = k.fetchDepositToAccount(ctx, &obj, owner, depositor, amount); err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&obj))

	return nil
}

func (k *keeper) AccountSettle(ctx sdk.Context, id types.AccountID) (bool, error) {
	_, _, od, err := k.doAccountSettle(ctx, id)
	return od, err
}

func (k *keeper) AccountClose(ctx sdk.Context, id types.AccountID) error {
	account, err := k.GetAccount(ctx, id)
	if err != nil {
		return err
	}

	if account.State != types.AccountOpen {
		return types.ErrAccountClosed
	}

	account, payments, od, err := k.doAccountSettle(ctx, id)
	if err != nil {
		return err
	}
	if od {
		return nil
	}

	account.State = types.AccountClosed
	if err := k.accountWithdraw(ctx, &account); err != nil {
		return err
	}

	for idx := range payments {
		payments[idx].State = types.PaymentClosed
		if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
			return err
		}
	}

	for _, hook := range k.hooks.onAccountClosed {
		hook(ctx, account)
	}

	for _, hook := range k.hooks.onPaymentClosed {
		for idx := range payments {
			hook(ctx, payments[idx])
		}
	}

	return nil
}

func (k *keeper) PaymentCreate(ctx sdk.Context, id types.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error {
	account, _, od, err := k.doAccountSettle(ctx, id)
	if err != nil {
		return err
	}
	if od {
		return types.ErrAccountOverdrawn
	}

	if rate.Denom != account.Balance.Denom {
		return types.ErrInvalidDenomination
	}

	if rate.IsZero() {
		return types.ErrPaymentRateZero
	}

	store := ctx.KVStore(k.skey)
	key := paymentKey(id, pid)

	if store.Has(key) {
		return types.ErrPaymentExists
	}

	obj := &types.FractionalPayment{
		AccountID: id,
		PaymentID: pid,
		Owner:     owner.String(),
		State:     types.PaymentOpen,
		Rate:      rate,
		Balance:   sdk.NewDecCoin(rate.Denom, sdk.ZeroInt()),
		Withdrawn: sdk.NewCoin(rate.Denom, sdk.ZeroInt()),
	}

	store.Set(key, k.cdc.MustMarshal(obj))

	return nil
}

func (k *keeper) PaymentWithdraw(ctx sdk.Context, id types.AccountID, pid string) error {
	payment, err := k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}
	if payment.State != types.PaymentOpen {
		return types.ErrPaymentClosed
	}

	od, err := k.AccountSettle(ctx, id)
	if err != nil {
		return err
	}
	if od {
		return nil
	}

	payment, err = k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}
	return k.paymentWithdraw(ctx, &payment)
}

func (k *keeper) PaymentClose(ctx sdk.Context, id types.AccountID, pid string) error {

	payment, err := k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	if payment.State != types.PaymentOpen {
		return types.ErrPaymentClosed
	}

	od, err := k.AccountSettle(ctx, id)

	if err != nil {
		return err
	}
	if od {
		return nil
	}

	payment, err = k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	payment.State = types.PaymentClosed

	if err := k.paymentWithdraw(ctx, &payment); err != nil {
		return err
	}

	for _, hook := range k.hooks.onPaymentClosed {
		hook(ctx, payment)
	}

	return nil
}

func (k *keeper) AddOnAccountClosedHook(hook AccountHook) Keeper {
	k.hooks.onAccountClosed = append(k.hooks.onAccountClosed, hook)
	return k
}

func (k *keeper) AddOnPaymentClosedHook(hook PaymentHook) Keeper {
	k.hooks.onPaymentClosed = append(k.hooks.onPaymentClosed, hook)
	return k
}

func (k *keeper) GetAccount(ctx sdk.Context, id types.AccountID) (types.Account, error) {

	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	if !store.Has(key) {
		return types.Account{}, types.ErrAccountNotFound
	}

	buf := store.Get(key)

	var obj types.Account

	k.cdc.MustUnmarshal(buf, &obj)

	return obj, nil
}

func (k *keeper) GetPayment(ctx sdk.Context, id types.AccountID, pid string) (types.FractionalPayment, error) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(id, pid)

	if !store.Has(key) {
		return types.FractionalPayment{}, types.ErrPaymentNotFound
	}

	buf := store.Get(key)

	var obj types.FractionalPayment

	k.cdc.MustUnmarshal(buf, &obj)

	return obj, nil
}

func (k *keeper) SaveAccount(ctx sdk.Context, obj types.Account) {
	k.saveAccount(ctx, &obj)
}

func (k *keeper) SavePayment(ctx sdk.Context, obj types.FractionalPayment) {
	k.savePayment(ctx, &obj)
}

func (k *keeper) WithAccounts(ctx sdk.Context, fn func(types.Account) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, types.AccountKeyPrefix())
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Account
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) WithPayments(ctx sdk.Context, fn func(types.FractionalPayment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, types.PaymentKeyPrefix())
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.FractionalPayment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) doAccountSettle(ctx sdk.Context, id types.AccountID) (types.Account, []types.FractionalPayment, bool, error) {
	account, err := k.GetAccount(ctx, id)

	if err != nil {
		return account, nil, false, err
	}

	if account.State != types.AccountOpen {
		return account, nil, false, types.ErrAccountClosed
	}

	heightDelta := sdk.NewInt(ctx.BlockHeight() - account.SettledAt)

	if heightDelta.IsZero() {
		return account, nil, false, nil
	}

	account.SettledAt = ctx.BlockHeight()

	payments := k.accountOpenPayments(ctx, id)

	if len(payments) == 0 {
		k.saveAccount(ctx, &account)
		return account, nil, false, nil
	}

	blockRate := sdk.NewDecCoin(account.Balance.Denom, sdk.ZeroInt())

	for _, payment := range payments {
		blockRate = blockRate.Add(payment.Rate)
	}

	account, payments, overdrawn, amountRemaining := accountSettleFullblocks(
		account, payments, heightDelta, blockRate)

	// all payments made in full
	if !overdrawn {

		// save objects
		k.saveAccount(ctx, &account)
		for idx := range payments {
			k.savePayment(ctx, &payments[idx])
		}

		// return early
		return account, payments, false, nil
	}

	//
	// overdrawn
	//

	// distribute weighted by payment block rate
	account, payments, amountRemaining = accountSettleDistributeWeighted(
		account, payments, blockRate, amountRemaining)

	if amountRemaining.Amount.GT(sdk.NewDec(1)) {
		return account, payments, false, fmt.Errorf("%w: Invalid settlement: %v remains", types.ErrInvalidSettlement, amountRemaining)
	}

	// save objects
	account.State = types.AccountOverdrawn
	k.saveAccount(ctx, &account)
	for idx := range payments {
		payments[idx].State = types.PaymentOverdrawn
		k.savePayment(ctx, &payments[idx])
		if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
			return account, payments, false, err
		}
	}

	// call hooks
	for _, hook := range k.hooks.onAccountClosed {
		hook(ctx, account)
	}

	for _, hook := range k.hooks.onPaymentClosed {
		for _, payment := range payments {
			hook(ctx, payment)
		}
	}

	return account, payments, true, nil
}

func (k *keeper) saveAccount(ctx sdk.Context, obj *types.Account) {
	store := ctx.KVStore(k.skey)
	key := accountKey(obj.ID)
	store.Set(key, k.cdc.MustMarshal(obj))
}

func (k *keeper) savePayment(ctx sdk.Context, obj *types.FractionalPayment) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(obj.AccountID, obj.PaymentID)
	store.Set(key, k.cdc.MustMarshal(obj))
}

func (k *keeper) accountPayments(ctx sdk.Context, id types.AccountID) []types.FractionalPayment {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, accountPaymentsKey(id))

	var payments []types.FractionalPayment

	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.FractionalPayment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		payments = append(payments, val)
	}

	return payments
}

func (k *keeper) accountOpenPayments(ctx sdk.Context, id types.AccountID) []types.FractionalPayment {
	allPayments := k.accountPayments(ctx, id)
	payments := make([]types.FractionalPayment, 0, len(allPayments))

	for _, payment := range allPayments {
		if payment.State != types.PaymentOpen {
			continue
		}
		payments = append(payments, payment)
	}
	return payments
}

func (k *keeper) accountWithdraw(ctx sdk.Context, obj *types.Account) error {
	if obj.Balance.Amount.LT(sdk.NewDec(1)) && obj.Funds.Amount.LT(sdk.NewDec(1)) {
		return nil
	}

	if !obj.Balance.Amount.LT(sdk.NewDec(1)) {
		owner, err := sdk.AccAddressFromBech32(obj.Owner)
		if err != nil {
			return err
		}

		withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Balance.Amount.TruncateInt())
		if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, owner, sdk.NewCoins(withdrawal)); err != nil {
			ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
			return err
		}
		obj.Balance = obj.Balance.Sub(sdk.NewDecCoinFromCoin(withdrawal))
	}

	if !obj.Funds.Amount.LT(sdk.NewDec(1)) {
		depositor, err := sdk.AccAddressFromBech32(obj.Depositor)
		if err != nil {
			return err
		}

		withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Funds.Amount.TruncateInt())
		if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositor, sdk.NewCoins(withdrawal)); err != nil {
			ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
			return err
		}
		obj.Funds = obj.Funds.Sub(sdk.NewDecCoinFromCoin(withdrawal))
	}

	k.saveAccount(ctx, obj)
	return nil
}

func (k *keeper) paymentWithdraw(ctx sdk.Context, obj *types.FractionalPayment) error {
	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	rawEarnings := sdk.NewCoin(obj.Balance.Denom, obj.Balance.Amount.TruncateInt())

	if rawEarnings.Amount.IsZero() {
		return nil
	}

	earnings, fee, err := k.tkeeper.SubtractFees(ctx, rawEarnings)
	if err != nil {
		return err
	}

	if err := k.sendFeeToCommunityPool(ctx, fee); err != nil {
		ctx.Logger().Error("payment withdraw - fees", "err", err, "account", obj.AccountID, "payment", obj.PaymentID)
		return err
	}

	if !earnings.IsZero() {
		if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, owner, sdk.NewCoins(earnings)); err != nil {
			ctx.Logger().Error("payment withdraw - earnings", "err", err, "account", obj.AccountID, "payment", obj.PaymentID)
			return err
		}
	}

	total := earnings.Add(fee)

	obj.Withdrawn = obj.Withdrawn.Add(total)
	obj.Balance = obj.Balance.Sub(sdk.NewDecCoinFromCoin(total))

	k.savePayment(ctx, obj)
	return nil
}

func (k keeper) sendFeeToCommunityPool(ctx sdk.Context, fee sdk.Coin) error {

	if fee.IsZero() {
		return nil
	}

	// see https://github.com/cosmos/cosmos-sdk/blob/c2a07cea272a7878b5bc2ec160eb58ca83794214/x/distribution/keeper/keeper.go#L251-L263

	if err := k.bkeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, distrtypes.ModuleName, sdk.NewCoins(fee)); err != nil {
		return err
	}

	pool := k.dkeeper.GetFeePool(ctx)

	pool.CommunityPool = pool.CommunityPool.Add(sdk.NewDecCoinFromCoin(fee))
	k.dkeeper.SetFeePool(ctx, pool)

	return nil
}

func accountSettleFullblocks(
	account types.Account,
	payments []types.FractionalPayment,
	heightDelta sdk.Int,
	blockRate sdk.DecCoin,
) (
	types.Account,
	[]types.FractionalPayment,
	bool,
	sdk.DecCoin,
) {
	numFullBlocks := account.TotalBalance().Amount.Quo(blockRate.Amount).TruncateInt()

	if numFullBlocks.GT(heightDelta) {
		numFullBlocks = heightDelta
	}

	for idx := range payments {
		p := payments[idx]
		payments[idx].Balance = p.Balance.Add(
			sdk.NewDecCoinFromDec(p.Rate.Denom, p.Rate.Amount.Mul(sdk.NewDecFromInt(numFullBlocks))),
		)
	}

	transferred := sdk.NewDecCoinFromDec(blockRate.Denom, blockRate.Amount.Mul(sdk.NewDecFromInt(numFullBlocks)))

	account.Transferred = account.Transferred.Add(transferred)
	// use funds before using balance
	account.Funds.Amount = account.Funds.Amount.Sub(transferred.Amount)
	if account.Funds.Amount.IsNegative() {
		account.Balance.Amount = account.Balance.Amount.Sub(account.Funds.Amount.Abs())
		account.Funds.Amount = sdk.ZeroDec()
	}

	remaining := account.TotalBalance()
	overdrawn := true
	if numFullBlocks.Equal(heightDelta) {
		remaining.Amount = sdk.ZeroDec()
		overdrawn = false
	}

	// only balance is used in later functions to do calculations, in case account is overdrawn
	// finally, balance will always reach zero, so we are not doing any mis-management here.
	if overdrawn {
		account.Balance = remaining
		account.Funds.Amount = sdk.ZeroDec()
	}

	return account, payments, overdrawn, remaining
}

func accountSettleDistributeWeighted(
	account types.Account,
	payments []types.FractionalPayment,
	blockRate sdk.DecCoin,
	amountRemaining sdk.DecCoin,
) (
	types.Account,
	[]types.FractionalPayment,
	sdk.DecCoin,
) {

	actualTransferred := sdk.ZeroDec()

	// distribute remaining balance weighted by rate

	for idx := range payments {
		payment := payments[idx]
		// amount := (rate / blockrate) * remaining
		amount := amountRemaining.Amount.
			Mul(payment.Rate.Amount).
			Quo(blockRate.Amount)

		payments[idx].Balance = payment.Balance.Add(sdk.NewDecCoinFromDec(payment.Balance.Denom, amount))

		actualTransferred = actualTransferred.Add(amount)
	}

	transferred := sdk.NewDecCoinFromDec(account.Balance.Denom, actualTransferred)

	account.Transferred = account.Transferred.Add(transferred)
	account.Balance = account.Balance.Sub(transferred)

	amountRemaining = amountRemaining.Sub(transferred)

	return account, payments, amountRemaining
}

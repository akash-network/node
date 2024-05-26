package keeper

import (
	"fmt"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	dv1 "pkg.akt.dev/go/node/deployment/v1"
	"pkg.akt.dev/go/node/escrow/v1"
)

type AccountHook func(sdk.Context, v1.Account)
type PaymentHook func(sdk.Context, v1.FractionalPayment)

type Keeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	AccountCreate(ctx sdk.Context, id v1.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id v1.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error
	AccountSettle(ctx sdk.Context, id v1.AccountID) (bool, error)
	AccountClose(ctx sdk.Context, id v1.AccountID) error
	PaymentCreate(ctx sdk.Context, id v1.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error
	PaymentWithdraw(ctx sdk.Context, id v1.AccountID, pid string) error
	PaymentClose(ctx sdk.Context, id v1.AccountID, pid string) error
	GetAccount(ctx sdk.Context, id v1.AccountID) (v1.Account, error)
	GetPayment(ctx sdk.Context, id v1.AccountID, pid string) (v1.FractionalPayment, error)
	AddOnAccountClosedHook(AccountHook) Keeper
	AddOnPaymentClosedHook(PaymentHook) Keeper
	WithAccounts(sdk.Context, func(v1.Account) bool)
	WithPayments(sdk.Context, func(v1.FractionalPayment) bool)
	SaveAccount(sdk.Context, v1.Account)
	SavePayment(sdk.Context, v1.FractionalPayment)
}

func NewKeeper(
	cdc codec.BinaryCodec,
	skey storetypes.StoreKey,
	bkeeper BankKeeper,
	tkeeper TakeKeeper,
	dkeeper DistrKeeper,
	akeeper AuthzKeeper,
) Keeper {
	return &keeper{
		cdc:         cdc,
		skey:        skey,
		bkeeper:     bkeeper,
		tkeeper:     tkeeper,
		dkeeper:     dkeeper,
		authzKeeper: akeeper,
	}
}

type keeper struct {
	cdc         codec.BinaryCodec
	skey        storetypes.StoreKey
	bkeeper     BankKeeper
	tkeeper     TakeKeeper
	dkeeper     DistrKeeper
	authzKeeper AuthzKeeper

	hooks struct {
		onAccountClosed []AccountHook
		onPaymentClosed []PaymentHook
	}
}

func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) AccountCreate(ctx sdk.Context, id v1.AccountID, owner, depositor sdk.AccAddress, deposit sdk.Coin) error {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	if store.Has(key) {
		return v1.ErrAccountExists
	}

	obj := &v1.Account{
		ID:          id,
		Owner:       owner.String(),
		State:       v1.AccountOpen,
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
func (k *keeper) fetchDepositToAccount(
	ctx sdk.Context,
	acc *v1.Account,
	owner sdk.AccAddress,
	depositor sdk.AccAddress,
	deposit sdk.Coin,
) error {
	if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, depositor, v1.ModuleName, sdk.NewCoins(deposit)); err != nil {
		return err
	}

	if owner.Equals(depositor) {
		acc.Balance = acc.Balance.Add(sdk.NewDecCoinFromCoin(deposit))
	} else {
		acc.Funds = acc.Funds.Add(sdk.NewDecCoinFromCoin(deposit))
	}

	return nil
}

func (k *keeper) GetAccountDepositor(ctx sdk.Context, id v1.AccountID) (sdk.AccAddress, error) {
	obj, err := k.GetAccount(ctx, id)
	if err != nil {
		return sdk.AccAddress{}, err
	}

	depositor, err := sdk.AccAddressFromBech32(obj.Depositor)
	if err != nil {
		return sdk.AccAddress{}, err
	}

	return depositor, nil
}

func (k *keeper) AccountDeposit(ctx sdk.Context, id v1.AccountID, depositor sdk.AccAddress, amount sdk.Coin) error {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	obj, err := k.GetAccount(ctx, id)
	if err != nil {
		return err
	}

	if obj.State != v1.AccountOpen {
		return v1.ErrAccountClosed
	}

	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	currDepositor, err := sdk.AccAddressFromBech32(obj.Depositor)
	if err != nil {
		return err
	}

	if !owner.Equals(depositor) {
		if currDepositor.Equals(owner) {
			obj.Depositor = depositor.String()
		} else if !currDepositor.Equals(depositor) {
			return v1.ErrInvalidAccountDepositor
		}
	}

	if err = k.fetchDepositToAccount(ctx, &obj, owner, depositor, amount); err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&obj))

	return nil
}

func (k *keeper) AccountSettle(ctx sdk.Context, id v1.AccountID) (bool, error) {
	_, _, od, err := k.doAccountSettle(ctx, id)

	return od, err
}

func (k *keeper) AccountClose(ctx sdk.Context, id v1.AccountID) error {
	// doAccountSettle checks if account is open
	account, payments, od, err := k.doAccountSettle(ctx, id)
	if err != nil {
		return err
	}

	if od {
		return nil
	}

	account.State = v1.AccountClosed
	if err := k.accountWithdraw(ctx, &account); err != nil {
		return err
	}

	for idx := range payments {
		payments[idx].State = v1.PaymentClosed
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

func (k *keeper) PaymentCreate(ctx sdk.Context, id v1.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error {
	account, _, od, err := k.doAccountSettle(ctx, id)
	if err != nil {
		return err
	}

	if od {
		return v1.ErrAccountOverdrawn
	}

	if rate.Denom != account.Balance.Denom {
		return v1.ErrInvalidDenomination
	}

	if rate.IsZero() {
		return v1.ErrPaymentRateZero
	}

	store := ctx.KVStore(k.skey)
	key := paymentKey(id, pid)

	if store.Has(key) {
		return v1.ErrPaymentExists
	}

	obj := &v1.FractionalPayment{
		AccountID: id,
		PaymentID: pid,
		Owner:     owner.String(),
		State:     v1.PaymentOpen,
		Rate:      rate,
		Balance:   sdk.NewDecCoin(rate.Denom, sdk.ZeroInt()),
		Withdrawn: sdk.NewCoin(rate.Denom, sdk.ZeroInt()),
	}

	store.Set(key, k.cdc.MustMarshal(obj))

	return nil
}

func (k *keeper) PaymentWithdraw(ctx sdk.Context, id v1.AccountID, pid string) error {
	payment, err := k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	if payment.State != v1.PaymentOpen {
		return v1.ErrPaymentClosed
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

func (k *keeper) PaymentClose(ctx sdk.Context, id v1.AccountID, pid string) error {
	payment, err := k.GetPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	if payment.State != v1.PaymentOpen {
		return v1.ErrPaymentClosed
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

	payment.State = v1.PaymentClosed

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

func (k *keeper) GetAccount(ctx sdk.Context, id v1.AccountID) (v1.Account, error) {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	if !store.Has(key) {
		return v1.Account{}, v1.ErrAccountNotFound
	}

	buf := store.Get(key)

	var obj v1.Account

	k.cdc.MustUnmarshal(buf, &obj)

	return obj, nil
}

func (k *keeper) GetPayment(ctx sdk.Context, id v1.AccountID, pid string) (v1.FractionalPayment, error) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(id, pid)

	if !store.Has(key) {
		return v1.FractionalPayment{}, v1.ErrPaymentNotFound
	}

	buf := store.Get(key)

	var obj v1.FractionalPayment

	k.cdc.MustUnmarshal(buf, &obj)

	return obj, nil
}

func (k *keeper) SaveAccount(ctx sdk.Context, obj v1.Account) {
	k.saveAccount(ctx, &obj)
}

func (k *keeper) SavePayment(ctx sdk.Context, obj v1.FractionalPayment) {
	k.savePayment(ctx, &obj)
}

func (k *keeper) WithAccounts(ctx sdk.Context, fn func(v1.Account) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, v1.AccountKeyPrefix())

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val v1.Account
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) WithPayments(ctx sdk.Context, fn func(v1.FractionalPayment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, v1.PaymentKeyPrefix())

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val v1.FractionalPayment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) doAccountSettle(ctx sdk.Context, id v1.AccountID) (v1.Account, []v1.FractionalPayment, bool, error) {
	account, err := k.GetAccount(ctx, id)
	if err != nil {
		return account, nil, false, err
	}

	if account.State != v1.AccountOpen {
		return account, nil, false, v1.ErrAccountClosed
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

	account, payments, overdrawn, amountRemaining := accountSettleFullBlocks(account, payments, heightDelta, blockRate)

	if account.Funds.Amount.IsPositive() {
		owner := sdk.MustAccAddressFromBech32(account.Owner)
		depositor := sdk.MustAccAddressFromBech32(account.Depositor)

		msg := &dv1.MsgDepositDeployment{Amount: sdk.NewCoin(account.Balance.Denom, sdk.NewInt(0))}

		authz, _ := k.authzKeeper.GetAuthorization(ctx, owner, depositor, sdk.MsgTypeURL(msg))

		// if authorization has been revoked or expired it cannot be used anymore
		// send coins back to the owner
		if authz == nil {
			withdrawal := sdk.NewCoin(account.Balance.Denom, account.Funds.Amount.TruncateInt())
			if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, depositor, sdk.NewCoins(withdrawal)); err != nil {
				ctx.Logger().Error("account withdraw", "err", err, "id", account.ID)
				return account, payments, overdrawn, err
			}

			account.Funds.Amount = sdk.ZeroDec()
		}
	}

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
	account, payments, amountRemaining = accountSettleDistributeWeighted(account, payments, blockRate, amountRemaining)

	if amountRemaining.Amount.GT(sdk.NewDec(1)) {
		return account, payments, false, fmt.Errorf("%w: Invalid settlement: %v remains", v1.ErrInvalidSettlement, amountRemaining)
	}

	// save objects
	account.State = v1.AccountOverdrawn
	k.saveAccount(ctx, &account)
	for idx := range payments {
		payments[idx].State = v1.PaymentOverdrawn
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

func (k *keeper) saveAccount(ctx sdk.Context, obj *v1.Account) {
	store := ctx.KVStore(k.skey)
	key := accountKey(obj.ID)
	store.Set(key, k.cdc.MustMarshal(obj))
}

func (k *keeper) savePayment(ctx sdk.Context, obj *v1.FractionalPayment) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(obj.AccountID, obj.PaymentID)
	store.Set(key, k.cdc.MustMarshal(obj))
}

func (k *keeper) accountPayments(ctx sdk.Context, id v1.AccountID) []v1.FractionalPayment {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, accountPaymentsKey(id))

	var payments []v1.FractionalPayment

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val v1.FractionalPayment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		payments = append(payments, val)
	}

	return payments
}

func (k *keeper) accountOpenPayments(ctx sdk.Context, id v1.AccountID) []v1.FractionalPayment {
	allPayments := k.accountPayments(ctx, id)
	payments := make([]v1.FractionalPayment, 0, len(allPayments))

	for _, payment := range allPayments {
		if payment.State != v1.PaymentOpen {
			continue
		}
		payments = append(payments, payment)
	}

	return payments
}

func (k *keeper) accountWithdraw(ctx sdk.Context, obj *v1.Account) error {
	if obj.Balance.Amount.LT(sdk.NewDec(1)) && obj.Funds.Amount.LT(sdk.NewDec(1)) {
		return nil
	}

	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	if !obj.Balance.Amount.LT(sdk.NewDec(1)) {
		withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Balance.Amount.TruncateInt())
		if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, owner, sdk.NewCoins(withdrawal)); err != nil {
			ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
			return err
		}
		obj.Balance = obj.Balance.Sub(sdk.NewDecCoinFromCoin(withdrawal))
	}

	if obj.Funds.IsPositive() {
		depositor, err := sdk.AccAddressFromBech32(obj.Depositor)
		if err != nil {
			return err
		}

		withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Funds.Amount.TruncateInt())
		if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, depositor, sdk.NewCoins(withdrawal)); err != nil {
			ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
			return err
		}

		obj.Funds = obj.Funds.Sub(sdk.NewDecCoinFromCoin(withdrawal))

		msg := &dv1.MsgDepositDeployment{Amount: sdk.NewCoin(obj.Balance.Denom, sdk.NewInt(0))}

		// Funds field is solely to track deposits via authz.
		// check if there is active deployment authorization from given depositor
		// if exists, increase allowed authz deposit by remainder in the Funds, it will allow owner to reuse active authz
		// without asking for renew.
		authorization, expiration := k.authzKeeper.GetAuthorization(ctx, owner, depositor, sdk.MsgTypeURL(msg))
		dauthz, valid := authorization.(*dv1.DepositAuthorization)
		if valid && authorization != nil {
			dauthz.SpendLimit = dauthz.SpendLimit.Add(withdrawal)
			err = k.authzKeeper.SaveGrant(ctx, owner, depositor, dauthz, expiration)
			if err != nil {
				return err
			}
		}
	}

	k.saveAccount(ctx, obj)

	return nil
}

func (k *keeper) paymentWithdraw(ctx sdk.Context, obj *v1.FractionalPayment) error {
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
		if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, owner, sdk.NewCoins(earnings)); err != nil {
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

func (k *keeper) sendFeeToCommunityPool(ctx sdk.Context, fee sdk.Coin) error {
	if fee.IsZero() {
		return nil
	}

	// see https://github.com/cosmos/cosmos-sdk/blob/c2a07cea272a7878b5bc2ec160eb58ca83794214/x/distribution/keeper/keeper.go#L251-L263
	if err := k.bkeeper.SendCoinsFromModuleToModule(ctx, v1.ModuleName, distrtypes.ModuleName, sdk.NewCoins(fee)); err != nil {
		return err
	}

	pool := k.dkeeper.GetFeePool(ctx)

	pool.CommunityPool = pool.CommunityPool.Add(sdk.NewDecCoinFromCoin(fee))
	k.dkeeper.SetFeePool(ctx, pool)

	return nil
}

func accountSettleFullBlocks(
	account v1.Account,
	payments []v1.FractionalPayment,
	heightDelta math.Int,
	blockRate sdk.DecCoin,
) (
	v1.Account,
	[]v1.FractionalPayment,
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
	account v1.Account,
	payments []v1.FractionalPayment,
	blockRate sdk.DecCoin,
	amountRemaining sdk.DecCoin,
) (
	v1.Account,
	[]v1.FractionalPayment,
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

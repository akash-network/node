package keeper

import (
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/escrow/types"
	"github.com/pkg/errors"
)

type AccountHook func(sdk.Context, types.Account)
type PaymentHook func(sdk.Context, types.Payment)

type Keeper interface {
	AccountCreate(ctx sdk.Context, id types.AccountID, owner sdk.AccAddress, deposit sdk.Coin) error
	AccountDeposit(ctx sdk.Context, id types.AccountID, amount sdk.Coin) error
	AccountSettle(ctx sdk.Context, id types.AccountID) (bool, error)
	AccountClose(ctx sdk.Context, id types.AccountID) error
	PaymentCreate(ctx sdk.Context, id types.AccountID, pid string, owner sdk.AccAddress, rate sdk.Coin) error
	PaymentWithdraw(ctx sdk.Context, id types.AccountID, pid string) error
	PaymentClose(ctx sdk.Context, id types.AccountID, pid string) error
	GetAccount(ctx sdk.Context, id types.AccountID) (types.Account, error)
	GetPayment(ctx sdk.Context, id types.AccountID, pid string) (types.Payment, error)
	AddOnAccountClosedHook(AccountHook) Keeper
	AddOnPaymentClosedHook(PaymentHook) Keeper

	// for genesis
	WithAccounts(sdk.Context, func(types.Account) bool)
	WithPayments(sdk.Context, func(types.Payment) bool)
	SaveAccount(sdk.Context, types.Account)
	SavePayment(sdk.Context, types.Payment)
}

func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey, bkeeper BankKeeper, authzkeeper AuthzKeeper) Keeper {
	return &keeper{
		cdc:         cdc,
		skey:        skey,
		bkeeper:     bkeeper,
		authzkeeper: authzkeeper,
	}
}

type keeper struct {
	cdc         codec.BinaryCodec
	skey        sdk.StoreKey
	bkeeper     BankKeeper
	authzkeeper AuthzKeeper

	hooks struct {
		onAccountClosed []AccountHook
		onPaymentClosed []PaymentHook
	}
}

const (
	// TODO: update this with actual default granter address
	granterDefaultAddr   string = "akash16q6s0tauc3cks5us7f57wds8c8lqg4jqs0qtaf"
	defaultCreditDeposit int64  = 5000000
)

func getGranterAddr() (sdk.AccAddress, error) {
	return sdk.AccAddressFromBech32(granterDefaultAddr)
}

func (k *keeper) AccountCreate(ctx sdk.Context, id types.AccountID, owner sdk.AccAddress, deposit sdk.Coin) error {
	store := ctx.KVStore(k.skey)
	key := accountKey(id)

	if store.Has(key) {
		return types.ErrAccountExists
	}

	obj := &types.Account{
		ID:             id,
		Owner:          owner.String(),
		State:          types.AccountOpen,
		CreditsBalance: sdk.NewCoin(deposit.Denom, sdk.ZeroInt()),
		TokensBalance:  sdk.NewCoin(deposit.Denom, sdk.ZeroInt()),
		TotalBalance:   sdk.NewCoin(deposit.Denom, sdk.ZeroInt()),
		Transferred:    sdk.NewCoin(deposit.Denom, sdk.ZeroInt()),
		SettledAt:      ctx.BlockHeight(),
	}

	if err := obj.ValidateBasic(); err != nil {
		return err
	}

	updatedObj, err := k.checkCreditsAndSendToModule(ctx, owner, obj, deposit)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(updatedObj))

	return nil
}

func (k *keeper) AccountDeposit(ctx sdk.Context, id types.AccountID, amount sdk.Coin) error {
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

	updatedObj, err := k.checkCreditsAndSendToModule(ctx, owner, &obj, amount)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(updatedObj))

	return nil
}

func (k *keeper) checkCreditsAndSendToModule(ctx sdk.Context, owner sdk.AccAddress, account *types.Account,
	deposit sdk.Coin) (*types.Account, error) {
	// initiating new temporary instance of account
	temp := &types.Account{
		CreditsBalance: sdk.NewCoin(deposit.Denom, sdk.ZeroInt()),
		TokensBalance:  deposit,
		TotalBalance:   deposit,
	}

	granter, err := getGranterAddr()
	if err != nil {
		return account, err
	}

	authorization, expiration, expired := k.getAuthorization(ctx, owner, granter, types.DefaultMsgType)
	if authorization != nil && !expired && authorization.Credits.Denom == deposit.Denom {
		if authorization.Credits.IsGTE(deposit) {
			temp.CreditsBalance = deposit
			temp.TokensBalance = sdk.NewCoin(deposit.Denom, sdk.ZeroInt())
			authorization.Credits = authorization.Credits.Sub(deposit)
		} else {
			temp.CreditsBalance = authorization.Credits
			remainingDeposit := deposit.Sub(authorization.Credits)
			temp.TokensBalance = remainingDeposit
			authorization.Credits = sdk.NewCoin(deposit.Denom, sdk.ZeroInt())
		}

		if err := k.authzkeeper.SaveGrant(ctx, owner, granter, authorization, expiration); err != nil {
			return account, err
		}
	}

	if !temp.CreditsBalance.IsZero() {
		if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, granter, types.ModuleName,
			sdk.NewCoins(temp.CreditsBalance)); err != nil {
			return account, err
		}
	}

	if !temp.TokensBalance.IsZero() {
		if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, owner, types.ModuleName,
			sdk.NewCoins(temp.TokensBalance)); err != nil {
			return account, err
		}
	}

	account.CreditsBalance = account.CreditsBalance.Add(temp.CreditsBalance)
	account.TokensBalance = account.TokensBalance.Add(temp.TokensBalance)
	account.TotalBalance = account.TotalBalance.Add(deposit)

	return account, nil
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

func (k *keeper) PaymentCreate(ctx sdk.Context, id types.AccountID, pid string, owner sdk.AccAddress, rate sdk.Coin) error {
	account, _, od, err := k.doAccountSettle(ctx, id)
	if err != nil {
		return err
	}
	if od {
		return types.ErrAccountOverdrawn
	}

	if rate.Denom != account.TotalBalance.Denom {
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

	obj := &types.Payment{
		AccountID: id,
		PaymentID: pid,
		Owner:     owner.String(),
		State:     types.PaymentOpen,
		Rate:      rate,
		Balance:   sdk.NewCoin(rate.Denom, sdk.ZeroInt()),
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

func (k *keeper) GetPayment(ctx sdk.Context, id types.AccountID, pid string) (types.Payment, error) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(id, pid)

	if !store.Has(key) {
		return types.Payment{}, types.ErrPaymentNotFound
	}

	buf := store.Get(key)

	var obj types.Payment

	k.cdc.MustUnmarshal(buf, &obj)

	return obj, nil
}

func (k *keeper) SaveAccount(ctx sdk.Context, obj types.Account) {
	k.saveAccount(ctx, &obj)
}

func (k *keeper) SavePayment(ctx sdk.Context, obj types.Payment) {
	k.savePayment(ctx, &obj)
}

func (k *keeper) WithAccounts(ctx sdk.Context, fn func(types.Account) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, accountKeyPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Account
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) WithPayments(ctx sdk.Context, fn func(types.Payment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, paymentKeyPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Payment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) doAccountSettle(ctx sdk.Context, id types.AccountID) (types.Account, []types.Payment, bool, error) {
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

	blockRate := sdk.NewCoin(account.TotalBalance.Denom, sdk.ZeroInt())

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
	// checking credits if present any
	//
	owner, err := sdk.AccAddressFromBech32(account.Owner)
	if err != nil {
		return account, nil, false, err
	}

	granter, err := getGranterAddr()
	if err != nil {
		return account, nil, false, err
	}

	authorization, expiration, expired := k.getAuthorization(ctx, owner, granter, types.DefaultMsgType)
	if authorization != nil && !expired && !authorization.Credits.IsZero() &&
		authorization.Credits.Denom == account.TotalBalance.Denom {
		sdkDefaultAmount := sdk.NewInt(defaultCreditDeposit)
		if authorization.Credits.Amount.LTE(sdkDefaultAmount) {
			account.CreditsBalance = account.CreditsBalance.Add(authorization.Credits)
			account.TotalBalance = account.TotalBalance.Add(authorization.Credits)
			authorization.Credits = sdk.NewCoin(account.TotalBalance.Denom, sdk.ZeroInt())
		} else {
			account.CreditsBalance = account.CreditsBalance.AddAmount(sdkDefaultAmount)
			account.TotalBalance = account.TotalBalance.AddAmount(sdkDefaultAmount)
			authorization.Credits = authorization.Credits.SubAmount(sdkDefaultAmount)
		}
		if err := k.authzkeeper.SaveGrant(ctx, owner, granter, authorization, expiration); err != nil {
			return account, nil, false, err
		}

		if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, granter, types.ModuleName,
			sdk.NewCoins(account.CreditsBalance)); err != nil {
			return account, nil, false, err
		}

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

	// distribute evenly
	account, payments, amountRemaining = accountSettleDistributeEvenly(
		account, payments, amountRemaining)

	if !amountRemaining.IsZero() {
		return account, payments, false, errors.Wrapf(types.ErrInvalidSettlement, "Invalid settlement: %v remains", amountRemaining)
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

func (k *keeper) savePayment(ctx sdk.Context, obj *types.Payment) {
	store := ctx.KVStore(k.skey)
	key := paymentKey(obj.AccountID, obj.PaymentID)
	store.Set(key, k.cdc.MustMarshal(obj))
}

func (k *keeper) accountPayments(ctx sdk.Context, id types.AccountID) []types.Payment {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, accountPaymentsKey(id))

	var payments []types.Payment

	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Payment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		payments = append(payments, val)
	}

	return payments
}

func (k *keeper) accountOpenPayments(ctx sdk.Context, id types.AccountID) []types.Payment {
	allPayments := k.accountPayments(ctx, id)
	payments := make([]types.Payment, 0, len(allPayments))

	for _, payment := range allPayments {
		if payment.State != types.PaymentOpen {
			continue
		}
		payments = append(payments, payment)
	}
	return payments
}

func (k *keeper) checkCreditsAndSendFromModule(ctx sdk.Context, owner sdk.AccAddress, account *types.Account) error {
	if !account.CreditsBalance.IsZero() {
		granter, err := getGranterAddr()
		if err != nil {
			return err
		}

		authorization, expiration, expired := k.getAuthorization(ctx, owner, granter, types.DefaultMsgType)
		if authorization != nil {
			if expired {
				if err := k.authzkeeper.DeleteGrant(ctx, owner, granter, types.DefaultMsgType); err != nil {
					return err
				}
			} else {
				authorization.Credits = authorization.Credits.Add(account.CreditsBalance)
				if err := k.authzkeeper.SaveGrant(ctx, owner, granter, authorization, expiration); err != nil {
					return err
				}
				if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, granter,
					sdk.NewCoins(account.CreditsBalance)); err != nil {
					return err
				}
			}
		}
	}

	if !account.TokensBalance.IsZero() {
		if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, owner,
			sdk.NewCoins(account.TokensBalance)); err != nil {
			return err
		}
	}

	return nil
}

func (k *keeper) accountWithdraw(ctx sdk.Context, obj *types.Account) error {
	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	if obj.TotalBalance.IsZero() {
		return nil
	}

	if err := k.checkCreditsAndSendFromModule(ctx, owner, obj); err != nil {
		ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
		return err
	}

	obj.TotalBalance = sdk.NewCoin(obj.TotalBalance.Denom, sdk.ZeroInt())
	obj.CreditsBalance = sdk.NewCoin(obj.CreditsBalance.Denom, sdk.ZeroInt())
	obj.TokensBalance = sdk.NewCoin(obj.TokensBalance.Denom, sdk.ZeroInt())

	k.saveAccount(ctx, obj)
	return nil
}

func (k *keeper) paymentWithdraw(ctx sdk.Context, obj *types.Payment) error {

	owner, err := sdk.AccAddressFromBech32(obj.Owner)
	if err != nil {
		return err
	}

	if obj.Balance.IsZero() {
		return nil
	}

	if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, owner, sdk.NewCoins(obj.Balance)); err != nil {
		ctx.Logger().Error("payment withdraw", "err", err, "account", obj.AccountID, "payment", obj.PaymentID)
		return err
	}

	obj.Withdrawn = obj.Withdrawn.Add(obj.Balance)
	obj.Balance = sdk.NewCoin(obj.Balance.Denom, sdk.ZeroInt())

	k.savePayment(ctx, obj)
	return nil
}

func deductTransferredAmount(account types.Account, transferred sdk.Int) types.Account {
	if transferred.GT(account.CreditsBalance.Amount) {
		// subtracting transferred amount from token and credit balances
		extra := transferred.Sub(account.CreditsBalance.Amount)
		account.TokensBalance = account.TokensBalance.SubAmount(extra)
		account.CreditsBalance = sdk.NewCoin(account.CreditsBalance.Denom, sdk.ZeroInt())
	} else {
		account.CreditsBalance = account.CreditsBalance.SubAmount(transferred)
	}

	return account
}

func accountSettleFullblocks(
	account types.Account,
	payments []types.Payment,
	heightDelta sdk.Int,
	blockRate sdk.Coin,
) (
	types.Account,
	[]types.Payment,
	bool,
	sdk.Coin,
) {

	numFullBlocks := account.TotalBalance.Amount.Quo(blockRate.Amount)

	if numFullBlocks.GT(heightDelta) {
		numFullBlocks = heightDelta
	}

	for idx := range payments {
		p := payments[idx]
		payments[idx].Balance = p.Balance.Add(
			sdk.NewCoin(p.Rate.Denom, p.Rate.Amount.Mul(numFullBlocks)))
	}

	transferred := sdk.NewCoin(blockRate.Denom, blockRate.Amount.Mul(numFullBlocks))

	account.Transferred = account.Transferred.Add(transferred)
	account.TotalBalance = account.TotalBalance.Sub(transferred)
	account = deductTransferredAmount(account, transferred.Amount)

	remaining := account.TotalBalance
	overdrawn := true
	if numFullBlocks.Equal(heightDelta) {
		remaining.Amount = sdk.ZeroInt()
		overdrawn = false
	}

	return account, payments, overdrawn, remaining
}

func accountSettleDistributeWeighted(
	account types.Account,
	payments []types.Payment,
	blockRate sdk.Coin,
	amountRemaining sdk.Coin,
) (
	types.Account,
	[]types.Payment,
	sdk.Coin,
) {

	actualTransferred := sdk.ZeroInt()

	// distribute remaining balance weighted by rate
	for idx := range payments {
		payment := payments[idx]
		// amount := (rate / blockrate) * remaining
		amount := amountRemaining.Amount.
			Mul(payment.Rate.Amount).
			Quo(blockRate.Amount)

		payments[idx].Balance = payment.Balance.Add(sdk.NewCoin(payment.Balance.Denom, amount))

		actualTransferred = actualTransferred.Add(amount)
	}

	transferred := sdk.NewCoin(account.TotalBalance.Denom, actualTransferred)

	account.Transferred = account.Transferred.Add(transferred)
	account.TotalBalance = account.TotalBalance.Sub(transferred)
	account = deductTransferredAmount(account, transferred.Amount)

	amountRemaining = amountRemaining.Sub(transferred)

	return account, payments, amountRemaining
}

func accountSettleDistributeEvenly(
	account types.Account,
	payments []types.Payment,
	amountRemaining sdk.Coin,
) (
	types.Account,
	[]types.Payment,
	sdk.Coin,
) {

	baseAmt := amountRemaining.Amount.QuoRaw(int64(len(payments)))
	numOverflow := amountRemaining.Amount.ModRaw(int64(len(payments))).Int64()

	transferred := sdk.ZeroInt()

	for idx := range payments {
		amt := baseAmt
		if int64(idx) < numOverflow {
			amt = amt.AddRaw(1)
		}
		payments[idx].Balance.Amount = payments[idx].Balance.Amount.Add(amt)
		transferred = transferred.Add(amt)
	}

	account.Transferred.Amount = account.Transferred.Amount.Add(transferred)
	account.TotalBalance.Amount = account.TotalBalance.Amount.Sub(transferred)
	account = deductTransferredAmount(account, transferred)

	amountRemaining.Amount = amountRemaining.Amount.Sub(transferred)

	return account, payments, amountRemaining
}

func (k *keeper) getAuthorization(ctx sdk.Context, grantee sdk.AccAddress, granter sdk.AccAddress,
	msgType string) (*types.EscrowAuthorization, time.Time, bool) {
	authorization, expiration := k.authzkeeper.GetCleanAuthorization(ctx, grantee, granter, msgType)
	if authorization == nil {
		return nil, expiration, false
	}

	escrowAuthorization, ok := authorization.(*types.EscrowAuthorization)
	if !ok {
		return nil, expiration, false
	}

	if expiration.Before(ctx.BlockTime()) {
		return escrowAuthorization, expiration, true
	}

	return escrowAuthorization, expiration, false
}

package keeper

import (
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	aauthz "pkg.akt.dev/go/node/types/authz/v1"

	"pkg.akt.dev/go/node/escrow/v1"
)

type AccountHook func(sdk.Context, v1.Account)
type PaymentHook func(sdk.Context, v1.FractionalPayment)

type Keeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	AccountCreate(ctx sdk.Context, id v1.AccountID, owner sdk.AccAddress, deposits []v1.Deposit) error
	AccountDeposit(ctx sdk.Context, id v1.AccountID, deposits []v1.Deposit) error
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
	SaveAccount(sdk.Context, v1.Account) error
	SavePayment(sdk.Context, v1.FractionalPayment)
}

func NewKeeper(
	cdc codec.BinaryCodec,
	skey storetypes.StoreKey,
	bkeeper BankKeeper,
	tkeeper TakeKeeper,
	akeeper AuthzKeeper,
	feepool collections.Item[distrtypes.FeePool],
) Keeper {
	return &keeper{
		cdc:         cdc,
		skey:        skey,
		bkeeper:     bkeeper,
		tkeeper:     tkeeper,
		authzKeeper: akeeper,
		feepool:     feepool,
	}
}

type keeper struct {
	cdc         codec.BinaryCodec
	skey        storetypes.StoreKey
	bkeeper     BankKeeper
	tkeeper     TakeKeeper
	authzKeeper AuthzKeeper
	feepool     collections.Item[distrtypes.FeePool]
	hooks       struct {
		onAccountClosed []AccountHook
		onPaymentClosed []PaymentHook
	}
}

type account struct {
	v1.Account
	dirty     bool
	prevState v1.State
}

type payment struct {
	v1.FractionalPayment
	dirty     bool
	prevState v1.State
}

func (k *keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k *keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

func (k *keeper) AccountCreate(ctx sdk.Context, id v1.AccountID, owner sdk.AccAddress, deposits []v1.Deposit) error {
	store := ctx.KVStore(k.skey)

	key := k.findAccount(ctx, id)
	if len(key) != 0 {
		return v1.ErrAccountExists
	}

	denoms := make(map[string]int)

	for _, deposit := range deposits {
		denoms[deposit.Amount.Denom] = 1
	}

	transferred := make(sdk.DecCoins, 0, len(denoms))
	funds := make([]v1.Funds, 0, len(denoms))

	for denom := range denoms {
		transferred = append(transferred, sdk.NewDecCoin(denom, sdkmath.ZeroInt()))
		funds = append(funds, v1.Funds{
			Balance:   sdk.NewDecCoin(denom, sdkmath.ZeroInt()),
			Overdraft: sdkmath.LegacyZeroDec(),
		})
	}

	obj := &account{
		Account: v1.Account{
			ID:          id,
			Owner:       owner.String(),
			State:       v1.StateOpen,
			Transferred: transferred,
			SettledAt:   ctx.BlockHeight(),
			Funds:       funds,
			Deposits:    deposits,
		},
		dirty:     true,
		prevState: v1.StateOpen,
	}

	if err := obj.ValidateBasic(); err != nil {
		return err
	}

	if err := k.fetchDepositsToAccount(ctx, obj, deposits); err != nil {
		return err
	}

	key = BuildAccountsKey(obj.State, &id)
	store.Set(key, k.cdc.MustMarshal(obj))

	return nil
}

func (k *keeper) AccountClose(ctx sdk.Context, id v1.AccountID) error {
	acc, payments, od, err := k.accountSettle(ctx, id)
	if err != nil {
		return err
	}

	if od {
		return nil
	}

	acc.State = v1.StateClosed
	acc.dirty = true

	if err := k.accountWithdraw(ctx, &acc); err != nil {
		return err
	}

	for idx := range payments {
		payments[idx].State = v1.StateClosed
		payments[idx].dirty = true

		if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
			return err
		}
	}

	err = k.save(ctx, acc, payments)
	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) AccountDeposit(ctx sdk.Context, id v1.AccountID, deposits []v1.Deposit) error {
	obj, err := k.getAccount(ctx, id)
	if err != nil {
		return err
	}

	if obj.State != v1.StateOpen {
		return v1.ErrAccountClosed
	}

	if err := k.fetchDepositsToAccount(ctx, &obj, deposits); err != nil {
		return err
	}

	if obj.dirty {
		err = k.saveAccount(ctx, obj)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *keeper) AccountSettle(ctx sdk.Context, id v1.AccountID) (bool, error) {
	acc, payments, od, err := k.accountSettle(ctx, id)
	if err != nil {
		return false, err
	}

	err = k.save(ctx, acc, payments)
	if err != nil {
		return false, err
	}

	return od, err
}

// fetchDepositToAccount fetches deposit amount from the depositor's account to the escrow
// account and accordingly updates the balance or funds.
func (k *keeper) fetchDepositsToAccount(ctx sdk.Context, acc *account, deposits []v1.Deposit) error {
	if len(deposits) > 0 {
		acc.dirty = true
	}

	for _, deposit := range deposits {
		depositor, err := sdk.AccAddressFromBech32(deposit.Depositor)
		if err != nil {
			return err
		}

		var funds *v1.Funds

		for i := range acc.Funds {
			if acc.Funds[i].Balance.Denom == deposit.Amount.Denom {
				funds = &acc.Funds[i]
			}
		}

		if funds == nil {
			return v1.ErrInvalidDenomination
		}

		if err := k.bkeeper.SendCoinsFromAccountToModule(ctx, depositor, v1.ModuleName, sdk.NewCoins(deposit.Amount)); err != nil {
			return err
		}

		funds.Balance = funds.Balance.Add(sdk.NewDecCoinFromCoin(deposit.Amount))
	}

	return nil
}

func (k *keeper) accountSettle(ctx sdk.Context, id v1.AccountID) (account, []payment, bool, error) {
	acc, err := k.getAccount(ctx, id)
	if err != nil {
		return account{}, nil, false, err
	}

	if acc.State != v1.StateOpen {
		return account{}, nil, false, v1.ErrAccountClosed
	}

	payments := k.accountOpenPayments(ctx, id)

	heightDelta := sdkmath.NewInt(ctx.BlockHeight() - acc.SettledAt)
	if heightDelta.IsZero() {
		return acc, nil, false, nil
	}

	acc.SettledAt = ctx.BlockHeight()
	acc.dirty = true

	if len(payments) == 0 {
		return acc, nil, false, nil
	}

	overdrawn := accountSettleFullBlocks(&acc, payments, heightDelta)

	// all payments made in full
	if !overdrawn {
		// return early
		return acc, payments, false, nil
	}

	//
	// overdrawn
	//
	acc.State = v1.StateOverdrawn

	for idx := range payments {
		payments[idx].State = v1.StateOverdrawn
		payments[idx].dirty = true
		if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
			return acc, payments, overdrawn, err
		}
	}

	return acc, payments, overdrawn, nil
}

func (k *keeper) PaymentCreate(ctx sdk.Context, id v1.AccountID, pid string, owner sdk.AccAddress, rate sdk.DecCoin) error {
	acc, _, od, err := k.accountSettle(ctx, id)
	if err != nil {
		return err
	}

	if od {
		return v1.ErrAccountOverdrawn
	}

	var funds *v1.Funds
	for i := range acc.Funds {
		if rate.Denom == acc.Funds[i].Balance.Denom {
			funds = &acc.Funds[i]
			break
		}
	}

	if funds == nil {
		return v1.ErrInvalidDenomination
	}

	if rate.IsZero() {
		return v1.ErrPaymentRateZero
	}

	key := k.findPayment(ctx, id, pid)
	if len(key) != 0 {
		return v1.ErrPaymentExists
	}

	if acc.dirty {
		err = k.saveAccount(ctx, acc)
		if err != nil {
			return err
		}
	}

	k.savePayment(ctx, payment{
		FractionalPayment: v1.FractionalPayment{
			AccountID: id,
			PaymentID: pid,
			Owner:     owner.String(),
			State:     v1.StateOpen,
			Rate:      rate,
			Balance:   sdk.NewDecCoin(rate.Denom, sdkmath.ZeroInt()),
			Withdrawn: sdk.NewCoin(rate.Denom, sdkmath.ZeroInt()),
		},
		dirty:     false,
		prevState: v1.StateOpen,
	})

	return nil
}

func (k *keeper) PaymentWithdraw(ctx sdk.Context, id v1.AccountID, pid string) error {
	pmnt, err := k.getPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	if pmnt.State != v1.StateOpen {
		return v1.ErrPaymentClosed
	}

	acc, payments, od, err := k.accountSettle(ctx, id)
	if err != nil {
		return err
	}

	if !od {
		pmnt = nil

		for i, p := range payments {
			if p.PaymentID == pid {
				pmnt = &payments[i]
			}
		}

		if pmnt == nil {
			panic(fmt.Sprintf("couldn't find payment"))
		}

		err = k.paymentWithdraw(ctx, pmnt)
		if err != nil {
			return err
		}
	}

	err = k.save(ctx, acc, payments)
	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) PaymentClose(ctx sdk.Context, id v1.AccountID, pid string) error {
	pmnt, err := k.getPayment(ctx, id, pid)
	if err != nil {
		return err
	}

	if pmnt.State != v1.StateOpen {
		return v1.ErrPaymentClosed
	}

	acc, payments, od, err := k.accountSettle(ctx, id)
	if err != nil {
		return err
	}
	if od {
		return nil
	}

	pmnt = nil

	for i, p := range payments {
		if p.PaymentID == pid {
			pmnt = &payments[i]
		}
	}

	if pmnt == nil {
		panic(fmt.Sprintf("couldn't find payment"))
	}

	if err := k.paymentWithdraw(ctx, pmnt); err != nil {
		return err
	}

	pmnt.State = v1.StateClosed
	pmnt.dirty = true

	err = k.save(ctx, acc, payments)
	if err != nil {
		return err
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
	obj, err := k.getAccount(ctx, id)
	if err != nil {
		return v1.Account{}, err
	}

	return obj.Account, nil
}

func (k *keeper) getAccount(ctx sdk.Context, id v1.AccountID) (account, error) {
	store := ctx.KVStore(k.skey)

	key := k.findAccount(ctx, id)
	if len(key) == 0 {
		return account{}, v1.ErrAccountNotFound
	}

	buf := store.Get(key)

	var obj account

	k.cdc.MustUnmarshal(buf, &obj.Account)
	obj.prevState = obj.State

	return obj, nil
}

func (k *keeper) GetPayment(ctx sdk.Context, id v1.AccountID, pid string) (v1.FractionalPayment, error) {
	obj, err := k.getPayment(ctx, id, pid)
	if err != nil {
		return v1.FractionalPayment{}, err
	}

	return obj.FractionalPayment, nil
}

func (k *keeper) getPayment(ctx sdk.Context, id v1.AccountID, pid string) (*payment, error) {
	store := ctx.KVStore(k.skey)

	key := k.findPayment(ctx, id, pid)
	if len(key) == 0 {
		return nil, v1.ErrPaymentNotFound
	}

	buf := store.Get(key)

	var obj payment

	k.cdc.MustUnmarshal(buf, &obj.FractionalPayment)
	obj.prevState = obj.State

	return &obj, nil
}

func (k *keeper) SaveAccount(ctx sdk.Context, obj v1.Account) error {
	err := k.saveAccount(ctx, account{
		Account:   obj,
		prevState: obj.State,
	})

	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) SavePayment(ctx sdk.Context, obj v1.FractionalPayment) {
	k.savePayment(ctx, payment{
		FractionalPayment: obj,
		prevState:         obj.State,
	})
}

func (k *keeper) WithAccounts(ctx sdk.Context, fn func(v1.Account) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, v1.AccountKeyPrefix())

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
	iter := storetypes.KVStorePrefixIterator(store, v1.PaymentKeyPrefix())

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

func (k *keeper) saveAccount(ctx sdk.Context, obj account) error {
	store := ctx.KVStore(k.skey)

	var key []byte
	if obj.State != obj.prevState {
		key := BuildAccountsKey(obj.prevState, &obj.ID)
		store.Delete(key)
	}

	key = BuildAccountsKey(obj.State, &obj.ID)

	if obj.State == v1.StateClosed || obj.State == v1.StateOverdrawn {
		for _, deposit := range obj.Deposits {
			if deposit.Balance.IsPositive() {
				depositor, err := sdk.AccAddressFromBech32(deposit.Depositor)
				if err != nil {
					return err
				}

				withdrawal := sdk.NewCoin(deposit.Balance.Denom, deposit.Balance.Amount.TruncateInt())

				err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, depositor, sdk.NewCoins(withdrawal))
				if err != nil {
					return err
				}

				// if depositor is not an owner then funds came from the grant.
				if deposit.Depositor != obj.Owner {
					owner, err := sdk.AccAddressFromBech32(obj.Owner)
					if err != nil {
						return err
					}

					// if exists, increase allowed authz deposit by remainder in the Balance, it will allow owner to reuse active authz
					// without asking for renew.
					msgTypeUrl := (&aauthz.DepositAuthorization{}).MsgTypeURL()

					authorization, expiration := k.authzKeeper.GetAuthorization(ctx, owner, depositor, msgTypeUrl)
					dauthz, valid := authorization.(*aauthz.DepositAuthorization)
					if valid && authorization != nil {
						dauthz.SpendLimit = dauthz.SpendLimit.Add(withdrawal)
						err = k.authzKeeper.SaveGrant(ctx, owner, depositor, dauthz, expiration)
						if err != nil {
							return err
						}
					}
				}

				obj.Funds[0].Balance = obj.Funds[0].Balance.Sub(sdk.NewDecCoinFromCoin(withdrawal))
			}
		}

		obj.Deposits = []v1.Deposit{}
	}

	store.Set(key, k.cdc.MustMarshal(&obj.Account))

	if obj.State == v1.StateClosed || obj.State == v1.StateOverdrawn {

		// call hooks
		for _, hook := range k.hooks.onAccountClosed {
			hook(ctx, obj.Account)
		}
	}

	return nil
}

func (k *keeper) savePayment(ctx sdk.Context, obj payment) {
	store := ctx.KVStore(k.skey)

	var key []byte
	if obj.State != obj.prevState {
		key := BuildPaymentsKey(obj.prevState, &obj.AccountID, obj.PaymentID)
		store.Delete(key)
	}

	key = BuildPaymentsKey(obj.State, &obj.AccountID, obj.PaymentID)
	store.Set(key, k.cdc.MustMarshal(&obj.FractionalPayment))

	if obj.State == v1.StateClosed || obj.State == v1.StateOverdrawn {
		// call hooks
		for _, hook := range k.hooks.onPaymentClosed {
			hook(ctx, obj.FractionalPayment)
		}
	}
}

func (k *keeper) save(ctx sdk.Context, acc account, payments []payment) error {
	if acc.dirty {
		err := k.saveAccount(ctx, acc)
		if err != nil {
			return err
		}
	}

	for _, pmnt := range payments {
		if pmnt.dirty {
			k.savePayment(ctx, pmnt)
		}
	}

	return nil
}

func (k *keeper) accountOpenPayments(ctx sdk.Context, id v1.AccountID) []payment {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, BuildPaymentsKey(v1.StateOpen, &id, ""))

	var payments []payment

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val v1.FractionalPayment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		payments = append(payments, payment{
			FractionalPayment: val,
			prevState:         val.State,
		})
	}

	return payments
}

func (k *keeper) accountWithdraw(ctx sdk.Context, obj *account) error {
	//if obj.Balance.Amount.LT(sdkmath.LegacyNewDec(1)) {
	//	return nil
	//}
	//
	//owner, err := sdk.AccAddressFromBech32(obj.Owner)
	//if err != nil {
	//	return err
	//}
	//
	//if !obj.Balance.Amount.LT(sdkmath.LegacyNewDec(1)) {
	//	withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Balance.Amount.TruncateInt())
	//
	//	if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, owner, sdk.NewCoins(withdrawal)); err != nil {
	//		ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
	//		return err
	//	}
	//	obj.Balance = obj.Balance.Sub(sdk.NewDecCoinFromCoin(withdrawal))
	//}

	//if obj.Balance.IsPositive() {
	//	depositor, err := sdk.AccAddressFromBech32(obj.Depositor)
	//	if err != nil {
	//		return err
	//	}
	//
	//	withdrawal := sdk.NewCoin(obj.Balance.Denom, obj.Funds.Amount.TruncateInt())
	//	if err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, v1.ModuleName, depositor, sdk.NewCoins(withdrawal)); err != nil {
	//		ctx.Logger().Error("account withdraw", "err", err, "id", obj.ID)
	//		return err
	//	}
	//
	//	obj.Funds = obj.Funds.Sub(sdk.NewDecCoinFromCoin(withdrawal))
	//
	//	msg := &dv1.MsgDepositDeployment{Amount: sdk.NewCoin(obj.Balance.Denom, sdkmath.NewInt(0))}
	//
	//	// Funds field is solely to track deposits via authz.
	//	// check if there is active deployment authorization from given depositor
	//	// if exists, increase allowed authz deposit by remainder in the Funds, it will allow owner to reuse active authz
	//	// without asking for renew.
	//	authorization, expiration := k.authzKeeper.GetAuthorization(ctx, owner, depositor, sdk.MsgTypeURL(msg))
	//	dauthz, valid := authorization.(*dv1.DepositAuthorization)
	//	if valid && authorization != nil {
	//		dauthz.SpendLimit = dauthz.SpendLimit.Add(withdrawal)
	//		err = k.authzKeeper.SaveGrant(ctx, owner, depositor, dauthz, expiration)
	//		if err != nil {
	//			return err
	//		}
	//	}
	//}

	return nil
}

func (k *keeper) paymentWithdraw(ctx sdk.Context, obj *payment) error {
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
	obj.dirty = true

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

	pool, err := k.feepool.Get(ctx)
	if err != nil {
		return err
	}

	pool.CommunityPool = pool.CommunityPool.Add(sdk.NewDecCoinFromCoin(fee))

	err = k.feepool.Set(ctx, pool)
	if err != nil {
		return err
	}

	return nil
}

func (acc *account) deductFromBalance(amount sdk.DecCoin) (sdk.DecCoin, bool) {
	remaining := sdkmath.LegacyZeroDec()
	remaining.AddMut(amount.Amount)

	withdrew := sdkmath.LegacyZeroDec()

	idx := 0

	var funds *v1.Funds

	for i := range acc.Funds {
		if amount.Denom == acc.Funds[i].Balance.Denom {
			funds = &acc.Funds[i]
		}
	}

	if funds == nil {
		panic(fmt.Sprintf("unknown denom \"%s\"", amount.Denom))
	}

	for i, deposit := range acc.Deposits {
		toWithdraw := sdkmath.LegacyZeroDec()

		if deposit.Balance.Amount.LT(remaining) {
			toWithdraw.AddMut(deposit.Balance.Amount)
		} else {
			toWithdraw.AddMut(remaining)
		}

		funds.Balance.Amount.SubMut(toWithdraw)
		acc.Deposits[i].Balance.Amount.SubMut(toWithdraw)
		if acc.Deposits[i].Balance.IsZero() {
			idx++
		}
		remaining.SubMut(toWithdraw)
		withdrew.AddMut(toWithdraw)

		if remaining.IsZero() {
			break
		}
	}

	if idx > 0 {
		acc.Deposits = acc.Deposits[idx:]
	}

	return sdk.NewDecCoinFromDec(amount.Denom, withdrew), !remaining.IsZero()
}

func accountSettleFullBlocks(acc *account, payments []payment, heightDelta sdkmath.Int) bool {
	funds := &acc.Funds[0]

	blockRate := sdk.NewDecCoin(funds.Balance.Denom, sdkmath.ZeroInt())
	remaining := sdk.NewDecCoin(funds.Balance.Denom, sdkmath.ZeroInt())
	paymentsTransfers := make([]sdk.DecCoin, 0, len(payments))

	for _, pmnt := range payments {
		blockRate = blockRate.Add(pmnt.Rate)
	}

	for idx := range payments {
		p := payments[idx]
		paymentTransfer := sdk.NewDecCoinFromDec(p.Rate.Denom, p.Rate.Amount.Mul(sdkmath.LegacyNewDecFromInt(heightDelta)))

		paymentsTransfers = append(paymentsTransfers, paymentTransfer)
		remaining.Amount.AddMut(paymentTransfer.Amount)
	}

	overdrawn := false

	for idx := range payments {
		withdrawn := sdk.NewDecCoin(funds.Balance.Denom, sdkmath.ZeroInt())

		withdrawn, overdrawn = acc.deductFromBalance(paymentsTransfers[idx])
		payments[idx].Balance.Amount.AddMut(withdrawn.Amount)
		remaining.Amount.SubMut(withdrawn.Amount)

		for j := range acc.Transferred {
			if acc.Transferred[j].Denom == withdrawn.Denom {
				acc.Transferred[j].Amount.AddMut(withdrawn.Amount)
			}
		}

		if overdrawn {
			funds.Overdraft.AddMut(remaining.Amount)

			break
		}
	}

	acc.dirty = true

	return overdrawn
}

func (k *keeper) findAccount(ctx sdk.Context, id v1.AccountID) []byte {
	store := ctx.KVStore(k.skey)

	okey := BuildAccountsKey(v1.StateOpen, &id)
	ckey := BuildAccountsKey(v1.StateClosed, &id)
	ovkey := BuildAccountsKey(v1.StateOverdrawn, &id)

	var key []byte

	if store.Has(okey) {
		key = okey
	} else if store.Has(ckey) {
		key = ckey
	} else if store.Has(ovkey) {
		key = ovkey
	}

	return key
}

func (k *keeper) findPayment(ctx sdk.Context, id v1.AccountID, pid string) []byte {
	store := ctx.KVStore(k.skey)

	okey := BuildPaymentsKey(v1.StateOpen, &id, pid)
	ckey := BuildPaymentsKey(v1.StateClosed, &id, pid)
	ovkey := BuildPaymentsKey(v1.StateOverdrawn, &id, pid)

	var key []byte

	if store.Has(okey) {
		key = okey
	} else if store.Has(ckey) {
		key = ckey
	} else if store.Has(ovkey) {
		key = ovkey
	}

	return key
}

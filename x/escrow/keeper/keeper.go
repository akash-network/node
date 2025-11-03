package keeper

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"

	dvbeta "pkg.akt.dev/go/node/deployment/v1beta4"
	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	"pkg.akt.dev/go/node/escrow/module"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	ev1 "pkg.akt.dev/go/node/escrow/v1"
	mv1 "pkg.akt.dev/go/node/market/v1"
	mtypes "pkg.akt.dev/go/node/market/v1beta5"
	deposit "pkg.akt.dev/go/node/types/deposit/v1"

	bmetypes "pkg.akt.dev/go/node/bme/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/x/escrow/imports"
)

type AccountHook func(sdk.Context, etypes.Account) error
type PaymentHook func(sdk.Context, etypes.Payment) error

type Keeper interface {
	Codec() codec.BinaryCodec
	StoreKey() storetypes.StoreKey
	EndBlocker(_ context.Context) error

	AuthorizeDeposits(sctx sdk.Context, msg sdk.Msg) ([]etypes.Depositor, error)
	AccountCreate(ctx sdk.Context, id escrowid.Account, owner sdk.AccAddress, deposits []etypes.Depositor) error
	AccountDeposit(ctx sdk.Context, id escrowid.Account, deposits []etypes.Depositor) error
	AccountSettle(ctx sdk.Context, id escrowid.Account) (bool, error)
	AccountClose(ctx sdk.Context, id escrowid.Account) error
	PaymentCreate(ctx sdk.Context, id escrowid.Payment, owner sdk.AccAddress, rate sdk.DecCoin) error
	PaymentWithdraw(ctx sdk.Context, id escrowid.Payment) error
	PaymentClose(ctx sdk.Context, id escrowid.Payment) error
	GetAccount(ctx sdk.Context, id escrowid.Account) (etypes.Account, error)
	GetPayment(ctx sdk.Context, id escrowid.Payment) (etypes.Payment, error)
	AddOnAccountClosedHook(AccountHook) Keeper
	AddOnPaymentClosedHook(PaymentHook) Keeper
	WithAccounts(sdk.Context, func(etypes.Account) bool)
	WithPayments(sdk.Context, func(etypes.Payment) bool)
	SaveAccount(sdk.Context, etypes.Account) error
	SavePayment(sdk.Context, etypes.Payment) error
	SaveAccountRaw(sdk.Context, etypes.Account) error
	SavePaymentRaw(sdk.Context, etypes.Payment) error
	GetAccountPayments(ctx sdk.Context, id escrowid.Account, states []etypes.State) []etypes.Payment
	NewQuerier() Querier
}

type keeper struct {
	cdc          codec.BinaryCodec
	skey         storetypes.StoreKey
	ac           address.Codec
	bkeeper      imports.BankKeeper
	authzKeeper  imports.AuthzKeeper
	oracleKeeper imports.OracleKeeper
	bmeKeeper    imports.BMEKeeper

	schema collections.Schema

	hooks struct {
		onAccountClosed []AccountHook
		onPaymentClosed []PaymentHook
	}
}

func NewKeeper(
	cdc codec.BinaryCodec,
	skey *storetypes.KVStoreKey,
	ac address.Codec,
	bkeeper imports.BankKeeper,
	akeeper imports.AuthzKeeper,
	okeeper imports.OracleKeeper,
	bmekeeper imports.BMEKeeper,
) Keeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	kpr := &keeper{
		cdc:          cdc,
		skey:         skey,
		ac:           ac,
		bkeeper:      bkeeper,
		authzKeeper:  akeeper,
		oracleKeeper: okeeper,
		bmeKeeper:    bmekeeper,
		schema:       schema,
	}

	return kpr
}

type account struct {
	etypes.Account
	dirty     bool
	prevState etypes.State
}

type payment struct {
	etypes.Payment
	dirty     bool
	prevState etypes.State
}

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

func (k *keeper) AccountCreate(ctx sdk.Context, id escrowid.Account, owner sdk.AccAddress, deposits []etypes.Depositor) error {
	store := ctx.KVStore(k.skey)

	key := k.findAccount(ctx, &id)
	if len(key) != 0 {
		return module.ErrAccountExists
	}

	// Create an account object with empty funds/transferred - will be populated based on actual deposit denoms
	obj := &account{
		Account: etypes.Account{
			ID: id,
			State: etypes.AccountState{
				Owner:       owner.String(),
				State:       etypes.StateOpen,
				Transferred: make(sdk.DecCoins, 0),
				SettledAt:   ctx.BlockHeight(),
				Funds:       make([]etypes.Balance, 0),
				Deposits:    make([]etypes.Depositor, 0),
			},
		},
		dirty:     true,
		prevState: etypes.StateOpen,
	}

	// Process deposits first to determine actual denoms (after BME conversion)
	if err := k.fetchDepositsToAccount(ctx, obj, deposits); err != nil {
		return err
	}

	if err := obj.ValidateBasic(); err != nil {
		return err
	}

	key = BuildAccountsKey(obj.State.State, &id)
	store.Set(key, k.cdc.MustMarshal(&obj.State))

	return nil
}

func (k *keeper) AuthorizeDeposits(sctx sdk.Context, msg sdk.Msg) ([]etypes.Depositor, error) {
	depositors := make([]etypes.Depositor, 0, 1)

	lMsg, valid := msg.(sdk.LegacyMsg)
	if !valid {
		return nil, fmt.Errorf("%w: message [%s] does not implement sdk.LegacyMsg", module.ErrInvalidDeposit, reflect.TypeOf(msg).String())
	}

	signers := lMsg.GetSigners()
	if len(signers) != 1 {
		return nil, fmt.Errorf("%w: invalid signers", module.ErrInvalidDeposit)
	}

	owner := signers[0]

	var deposits deposit.Deposits
	switch mt := msg.(type) {
	case deposit.HasDeposit:
		deposits = deposit.Deposits{mt.GetDeposit()}
	case deposit.HasDeposits:
		deposits = mt.GetDeposits()
	default:
		return nil, fmt.Errorf("%w: message [%s] does not implement deposit.HasDeposit or deposit.HasDeposits", module.ErrInvalidDeposit, reflect.TypeOf(msg).String())
	}

	// Process each deposit
	for _, dep := range deposits {
		denom := dep.Amount.Denom
		remainder := sdkmath.NewInt(dep.Amount.Amount.Int64())

		for _, source := range dep.Sources {
			switch source {
			case deposit.SourceBalance:
				spendableAmount := k.bkeeper.SpendableCoin(sctx, owner, denom)

				if spendableAmount.Amount.IsPositive() {
					requestedSpend := sdk.NewCoin(denom, remainder)

					if spendableAmount.IsLT(requestedSpend) {
						requestedSpend = spendableAmount
					}
					depositors = append(depositors, etypes.Depositor{
						Owner:   owner.String(),
						Height:  sctx.BlockHeight(),
						Source:  deposit.SourceBalance,
						Balance: sdk.NewDecCoinFromCoin(requestedSpend),
					})

					remainder = remainder.Sub(requestedSpend.Amount)
				}
			case deposit.SourceGrant:
				// find the DepositDeploymentAuthorization given to the owner by the depositor and check
				// acceptance
				msgTypeUrl := (&ev1.DepositAuthorization{}).MsgTypeURL()

				k.authzKeeper.GetGranteeGrantsByMsgType(sctx, owner, msgTypeUrl, func(ctx context.Context, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) bool {
					depositAuthz, valid := authorization.(ev1.Authorization)
					if !valid {
						return false
					}

					spendableAmount := depositAuthz.GetSpendLimits()
					if spendLimit := depositAuthz.GetSpendLimit(); !spendLimit.IsNil() {
						spendableAmount = spendableAmount.Add(spendLimit)
					}

					if spendableAmount.IsZero() {
						return false
					}

					requestedSpend := sdk.NewCoin(denom, remainder)

					var authzMsg sdk.Msg

					// bc authz.Accepts take sdk.Msg as an argument, the deposit amount from incoming message
					// has to be modified in place to correctly calculate what deposits to take from grants
					switch mt := msg.(type) {
					case *ev1.MsgAccountDeposit:
						authzMsg = &ev1.MsgAccountDeposit{
							Signer: mt.Signer,
							ID:     mt.ID,
							Deposit: deposit.Deposit{
								Amount:  requestedSpend,
								Sources: mt.Deposit.Sources,
							},
						}
					case *dvbeta.MsgCreateDeployment:
						authzMsg = &dvbeta.MsgCreateDeployment{
							ID:     mt.ID,
							Groups: mt.Groups,
							Hash:   mt.Hash,
							Deposit: deposit.Deposit{
								Amount:  requestedSpend,
								Sources: dep.Sources,
							},
						}
					case *mtypes.MsgCreateBid:
						authzMsg = &mtypes.MsgCreateBid{
							ID:    mt.ID,
							Price: mt.Price,
							Deposit: deposit.Deposit{
								Amount:  requestedSpend,
								Sources: dep.Sources,
							},
							ResourcesOffer: mt.ResourcesOffer,
						}
					}

					resp, err := depositAuthz.TryAccept(ctx, authzMsg, true)
					if err != nil {
						return false
					}

					if !resp.Accept {
						return false
					}

					// Delete is ignored here as not all funds may be used during deployment lifetime.
					// also, there can be another deployment using same authorization and may return funds before deposit is fully used
					err = k.authzKeeper.SaveGrant(ctx, owner, granter, resp.Updated, expiration)
					if err != nil {
						return false
					}

					depositAuthz = resp.Updated.(ev1.Authorization)

					spendableAmount = spendableAmount.Sub(depositAuthz.GetSpendLimits()...)

					var spendableCoin sdk.Coin

					for _, c := range spendableAmount {
						if c.Denom == denom {
							spendableCoin = c
							break
						}
					}

					if spendableCoin.IsNil() {
						return false
					}

					depositors = append(depositors, etypes.Depositor{
						Owner:   granter.String(),
						Height:  sctx.BlockHeight(),
						Source:  deposit.SourceGrant,
						Balance: sdk.NewDecCoinFromCoin(spendableCoin),
					})
					remainder = remainder.Sub(spendableCoin.Amount)

					return remainder.IsZero()
				})
			}

			if remainder.IsZero() {
				break
			}
		}

		if !remainder.IsZero() {
			// the following check is for sanity. if value is negative, math above went horribly wrong
			if remainder.IsNegative() {
				return nil, fmt.Errorf("%w: deposit overflow", mv1.ErrInvalidDeposit)
			}

			return nil, fmt.Errorf("%w: insufficient balance", mv1.ErrInvalidDeposit)
		}
	}

	return depositors, nil
}

func (k *keeper) AccountClose(ctx sdk.Context, id escrowid.Account) error {
	acc, err := k.getAccount(ctx, id)
	if err != nil {
		return err
	}

	switch acc.State.State {
	case etypes.StateOpen:
	case etypes.StateOverdrawn:
		// if the account is overdrawn try to settle it
		// if settling fails it s still triggers deployment close
	case etypes.StateClosed:
		fallthrough
	default:
		return module.ErrAccountClosed
	}

	// ignore overdraft return value
	// all objects have correct values set
	payments, _, err := k.accountSettle(ctx, acc)
	if err != nil {
		return err
	}

	acc.dirty = true

	// call to accountSettle above will set account and payments to overdrawn state
	if acc.State.State == etypes.StateOpen {
		acc.State.State = etypes.StateClosed
	}

	for idx := range payments {
		payments[idx].dirty = true

		if payments[idx].State.State == etypes.StateOpen {
			payments[idx].State.State = etypes.StateClosed
		}

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

func (k *keeper) AccountDeposit(ctx sdk.Context, id escrowid.Account, deposits []etypes.Depositor) error {
	acc, err := k.getAccount(ctx, id)
	if err != nil {
		return err
	}

	if acc.State.State == etypes.StateClosed {
		return module.ErrAccountClosed
	}

	if err = k.fetchDepositsToAccount(ctx, acc, deposits); err != nil {
		return err
	}

	if acc.State.State == etypes.StateOverdrawn {
		payments, od, err := k.accountSettle(ctx, acc)
		if err != nil {
			return err
		}

		for idx := range payments {
			payments[idx].dirty = true

			if payments[idx].State.State == etypes.StateOpen {
				payments[idx].State.State = etypes.StateClosed
			}

			if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
				return err
			}
		}

		if !od {
			acc.State.State = etypes.StateClosed
		}

		err = k.save(ctx, acc, payments)
		if err != nil {
			return err
		}
	} else if acc.dirty {
		err = k.saveAccount(ctx, acc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *keeper) AccountSettle(ctx sdk.Context, id escrowid.Account) (bool, error) {
	acc, err := k.getAccount(ctx, id)
	if err != nil {
		return false, err
	}

	payments, od, err := k.accountSettle(ctx, acc)
	if err != nil {
		return false, err
	}

	err = k.save(ctx, acc, payments)
	if err != nil {
		return false, err
	}

	return od, err
}

// fetchDepositToAccount fetches the deposit amount from the depositor's account to the escrow
// account and accordingly updates the balance or funds.
// When circuit breaker is active, deposits are processed directly without BME conversion,
// keeping funds in their original denomination (AKT).
func (k *keeper) fetchDepositsToAccount(ctx sdk.Context, acc *account, deposits []etypes.Depositor) error {
	if len(deposits) > 0 {
		acc.dirty = true
	}

	processedDeposits := make([]etypes.Depositor, 0, len(deposits))

	for _, d := range deposits {
		// Now find or create funds entry with the actual denom (after potential BME conversion)
		var funds *etypes.Balance
		var transferred *sdk.DecCoin

		for i := range acc.State.Funds {
			if acc.State.Funds[i].Denom == d.Balance.Denom {
				funds = &acc.State.Funds[i]
			}
		}

		for i := range acc.State.Transferred {
			if acc.State.Transferred[i].Denom == d.Balance.Denom {
				transferred = &acc.State.Transferred[i]
			}
		}

		// If this is a new denom, initialize funds and transferred entries
		if funds == nil {
			acc.State.Funds = append(acc.State.Funds, etypes.Balance{
				Denom:  d.Balance.Denom,
				Amount: sdkmath.LegacyZeroDec(),
			})
			funds = &acc.State.Funds[len(acc.State.Funds)-1]
		}

		if transferred == nil {
			acc.State.Transferred = append(acc.State.Transferred, sdk.NewDecCoin(d.Balance.Denom, sdkmath.ZeroInt()))
			transferred = &acc.State.Transferred[len(acc.State.Transferred)-1]
		}

		if funds.Amount.IsNegative() {
			funds.Amount = sdkmath.LegacyZeroDec()
		}

		processedDeposits = append(processedDeposits, d)

		depositor, err := k.ac.StringToBytes(d.Owner)
		if err != nil {
			return err
		}

		// if balance is negative then reset it to zero and start accumulating fund.
		// later down in this function it will trigger account settlement and recalculate
		// the owed balance
		if err = k.bkeeper.SendCoinsFromAccountToModule(ctx, depositor, module.ModuleName, sdk.NewCoins(sdk.NewCoin(d.Balance.Denom, d.Balance.Amount.TruncateInt()))); err != nil {
			return err
		}

		funds.Amount.AddMut(d.Balance.Amount)
	}

	acc.State.Deposits = append(acc.State.Deposits, processedDeposits...)

	return nil
}

func (k *keeper) accountSettle(ctx sdk.Context, acc *account) ([]payment, bool, error) {
	if acc.State.State == etypes.StateClosed {
		return nil, false, module.ErrAccountClosed
	}

	if acc.State.Funds[0].Amount.IsNegative() {
		return nil, true, nil
	}

	// overdrawn account does not update settledAt, as associated objects like deployment
	// are closed
	heightDelta := sdkmath.NewInt(0)
	if acc.State.State != etypes.StateOverdrawn {
		heightDelta = heightDelta.AddRaw(ctx.BlockHeight() - acc.State.SettledAt)
		acc.State.SettledAt = ctx.BlockHeight()
	}

	pStates := []etypes.State{
		etypes.StateOverdrawn,
	}

	if !heightDelta.IsZero() {
		pStates = append(pStates, etypes.StateOpen)
	}

	acc.dirty = true

	payments := k.accountPayments(ctx, acc.ID, pStates)
	if len(payments) == 0 {
		return nil, false, nil
	}

	overdrawn := accountSettleFullBlocks(acc, payments, heightDelta)

	// AKT fallback: if overdrawn, try settling from AKT funds
	if overdrawn {
		if err := k.settleFromAktFallback(ctx, acc, payments); err != nil {
			return payments, true, err
		}

		// recheck overdrawn status
		overdrawn = false
		for idx := range payments {
			if payments[idx].State.State == etypes.StateOverdrawn {
				overdrawn = true
				break
			}
		}
	}

	// all payments made in full
	if !overdrawn {
		return payments, false, nil
	}

	//
	// overdrawn
	//
	acc.State.State = etypes.StateOverdrawn

	for idx := range payments {
		payments[idx].State.State = etypes.StateOverdrawn
		payments[idx].dirty = true
		if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
			return payments, overdrawn, err
		}
	}

	return payments, overdrawn, nil
}

// settleFromAktFallback attempts to settle overdrawn payments using AKT funds
// when BME is halted due to low collateral ratio and oracle is available.
// It transfers uakt directly to the payment owner (provider).
func (k *keeper) settleFromAktFallback(ctx sdk.Context, acc *account, payments []payment) error {
	// Check AKT funds exist
	var aktFunds *etypes.Balance
	for i := range acc.State.Funds {
		if acc.State.Funds[i].Denom == sdkutil.DenomUakt {
			aktFunds = &acc.State.Funds[i]
			break
		}
	}
	if aktFunds == nil || !aktFunds.Amount.IsPositive() {
		return nil
	}

	// Check BME halt status
	status, err := k.bmeKeeper.GetMintStatus(ctx)
	if err != nil || status < bmetypes.MintStatusHaltCR {
		return nil
	}

	// Check oracle availability
	aktPrice, err := k.oracleKeeper.GetAggregatedPrice(ctx, sdkutil.DenomUakt)
	if err != nil || !aktPrice.IsPositive() {
		return nil
	}

	for idx := range payments {
		if payments[idx].State.State != etypes.StateOverdrawn {
			continue
		}
		unsettled := payments[idx].State.Unsettled.Amount
		if unsettled.IsZero() {
			continue
		}

		// Convert uact unsettled → uakt needed: unsettled_uact / akt_price
		uaktNeeded := unsettled.Quo(aktPrice)
		aktCoin := sdk.NewDecCoinFromDec(sdkutil.DenomUakt, uaktNeeded)

		// Deduct from AKT funds/deposits
		settled, od := acc.deductFromBalance(aktCoin)
		if settled.Amount.IsZero() {
			continue
		}

		// AKT fallback doesn't carry debt - reset to zero if overdrawn
		if od {
			aktFunds.Amount = sdkmath.LegacyZeroDec()
		}

		// Transfer settled uakt directly to payment owner (provider)
		owner, err := k.ac.StringToBytes(payments[idx].State.Owner)
		if err != nil {
			return err
		}

		uaktTransfer := sdk.NewCoin(sdkutil.DenomUakt, settled.Amount.TruncateInt())
		if uaktTransfer.IsPositive() {
			if err := k.bkeeper.SendCoinsFromModuleToAccount(ctx, module.ModuleName, owner, sdk.NewCoins(uaktTransfer)); err != nil {
				return err
			}
		}

		// Clear the equivalent uact unsettled amount
		settledUact := settled.Amount.Mul(aktPrice)
		payments[idx].State.Unsettled.Amount.SubMut(settledUact)

		if !payments[idx].State.Unsettled.Amount.IsPositive() {
			payments[idx].State.Unsettled.Amount = sdkmath.LegacyZeroDec()
			payments[idx].State.State = etypes.StateOpen
		}
		payments[idx].dirty = true

		if od {
			break // AKT funds exhausted
		}
	}

	return nil
}

func (k *keeper) PaymentCreate(ctx sdk.Context, id escrowid.Payment, owner sdk.AccAddress, rate sdk.DecCoin) error {
	acc, err := k.getAccount(ctx, id.Account())
	if err != nil {
		return err
	}

	_, od, err := k.accountSettle(ctx, acc)
	if err != nil {
		return err
	}

	if od {
		return module.ErrAccountOverdrawn
	}

	var funds *etypes.Balance
	for i := range acc.State.Funds {
		if rate.Denom == acc.State.Funds[i].Denom {
			funds = &acc.State.Funds[i]
			break
		}
	}

	if funds == nil {
		return module.ErrInvalidDenomination
	}

	if rate.IsZero() {
		return module.ErrPaymentRateZero
	}

	key := k.findPayment(ctx, &id)
	if len(key) != 0 {
		return module.ErrPaymentExists
	}

	if acc.dirty {
		err = k.saveAccount(ctx, acc)
		if err != nil {
			return err
		}
	}

	err = k.savePayment(ctx, payment{
		Payment: etypes.Payment{
			ID: id,
			State: etypes.PaymentState{
				Owner:     owner.String(),
				State:     etypes.StateOpen,
				Rate:      rate,
				Balance:   sdk.NewDecCoin(rate.Denom, sdkmath.ZeroInt()),
				Unsettled: sdk.NewDecCoin(rate.Denom, sdkmath.ZeroInt()),
				Withdrawn: sdk.NewCoin(rate.Denom, sdkmath.ZeroInt()),
			}},
		dirty:     false,
		prevState: etypes.StateOpen,
	})

	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) PaymentWithdraw(ctx sdk.Context, id escrowid.Payment) error {
	acc, err := k.getAccount(ctx, id.Account())
	if err != nil {
		return err
	}

	pmnt, err := k.getPayment(ctx, id)
	if err != nil {
		return err
	}

	if pmnt.State.State != etypes.StateOpen {
		return module.ErrPaymentClosed
	}

	payments, od, err := k.accountSettle(ctx, acc)
	if err != nil {
		return err
	}

	if !od {
		pmnt = nil

		for i, p := range payments {
			if p.ID.Key() == id.Key() {
				pmnt = &payments[i]
			}
		}

		if pmnt == nil {
			panic(fmt.Sprintf("couldn't find payment %s", id.String()))
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

func (k *keeper) PaymentClose(ctx sdk.Context, id escrowid.Payment) error {
	acc, err := k.getAccount(ctx, id.Account())
	if err != nil {
		return err
	}

	pmnt, err := k.getPayment(ctx, id)
	if err != nil {
		return err
	}

	switch pmnt.State.State {
	case etypes.StateOpen:
	case etypes.StateOverdrawn:
		// if payment is overdrawn try to settle it
		// if settling fails it s still triggers deployment close
	case etypes.StateClosed:
		fallthrough
	default:
		return module.ErrPaymentClosed
	}

	payments, _, err := k.accountSettle(ctx, acc)
	if err != nil {
		return err
	}

	acc.dirty = true

	for idx := range payments {
		payments[idx].dirty = true

		if payments[idx].ID.String() == pmnt.ID.String() {
			if payments[idx].State.State == etypes.StateOpen {
				payments[idx].State.State = etypes.StateClosed
			}

			if err := k.paymentWithdraw(ctx, &payments[idx]); err != nil {
				return err
			}
		}
	}

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

func (k *keeper) GetAccount(ctx sdk.Context, id escrowid.Account) (etypes.Account, error) {
	obj, err := k.getAccount(ctx, id)
	if err != nil {
		return etypes.Account{}, err
	}

	return obj.Account, nil
}

func (k *keeper) getAccount(ctx sdk.Context, id escrowid.Account) (*account, error) {
	store := ctx.KVStore(k.skey)

	key := k.findAccount(ctx, &id)
	if len(key) == 0 {
		return &account{}, module.ErrAccountNotFound
	}

	buf := store.Get(key)

	obj := &account{
		Account: etypes.Account{
			ID: id,
		},
	}

	k.cdc.MustUnmarshal(buf, &obj.State)
	obj.prevState = obj.State.State

	return obj, nil
}

func (k *keeper) GetPayment(ctx sdk.Context, id escrowid.Payment) (etypes.Payment, error) {
	obj, err := k.getPayment(ctx, id)
	if err != nil {
		return etypes.Payment{}, err
	}

	return obj.Payment, nil
}

func (k *keeper) getPayment(ctx sdk.Context, id escrowid.Payment) (*payment, error) {
	store := ctx.KVStore(k.skey)

	key := k.findPayment(ctx, &id)
	if len(key) == 0 {
		return nil, module.ErrPaymentNotFound
	}

	buf := store.Get(key)

	obj := payment{
		Payment: etypes.Payment{
			ID: id,
		},
	}

	k.cdc.MustUnmarshal(buf, &obj.State)
	obj.prevState = obj.State.State

	return &obj, nil
}

func (k *keeper) SaveAccount(ctx sdk.Context, obj etypes.Account) error {
	err := k.saveAccount(ctx, &account{
		Account:   obj,
		prevState: obj.State.State,
	})

	if err != nil {
		return err
	}

	return nil
}

func (k *keeper) SavePayment(ctx sdk.Context, obj etypes.Payment) error {
	return k.savePayment(ctx, payment{
		Payment:   obj,
		prevState: obj.State.State,
	})
}

// SaveAccountRaw writes an account to the store without triggering hooks or
// returning funds. Used during denom migration where we need to update account
// data in-place without side effects.
func (k *keeper) SaveAccountRaw(ctx sdk.Context, obj etypes.Account) error {
	store := ctx.KVStore(k.skey)
	key := BuildAccountsKey(obj.State.State, &obj.ID)
	store.Set(key, k.cdc.MustMarshal(&obj.State))
	return nil
}

// SavePaymentRaw writes a payment to the store without triggering hooks.
// Used during denom migration where we need to update payment data in-place
// without side effects.
func (k *keeper) SavePaymentRaw(ctx sdk.Context, obj etypes.Payment) error {
	store := ctx.KVStore(k.skey)
	key := BuildPaymentsKey(obj.State.State, &obj.ID)
	store.Set(key, k.cdc.MustMarshal(&obj.State))
	return nil
}

// GetAccountPayments returns payments for a specific escrow account filtered
// by the given states. Public wrapper around accountPayments.
func (k *keeper) GetAccountPayments(ctx sdk.Context, id escrowid.Account, states []etypes.State) []etypes.Payment {
	internal := k.accountPayments(ctx, id, states)
	result := make([]etypes.Payment, len(internal))
	for i, p := range internal {
		result[i] = p.Payment
	}
	return result
}

func (k *keeper) WithAccounts(ctx sdk.Context, fn func(etypes.Account) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, AccountPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id, _, err := ParseAccountKey(iter.Key())
		if err != nil {
			panic(err)
		}
		val := etypes.Account{
			ID: id,
		}

		k.cdc.MustUnmarshal(iter.Value(), &val.State)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) WithPayments(ctx sdk.Context, fn func(etypes.Payment) bool) {
	store := ctx.KVStore(k.skey)
	iter := storetypes.KVStorePrefixIterator(store, PaymentPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		id, _, err := ParsePaymentKey(iter.Key())
		if err != nil {
			panic(err)
		}
		val := etypes.Payment{
			ID: id,
		}
		k.cdc.MustUnmarshal(iter.Value(), &val.State)
		if stop := fn(val); stop {
			break
		}
	}
}

func (k *keeper) saveAccount(ctx sdk.Context, obj *account) error {
	store := ctx.KVStore(k.skey)

	var key []byte
	if obj.State.State != obj.prevState {
		key := BuildAccountsKey(obj.prevState, &obj.ID)
		store.Delete(key)
	}

	key = BuildAccountsKey(obj.State.State, &obj.ID)

	if obj.State.State == etypes.StateClosed || obj.State.State == etypes.StateOverdrawn {
		for _, d := range obj.State.Deposits {
			if d.Balance.IsPositive() {
				depositor, err := k.ac.StringToBytes(d.Owner)
				if err != nil {
					return err
				}

				// withdrawal is the amount to withdraw in the current denom (uact for BME deposits)
				withdrawal := sdk.NewCoin(d.Balance.Denom, d.Balance.Amount.TruncateInt())
				fundsToSubtract := d.Balance.Amount

				err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, module.ModuleName, depositor, sdk.NewCoins(withdrawal))
				if err != nil {
					return err
				}

				// if depositor is not an owner then funds came from the grant.
				if d.Source == deposit.SourceGrant {
					owner, err := k.ac.StringToBytes(obj.State.Owner)
					if err != nil {
						return err
					}

					// if exists, increase allowed authz deposit by remainder in the Balance; it will allow an owner to reuse active authz
					// without asking for renewal.
					msgTypeUrl := (&ev1.DepositAuthorization{}).MsgTypeURL()

					authorization, expiration := k.authzKeeper.GetAuthorization(ctx, owner, depositor, msgTypeUrl)
					dauthz, valid := authorization.(*ev1.DepositAuthorization)
					if valid && authorization != nil {
						if dauthz.SpendLimit.Amount.GT(sdkmath.ZeroInt()) {
							dauthz.SpendLimits = dauthz.SpendLimits.Add(dauthz.SpendLimit)
							dauthz.SpendLimit.Amount = sdkmath.ZeroInt()
						}

						dauthz.SpendLimits = dauthz.SpendLimits.Add(withdrawal)
						err = k.authzKeeper.SaveGrant(ctx, owner, depositor, dauthz, expiration)
						if err != nil {
							return err
						}
					}
				}

				// Subtract from funds using the original balance amount (in funds denom)
				// Find the correct funds entry by denom
				for i := range obj.State.Funds {
					if obj.State.Funds[i].Denom == d.Balance.Denom {
						obj.State.Funds[i].Amount.SubMut(fundsToSubtract)
						break
					}
				}
			}
		}

		obj.State.Deposits = []etypes.Depositor{}
	}

	store.Set(key, k.cdc.MustMarshal(&obj.State))

	if obj.State.State == etypes.StateClosed || obj.State.State == etypes.StateOverdrawn {
		// call hooks
		for _, hook := range k.hooks.onAccountClosed {
			err := hook(ctx, obj.Account)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (k *keeper) savePayment(ctx sdk.Context, obj payment) error {
	store := ctx.KVStore(k.skey)

	var key []byte
	if obj.State.State != obj.prevState {
		key := BuildPaymentsKey(obj.prevState, &obj.ID)
		store.Delete(key)
	}

	key = BuildPaymentsKey(obj.State.State, &obj.ID)
	store.Set(key, k.cdc.MustMarshal(&obj.State))

	if obj.State.State == etypes.StateClosed || obj.State.State == etypes.StateOverdrawn {
		// call hooks
		for _, hook := range k.hooks.onPaymentClosed {
			err := hook(ctx, obj.Payment)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (k *keeper) save(ctx sdk.Context, acc *account, payments []payment) error {
	if acc.dirty {
		err := k.saveAccount(ctx, acc)
		if err != nil {
			return err
		}
	}

	for _, pmnt := range payments {
		if pmnt.dirty {
			err := k.savePayment(ctx, pmnt)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (k *keeper) accountPayments(ctx sdk.Context, id escrowid.Account, states []etypes.State) []payment {
	store := ctx.KVStore(k.skey)

	iters := make([]storetypes.Iterator, 0, len(states))
	defer func() {
		for _, iter := range iters {
			_ = iter.Close()
		}
	}()

	var payments []payment

	for _, state := range states {
		prefix := BuildPaymentsKey(state, &id)
		iter := storetypes.KVStorePrefixIterator(store, prefix)
		iters = append(iters, iter)

		for ; iter.Valid(); iter.Next() {
			id, _, err := ParsePaymentKey(iter.Key())
			if err != nil {
				panic(err)
			}
			val := etypes.Payment{
				ID: id,
			}
			k.cdc.MustUnmarshal(iter.Value(), &val.State)
			payments = append(payments, payment{
				Payment:   val,
				prevState: val.State.State,
			})
		}
	}

	return payments
}

func (k *keeper) paymentWithdraw(ctx sdk.Context, obj *payment) error {
	owner, err := k.ac.StringToBytes(obj.State.Owner)
	if err != nil {
		return err
	}

	earnings := sdk.NewCoin(obj.State.Balance.Denom, obj.State.Balance.Amount.TruncateInt())

	if earnings.Amount.IsZero() {
		return nil
	}

	err = k.bkeeper.SendCoinsFromModuleToAccount(ctx, module.ModuleName, owner, sdk.NewCoins(earnings))
	if err != nil {
		return err
	}

	obj.State.Withdrawn = obj.State.Withdrawn.Add(earnings)
	obj.State.Balance = obj.State.Balance.Sub(sdk.NewDecCoinFromCoin(earnings))
	obj.dirty = true

	return nil
}

func (acc *account) deductFromBalance(amount sdk.DecCoin) (sdk.DecCoin, bool) {
	remaining := sdkmath.LegacyZeroDec()
	remaining.AddMut(amount.Amount)

	withdrew := sdkmath.LegacyZeroDec()

	var funds *etypes.Balance
	var transferred *sdk.DecCoin

	for i := range acc.State.Funds {
		if amount.Denom == acc.State.Funds[i].Denom {
			funds = &acc.State.Funds[i]
		}
	}

	for i := range acc.State.Transferred {
		if amount.Denom == acc.State.Transferred[i].Denom {
			transferred = &acc.State.Transferred[i]
		}
	}

	if funds == nil || transferred == nil {
		panic(fmt.Sprintf("unknown denom \"%s\"", amount.Denom))
	}

	for i, d := range acc.State.Deposits {
		if d.Balance.Denom != amount.Denom {
			continue
		}

		toWithdraw := sdkmath.LegacyZeroDec()

		if d.Balance.Amount.LT(remaining) {
			toWithdraw.AddMut(d.Balance.Amount)
		} else {
			toWithdraw.AddMut(remaining)
		}

		acc.State.Deposits[i].Balance.Amount.SubMut(toWithdraw)
		remaining.SubMut(toWithdraw)
		withdrew.AddMut(toWithdraw)
		transferred.Amount.AddMut(toWithdraw)

		if remaining.IsZero() {
			break
		}
	}

	// clean empty deposits
	n := 0
	for _, d := range acc.State.Deposits {
		if !d.Balance.IsZero() {
			acc.State.Deposits[n] = d
			n++
		}
	}
	acc.State.Deposits = acc.State.Deposits[:n]

	funds.Amount.SubMut(withdrew)
	res := sdk.NewDecCoinFromDec(amount.Denom, withdrew)

	if remaining.IsZero() {
		return res, false
	}

	// at this point the account is overdrawn
	funds.Amount.SubMut(remaining)

	return res, true
}

func accountSettleFullBlocks(acc *account, payments []payment, heightDelta sdkmath.Int) bool {
	funds := &acc.State.Funds[0]

	blockRate := sdk.NewDecCoin(funds.Denom, sdkmath.ZeroInt())
	paymentsTransfers := make([]sdk.DecCoin, 0, len(payments))

	for _, pmnt := range payments {
		blockRate = blockRate.Add(pmnt.State.Rate)
	}

	for idx := range payments {
		p := &payments[idx]

		var transfer sdk.DecCoin

		if p.prevState == etypes.StateOverdrawn {
			transfer = sdk.NewDecCoinFromDec(p.State.Unsettled.Denom, sdkmath.LegacyZeroDec())
			transfer.Amount.AddMut(p.State.Unsettled.Amount)

			p.State.Unsettled.Amount = sdkmath.LegacyZeroDec()
		} else {
			transfer = sdk.NewDecCoinFromDec(p.State.Rate.Denom, p.State.Rate.Amount.Mul(sdkmath.LegacyNewDecFromInt(heightDelta)).TruncateDec())
		}
		paymentsTransfers = append(paymentsTransfers, transfer)
	}

	overdrawn := false

	for idx := range payments {
		unsettledAmount := paymentsTransfers[idx]
		settledAmount, od := acc.deductFromBalance(unsettledAmount)

		unsettledAmount.Amount.SubMut(settledAmount.Amount)

		payments[idx].dirty = true
		if settledAmount.IsPositive() {
			payments[idx].State.Balance.Amount.AddMut(settledAmount.Amount)
		}

		if od {
			overdrawn = true
			payments[idx].State.State = etypes.StateOverdrawn
			payments[idx].State.Unsettled.Amount.AddMut(unsettledAmount.Amount)
		} else {
			payments[idx].State.State = etypes.StateOpen
		}
	}

	acc.dirty = true

	return overdrawn
}

func (k *keeper) findAccount(ctx sdk.Context, id escrowid.ID) []byte {
	store := ctx.KVStore(k.skey)

	okey := BuildAccountsKey(etypes.StateOpen, id)
	ckey := BuildAccountsKey(etypes.StateClosed, id)
	ovkey := BuildAccountsKey(etypes.StateOverdrawn, id)

	var key []byte

	//nolint: gocritic
	if store.Has(okey) {
		key = okey
	} else if store.Has(ckey) {
		key = ckey
	} else if store.Has(ovkey) {
		key = ovkey
	}

	return key
}

func (k *keeper) findPayment(ctx sdk.Context, id escrowid.ID) []byte {
	store := ctx.KVStore(k.skey)

	okey := BuildPaymentsKey(etypes.StateOpen, id)
	ckey := BuildPaymentsKey(etypes.StateClosed, id)
	ovkey := BuildPaymentsKey(etypes.StateOverdrawn, id)

	var key []byte

	//nolint: gocritic
	if store.Has(okey) {
		key = okey
	} else if store.Has(ckey) {
		key = ckey
	} else if store.Has(ovkey) {
		key = ovkey
	}

	return key
}

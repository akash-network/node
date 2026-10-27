package keeper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	types "pkg.akt.dev/go/node/sink/v1"
)

type AccountKeeper interface {
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI
	GetModuleAccount(context.Context, string) sdk.ModuleAccountI
	SetAccount(context.Context, sdk.AccountI)
}

type BankKeeper interface {
	SendCoinsFromAccountToModule(context.Context, sdk.AccAddress, string, sdk.Coins) error
	SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error
	GetAllBalances(context.Context, sdk.AccAddress) sdk.Coins
}

type Keeper struct {
	cdc       codec.BinaryCodec
	accounts  AccountKeeper
	bank      BankKeeper
	authority string

	schema      collections.Schema
	params      collections.Item[types.Params]
	totals      collections.Item[types.Totals]
	transfers   collections.Map[uint64, []byte]
	voids       collections.Map[uint64, []byte]
	settlements collections.Map[uint64, types.Settlement]
	usage       collections.Map[sdk.AccAddress, types.AccountUsage]
	// open indexes transfers that are neither minted nor refunded.
	open collections.KeySet[uint64]
	// refunds indexes refundable transfers by challenge end, then ID.
	refunds collections.KeySet[collections.Pair[uint64, uint64]]
}

func NewKeeper(cdc codec.BinaryCodec, key *storetypes.KVStoreKey, accounts AccountKeeper, bank BankKeeper, authority string) Keeper {
	sb := collections.NewSchemaBuilder(runtime.NewKVStoreService(key))
	k := Keeper{
		cdc:         cdc,
		accounts:    accounts,
		bank:        bank,
		authority:   authority,
		params:      collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		totals:      collections.NewItem(sb, TotalsKey, "totals", codec.CollValue[types.Totals](cdc)),
		transfers:   collections.NewMap(sb, TransfersKey, "transfers", collections.Uint64Key, collections.BytesValue),
		voids:       collections.NewMap(sb, VoidsKey, "voids", collections.Uint64Key, collections.BytesValue),
		settlements: collections.NewMap(sb, SettlementsKey, "settlements", collections.Uint64Key, codec.CollValue[types.Settlement](cdc)),
		usage:       collections.NewMap(sb, AccountsKey, "accounts", sdk.AccAddressKey, codec.CollValue[types.AccountUsage](cdc)),
		open:        collections.NewKeySet(sb, OpenTransfersKey, "open_transfers", collections.Uint64Key),
		refunds: collections.NewKeySet(sb, RefundQueueKey, "refund_queue",
			collections.PairKeyCodec(collections.Uint64Key, collections.Uint64Key)),
	}
	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.schema = schema
	return k
}

func (k Keeper) Schema() collections.Schema { return k.schema }

// sinkAddress is computed once: SendRestriction checks it on every bank send.
var sinkAddress = authtypes.NewModuleAddress(types.ModuleName)

func Address() sdk.AccAddress { return bytes.Clone(sinkAddress) }

// get returns the stored value, fallback when there is none, and panics on
// unreadable state, as unmarshalling raw values did before collections.
func get[K, V any](ctx context.Context, m collections.Map[K, V], key K, fallback V) (V, bool) {
	v, err := m.Get(ctx, key)
	if errors.Is(err, collections.ErrNotFound) {
		return fallback, false
	}
	if err != nil {
		panic(err)
	}
	return v, true
}

func has[K, V any](ctx context.Context, m collections.Map[K, V], key K) bool {
	ok, err := m.Has(ctx, key)
	if err != nil {
		panic(err)
	}
	return ok
}

// initialized reports whether genesis stored the params.
func (k Keeper) initialized(ctx context.Context) bool {
	ok, err := k.params.Has(ctx)
	if err != nil {
		panic(err)
	}
	return ok
}

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	p, err := k.params.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return types.DefaultParams()
	}
	if err != nil {
		panic(err)
	}
	return p
}

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	old := k.GetParams(ctx)
	// A transfer accepted under a larger max_transfer must stay refundable.
	if p.DailyRefundCap < old.DailyRefundCap {
		largest, err := k.largestOpenAmount(ctx)
		if err != nil {
			return err
		}
		if p.DailyRefundCap < largest {
			return fmt.Errorf("daily refund cap %d is below open transfer amount %d", p.DailyRefundCap, largest)
		}
	}
	if !p.SameReporterSet(old) {
		totals := k.GetTotals(ctx)
		if totals.ReporterEpoch == math.MaxUint64 {
			return fmt.Errorf("reporter epoch exhausted")
		}
		totals.ReporterEpoch++
		if err := k.setTotals(ctx, totals); err != nil {
			return err
		}
	}
	return k.params.Set(ctx, p)
}

func (k Keeper) GetTotals(ctx sdk.Context) types.Totals {
	t, err := k.totals.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return types.Totals{}
	}
	if err != nil {
		panic(err)
	}
	return t
}

func (k Keeper) setTotals(ctx sdk.Context, totals types.Totals) error {
	return k.totals.Set(ctx, totals)
}

func (k Keeper) GetAccountUsage(ctx sdk.Context, sender sdk.AccAddress) types.AccountUsage {
	u, _ := get(ctx, k.usage, sender, types.AccountUsage{Sender: sender})
	return u
}

func (k Keeper) GetTransfer(ctx sdk.Context, id uint64) (types.TransferState, bool) {
	record, ok := get(ctx, k.transfers, id, nil)
	settlement, _ := get(ctx, k.settlements, id, types.Settlement{Round: 1})
	return types.TransferState{Record: record, VoidOrdered: has(ctx, k.voids, id), Settlement: settlement}, ok
}

// Capabilities are private to this package. Ante authenticates transaction shape;
// only Sink grants bank ingress, and refund authority stays separate.
type sinkRequestContextKey struct{}
type depositContextKey struct{}
type sinkRequest struct {
	sender   string
	destType uint32
	dest     string
	denom    string
	amount   string
}
type depositAuthorization struct {
	sender sdk.AccAddress
	amount sdk.Coins
}

// SendRestriction guards every bank path into and out of escrow. It never
// creates records: the MsgSink handler owns the complete atomic deposit.
func (k Keeper) SendRestriction(goCtx context.Context, from, to sdk.AccAddress, coins sdk.Coins) (sdk.AccAddress, error) {
	if from.Equals(sinkAddress) {
		ctx := sdk.UnwrapSDKContext(goCtx)
		if refund, ok := ctx.Value(refundContextKey{}).(refundAuthorization); ok && to.Equals(refund.recipient) && coins.Equal(amountCoins(refund.amount)) {
			return to, nil
		}
		// Only PrepareInstall sets this, and only before the store holds any escrow.
		if sweep, ok := ctx.Value(sweepContextKey{}).(sweepAuthorization); ok && to.Equals(sweep.recipient) && coins.Equal(sweep.amount) &&
			!k.initialized(ctx) {
			return to, nil
		}
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink escrow cannot be sent")
	}
	if !to.Equals(sinkAddress) {
		return to, nil
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	d, ok := ctx.Value(depositContextKey{}).(depositAuthorization)
	if !ok || !from.Equals(d.sender) || !coins.Equal(d.amount) {
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink deposits require MsgSink")
	}
	return to, nil
}

// Sink validates and escrows one deposit, writes its immutable proof record and
// returns that record's ID. A local cache also protects native/test callers from
// partial bank writes; BaseApp discards the whole operation on message failure.
func (k Keeper) Sink(ctx sdk.Context, msg *types.MsgSink) (uint64, error) {
	if msg == nil {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("nil sink request")
	}
	if msg.Amount.Amount.IsNil() || msg.Amount.Denom != types.Denom || !msg.Amount.Amount.IsPositive() || !msg.Amount.Amount.IsUint64() {
		return 0, sdkerrors.ErrInvalidCoins.Wrap("sink accepts positive uint64 uakt only")
	}
	request, ok := ctx.Value(sinkRequestContextKey{}).(sinkRequest)
	if !ok || request.sender != msg.Sender || request.destType != msg.DestType || request.dest != msg.Dest ||
		request.denom != msg.Amount.Denom || request.amount != msg.Amount.Amount.String() {
		return 0, sdkerrors.ErrUnauthorized.Wrap("sink requires a sole top-level MsgSink")
	}
	if msg.DestType != 1 {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink destination type must be 1 (Solana)")
	}
	sender, err := sdk.AccAddressFromBech32(msg.Sender)
	if err != nil {
		return 0, err
	}
	p := k.GetParams(ctx)
	recipient, err := types.ParseDestination(msg.Dest, p.VaultAllowlist)
	if err != nil {
		return 0, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	amount := msg.Amount.Amount.Uint64()
	acc := k.accounts.GetAccount(ctx, sender)
	if _, module := acc.(sdk.ModuleAccountI); module || acc == nil || len(sender) != 20 || acc.GetPubKey() == nil {
		return 0, sdkerrors.ErrUnauthorized.Wrap("sink sender must be an ordinary signing account")
	}
	if !p.Enabled {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink deposits disabled")
	}
	// Zero means no minimum. A minimum keeps each deposit worth the Solana rent
	// the relayer pays to settle it.
	if amount < p.MinTransfer {
		return 0, sdkerrors.ErrInvalidRequest.Wrapf("sink amount %d uakt is below the minimum transfer of %d uakt", amount, p.MinTransfer)
	}
	if amount > p.MaxTransfer {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink per-transfer limit")
	}
	if ctx.BlockHeight() <= 0 || ctx.BlockTime().Unix() < 0 {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("invalid sink block height or time")
	}
	day := uint64(ctx.BlockTime().Unix() / 86400) //nolint:gosec // time checked nonnegative above
	u := k.GetAccountUsage(ctx, sender)
	if day < u.Day {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink clock moved backwards")
	}
	if day != u.Day {
		u.Day, u.DailyCount, u.DailyAmount = day, 0, 0
	}
	if u.DailyCount >= p.MaxDailyPerAccount || u.PendingCount >= p.MaxPendingPerAccount {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink account limit")
	}
	// Zero means no amount cap. Counting starts at zero for usage stored before
	// this field existed.
	if p.MaxDailyAmountPerAccount != 0 && (u.DailyAmount > p.MaxDailyAmountPerAccount || amount > p.MaxDailyAmountPerAccount-u.DailyAmount) {
		return 0, sdkerrors.ErrInvalidRequest.Wrapf("sink amount %d uakt exceeds the account's remaining daily amount of %d uakt",
			amount, p.MaxDailyAmountPerAccount-min(u.DailyAmount, p.MaxDailyAmountPerAccount))
	}
	totals := k.GetTotals(ctx)
	deposited, err := totals.DepositedAmount()
	if err != nil {
		return 0, err
	}
	if totals.LastTransferId == math.MaxUint64 || totals.PendingCount == math.MaxUint64 ||
		amount > math.MaxUint64-deposited {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink counter overflow")
	}
	t := types.Transfer{ID: totals.LastTransferId + 1, Sender: [20]byte(sender), Recipient: recipient, Amount: amount, Height: uint64(ctx.BlockHeight())} //nolint:gosec // height checked positive above
	record, err := t.Marshal()
	if err != nil {
		return 0, err
	}
	if has(ctx, k.transfers, t.ID) {
		return 0, sdkerrors.ErrLogic.Wrap("sink ID already exists")
	}
	cache, write := ctx.CacheContext()
	coins := amountCoins(amount)
	cache = cache.WithValue(depositContextKey{}, depositAuthorization{sender: sender, amount: coins})
	if err := k.bank.SendCoinsFromAccountToModule(cache, sender, types.ModuleName, coins); err != nil {
		return 0, err
	}
	if err := k.transfers.Set(cache, t.ID, record); err != nil {
		return 0, err
	}
	if err := k.open.Set(cache, t.ID); err != nil {
		return 0, err
	}
	u.DailyCount++
	u.PendingCount++
	// Only an uncapped account could reach uint64; a saturated total exceeds any cap.
	if amount > math.MaxUint64-u.DailyAmount {
		u.DailyAmount = math.MaxUint64
	} else {
		u.DailyAmount += amount
	}
	if err := k.usage.Set(cache, sender, u); err != nil {
		return 0, err
	}
	totals.LastTransferId = t.ID
	totals.PendingAmount += amount
	totals.PendingCount++
	if err := k.setTotals(cache, totals); err != nil {
		return 0, err
	}
	// Later bank restrictions may reject or redirect the transfer. No record
	// may survive unless its exact escrow backing arrived in the module.
	if err := k.CheckBalance(cache); err != nil {
		return 0, err
	}
	cache.EventManager().EmitEvent(sdk.NewEvent("sink_deposit",
		sdk.NewAttribute("transfer_id", strconv.FormatUint(t.ID, 10)),
		sdk.NewAttribute("sender", sender.String()), sdk.NewAttribute("amount", strconv.FormatUint(amount, 10)),
		sdk.NewAttribute("recipient", msg.Dest), sdk.NewAttribute("height", strconv.FormatUint(t.Height, 10))))
	write()
	return t.ID, nil
}

// RequestVoid commits a governance order for proof on Solana. It does not
// establish that Solana executed the void and cannot release any escrow.
func (k Keeper) RequestVoid(ctx sdk.Context, authority string, id uint64) error {
	if authority != k.authority {
		return sdkerrors.ErrUnauthorized.Wrap("sink authority must be governance")
	}
	entry, ok := k.GetTransfer(ctx, id)
	if !ok {
		return sdkerrors.ErrNotFound.Wrap("sink transfer")
	}
	// A Minted transfer under a conflict hold may still be voided: a false mint
	// quorum can close a transfer that Solana voided (ResolveTransfer).
	s := entry.Settlement
	if s.Status == types.Status_STATUS_REFUNDED || (s.Status == types.Status_STATUS_MINTED && s.Hold != types.Hold_HOLD_CONFLICT) {
		return sdkerrors.ErrInvalidRequest.Wrap("sink transfer already closed")
	}
	if err := k.voids.Set(ctx, id, types.VoidValue(id)); err != nil {
		return err
	}
	ctx.EventManager().EmitEvent(sdk.NewEvent("sink_void_order", sdk.NewAttribute("transfer_id", strconv.FormatUint(id, 10))))
	return nil
}

func (k Keeper) CheckBalance(ctx sdk.Context) error {
	t := k.GetTotals(ctx)
	escrow, err := t.EscrowAmount()
	if err != nil {
		return err
	}
	expected := amountCoins(escrow)
	if !k.bank.GetAllBalances(ctx, Address()).Equal(expected) {
		return fmt.Errorf("sink bank balance does not match pending, minted and voided escrow")
	}
	return nil
}

func (k Keeper) InitGenesis(ctx sdk.Context, g types.GenesisState) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if k.initialized(ctx) {
		return fmt.Errorf("sink already initialized")
	}
	// Day counters from the future would stop deposits or refunds until then.
	today := uint64(max(ctx.BlockTime().Unix(), 0) / 86400) //nolint:gosec // nonnegative
	if g.Totals.RefundDay > today {
		return fmt.Errorf("refund day is in the future")
	}
	for _, a := range g.Accounts {
		if a.Day > today {
			return fmt.Errorf("account usage day is in the future")
		}
	}
	cache, write := ctx.CacheContext()
	k.accounts.GetModuleAccount(cache, types.ModuleName)
	if err := k.setTotals(cache, g.Totals); err != nil {
		return err
	}
	for _, e := range g.Transfers {
		t, _ := types.DecodeTransfer(e.Record) // Validate checked every record.
		if err := k.transfers.Set(cache, t.ID, e.Record); err != nil {
			return err
		}
		if err := k.putSettlement(cache, t.ID, types.Settlement{}, e.Settlement); err != nil {
			return err
		}
		if !e.Settlement.Terminal() {
			if err := k.open.Set(cache, t.ID); err != nil {
				return err
			}
		}
		if e.VoidOrdered {
			if err := k.voids.Set(cache, t.ID, types.VoidValue(t.ID)); err != nil {
				return err
			}
		}
	}
	for _, a := range g.Accounts {
		if err := k.usage.Set(cache, a.Sender, a); err != nil {
			return err
		}
	}
	if err := k.CheckBalance(cache); err != nil {
		return err
	}
	// Genesis validates the complete configuration and must preserve an
	// exported reporter epoch, rather than treating import as a rotation.
	if err := k.params.Set(cache, g.Params); err != nil {
		return err
	}
	write()
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) types.GenesisState {
	g := types.GenesisState{Params: k.GetParams(ctx), Totals: k.GetTotals(ctx)}
	err := k.transfers.Walk(ctx, nil, func(id uint64, record []byte) (bool, error) {
		t, err := types.DecodeTransfer(record)
		if err != nil {
			return true, err
		}
		if t.ID != id {
			return true, fmt.Errorf("sink transfer %d is stored under ID %d", t.ID, id)
		}
		entry, _ := k.GetTransfer(ctx, id)
		entry.Record = bytes.Clone(record)
		g.Transfers = append(g.Transfers, entry)
		return false, nil
	})
	if err != nil {
		panic(err)
	}
	err = k.usage.Walk(ctx, nil, func(_ sdk.AccAddress, u types.AccountUsage) (bool, error) {
		g.Accounts = append(g.Accounts, u)
		return false, nil
	})
	if err != nil {
		panic(err)
	}
	return g
}

const (
	DefaultOpenTransfersLimit uint32 = 100
	MaxOpenTransfersLimit     uint32 = 500
)

// OpenTransfers returns up to limit transfers that are neither minted nor
// refunded, with IDs above startAfter, in ascending ID order. next is the last
// returned ID when more remain, otherwise zero. Zero limit means the default;
// larger limits are capped.
func (k Keeper) OpenTransfers(ctx sdk.Context, startAfter uint64, limit uint32) (transfers []types.TransferState, next uint64, err error) {
	if limit == 0 {
		limit = DefaultOpenTransfersLimit
	}
	limit = min(limit, MaxOpenTransfersLimit)
	if startAfter == math.MaxUint64 {
		return nil, 0, nil
	}
	it, err := k.open.Iterate(ctx, new(collections.Range[uint64]).StartInclusive(startAfter+1))
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = it.Close() }()
	var last uint64
	for ; it.Valid(); it.Next() {
		id, err := it.Key()
		if err != nil {
			return nil, 0, fmt.Errorf("malformed open sink transfer key: %w", err)
		}
		if uint32(len(transfers)) == limit { //nolint:gosec // len(transfers) <= limit
			return transfers, last, nil
		}
		entry, ok := k.GetTransfer(ctx, id)
		if !ok || entry.Settlement.Terminal() {
			return nil, 0, fmt.Errorf("open sink transfer index is inconsistent at %d", id)
		}
		transfers = append(transfers, entry)
		last = id
	}
	return transfers, 0, nil
}

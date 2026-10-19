package keeper

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
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
	key       storetypes.StoreKey
	accounts  AccountKeeper
	bank      BankKeeper
	authority string
}

func NewKeeper(cdc codec.BinaryCodec, key storetypes.StoreKey, accounts AccountKeeper, bank BankKeeper, authority string) Keeper {
	return Keeper{cdc: cdc, key: key, accounts: accounts, bank: bank, authority: authority}
}

var (
	paramsKey  = []byte("params")
	totalsKey  = []byte("totals")
	openPrefix = []byte("open/")
	openValue  = []byte{1}
)

func Address() sdk.AccAddress                 { return authtypes.NewModuleAddress(types.ModuleName) }
func accountKey(sender sdk.AccAddress) []byte { return append([]byte("account/"), sender...) }

// openKey indexes a transfer that is neither minted nor refunded. The
// big-endian ID makes store order ascending ID order. The index is derived:
// InitGenesis rebuilds it from settlements and export omits it.
func openKey(id uint64) []byte { return binary.BigEndian.AppendUint64(bytes.Clone(openPrefix), id) }

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	b := ctx.KVStore(k.key).Get(paramsKey)
	if b == nil {
		return types.DefaultParams()
	}
	var p types.Params
	k.cdc.MustUnmarshal(b, &p)
	return p
}

func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.Enabled && k.GetTotals(ctx).LaunchReserve != types.LaunchReserve {
		return fmt.Errorf("launch reserve must be funded before enabling deposits")
	}
	if !p.SameReporterSet(k.GetParams(ctx)) {
		totals := k.GetTotals(ctx)
		if totals.ReporterEpoch == math.MaxUint64 {
			return fmt.Errorf("reporter epoch exhausted")
		}
		totals.ReporterEpoch++
		k.setTotals(ctx, totals)
	}
	ctx.KVStore(k.key).Set(paramsKey, k.cdc.MustMarshal(&p))
	return nil
}

func (k Keeper) GetTotals(ctx sdk.Context) types.Totals {
	var t types.Totals
	if b := ctx.KVStore(k.key).Get(totalsKey); b != nil {
		k.cdc.MustUnmarshal(b, &t)
	}
	return t
}

func (k Keeper) setTotals(ctx sdk.Context, totals types.Totals) {
	ctx.KVStore(k.key).Set(totalsKey, k.cdc.MustMarshal(&totals))
}

func (k Keeper) GetAccountUsage(ctx sdk.Context, sender sdk.AccAddress) types.AccountUsage {
	u := types.AccountUsage{Sender: sender}
	if b := ctx.KVStore(k.key).Get(accountKey(sender)); b != nil {
		k.cdc.MustUnmarshal(b, &u)
	}
	return u
}

func (k Keeper) GetTransfer(ctx sdk.Context, id uint64) (types.TransferState, bool) {
	s := ctx.KVStore(k.key)
	b := s.Get(types.TransferKey(id))
	settlement := types.Settlement{Round: 1}
	if value := s.Get(types.SettlementKey(id)); value != nil {
		k.cdc.MustUnmarshal(value, &settlement)
	}
	return types.TransferState{Record: b, VoidOrdered: s.Has(types.VoidKey(id)), Settlement: settlement}, b != nil
}

// Capabilities are private to this package. Ante authenticates transaction shape;
// only Sink grants bank ingress, and reserve/refund authority stays separate.
type sinkRequestContextKey struct{}
type depositContextKey struct{}
type reserveContextKey struct{}
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
	if from.Equals(Address()) {
		ctx := sdk.UnwrapSDKContext(goCtx)
		if refund, ok := ctx.Value(refundContextKey{}).(refundAuthorization); ok && to.Equals(refund.recipient) && coins.Equal(amountCoins(refund.amount)) {
			return to, nil
		}
		// Only PrepareInstall sets this, and only before the store holds any escrow.
		if sweep, ok := ctx.Value(sweepContextKey{}).(sweepAuthorization); ok && to.Equals(sweep.recipient) && coins.Equal(sweep.amount) &&
			!ctx.KVStore(k.key).Has(paramsKey) {
			return to, nil
		}
		return nil, sdkerrors.ErrUnauthorized.Wrap("sink escrow cannot be sent")
	}
	if !to.Equals(Address()) {
		return to, nil
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	if source, ok := ctx.Value(reserveContextKey{}).(sdk.AccAddress); ok {
		if !from.Equals(source) || !coins.Equal(reserveCoins()) || k.GetTotals(ctx).LaunchReserve != 0 {
			return nil, sdkerrors.ErrUnauthorized.Wrap("invalid reserve funding")
		}
		return to, nil
	}
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
	recipient, err := types.ParseDestination(msg.Dest, k.GetParams(ctx).VaultAllowlist)
	if err != nil {
		return 0, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	amount := msg.Amount.Amount.Uint64()
	acc := k.accounts.GetAccount(ctx, sender)
	if _, module := acc.(sdk.ModuleAccountI); module || acc == nil || len(sender) != 20 || acc.GetPubKey() == nil {
		return 0, sdkerrors.ErrUnauthorized.Wrap("sink sender must be an ordinary signing account")
	}
	p := k.GetParams(ctx)
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
		deposited > math.MaxUint64-types.LaunchReserve || amount > math.MaxUint64-types.LaunchReserve-deposited {
		return 0, sdkerrors.ErrInvalidRequest.Wrap("sink counter overflow")
	}
	t := types.Transfer{ID: totals.LastTransferId + 1, Sender: [20]byte(sender), Recipient: recipient, Amount: amount, Height: uint64(ctx.BlockHeight())} //nolint:gosec // height checked positive above
	record, err := t.Marshal()
	if err != nil {
		return 0, err
	}
	s := ctx.KVStore(k.key)
	if s.Has(types.TransferKey(t.ID)) {
		return 0, sdkerrors.ErrLogic.Wrap("sink ID already exists")
	}
	cache, write := ctx.CacheContext()
	coins := amountCoins(amount)
	cache = cache.WithValue(depositContextKey{}, depositAuthorization{sender: sender, amount: coins})
	if err := k.bank.SendCoinsFromAccountToModule(cache, sender, types.ModuleName, coins); err != nil {
		return 0, err
	}
	s = cache.KVStore(k.key)
	s.Set(types.TransferKey(t.ID), record)
	s.Set(openKey(t.ID), openValue)
	u.DailyCount++
	u.PendingCount++
	// Only an uncapped account could reach uint64; a saturated total exceeds any cap.
	if amount > math.MaxUint64-u.DailyAmount {
		u.DailyAmount = math.MaxUint64
	} else {
		u.DailyAmount += amount
	}
	s.Set(accountKey(sender), k.cdc.MustMarshal(&u))
	totals.LastTransferId = t.ID
	totals.PendingAmount += amount
	totals.PendingCount++
	k.setTotals(cache, totals)
	// Later bank restrictions may reject or redirect the transfer. No record
	// may survive unless its exact escrow backing arrived in the module.
	if err := k.CheckBalance(cache); err != nil {
		return 0, err
	}
	cache.EventManager().EmitEvent(sdk.NewEvent("sink_deposit",
		sdk.NewAttribute("transfer_id", strconv.FormatUint(t.ID, 10)),
		sdk.NewAttribute("sender", sender.String()), sdk.NewAttribute("amount", strconv.FormatUint(amount, 10))))
	write()
	return t.ID, nil
}

func reserveCoins() sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(types.Denom, sdkmath.NewIntFromUint64(types.LaunchReserve)))
}

// FundLaunchReserve is for native genesis/upgrade code only; it is not exposed
// as a transaction. Insufficient backing fails atomically, never enables deposits.
func (k Keeper) FundLaunchReserve(ctx sdk.Context, source sdk.AccAddress) error {
	if len(source) != 20 || source.Equals(Address()) {
		return fmt.Errorf("invalid reserve source")
	}
	if _, module := k.accounts.GetAccount(ctx, source).(sdk.ModuleAccountI); module {
		return fmt.Errorf("reserve source must not be a module")
	}
	totals := k.GetTotals(ctx)
	if totals.LaunchReserve != 0 {
		return fmt.Errorf("launch reserve already funded")
	}
	cache, write := ctx.CacheContext()
	cache = cache.WithValue(reserveContextKey{}, source)
	if err := k.bank.SendCoinsFromAccountToModule(cache, source, types.ModuleName, reserveCoins()); err != nil {
		return err
	}
	totals.LaunchReserve = types.LaunchReserve
	k.setTotals(cache, totals)
	write()
	return nil
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
	if entry.Settlement.Terminal() {
		return sdkerrors.ErrInvalidRequest.Wrap("sink transfer already closed")
	}
	ctx.KVStore(k.key).Set(types.VoidKey(id), types.VoidValue(id))
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
		return fmt.Errorf("sink bank balance does not match reserve plus pending, minted and voided escrow")
	}
	return nil
}

func (k Keeper) InitGenesis(ctx sdk.Context, g types.GenesisState) error {
	if err := g.Validate(); err != nil {
		return err
	}
	if ctx.KVStore(k.key).Has(paramsKey) {
		return fmt.Errorf("sink already initialized")
	}
	cache, write := ctx.CacheContext()
	k.accounts.GetModuleAccount(cache, types.ModuleName)
	k.setTotals(cache, g.Totals)
	for _, e := range g.Transfers {
		t, _ := types.DecodeTransfer(e.Record) // Validate checked every record.
		cache.KVStore(k.key).Set(types.TransferKey(t.ID), e.Record)
		k.setSettlement(cache, t.ID, e.Settlement)
		if !e.Settlement.Terminal() {
			cache.KVStore(k.key).Set(openKey(t.ID), openValue)
		}
		if e.VoidOrdered {
			cache.KVStore(k.key).Set(types.VoidKey(t.ID), types.VoidValue(t.ID))
		}
	}
	for _, a := range g.Accounts {
		cache.KVStore(k.key).Set(accountKey(a.Sender), k.cdc.MustMarshal(&a))
	}
	if err := k.CheckBalance(cache); err != nil {
		return err
	}
	if g.LaunchReserveSource != "" {
		source, _ := sdk.AccAddressFromBech32(g.LaunchReserveSource)
		if err := k.FundLaunchReserve(cache, source); err != nil {
			return err
		}
	}
	// Genesis validates the complete configuration and must preserve an
	// exported reporter epoch, rather than treating import as a rotation.
	cache.KVStore(k.key).Set(paramsKey, k.cdc.MustMarshal(&g.Params))
	write()
	return nil
}

func (k Keeper) ExportGenesis(ctx sdk.Context) types.GenesisState {
	g := types.GenesisState{Params: k.GetParams(ctx), Totals: k.GetTotals(ctx)}
	s := ctx.KVStore(k.key)
	it := storetypes.KVStorePrefixIterator(s, []byte("transfer/"))
	for ; it.Valid(); it.Next() {
		t, err := types.DecodeTransfer(it.Value())
		if err != nil {
			panic(err)
		}
		entry, _ := k.GetTransfer(ctx, t.ID)
		entry.Record = bytes.Clone(it.Value())
		g.Transfers = append(g.Transfers, entry)
	}
	_ = it.Close()
	it = storetypes.KVStorePrefixIterator(s, []byte("account/"))
	defer func() { _ = it.Close() }()
	for ; it.Valid(); it.Next() {
		var a types.AccountUsage
		k.cdc.MustUnmarshal(it.Value(), &a)
		g.Accounts = append(g.Accounts, a)
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
	it := ctx.KVStore(k.key).Iterator(openKey(startAfter+1), storetypes.PrefixEndBytes(openPrefix))
	defer func() { _ = it.Close() }()
	var last uint64
	for ; it.Valid(); it.Next() {
		if len(it.Key()) != len(openPrefix)+8 {
			return nil, 0, fmt.Errorf("malformed open sink transfer key")
		}
		id := binary.BigEndian.Uint64(it.Key()[len(openPrefix):])
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

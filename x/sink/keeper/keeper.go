package keeper

import (
	"bytes"
	"context"
	"errors"
	"math"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/corecompat"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
	skey      *storetypes.KVStoreKey
	accounts  AccountKeeper
	bank      BankKeeper
	authority string

	schema      collections.Schema
	params      collections.Item[types.Params]
	totals      collections.Item[types.Totals]
	policies    collections.Map[uint64, types.ReporterPolicy]
	sendUsage   collections.Item[types.EpochUsage]
	refundUsage collections.Item[types.EpochUsage]
	transfers   collections.Map[types.ID, types.RecordData]
	voids       collections.Map[types.ID, types.VoidOrder]
	settlements collections.Map[types.ID, types.SettlementData]
	usage       collections.Map[sdk.AccAddress, types.AccountUsage]
	byStatus    collections.KeySet[types.SettlementID]
	history     collections.KeySet[types.ID]
	disputes    collections.KeySet[types.ID]
	refunds     collections.KeySet[collections.Triple[uint64, string, types.ID]]
	sequence    collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	key *storetypes.KVStoreKey,
	tkey *storetypes.TransientStoreKey,
	accounts AccountKeeper,
	bank BankKeeper,
	authority string,
) Keeper {
	sb := collections.NewSchemaBuilder(runtime.NewKVStoreService(key))
	tsvc := runtime.NewTransientStoreService(tkey)
	tsb := collections.NewSchemaBuilderFromAccessor(func(ctx context.Context) corecompat.KVStore {
		return tsvc.OpenTransientStore(ctx)
	})

	k := Keeper{
		skey:        key,
		accounts:    accounts,
		bank:        bank,
		authority:   authority,
		params:      collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		totals:      collections.NewItem(sb, TotalsKey, "totals", codec.CollValue[types.Totals](cdc)),
		policies:    collections.NewMap(sb, PoliciesKey, "policies", collections.Uint64Key, codec.CollValue[types.ReporterPolicy](cdc)),
		sendUsage:   collections.NewItem(sb, SendUsageKey, "send_usage", codec.CollValue[types.EpochUsage](cdc)),
		refundUsage: collections.NewItem(sb, RefundUsageKey, "refund_usage", codec.CollValue[types.EpochUsage](cdc)),
		transfers:   collections.NewMap(sb, TransfersKey, "transfers", types.IDKey, types.RecordDataValue),
		voids:       collections.NewMap(sb, VoidsKey, "void_orders", types.IDKey, types.VoidOrderValue),
		settlements: collections.NewMap(sb, SettlementsKey, "settlements", types.IDKey, codec.CollValue[types.SettlementData](cdc)),
		usage:       collections.NewMap(sb, AccountsKey, "accounts", sdk.AccAddressKey, codec.CollValue[types.AccountUsage](cdc)),
		byStatus:    collections.NewKeySet(sb, StatusKey, "by_status", SettlementIDKey),
		history:     collections.NewKeySet(sb, HistoryKey, "history", chronoIDKey),
		disputes:    collections.NewKeySet(sb, DisputesKey, "disputes", chronoIDKey),
		refunds:     collections.NewKeySet(sb, RefundQueueKey, "refund_queue", collections.TripleKeyCodec(collections.Uint64Key, collections.StringKey, types.IDKey)),
		sequence:    collections.NewSequence(tsb, SequenceKey, "sequence"),
	}

	var err error
	k.schema, err = sb.Build()
	if err != nil {
		panic(err)
	}

	if _, err = tsb.Build(); err != nil {
		panic(err)
	}

	return k
}

func (k Keeper) Schema() collections.Schema { return k.schema }

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

var sinkAddress = authtypes.NewModuleAddress(types.ModuleName)

func Address() sdk.AccAddress {
	return bytes.Clone(sinkAddress)
}

func (k Keeper) initialized(ctx context.Context) (bool, error) {
	return k.params.Has(ctx)
}

func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	return k.params.Get(ctx)
}

func (k Keeper) GetTotals(ctx context.Context) (types.Totals, error) {
	return k.totals.Get(ctx)
}

func (k Keeper) GetAccountUsage(ctx context.Context, owner sdk.AccAddress) (types.AccountUsage, error) {
	u, err := k.usage.Get(ctx, owner)
	if errors.Is(err, collections.ErrNotFound) {
		return types.AccountUsage{TotalSent: types.ZeroAmount(), EpochSent: types.ZeroAmount()}, nil
	}

	return u, err
}

func (k Keeper) GetTransfer(ctx context.Context, id types.ID) (types.TransferState, error) {
	data, err := k.transfers.Get(ctx, id)
	if errors.Is(err, collections.ErrNotFound) {
		return types.TransferState{}, types.ErrTransferNotFound
	}
	if err != nil {
		return types.TransferState{}, err
	}

	s, err := k.settlements.Get(ctx, id)
	if errors.Is(err, collections.ErrNotFound) {
		return types.TransferState{}, types.ErrInternal.Wrapf("missing settlement for %s", id)
	}
	if err != nil {
		return types.TransferState{}, err
	}

	_, err = k.voids.Get(ctx, id)
	ordered := err == nil

	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return types.TransferState{}, err
	}

	return types.TransferState{ID: id, Data: data, Settlement: s, VoidOrdered: ordered}, nil
}

func (k Keeper) currentPolicy(ctx context.Context) (types.ReporterPolicy, error) {
	it, err := k.policies.Iterate(ctx, new(collections.Range[uint64]).Descending())
	if err != nil {
		return types.ReporterPolicy{}, err
	}

	if !it.Valid() {
		return types.ReporterPolicy{}, errors.Join(types.ErrInternal.Wrap("missing reporter policy"), it.Close())
	}

	policy, err := it.Value()
	return policy, errors.Join(err, it.Close())
}

func (k Keeper) referencedPolicies(ctx context.Context, s types.SettlementData) (map[uint64]types.ReporterPolicy, error) {
	policies := make(map[uint64]types.ReporterPolicy, 1)
	versions := append(make([]uint64, 0, 1+len(s.Votes)), s.PolicyVersion)
	for _, vote := range s.Votes {
		versions = append(versions, vote.PolicyVersion)
	}

	for _, version := range versions {
		if _, loaded := policies[version]; loaded {
			continue
		}

		policy, err := k.policies.Get(ctx, version)
		if errors.Is(err, collections.ErrNotFound) {
			return nil, types.ErrInvalidSettlement.Wrapf("missing policy version %d", version)
		}
		if err != nil {
			return nil, err
		}

		policies[version] = policy
	}

	return policies, nil
}

func (k Keeper) allPolicies(ctx context.Context) ([]types.ReporterPolicy, error) {
	var result []types.ReporterPolicy
	err := k.policies.Walk(ctx, nil, func(_ uint64, p types.ReporterPolicy) (bool, error) {
		result = append(result, p)
		return false, nil
	})
	return result, err
}

func (k Keeper) BeginBlocker(ctx context.Context) error {
	return k.sequence.Set(ctx, 0)
}

func (k Keeper) nextID(ctx sdk.Context, owner sdk.AccAddress) (types.ID, error) {
	n, err := k.sequence.Peek(ctx)
	if err != nil {
		return types.ID{}, err
	}

	if n == math.MaxUint64 {
		return types.ID{}, types.ErrOverflow.Wrap("per-block sequence")
	}

	id := types.ID{Owner: owner.String(), Height: ctx.BlockHeight(), Sequence: n + 1}
	if err = id.ValidateBasic(); err != nil {
		return types.ID{}, err
	}

	if err = k.sequence.Set(ctx, n+1); err != nil {
		return types.ID{}, err
	}

	return id, nil
}

func (k Keeper) CheckBalance(ctx sdk.Context) error {
	t, err := k.totals.Get(ctx)
	if err != nil {
		return err
	}

	if err = t.Validate(); err != nil {
		return err
	}

	escrow, err := t.EscrowAmount()
	if err != nil {
		return err
	}

	expected := sdk.NewCoins(escrow)
	if !k.bank.GetAllBalances(ctx, Address()).Equal(expected) {
		return types.ErrBalanceMismatch
	}

	return nil
}

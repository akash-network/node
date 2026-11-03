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

	epochstypes "pkg.akt.dev/go/node/epochs/v1beta1"
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

type EpochKeeper interface {
	GetEpoch(sdk.Context, string) (epochstypes.EpochInfo, error)
}

type Keeper interface {
	Schema() collections.Schema
	NewQuerier() Querier
	BeginBlocker(context.Context) error

	GetParams(context.Context) (types.Params, error)
	GetTotals(context.Context) (types.Totals, error)
	GetAccountUsage(context.Context, sdk.AccAddress) (types.AccountUsage, error)
	GetTransfer(context.Context, types.ID) (types.TransferState, error)

	Deposit(sdk.Context, *types.MsgSink) (types.ID, error)
	UpdateParams(sdk.Context, *types.MsgUpdateParams) error
	GuardianPause(sdk.Context, *types.MsgGuardianPause) error
	RequestVoid(sdk.Context, *types.MsgRequestVoid) error
	ReportOutcome(sdk.Context, *types.MsgReportOutcome) error
	VetoVoid(sdk.Context, *types.MsgVetoVoid) error
	Resolve(sdk.Context, types.Resolution) error
	FinalizeRefund(sdk.Context, *types.MsgFinalizeRefund) error

	CheckBalance(sdk.Context) error
	PrepareInstall(sdk.Context, Distribution) (sdk.Coins, error)
	InitGenesis(sdk.Context, *types.GenesisState) error
	ExportGenesis(sdk.Context) (*types.GenesisState, error)
}

type keeper struct {
	skey      *storetypes.KVStoreKey
	accounts  AccountKeeper
	bank      BankKeeper
	epochs    EpochKeeper
	authority string

	schema               collections.Schema
	params               collections.Item[types.Params]
	totals               collections.Item[types.Totals]
	policies             collections.Map[uint64, types.ReporterPolicy]
	currentPolicyVersion collections.Item[uint64]
	sendUsage            collections.Item[types.EpochUsage]
	refundUsage          collections.Item[types.EpochUsage]
	transfers            collections.Map[types.ID, types.RecordData]
	settlements          collections.Map[types.SettlementID, types.SettlementData]
	usage                collections.Map[sdk.AccAddress, types.AccountUsage]
	history              collections.KeySet[types.ID]
	disputes             collections.KeySet[types.ID]
	refunds              collections.KeySet[collections.Triple[uint64, int64, types.ID]]
	sequence             collections.Sequence
}

func NewKeeper(
	cdc codec.BinaryCodec,
	key *storetypes.KVStoreKey,
	tkey *storetypes.TransientStoreKey,
	accounts AccountKeeper,
	bank BankKeeper,
	epochs EpochKeeper,
	authority string,
) Keeper {
	sb := collections.NewSchemaBuilder(runtime.NewKVStoreService(key))
	tsvc := runtime.NewTransientStoreService(tkey)
	tsb := collections.NewSchemaBuilderFromAccessor(func(ctx context.Context) corecompat.KVStore {
		return tsvc.OpenTransientStore(ctx)
	})

	k := &keeper{
		skey:                 key,
		accounts:             accounts,
		bank:                 bank,
		epochs:               epochs,
		authority:            authority,
		params:               collections.NewItem(sb, ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		totals:               collections.NewItem(sb, TotalsKey, "totals", codec.CollValue[types.Totals](cdc)),
		policies:             collections.NewMap(sb, PoliciesKey, "policies", collections.Uint64Key, codec.CollValue[types.ReporterPolicy](cdc)),
		currentPolicyVersion: collections.NewItem(sb, CurrentPolicyKey, "current_policy_version", collections.Uint64Value),
		sendUsage:            collections.NewItem(sb, SendUsageKey, "send_usage", codec.CollValue[types.EpochUsage](cdc)),
		refundUsage:          collections.NewItem(sb, RefundUsageKey, "refund_usage", codec.CollValue[types.EpochUsage](cdc)),
		transfers:            collections.NewMap(sb, TransfersKey, "transfers", types.IDKey, codec.CollValue[types.RecordData](cdc)),
		settlements:          collections.NewMap(sb, SettlementsKey, "settlements", types.SettlementIDKey, codec.CollValue[types.SettlementData](cdc)),
		usage:                collections.NewMap(sb, AccountsKey, "accounts", sdk.AccAddressKey, codec.CollValue[types.AccountUsage](cdc)),
		history:              collections.NewKeySet(sb, HistoryKey, "history", chronoIDKey),
		disputes:             collections.NewKeySet(sb, DisputesKey, "disputes", chronoIDKey),
		refunds:              collections.NewKeySet(sb, RefundQueueKey, "refund_queue", collections.TripleKeyCodec(collections.Uint64Key, collections.Int64Key, types.IDKey)),
		sequence:             collections.NewSequence(tsb, SequenceKey, "sequence"),
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

func (k *keeper) Schema() collections.Schema { return k.schema }

func (k *keeper) NewQuerier() Querier {
	return Querier{k}
}

var sinkAddress = authtypes.NewModuleAddress(types.ModuleName)

func Address() sdk.AccAddress {
	return bytes.Clone(sinkAddress)
}

func (k *keeper) initialized(ctx context.Context) (bool, error) {
	return k.params.Has(ctx)
}

func (k *keeper) GetParams(ctx context.Context) (types.Params, error) {
	return k.params.Get(ctx)
}

func (k *keeper) GetTotals(ctx context.Context) (types.Totals, error) {
	return k.totals.Get(ctx)
}

func (k *keeper) GetAccountUsage(ctx context.Context, owner sdk.AccAddress) (types.AccountUsage, error) {
	u, err := k.usage.Get(ctx, owner)
	if errors.Is(err, collections.ErrNotFound) {
		return types.AccountUsage{
			TotalSent: types.ZeroAmount(),
			EpochSent: types.ZeroAmount(),
		}, nil
	}

	return u, err
}

func (k *keeper) GetTransfer(ctx context.Context, id types.ID) (types.TransferState, error) {
	data, err := k.transfers.Get(ctx, id)
	if errors.Is(err, collections.ErrNotFound) {
		return types.TransferState{}, types.ErrTransferNotFound
	}
	if err != nil {
		return types.TransferState{}, err
	}

	var settlement types.SettlementData
	status := types.StatusInvalid
	for candidate := types.StatusTransferPending; candidate <= types.StatusRefunded; candidate++ {
		value, err := k.settlements.Get(ctx, types.NewSettlementID(candidate, id))
		if errors.Is(err, collections.ErrNotFound) {
			continue
		}
		if err != nil {
			return types.TransferState{}, err
		}

		if status != types.StatusInvalid {
			return types.TransferState{}, types.ErrInternal.Wrapf("multiple canonical settlements for %s", id)
		}

		status, settlement = candidate, value
	}

	if status == types.StatusInvalid {
		return types.TransferState{}, types.ErrInternal.Wrapf("missing canonical settlement for %s", id)
	}

	return types.TransferState{
		ID:         id,
		Data:       data,
		Settlement: settlement,
		Status:     status,
	}, nil
}

func (k *keeper) currentPolicy(ctx context.Context) (types.ReporterPolicy, error) {
	version, err := k.currentPolicyVersion.Get(ctx)
	if err != nil {
		return types.ReporterPolicy{}, err
	}

	policy, err := k.policies.Get(ctx, version)
	if err != nil {
		return types.ReporterPolicy{}, err
	}

	if version != policy.Version {
		return types.ReporterPolicy{}, types.ErrInternal.Wrap("policy key mismatch")
	}

	return policy, nil
}

func (k *keeper) referencedPolicies(ctx context.Context, s types.SettlementData) (map[uint64]types.ReporterPolicy, error) {
	policies := make(map[uint64]types.ReporterPolicy, 1)
	versions := append(make([]uint64, 0, 1+len(s.Votes)), s.Policy.Version)
	if s.VetoPolicyVersion != 0 {
		versions = append(versions, s.VetoPolicyVersion)
	}

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

		if policy.Version != version {
			return nil, types.ErrInternal.Wrap("policy key mismatch")
		}

		policies[version] = policy
	}

	return policies, nil
}

func (k *keeper) allPolicies(ctx context.Context) ([]types.ReporterPolicy, error) {
	var result []types.ReporterPolicy
	err := k.policies.Walk(ctx, nil, func(_ uint64, p types.ReporterPolicy) (bool, error) {
		result = append(result, p)

		return false, nil
	})

	return result, err
}

func (k *keeper) BeginBlocker(ctx context.Context) error {
	return k.sequence.Set(ctx, 0)
}

func (k *keeper) nextID(ctx sdk.Context, owner sdk.AccAddress) (types.ID, error) {
	n, err := k.sequence.Peek(ctx)
	if err != nil {
		return types.ID{}, err
	}

	if n == math.MaxUint64 {
		return types.ID{}, types.ErrOverflow.Wrap("per-block sequence")
	}

	id := types.ID{
		Owner:    owner.String(),
		Height:   ctx.BlockHeight(),
		Sequence: n + 1,
	}
	if err = id.ValidateBasic(); err != nil {
		return types.ID{}, err
	}

	if err = k.sequence.Set(ctx, n+1); err != nil {
		return types.ID{}, err
	}

	return id, nil
}

func (k *keeper) CheckBalance(ctx sdk.Context) error {
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

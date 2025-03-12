package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/errors"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
)

type IKeeper interface {
	StoreKey() sdk.StoreKey
	Codec() codec.BinaryCodec
	GetDeployment(ctx sdk.Context, id types.DeploymentID) (types.Deployment, bool)
	GetGroup(ctx sdk.Context, id types.GroupID) (types.Group, bool)
	GetGroups(ctx sdk.Context, id types.DeploymentID) []types.Group
	Create(ctx sdk.Context, deployment types.Deployment, groups []types.Group) error
	UpdateDeployment(ctx sdk.Context, deployment types.Deployment) error
	CloseDeployment(ctx sdk.Context, deployment types.Deployment)
	OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error
	OnPauseGroup(ctx sdk.Context, group types.Group) error
	OnStartGroup(ctx sdk.Context, group types.Group) error
	WithDeployments(ctx sdk.Context, fn func(types.Deployment) bool)
	OnBidClosed(ctx sdk.Context, id types.GroupID) error
	OnLeaseClosed(ctx sdk.Context, id types.GroupID) (types.Group, error)
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
	NewQuerier() Querier
}

// Keeper of the deployment store
type Keeper struct {
	skey    sdk.StoreKey
	cdc     codec.BinaryCodec
	pspace  paramtypes.Subspace
	ekeeper EscrowKeeper
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc codec.BinaryCodec, skey sdk.StoreKey, pspace paramtypes.Subspace, ekeeper EscrowKeeper) IKeeper {
	if !pspace.HasKeyTable() {
		pspace = pspace.WithKeyTable(types.ParamKeyTable())
	}

	return Keeper{
		skey:    skey,
		cdc:     cdc,
		pspace:  pspace,
		ekeeper: ekeeper,
	}
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

func (k Keeper) StoreKey() sdk.StoreKey {
	return k.skey
}

// GetDeployment returns deployment details with provided DeploymentID
func (k Keeper) GetDeployment(ctx sdk.Context, id types.DeploymentID) (types.Deployment, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findDeployment(ctx, id)

	if len(key) == 0 {
		return types.Deployment{}, false
	}

	buf := store.Get(key)

	var val types.Deployment

	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

// GetGroup returns group details with given GroupID from deployment store
func (k Keeper) GetGroup(ctx sdk.Context, id types.GroupID) (types.Group, bool) {
	store := ctx.KVStore(k.skey)

	key := k.findGroup(ctx, id)

	if len(key) == 0 {
		return types.Group{}, false
	}

	buf := store.Get(key)

	var val types.Group

	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

// GetGroups returns all groups of a deployment with given DeploymentID from deployment store
func (k Keeper) GetGroups(ctx sdk.Context, id types.DeploymentID) []types.Group {
	store := ctx.KVStore(k.skey)
	keys := [][]byte{
		MustGroupsKey(GroupStateOpenPrefix, id),
		MustGroupsKey(GroupStatePausedPrefix, id),
		MustGroupsKey(GroupStateInsufficientFundsPrefix, id),
		MustGroupsKey(GroupStateClosedPrefix, id),
	}

	var vals []types.Group

	iters := make([]sdk.Iterator, 0, len(keys))

	defer func() {
		for _, iter := range iters {
			_ = iter.Close()
		}
	}()

	for _, key := range keys {
		iter := sdk.KVStorePrefixIterator(store, key)
		iters = append(iters, iter)

		for ; iter.Valid(); iter.Next() {
			var val types.Group
			k.cdc.MustUnmarshal(iter.Value(), &val)
			vals = append(vals, val)
		}
	}

	return vals
}

// Create creates a new deployment with given deployment and group specifications
func (k Keeper) Create(ctx sdk.Context, deployment types.Deployment, groups []types.Group) error {
	store := ctx.KVStore(k.skey)

	key := k.findDeployment(ctx, deployment.ID())

	if len(key) != 0 {
		return types.ErrDeploymentExists
	}

	key = MustDeploymentKey(DeploymentStateToPrefix(deployment.State), deployment.ID())

	store.Set(key, k.cdc.MustMarshal(&deployment))

	for idx := range groups {
		group := groups[idx]

		if !group.ID().DeploymentID().Equals(deployment.ID()) {
			return types.ErrInvalidGroupID
		}

		gkey, err := GroupKey(GroupStateToPrefix(group.State), group.ID())
		if err != nil {
			return errors.Wrap(err, "failed to create group key")
		}

		store.Set(gkey, k.cdc.MustMarshal(&group))
	}

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentCreated(deployment.ID(), deployment.Version).
			ToSDKEvent(),
	)

	telemetry.IncrCounter(1.0, "akash.deployment_created")

	return nil
}

// UpdateDeployment updates deployment details
func (k Keeper) UpdateDeployment(ctx sdk.Context, deployment types.Deployment) error {
	store := ctx.KVStore(k.skey)

	key := k.findDeployment(ctx, deployment.ID())

	if len(key) == 0 {
		return types.ErrDeploymentNotFound
	}

	key = MustDeploymentKey(DeploymentStateToPrefix(deployment.State), deployment.ID())
	store.Set(key, k.cdc.MustMarshal(&deployment))

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentUpdated(deployment.ID(), deployment.Version).
			ToSDKEvent(),
	)

	return nil
}

// CloseDeployment updates deployment details
func (k Keeper) CloseDeployment(ctx sdk.Context, deployment types.Deployment) {
	if deployment.State == types.DeploymentClosed {
		return
	}

	store := ctx.KVStore(k.skey)
	key := k.findDeployment(ctx, deployment.ID())

	if len(key) == 0 {
		return
	}

	store.Delete(key)

	deployment.State = types.DeploymentClosed

	key = MustDeploymentKey(DeploymentStateToPrefix(deployment.State), deployment.DeploymentID)

	store.Set(key, k.cdc.MustMarshal(&deployment))

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentClosed(deployment.ID()).
			ToSDKEvent(),
	)

}

// OnCloseGroup provides shutdown API for a Group
func (k Keeper) OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error {
	store := ctx.KVStore(k.skey)

	key := k.findGroup(ctx, group.ID())
	if len(key) == 0 {
		return types.ErrGroupNotFound
	}

	store.Delete(key)

	group.State = state

	key, err := GroupKey(GroupStateToPrefix(group.State), group.ID())
	if err != nil {
		return errors.Wrap(err, "failed to encode group key")
	}

	store.Set(key, k.cdc.MustMarshal(&group))

	ctx.EventManager().EmitEvent(
		types.NewEventGroupClosed(group.ID()).
			ToSDKEvent(),
	)

	return nil
}

// OnPauseGroup provides shutdown API for a Group
func (k Keeper) OnPauseGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)

	key := k.findGroup(ctx, group.ID())
	if len(key) == 0 {
		return types.ErrGroupNotFound
	}

	store.Delete(key)

	group.State = types.GroupPaused
	store.Set(key, k.cdc.MustMarshal(&group))

	ctx.EventManager().EmitEvent(
		types.NewEventGroupPaused(group.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshal(&group))
	return nil
}

// OnStartGroup provides shutdown API for a Group
func (k Keeper) OnStartGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)

	key := k.findGroup(ctx, group.ID())
	if len(key) == 0 {
		return types.ErrGroupNotFound
	}

	store.Delete(key)

	group.State = types.GroupOpen
	key, err := GroupKey(GroupStateToPrefix(group.State), group.ID())
	if err != nil {
		return errors.Wrap(err, "failed to encode group key")
	}

	store.Set(key, k.cdc.MustMarshal(&group))

	ctx.EventManager().EmitEvent(
		types.NewEventGroupStarted(group.ID()).
			ToSDKEvent(),
	)

	return nil
}

// WithDeployments iterates all deployments in deployment store
func (k Keeper) WithDeployments(ctx sdk.Context, fn func(types.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, DeploymentPrefix)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Deployment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// OnBidClosed sets the group to state paused.
func (k Keeper) OnBidClosed(ctx sdk.Context, id types.GroupID) error {
	group, ok := k.GetGroup(ctx, id)
	if !ok {
		return types.ErrGroupNotFound
	}
	return k.OnPauseGroup(ctx, group)
}

// OnLeaseClosed keeps the group at state open
func (k Keeper) OnLeaseClosed(ctx sdk.Context, id types.GroupID) (types.Group, error) {
	group, ok := k.GetGroup(ctx, id)
	if !ok {
		return types.Group{}, types.ErrGroupNotFound
	}
	return group, nil
}

// GetParams returns the total set of deployment parameters.
func (k Keeper) GetParams(ctx sdk.Context) (params types.Params) {
	k.pspace.GetParamSet(ctx, &params)
	return params
}

// SetParams sets the deployment parameters to the paramspace.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) {
	k.pspace.SetParamSet(ctx, &params)
}

func (k Keeper) findDeployment(ctx sdk.Context, id types.DeploymentID) []byte {
	store := ctx.KVStore(k.skey)

	aKey := MustDeploymentKey(DeploymentStateActivePrefix, id)
	cKey := MustDeploymentKey(DeploymentStateClosedPrefix, id)

	var key []byte

	if store.Has(aKey) {
		key = aKey
	} else if store.Has(cKey) {
		key = cKey
	}

	return key
}

func (k Keeper) findGroup(ctx sdk.Context, id types.GroupID) []byte {
	store := ctx.KVStore(k.skey)

	oKey := MustGroupKey(GroupStateOpenPrefix, id)
	pKey := MustGroupKey(GroupStatePausedPrefix, id)
	iKey := MustGroupKey(GroupStateInsufficientFundsPrefix, id)
	cKey := MustGroupKey(GroupStateClosedPrefix, id)

	var key []byte

	// nolint: gocritic
	if store.Has(oKey) {
		key = oKey
	} else if store.Has(pKey) {
		key = pKey
	} else if store.Has(iKey) {
		key = iKey
	} else if store.Has(cKey) {
		key = cKey
	}

	return key
}

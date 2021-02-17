package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/ovrclk/akash/x/deployment/types"
)

type IKeeper interface {
	Codec() codec.BinaryMarshaler
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
	updateDeployment(ctx sdk.Context, obj types.Deployment)

	NewQuerier() Querier
}

// Keeper of the deployment store
type Keeper struct {
	skey    sdk.StoreKey
	cdc     codec.BinaryMarshaler
	pspace  paramtypes.Subspace
	ekeeper EscrowKeeper
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc codec.BinaryMarshaler, skey sdk.StoreKey, pspace paramtypes.Subspace, ekeeper EscrowKeeper) IKeeper {

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
func (k Keeper) Codec() codec.BinaryMarshaler {
	return k.cdc
}

// GetDeployment returns deployment details with provided DeploymentID
func (k Keeper) GetDeployment(ctx sdk.Context, id types.DeploymentID) (types.Deployment, bool) {
	store := ctx.KVStore(k.skey)

	key := deploymentKey(id)

	if !store.Has(key) {
		return types.Deployment{}, false
	}

	buf := store.Get(key)

	var val types.Deployment

	k.cdc.MustUnmarshalBinaryBare(buf, &val)

	return val, true
}

// GetGroup returns group details with given GroupID from deployment store
func (k Keeper) GetGroup(ctx sdk.Context, id types.GroupID) (types.Group, bool) {
	store := ctx.KVStore(k.skey)

	key := groupKey(id)

	if !store.Has(key) {
		return types.Group{}, false
	}

	buf := store.Get(key)

	var val types.Group

	k.cdc.MustUnmarshalBinaryBare(buf, &val)

	return val, true
}

// GetGroups returns all groups of a deployment with given DeploymentID from deployment store
func (k Keeper) GetGroups(ctx sdk.Context, id types.DeploymentID) []types.Group {
	store := ctx.KVStore(k.skey)
	key := groupsKey(id)

	var vals []types.Group

	iter := sdk.KVStorePrefixIterator(store, key)

	for ; iter.Valid(); iter.Next() {
		var val types.Group
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		vals = append(vals, val)
	}

	iter.Close()
	return vals
}

// Create creates a new deployment with given deployment and group specifications
func (k Keeper) Create(ctx sdk.Context, deployment types.Deployment, groups []types.Group) error {
	store := ctx.KVStore(k.skey)

	key := deploymentKey(deployment.ID())

	if store.Has(key) {
		return types.ErrDeploymentExists
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(&deployment))

	for idx := range groups {
		group := groups[idx]

		if !group.ID().DeploymentID().Equals(deployment.ID()) {
			return types.ErrInvalidGroupID
		}
		gkey := groupKey(group.ID())
		store.Set(gkey, k.cdc.MustMarshalBinaryBare(&group))
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
	key := deploymentKey(deployment.ID())

	if !store.Has(key) {
		return types.ErrDeploymentNotFound
	}

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentUpdated(deployment.ID(), deployment.Version).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(&deployment))
	return nil
}

// UpdateDeployment updates deployment details
func (k Keeper) CloseDeployment(ctx sdk.Context, deployment types.Deployment) {
	if deployment.State == types.DeploymentClosed {
		return
	}

	store := ctx.KVStore(k.skey)
	key := deploymentKey(deployment.ID())

	if !store.Has(key) {
		return
	}

	deployment.State = types.DeploymentClosed
	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentClosed(deployment.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(&deployment))
}

// OnCloseGroup provides shutdown API for a Group
func (k Keeper) OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	if !store.Has(key) {
		return types.ErrGroupNotFound
	}
	group.State = state

	ctx.EventManager().EmitEvent(
		types.NewEventGroupClosed(group.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(&group))
	return nil
}

// OnPauseGroup provides shutdown API for a Group
func (k Keeper) OnPauseGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	if !store.Has(key) {
		return types.ErrGroupNotFound
	}
	group.State = types.GroupPaused

	ctx.EventManager().EmitEvent(
		types.NewEventGroupPaused(group.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(&group))
	return nil
}

// OnStartGroup provides shutdown API for a Group
func (k Keeper) OnStartGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	if !store.Has(key) {
		return types.ErrGroupNotFound
	}
	group.State = types.GroupOpen

	ctx.EventManager().EmitEvent(
		types.NewEventGroupStarted(group.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(&group))
	return nil
}

// WithDeployments iterates all deployments in deployment store
func (k Keeper) WithDeployments(ctx sdk.Context, fn func(types.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, deploymentPrefix)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var val types.Deployment
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
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

func (k Keeper) updateDeployment(ctx sdk.Context, obj types.Deployment) {
	store := ctx.KVStore(k.skey)
	key := deploymentKey(obj.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(&obj))
}

func (k Keeper) updateGroup(ctx sdk.Context, group types.Group) {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	store.Set(key, k.cdc.MustMarshalBinaryBare(&group))
}

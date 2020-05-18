package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/pkg/errors"

	"github.com/ovrclk/akash/x/deployment/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	ErrDeploymentAlreadyExists = errors.New("keeper: deployment already exists")
	ErrDeploymentNotFound      = errors.New("keeper: deployment not found")
)

// Keeper of the deployment store
type Keeper struct {
	skey sdk.StoreKey
	cdc  *codec.Codec
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc *codec.Codec, skey sdk.StoreKey) Keeper {
	return Keeper{
		skey: skey,
		cdc:  cdc,
	}
}

// Codec returns keeper codec
func (k Keeper) Codec() *codec.Codec {
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
		return ErrDeploymentAlreadyExists
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))

	for _, group := range groups {
		gkey := groupKey(group.ID())
		store.Set(gkey, k.cdc.MustMarshalBinaryBare(group))
	}

	ctx.EventManager().EmitEvent(
		types.EventDeploymentCreate{ID: deployment.ID()}.ToSDKEvent(),
	)

	return nil
}

// UpdateDeployment updates deployment details
func (k Keeper) UpdateDeployment(ctx sdk.Context, deployment types.Deployment) error {
	store := ctx.KVStore(k.skey)
	key := deploymentKey(deployment.ID())

	if !store.Has(key) {
		return ErrDeploymentNotFound
	}

	ctx.EventManager().EmitEvent(
		types.EventDeploymentUpdate{ID: deployment.ID()}.ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))
	return nil
}

// WithDeployments iterates all deployments in deployment store
func (k Keeper) WithDeployments(ctx sdk.Context, fn func(types.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, deploymentPrefix)
	for ; iter.Valid(); iter.Next() {
		var val types.Deployment
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// OnOrderCreated updates group state to group ordered
func (k Keeper) OnOrderCreated(ctx sdk.Context, group types.Group) {
	// TODO: assert state transition
	group.State = types.GroupOrdered
	k.updateGroup(ctx, group)
}

// OnLeaseCreated updates group state to group matched
func (k Keeper) OnLeaseCreated(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupMatched
	k.updateGroup(ctx, group)
}

// OnLeaseInsufficientFunds updates group state to group insufficient funds
func (k Keeper) OnLeaseInsufficientFunds(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupInsufficientFunds
	k.updateGroup(ctx, group)
}

// OnLeaseClosed updates group state to group opened
func (k Keeper) OnLeaseClosed(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupOpen
	k.updateGroup(ctx, group)
}

// OnDeploymentClosed updates group state to group closed
func (k Keeper) OnDeploymentClosed(ctx sdk.Context, group types.Group) {
	if group.State == types.GroupClosed {
		return
	}
	group.State = types.GroupClosed
	k.updateGroup(ctx, group)
}

func (k Keeper) updateGroup(ctx sdk.Context, group types.Group) {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())
	store.Set(key, k.cdc.MustMarshalBinaryBare(group))
}

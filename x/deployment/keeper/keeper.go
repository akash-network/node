package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/x/deployment/types"
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

// getOpenGroups returns all *open* groups of a deployment with given DeploymentID.
func (k Keeper) getOpenGroups(ctx sdk.Context, id types.DeploymentID) []types.Group {
	store := ctx.KVStore(k.skey)
	key := groupsOpenKey(id)

	var vals []types.Group
	iter := sdk.KVStorePrefixIterator(store, key)
	for ; iter.Valid(); iter.Next() {
		gkey, _ := groupOpenKeyConvert(iter.Key())

		buf := store.Get(gkey)
		var val types.Group
		k.cdc.MustUnmarshalBinaryBare(buf, &val)
		if err := val.ValidateOrderable(); err == nil {
			vals = append(vals, val)
		}
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

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))

	for _, group := range groups {
		if !group.ID().DeploymentID().Equals(deployment.ID()) {
			return types.ErrInvalidGroupID
		}
		gkey := groupKey(group.ID())
		store.Set(gkey, k.cdc.MustMarshalBinaryBare(group))
		k.updateOpenGroupsIndex(ctx, group)
	}

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentCreated(deployment.ID()).
			ToSDKEvent(),
	)

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
		types.NewEventDeploymentUpdated(deployment.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))
	return nil
}

// OnCloseGroup provides shutdown API for a Group
func (k Keeper) OnCloseGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	if !store.Has(key) {
		return types.ErrGroupNotFound
	}
	group.State = types.GroupClosed

	ctx.EventManager().EmitEvent(
		types.NewEventGroupClosed(group.ID()).
			ToSDKEvent(),
	)

	store.Set(key, k.cdc.MustMarshalBinaryBare(group))
	k.updateOpenGroupsIndex(ctx, group)
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

// WithDeploymentsActive filters to only those with State: Active
func (k Keeper) WithDeploymentsActive(ctx sdk.Context, fn func(types.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, deploymentPrefix)
	for ; iter.Valid(); iter.Next() {
		var val types.Deployment
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &val)
		if val.State != types.DeploymentActive {
			continue
		}
		if stop := fn(val); stop {
			break
		}
	}
}

// WithOpenGroups filters to only those with State: Open
func (k Keeper) WithOpenGroups(ctx sdk.Context, fn func(types.Group) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, groupOpenPrefix)
	for ; iter.Valid(); iter.Next() {
		gKey, err := groupOpenKeyConvert(iter.Key())
		if err != nil {
			continue
		}
		buf := store.Get(gKey)
		var val types.Group
		k.cdc.MustUnmarshalBinaryBare(buf, &val)
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

	ctx.EventManager().EmitEvent(
		types.NewEventDeploymentClosed(group.DeploymentID()).
			ToSDKEvent(),
	)
}

func (k Keeper) updateGroup(ctx sdk.Context, group types.Group) {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID())

	store.Set(key, k.cdc.MustMarshalBinaryBare(group))
	k.updateOpenGroupsIndex(ctx, group)
}

// updateOpenGroupsIndex wraps all calls to the index which tracks open Groups.
func (k Keeper) updateOpenGroupsIndex(ctx sdk.Context, group types.Group) {
	// Update the Open Groups prefixed index
	store := ctx.KVStore(k.skey)
	openKey := groupOpenKey(group.ID())

	switch group.State {
	case types.GroupOpen:
		store.Set(openKey, []byte{})
	default:
		store.Delete(openKey)
	}
}

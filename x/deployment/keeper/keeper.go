package keeper

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/ovrclk/akash/x/deployment/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Keeper struct {
	skey sdk.StoreKey
	cdc  *codec.Codec
}

func NewKeeper(cdc *codec.Codec, skey sdk.StoreKey) Keeper {
	return Keeper{
		skey: skey,
		cdc:  cdc,
	}
}

func (k Keeper) Codec() *codec.Codec {
	return k.cdc
}

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

func (k Keeper) Create(ctx sdk.Context, deployment types.Deployment, groups []types.Group) error {
	store := ctx.KVStore(k.skey)

	key := deploymentKey(deployment.ID())

	if store.Has(key) {
		return fmt.Errorf("deployment already exists")
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))

	for _, group := range groups {
		gkey := groupKey(group.ID())
		store.Set(gkey, k.cdc.MustMarshalBinaryBare(group))
	}
	return nil
}

func (k Keeper) UpdateDeployment(ctx sdk.Context, deployment types.Deployment) error {
	store := ctx.KVStore(k.skey)
	key := deploymentKey(deployment.ID())

	if !store.Has(key) {
		return fmt.Errorf("deployment not found")
	}

	store.Set(key, k.cdc.MustMarshalBinaryBare(deployment))
	return nil
}

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

func (k Keeper) OnOrderCreated(ctx sdk.Context, group types.Group) {
	// TODO: assert state transition
	group.State = types.GroupOrdered
	k.updateGroup(ctx, group)
}

func (k Keeper) OnLeaseCreated(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupMatched
	k.updateGroup(ctx, group)
}

func (k Keeper) OnLeaseInsufficientFunds(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupInsufficientFunds
	k.updateGroup(ctx, group)
}

func (k Keeper) OnLeaseClosed(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupOpen
	k.updateGroup(ctx, group)
}

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

package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"

	"pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"
)

type IKeeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetDeployment(ctx sdk.Context, id v1.DeploymentID) (v1.Deployment, bool)
	GetGroup(ctx sdk.Context, id v1.GroupID) (types.Group, bool)
	GetGroups(ctx sdk.Context, id v1.DeploymentID) []types.Group
	Create(ctx sdk.Context, deployment v1.Deployment, groups []types.Group) error
	UpdateDeployment(ctx sdk.Context, deployment v1.Deployment) error
	CloseDeployment(ctx sdk.Context, deployment v1.Deployment) error
	OnCloseGroup(ctx sdk.Context, group types.Group, state types.GroupState) error
	OnPauseGroup(ctx sdk.Context, group types.Group) error
	OnStartGroup(ctx sdk.Context, group types.Group) error
	WithDeployments(ctx sdk.Context, fn func(v1.Deployment) bool)
	OnBidClosed(ctx sdk.Context, id v1.GroupID) error
	OnLeaseClosed(ctx sdk.Context, id v1.GroupID) (types.Group, error)
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
	updateDeployment(ctx sdk.Context, obj v1.Deployment)

	NewQuerier() Querier
}

// Keeper of the deployment store
type Keeper struct {
	skey    storetypes.StoreKey
	cdc     codec.BinaryCodec
	pspace  paramtypes.Subspace
	ekeeper EscrowKeeper
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(cdc codec.BinaryCodec, skey storetypes.StoreKey, pspace paramtypes.Subspace, ekeeper EscrowKeeper) IKeeper {
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

func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetDeployment returns deployment details with provided DeploymentID
func (k Keeper) GetDeployment(ctx sdk.Context, id v1.DeploymentID) (v1.Deployment, bool) {
	store := ctx.KVStore(k.skey)

	key := deploymentKey(id)

	if !store.Has(key) {
		return v1.Deployment{}, false
	}

	buf := store.Get(key)

	var val v1.Deployment

	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

// GetGroup returns group details with given GroupID from deployment store
func (k Keeper) GetGroup(ctx sdk.Context, id v1.GroupID) (types.Group, bool) {
	store := ctx.KVStore(k.skey)

	key := groupKey(id)

	if !store.Has(key) {
		return types.Group{}, false
	}

	buf := store.Get(key)

	var val types.Group

	k.cdc.MustUnmarshal(buf, &val)

	return val, true
}

// GetGroups returns all groups of a deployment with given DeploymentID from deployment store
func (k Keeper) GetGroups(ctx sdk.Context, id v1.DeploymentID) []types.Group {
	store := ctx.KVStore(k.skey)
	key := groupsKey(id)

	var vals []types.Group

	iter := sdk.KVStorePrefixIterator(store, key)

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val types.Group
		k.cdc.MustUnmarshal(iter.Value(), &val)
		vals = append(vals, val)
	}

	return vals
}

// Create creates a new deployment with given deployment and group specifications
func (k Keeper) Create(ctx sdk.Context, deployment v1.Deployment, groups []types.Group) error {
	store := ctx.KVStore(k.skey)

	key := deploymentKey(deployment.ID)

	if store.Has(key) {
		return v1.ErrDeploymentExists
	}

	store.Set(key, k.cdc.MustMarshal(&deployment))

	for idx := range groups {
		group := groups[idx]

		if !group.ID.DeploymentID().Equals(deployment.ID) {
			return v1.ErrInvalidGroupID
		}
		gkey := groupKey(group.ID)
		store.Set(gkey, k.cdc.MustMarshal(&group))
	}

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventDeploymentCreated{
			ID:   deployment.ID,
			Hash: deployment.Hash,
		},
	)
	if err != nil {
		return err
	}

	telemetry.IncrCounter(1.0, "akash.deployment_created")

	return nil
}

// UpdateDeployment updates deployment details
func (k Keeper) UpdateDeployment(ctx sdk.Context, deployment v1.Deployment) error {
	store := ctx.KVStore(k.skey)
	key := deploymentKey(deployment.ID)

	if !store.Has(key) {
		return v1.ErrDeploymentNotFound
	}

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventDeploymentUpdated{
			ID:   deployment.ID,
			Hash: deployment.Hash,
		},
	)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&deployment))
	return nil
}

// CloseDeployment close deployment
func (k Keeper) CloseDeployment(ctx sdk.Context, deployment v1.Deployment) error {
	if deployment.State == v1.DeploymentClosed {
		return v1.ErrDeploymentClosed
	}

	store := ctx.KVStore(k.skey)
	key := deploymentKey(deployment.ID)

	if !store.Has(key) {
		return v1.ErrDeploymentNotFound
	}

	deployment.State = v1.DeploymentClosed

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventDeploymentClosed{
			ID: deployment.ID,
		},
	)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&deployment))

	return nil
}

// OnCloseGroup provides shutdown API for a Group
func (k Keeper) OnCloseGroup(ctx sdk.Context, group types.Group, state types.GroupState) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID)

	if !store.Has(key) {
		return v1.ErrGroupNotFound
	}
	group.State = state

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupClosed{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&group))
	return nil
}

// OnPauseGroup provides shutdown API for a Group
func (k Keeper) OnPauseGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID)

	if !store.Has(key) {
		return v1.ErrGroupNotFound
	}
	group.State = types.GroupPaused

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupPaused{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&group))
	return nil
}

// OnStartGroup provides shutdown API for a Group
func (k Keeper) OnStartGroup(ctx sdk.Context, group types.Group) error {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID)

	if !store.Has(key) {
		return v1.ErrGroupNotFound
	}
	group.State = types.GroupOpen

	err := ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupStarted{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	store.Set(key, k.cdc.MustMarshal(&group))
	return nil
}

// WithDeployments iterates all deployments in deployment store
func (k Keeper) WithDeployments(ctx sdk.Context, fn func(v1.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, v1.DeploymentPrefix())

	defer func() {
		_ = iter.Close()
	}()

	for ; iter.Valid(); iter.Next() {
		var val v1.Deployment
		k.cdc.MustUnmarshal(iter.Value(), &val)
		if stop := fn(val); stop {
			break
		}
	}
}

// OnBidClosed sets the group to state paused.
func (k Keeper) OnBidClosed(ctx sdk.Context, id v1.GroupID) error {
	group, ok := k.GetGroup(ctx, id)
	if !ok {
		return v1.ErrGroupNotFound
	}
	return k.OnPauseGroup(ctx, group)
}

// OnLeaseClosed keeps the group at state open
func (k Keeper) OnLeaseClosed(ctx sdk.Context, id v1.GroupID) (types.Group, error) {
	group, ok := k.GetGroup(ctx, id)
	if !ok {
		return types.Group{}, v1.ErrGroupNotFound
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

func (k Keeper) updateDeployment(ctx sdk.Context, obj v1.Deployment) {
	store := ctx.KVStore(k.skey)
	key := deploymentKey(obj.ID)
	store.Set(key, k.cdc.MustMarshal(&obj))
}

// nolint: unused
func (k Keeper) updateGroup(ctx sdk.Context, group types.Group) {
	store := ctx.KVStore(k.skey)
	key := groupKey(group.ID)

	store.Set(key, k.cdc.MustMarshal(&group))
}

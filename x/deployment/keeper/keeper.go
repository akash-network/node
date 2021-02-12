package keeper

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/ovrclk/akash/x/deployment/types"
	etypes "github.com/ovrclk/akash/x/escrow/types"
)

type IKeeper interface {
	Codec() codec.BinaryMarshaler
	GetDeployment(ctx sdk.Context, id types.DeploymentID) (types.Deployment, bool)
	GetGroup(ctx sdk.Context, id types.GroupID) (types.Group, bool)
	GetGroups(ctx sdk.Context, id types.DeploymentID) []types.Group
	Create(ctx sdk.Context, deployment types.Deployment, groups []types.Group) error
	UpdateDeployment(ctx sdk.Context, deployment types.Deployment) error
	OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error
	OnPauseGroup(ctx sdk.Context, group types.Group) error
	OnStartGroup(ctx sdk.Context, group types.Group) error
	WithDeployments(ctx sdk.Context, fn func(types.Deployment) bool)
	WithDeploymentsActive(ctx sdk.Context, fn func(types.Deployment) bool)
	OnOrderCreated(ctx sdk.Context, group types.Group)
	OnLeaseCreated(ctx sdk.Context, id types.GroupID)
	OnBidClosed(ctx sdk.Context, id types.GroupID)
	OnOrderClosed(ctx sdk.Context, id types.GroupID)
	OnDeploymentClosed(ctx sdk.Context, group types.Group)
	GetParams(ctx sdk.Context) (params types.Params)
	SetParams(ctx sdk.Context, params types.Params)
	OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account)
	OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.Payment)
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

// WithDeploymentsActive filters to only those with State: Active
func (k Keeper) WithDeploymentsActive(ctx sdk.Context, fn func(types.Deployment) bool) {
	store := ctx.KVStore(k.skey)
	iter := sdk.KVStorePrefixIterator(store, deploymentPrefix)
	defer iter.Close()
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

// OnOrderCreated updates group state to group ordered
func (k Keeper) OnOrderCreated(ctx sdk.Context, group types.Group) {
	// TODO: assert state transition
	group.State = types.GroupOpen
	k.updateGroup(ctx, group)
}

// OnLeaseCreated updates group state to group matched
func (k Keeper) OnLeaseCreated(ctx sdk.Context, id types.GroupID) {
	// TODO: assert state transition
	group, _ := k.GetGroup(ctx, id)
	group.State = types.GroupOpen
	k.updateGroup(ctx, group)
}

// OnBidClosed sets the group to state paused.
func (k Keeper) OnBidClosed(ctx sdk.Context, id types.GroupID) {
	group, ok := k.GetGroup(ctx, id)
	if !ok {
		return
	}

	if group.State != types.GroupOpen {
		return
	}

	ctx.EventManager().EmitEvent(
		types.NewEventGroupPaused(group.ID()).
			ToSDKEvent(),
	)

	group.State = types.GroupPaused
	k.updateGroup(ctx, group)
}

// OnOrderClosed updates group state to group opened
func (k Keeper) OnOrderClosed(ctx sdk.Context, id types.GroupID) {
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
		types.NewEventDeploymentClosed(group.ID().DeploymentID()).
			ToSDKEvent(),
	)
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

func (k Keeper) OnEscrowAccountClosed(ctx sdk.Context, obj etypes.Account) {
	id, found := types.DeploymentIDFromEscrowAccount(obj.ID)
	if !found {
		return
	}

	deployment, found := k.GetDeployment(ctx, id)
	if !found {
		return
	}

	if deployment.State != types.DeploymentActive {
		return
	}

	gstate := types.GroupClosed
	if obj.State == etypes.AccountOverdrawn {
		gstate = types.GroupInsufficientFunds
	}

	deployment.State = types.DeploymentClosed
	k.updateDeployment(ctx, deployment)
	for _, group := range k.GetGroups(ctx, deployment.ID()) {
		if group.ValidateClosable() == nil {
			_ = k.OnCloseGroup(ctx, group, gstate)
		}
	}
}

func (k Keeper) OnEscrowPaymentClosed(ctx sdk.Context, obj etypes.Payment) {
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

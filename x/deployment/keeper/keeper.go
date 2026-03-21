package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/collections/indexes"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	dimports "pkg.akt.dev/node/v2/x/deployment/imports"
	"pkg.akt.dev/node/v2/x/deployment/keeper/keys"
)

type IKeeper interface {
	StoreKey() storetypes.StoreKey
	Codec() codec.BinaryCodec
	GetDeployment(ctx sdk.Context, id v1.DeploymentID) (v1.Deployment, bool)
	GetGroup(ctx sdk.Context, id v1.GroupID) (types.Group, bool)
	GetGroups(ctx sdk.Context, id v1.DeploymentID) (types.Groups, error)
	SaveGroup(ctx sdk.Context, group types.Group) error
	Create(ctx sdk.Context, deployment v1.Deployment, groups []types.Group) error
	UpdateDeployment(ctx sdk.Context, deployment v1.Deployment) error
	CloseDeployment(ctx sdk.Context, deployment v1.Deployment) error
	OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error
	OnPauseGroup(ctx sdk.Context, group types.Group) error
	OnStartGroup(ctx sdk.Context, group types.Group) error
	WithDeployments(ctx sdk.Context, fn func(v1.Deployment) bool) error
	OnBidClosed(ctx sdk.Context, id v1.GroupID) error
	OnLeaseClosed(ctx sdk.Context, id v1.GroupID) (types.Group, error)
	GetParams(ctx sdk.Context) (types.Params, error)
	SetParams(ctx sdk.Context, params types.Params) error
	GetAuthority() string
	NewQuerier() Querier

	EndBlocker(context.Context) error

	AddPendingDenomMigration(ctx sdk.Context, did v1.DeploymentID) error
}

// Keeper of the deployment store
type Keeper struct {
	cdc          codec.BinaryCodec
	skey         storetypes.StoreKey
	ekeeper      dimports.EscrowKeeper
	oracleKeeper dimports.OracleKeeper
	marketKeeper dimports.MarketKeeper
	authzKeeper  dimports.AuthzKeeper
	bankKeeper   dimports.BankKeeper

	// The address capable of executing a MsgUpdateParams message.
	// This should be the x/gov module account.
	authority string

	schema                 collections.Schema
	deployments            *collections.IndexedMap[keys.DeploymentPrimaryKey, v1.Deployment, DeploymentIndexes]
	groups                 *collections.IndexedMap[keys.GroupPrimaryKey, types.Group, GroupIndexes]
	pendingDenomMigrations collections.Map[keys.DeploymentPrimaryKey, sdkmath.Int]
	Params                 collections.Item[types.Params]
}

// NewKeeper creates and returns an instance for deployment keeper
func NewKeeper(
	cdc codec.BinaryCodec,
	skey *storetypes.KVStoreKey,
	ekeeper dimports.EscrowKeeper,
	oracleKeeper dimports.OracleKeeper,
	marketKeeper dimports.MarketKeeper,
	authzKeeper dimports.AuthzKeeper,
	bankKeeper dimports.BankKeeper,
	authority string,
) IKeeper {
	ssvc := runtime.NewKVStoreService(skey)
	sb := collections.NewSchemaBuilder(ssvc)

	deploymentIndexes := NewDeploymentIndexes(sb)
	groupIndexes := NewGroupIndexes(sb)

	deployments := collections.NewIndexedMap(sb, collections.NewPrefix(keys.DeploymentPrefix), "deployments", keys.DeploymentPrimaryKeyCodec, codec.CollValue[v1.Deployment](cdc), deploymentIndexes)
	groups := collections.NewIndexedMap(sb, collections.NewPrefix(keys.GroupPrefix), "groups", keys.GroupPrimaryKeyCodec, codec.CollValue[types.Group](cdc), groupIndexes)
	pendingDenomMigrations := collections.NewMap(sb, collections.NewPrefix(keys.PendingDenomMigrationPrefix), "pending_denom_migrations", keys.DeploymentPrimaryKeyCodec, sdk.IntValue)
	params := collections.NewItem(sb, keys.ParamsKey, "params", codec.CollValue[types.Params](cdc))

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	return &Keeper{
		skey:                   skey,
		cdc:                    cdc,
		ekeeper:                ekeeper,
		oracleKeeper:           oracleKeeper,
		marketKeeper:           marketKeeper,
		authzKeeper:            authzKeeper,
		bankKeeper:             bankKeeper,
		authority:              authority,
		schema:                 schema,
		deployments:            deployments,
		groups:                 groups,
		pendingDenomMigrations: pendingDenomMigrations,
		Params:                 params,
	}
}

func (k Keeper) NewQuerier() Querier {
	return Querier{k}
}

// Codec returns keeper codec
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// StoreKey returns store key
func (k Keeper) StoreKey() storetypes.StoreKey {
	return k.skey
}

// GetAuthority returns the x/deployment module's authority.
func (k Keeper) GetAuthority() string {
	return k.authority
}

// Deployments returns the deployment IndexedMap for direct access (used by genesis and migration)
func (k Keeper) Deployments() *collections.IndexedMap[keys.DeploymentPrimaryKey, v1.Deployment, DeploymentIndexes] {
	return k.deployments
}

// Groups returns the group IndexedMap for direct access (used by genesis and migration)
func (k Keeper) Groups() *collections.IndexedMap[keys.GroupPrimaryKey, types.Group, GroupIndexes] {
	return k.groups
}

// SetParams sets the x/deployment module parameters.
func (k Keeper) SetParams(ctx sdk.Context, p types.Params) error {
	if err := p.Validate(); err != nil {
		return err
	}

	return k.Params.Set(ctx, p)
}

// GetParams returns the current x/deployment module parameters.
func (k Keeper) GetParams(ctx sdk.Context) (types.Params, error) {
	return k.Params.Get(ctx)
}

// GetDeployment returns deployment details with provided DeploymentID
func (k Keeper) GetDeployment(ctx sdk.Context, id v1.DeploymentID) (v1.Deployment, bool) {
	deployment, err := k.deployments.Get(ctx, keys.DeploymentIDToKey(id))
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("unexpected error getting deployment", "id", id, "err", err)
		}
		return v1.Deployment{}, false
	}
	return deployment, true
}

// GetGroup returns group details with given GroupID from deployment store
func (k Keeper) GetGroup(ctx sdk.Context, id v1.GroupID) (types.Group, bool) {
	group, err := k.groups.Get(ctx, keys.GroupIDToKey(id))
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			ctx.Logger().Error("unexpected error getting group", "id", id, "err", err)
		}
		return types.Group{}, false
	}
	return group, true
}

// GetGroups returns all groups of a deployment with given DeploymentID from deployment store
func (k Keeper) GetGroups(ctx sdk.Context, id v1.DeploymentID) (types.Groups, error) {
	var vals types.Groups

	deploymentKey := keys.DeploymentIDToKey(id)
	iter, err := k.groups.Indexes.Deployment.MatchExact(ctx, deploymentKey)
	if err != nil {
		return nil, fmt.Errorf("GetGroups iteration failed: %w", err)
	}

	err = indexes.ScanValues(ctx, k.groups, iter, func(group types.Group) bool {
		vals = append(vals, group)
		return false
	})
	if err != nil {
		return nil, fmt.Errorf("GetGroups scan failed: %w", err)
	}

	return vals, nil
}

// Create creates a new deployment with given deployment and group specifications
func (k Keeper) Create(ctx sdk.Context, deployment v1.Deployment, groups []types.Group) error {
	pk := keys.DeploymentIDToKey(deployment.ID)
	has, err := k.deployments.Has(ctx, pk)
	if err != nil {
		return err
	}
	if has {
		return v1.ErrDeploymentExists
	}

	if err := k.deployments.Set(ctx, pk, deployment); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}

	for idx := range groups {
		group := groups[idx]

		if !group.ID.DeploymentID().Equals(deployment.ID) {
			return v1.ErrInvalidGroupID
		}

		gpk := keys.GroupIDToKey(group.ID)
		if err := k.groups.Set(ctx, gpk, group); err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}
	}

	err = ctx.EventManager().EmitTypedEvent(
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
	pk := keys.DeploymentIDToKey(deployment.ID)
	has, err := k.deployments.Has(ctx, pk)
	if err != nil {
		return err
	}
	if !has {
		return v1.ErrDeploymentNotFound
	}

	if err := k.deployments.Set(ctx, pk, deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	err = ctx.EventManager().EmitTypedEvent(
		&v1.EventDeploymentUpdated{
			ID:   deployment.ID,
			Hash: deployment.Hash,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// CloseDeployment close deployment
func (k Keeper) CloseDeployment(ctx sdk.Context, deployment v1.Deployment) error {
	if deployment.State == v1.DeploymentClosed {
		return v1.ErrDeploymentClosed
	}

	pk := keys.DeploymentIDToKey(deployment.ID)
	has, err := k.deployments.Has(ctx, pk)
	if err != nil {
		return err
	}
	if !has {
		return v1.ErrDeploymentNotFound
	}

	deployment.State = v1.DeploymentClosed

	if err := k.deployments.Set(ctx, pk, deployment); err != nil {
		return fmt.Errorf("failed to close deployment: %w", err)
	}

	err = ctx.EventManager().EmitTypedEvent(
		&v1.EventDeploymentClosed{
			ID: deployment.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnCloseGroup provides shutdown API for a Group
func (k Keeper) OnCloseGroup(ctx sdk.Context, group types.Group, state types.Group_State) error {
	pk := keys.GroupIDToKey(group.ID)
	has, err := k.groups.Has(ctx, pk)
	if err != nil {
		return err
	}
	if !has {
		return v1.ErrGroupNotFound
	}

	group.State = state

	if err := k.groups.Set(ctx, pk, group); err != nil {
		return fmt.Errorf("failed to close group: %w", err)
	}

	err = ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupClosed{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnPauseGroup provides pause API for a Group
func (k Keeper) OnPauseGroup(ctx sdk.Context, group types.Group) error {
	pk := keys.GroupIDToKey(group.ID)
	has, err := k.groups.Has(ctx, pk)
	if err != nil {
		return err
	}
	if !has {
		return v1.ErrGroupNotFound
	}

	group.State = types.GroupPaused

	if err := k.groups.Set(ctx, pk, group); err != nil {
		return fmt.Errorf("failed to pause group: %w", err)
	}

	err = ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupPaused{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// OnStartGroup provides start API for a Group
func (k Keeper) OnStartGroup(ctx sdk.Context, group types.Group) error {
	pk := keys.GroupIDToKey(group.ID)
	has, err := k.groups.Has(ctx, pk)
	if err != nil {
		return err
	}
	if !has {
		return v1.ErrGroupNotFound
	}

	group.State = types.GroupOpen

	if err := k.groups.Set(ctx, pk, group); err != nil {
		return fmt.Errorf("failed to start group: %w", err)
	}

	err = ctx.EventManager().EmitTypedEvent(
		&v1.EventGroupStarted{
			ID: group.ID,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

// WithDeployments iterates all deployments in deployment store
func (k Keeper) WithDeployments(ctx sdk.Context, fn func(v1.Deployment) bool) error {
	err := k.deployments.Walk(ctx, nil, func(_ keys.DeploymentPrimaryKey, deployment v1.Deployment) (bool, error) {
		return fn(deployment), nil
	})
	if err != nil {
		return fmt.Errorf("WithDeployments iteration failed: %w", err)
	}
	return nil
}

// IterateDeploymentFiltered iterates all deployments in deployment store
func (k Keeper) IterateDeploymentFiltered(ctx sdk.Context, state v1.Deployment_State, fn func(v1.Deployment) bool) error {
	iter, err := k.deployments.Indexes.State.MatchExact(ctx, int32(state))
	if err != nil {
		return err
	}

	err = indexes.ScanValues(ctx, k.deployments, iter, func(deployment v1.Deployment) bool {
		return fn(deployment)
	})

	if err != nil {
		return fmt.Errorf("WithDeployments iteration failed: %w", err)
	}

	return nil
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

// SaveGroup persists a group to the store. Used during denom migration.
func (k Keeper) SaveGroup(ctx sdk.Context, group types.Group) error {
	pk := keys.GroupIDToKey(group.ID)
	return k.groups.Set(ctx, pk, group)
}

// AddPendingDenomMigration marks a deployment for deferred denom migration.
func (k Keeper) AddPendingDenomMigration(ctx sdk.Context, did v1.DeploymentID) error {
	return k.pendingDenomMigrations.Set(ctx, keys.DeploymentIDToKey(did), sdkmath.OneInt())
}

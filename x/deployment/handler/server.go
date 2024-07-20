package handler

import (
	"bytes"
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/akashd/x/deployment/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	deployment  keeper.IKeeper
	market      MarketKeeper
	escrow      EscrowKeeper
	authzKeeper AuthzKeeper
}

// NewServer returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewServer(k keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper, authzKeeper AuthzKeeper) types.MsgServer {
	return &msgServer{
		deployment:  k,
		market:      mkeeper,
		escrow:      ekeeper,
		authzKeeper: authzKeeper,
	}
}

func (ms msgServer) CreateDeployment(goCtx context.Context, msg *types.MsgCreateDeployment) (*types.MsgCreateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, found := ms.deployment.GetDeployment(ctx, msg.ID); found {
		return nil, v1.ErrDeploymentExists
	}

	params := ms.deployment.GetParams(ctx)
	if err := params.ValidateDeposit(msg.Deposit); err != nil {
		return nil, err
	}

	deployment := v1.Deployment{
		ID:        msg.ID,
		State:     v1.DeploymentActive,
		Hash:      msg.Hash,
		CreatedAt: ctx.BlockHeight(),
	}

	if err := types.ValidateDeploymentGroups(msg.Groups); err != nil {
		return nil, fmt.Errorf("%w: %s", v1.ErrInvalidGroups, err.Error())
	}

	owner, err := sdk.AccAddressFromBech32(msg.ID.Owner)
	if err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	depositor, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	if err = ms.authorizeDeposit(ctx, owner, depositor, msg.Deposit); err != nil {
		return nil, err
	}

	groups := make([]types.Group, 0, len(msg.Groups))

	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			ID:        v1.MakeGroupID(deployment.ID, uint32(idx+1)),
			State:     types.GroupOpen,
			GroupSpec: spec,
			CreatedAt: ctx.BlockHeight(),
		})
	}

	if err := ms.deployment.Create(ctx, deployment, groups); err != nil {
		return nil, fmt.Errorf("%w: %s", v1.ErrInternal, err.Error())
	}

	// create orders
	for _, group := range groups {
		if _, err := ms.market.CreateOrder(ctx, group.ID, group.GroupSpec); err != nil {
			return &types.MsgCreateDeploymentResponse{}, err
		}
	}

	if err := ms.escrow.AccountCreate(ctx,
		types.EscrowAccountForDeployment(deployment.ID),
		owner,
		depositor,
		msg.Deposit,
	); err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	return &types.MsgCreateDeploymentResponse{}, nil
}

func (ms msgServer) authorizeDeposit(ctx sdk.Context, owner, depositor sdk.AccAddress, deposit sdk.Coin) error {
	// if owner is the depositor, then no need to check authorization
	if owner.Equals(depositor) {
		return nil
	}

	// find the DepositDeploymentAuthorization given to the owner by the depositor and check
	// acceptance
	msg := &v1.MsgDepositDeployment{Amount: deposit}
	authorization, expiration := ms.authzKeeper.GetAuthorization(ctx, owner, depositor, sdk.MsgTypeURL(msg))
	if authorization == nil {
		return sdkerrors.ErrUnauthorized.Wrap("authorization not found")
	}

	resp, err := authorization.Accept(ctx, msg)
	if err != nil {
		return err
	}

	if resp.Delete {
		err = ms.authzKeeper.DeleteGrant(ctx, owner, depositor, sdk.MsgTypeURL(msg))
	} else if resp.Updated != nil {
		err = ms.authzKeeper.SaveGrant(ctx, owner, depositor, resp.Updated, expiration)
	}
	if err != nil {
		return err
	}

	if !resp.Accept {
		return sdkerrors.ErrUnauthorized
	}

	return nil
}

func (ms msgServer) DepositDeployment(goCtx context.Context, msg *v1.MsgDepositDeployment) (*v1.MsgDepositDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return &v1.MsgDepositDeploymentResponse{}, v1.ErrDeploymentNotFound
	}

	if deployment.State != v1.DeploymentActive {
		return &v1.MsgDepositDeploymentResponse{}, v1.ErrDeploymentClosed
	}

	owner, err := sdk.AccAddressFromBech32(deployment.ID.Owner)
	if err != nil {
		return &v1.MsgDepositDeploymentResponse{}, err
	}

	depositor, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return &v1.MsgDepositDeploymentResponse{}, err
	}

	eID := types.EscrowAccountForDeployment(msg.ID)

	eAccount, err := ms.escrow.GetAccount(ctx, eID)
	if err != nil {
		return &v1.MsgDepositDeploymentResponse{}, err
	}

	// error if depositor is not an owner and there is already exists authorization from another account
	if (msg.Depositor != msg.ID.Owner) && eAccount.HasDepositor() && (eAccount.Depositor != msg.Depositor) {
		return &v1.MsgDepositDeploymentResponse{}, v1.ErrInvalidDeploymentDepositor
	}

	if err = ms.authorizeDeposit(ctx, owner, depositor, msg.Amount); err != nil {
		return nil, err
	}

	if err := ms.escrow.AccountDeposit(ctx,
		eID,
		depositor,
		msg.Amount); err != nil {
		return &v1.MsgDepositDeploymentResponse{}, err
	}

	return &v1.MsgDepositDeploymentResponse{}, nil
}

func (ms msgServer) UpdateDeployment(goCtx context.Context, msg *types.MsgUpdateDeployment) (*types.MsgUpdateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return nil, v1.ErrDeploymentNotFound
	}

	// If the deployment is not active, do not allow it to be updated
	if deployment.State != v1.DeploymentActive {
		return &types.MsgUpdateDeploymentResponse{}, v1.ErrDeploymentClosed
	}

	// If the version is not identical do not allow the update, there is nothing to change in this transaction
	if bytes.Equal(msg.Hash, deployment.Hash) {
		return &types.MsgUpdateDeploymentResponse{}, v1.ErrInvalidHash
	}

	deployment.Hash = msg.Hash

	if err := ms.deployment.UpdateDeployment(ctx, deployment); err != nil {
		return &types.MsgUpdateDeploymentResponse{}, fmt.Errorf("%w: %s", v1.ErrInternal, err.Error())
	}

	return &types.MsgUpdateDeploymentResponse{}, nil
}

func (ms msgServer) CloseDeployment(goCtx context.Context, msg *types.MsgCloseDeployment) (*types.MsgCloseDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
	if !found {
		return &types.MsgCloseDeploymentResponse{}, v1.ErrDeploymentNotFound
	}

	if deployment.State != v1.DeploymentActive {
		return &types.MsgCloseDeploymentResponse{}, v1.ErrDeploymentClosed
	}

	if err := ms.escrow.AccountClose(ctx,
		types.EscrowAccountForDeployment(deployment.ID),
	); err != nil {
		return &types.MsgCloseDeploymentResponse{}, err
	}

	// Update state via escrow hooks.

	return &types.MsgCloseDeploymentResponse{}, nil
}

func (ms msgServer) CloseGroup(goCtx context.Context, msg *types.MsgCloseGroup) (*types.MsgCloseGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return nil, v1.ErrGroupNotFound
	}

	// if Group already closed; return the validation error
	err := group.ValidateClosable()
	if err != nil {
		return nil, err
	}

	// Update the Group's state
	err = ms.deployment.OnCloseGroup(ctx, group, types.GroupClosed)
	if err != nil {
		return nil, err
	}
	_ = ms.market.OnGroupClosed(ctx, group.ID)

	return &types.MsgCloseGroupResponse{}, nil
}

func (ms msgServer) PauseGroup(goCtx context.Context, msg *types.MsgPauseGroup) (*types.MsgPauseGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return nil, v1.ErrGroupNotFound
	}

	// if Group already closed; return the validation error
	err := group.ValidatePausable()
	if err != nil {
		return nil, err
	}

	// Update the Group's state
	err = ms.deployment.OnPauseGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	_ = ms.market.OnGroupClosed(ctx, group.ID)

	return &types.MsgPauseGroupResponse{}, nil
}

func (ms msgServer) StartGroup(goCtx context.Context, msg *types.MsgStartGroup) (*types.MsgStartGroupResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	group, found := ms.deployment.GetGroup(ctx, msg.ID)
	if !found {
		return &types.MsgStartGroupResponse{}, v1.ErrGroupNotFound
	}

	err := group.ValidateStartable()
	if err != nil {
		return &types.MsgStartGroupResponse{}, err
	}

	err = ms.deployment.OnStartGroup(ctx, group)
	if err != nil {
		return &types.MsgStartGroupResponse{}, err
	}
	if _, err := ms.market.CreateOrder(ctx, group.ID, group.GroupSpec); err != nil {
		return &types.MsgStartGroupResponse{}, err
	}

	return &types.MsgStartGroupResponse{}, nil
}

func (ms msgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.deployment.GetAuthority() != req.Authority {
		return nil, govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", ms.deployment.GetAuthority(), req.Authority)
	}

	ctx := sdk.UnwrapSDKContext(goCtx)
	if err := ms.deployment.SetParams(ctx, req.Params); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

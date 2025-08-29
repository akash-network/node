package handler

import (
	"bytes"
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	v1 "pkg.akt.dev/go/node/deployment/v1"
	types "pkg.akt.dev/go/node/deployment/v1beta4"

	"pkg.akt.dev/node/x/deployment/keeper"
)

var _ types.MsgServer = msgServer{}

type msgServer struct {
	deployment keeper.IKeeper
	market     MarketKeeper
	escrow     EscrowKeeper
}

// NewServer returns an implementation of the deployment MsgServer interface
// for the provided Keeper.
func NewServer(k keeper.IKeeper, mkeeper MarketKeeper, ekeeper EscrowKeeper) types.MsgServer {
	return &msgServer{
		deployment: k,
		market:     mkeeper,
		escrow:     ekeeper,
	}
}

func (ms msgServer) CreateDeployment(goCtx context.Context, msg *types.MsgCreateDeployment) (*types.MsgCreateDeploymentResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	did := msg.ID

	if _, found := ms.deployment.GetDeployment(ctx, did); found {
		return nil, v1.ErrDeploymentExists
	}

	params := ms.deployment.GetParams(ctx)
	if err := params.ValidateDeposit(msg.Deposit.Amount); err != nil {
		return nil, err
	}

	deployment := v1.Deployment{
		ID:        did,
		State:     v1.DeploymentActive,
		Hash:      msg.Hash,
		CreatedAt: ctx.BlockHeight(),
	}

	if err := types.ValidateDeploymentGroups(msg.Groups); err != nil {
		return nil, fmt.Errorf("%w: %s", v1.ErrInvalidGroups, err.Error())
	}

	deposits, err := ms.escrow.AuthorizeDeposits(ctx, msg)
	if err != nil {
		return nil, err
	}

	groups := make([]types.Group, 0, len(msg.Groups))

	for idx, spec := range msg.Groups {
		groups = append(groups, types.Group{
			ID:        v1.MakeGroupID(deployment.ID, uint32(idx+1)), // nolint gosec
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

	owner, _ := sdk.AccAddressFromBech32(did.Owner)
	if err := ms.escrow.AccountCreate(ctx, deployment.ID.ToEscrowAccountID(), owner, deposits); err != nil {
		return &types.MsgCreateDeploymentResponse{}, err
	}

	return &types.MsgCreateDeploymentResponse{}, nil
}

//func (ms msgServer) authorizeDeposit(sctx sdk.Context, msg sdk.Msg) ([]ev1.Deposit, error) {
//	// find the DepositDeploymentAuthorization given to the owner by the depositor and check
//	// acceptance
//
//	depositors := make([]ev1.Deposit, 0, 1)
//
//	hasDeposit, valid := msg.(types.HasDeposit)
//	if !valid {
//
//	}
//
//	hasID, valid := msg.(types.HasDeploymentID)
//	if !valid {
//
//	}
//
//	did := hasID.GetID()
//	owner, err := sdk.AccAddressFromBech32(did.Owner)
//	if err != nil {
//		return nil, err
//	}
//
//	dep := hasDeposit.GetDeposit()
//	denom := dep.Amount.Denom
//
//	remainder := sdkmath.NewInt(dep.Amount.Amount.Int64())
//
//	for _, source := range dep.Sources {
//		switch source {
//		case deposit.SourceBalance:
//			spendableAmount := ms.bkeeper.SpendableCoin(sctx, owner, denom)
//
//			if spendableAmount.Amount.IsPositive() {
//				requestedSpend := sdk.NewCoin(denom, remainder)
//
//				if spendableAmount.IsLT(requestedSpend) {
//					requestedSpend = spendableAmount
//				}
//				depositors = append(depositors, ev1.Deposit{
//					Depositor: owner.String(),
//					Height:    sctx.BlockHeight(),
//					Balance:   sdk.NewDecCoinFromCoin(requestedSpend),
//				})
//
//				remainder = remainder.Sub(requestedSpend.Amount)
//			}
//		case deposit.SourceGrant:
//			msgTypeUrl := (&types.DepositAuthorization{}).MsgTypeURL()
//
//			ms.authzKeeper.GetGranteeGrantsByMsgType(sctx, owner, msgTypeUrl, func(ctx context.Context, granter sdk.AccAddress, authorization authz.Authorization, expiration *time.Time) bool {
//				deplAuthz, valid := authorization.(*types.DepositAuthorization)
//				if !valid {
//					return false
//				}
//
//				allowedSpend := sdk.Coin{
//					Denom:  denom,
//					Amount: sdkmath.NewInt(deplAuthz.SpendLimit.Amount.Int64()),
//				}
//
//				nDeposit := deposit.Deposit{
//					Amount:  sdk.NewCoin(denom, remainder),
//					Sources: nil,
//				}
//
//				var authzMsg sdk.Msg
//				switch m := msg.(type) {
//				case *types.MsgCreateDeployment:
//					authzMsg = types.NewMsgCreateDeployment(m.ID, m.Groups, m.Hash, nDeposit)
//				case *types.MsgDepositDeployment:
//					authzMsg = types.NewMsgDepositDeployment(m.ID, nDeposit)
//				}
//
//				resp, err := authorization.Accept(ctx, authzMsg)
//				if err != nil {
//					return false
//				}
//
//				if resp.Delete {
//					err = ms.authzKeeper.DeleteGrant(ctx, owner, granter, msgTypeUrl)
//				} else if resp.Updated != nil {
//					err = ms.authzKeeper.SaveGrant(ctx, owner, granter, resp.Updated, expiration)
//				}
//
//				if !resp.Accept {
//					return false
//				}
//
//				deplAuthz = resp.Updated.(*types.DepositAuthorization)
//
//				allowedSpend = allowedSpend.Sub(deplAuthz.SpendLimit)
//
//				depositors = append(depositors, ev1.Deposit{
//					Depositor: owner.String(),
//					Height:    sctx.BlockHeight(),
//					Balance:   sdk.NewDecCoinFromCoin(allowedSpend),
//				})
//				remainder = remainder.Sub(allowedSpend.Amount)
//
//				return remainder.IsZero()
//			})
//		}
//
//		if remainder.IsZero() {
//			break
//		}
//	}
//
//	if !remainder.IsZero() {
//		// following check is for sanity. if value is negative, math above went horribly wrong
//		if remainder.IsNegative() {
//			return nil, fmt.Errorf("%w: deposit overflow", v1.ErrInvalidDeposit)
//		} else {
//			return nil, fmt.Errorf("%w: insufficient balance", v1.ErrInvalidDeposit)
//		}
//	}
//
//	return depositors, nil
//}

//func (ms msgServer) DepositDeployment(goCtx context.Context, msg *types.MsgDepositDeployment) (*types.MsgDepositDeploymentResponse, error) {
//	ctx := sdk.UnwrapSDKContext(goCtx)
//
//	deployment, found := ms.deployment.GetDeployment(ctx, msg.ID)
//	if !found {
//		return &types.MsgDepositDeploymentResponse{}, v1.ErrDeploymentNotFound
//	}
//
//	if deployment.State != v1.DeploymentActive {
//		return &types.MsgDepositDeploymentResponse{}, v1.ErrDeploymentClosed
//	}
//
//	deposits, err := aauthz.AuthorizeDeposit(ctx, ms.authzKeeper, ms.bkeeper, msg)
//	if err != nil {
//		return nil, err
//	}
//
//	eID := types.EscrowAccountForDeployment(msg.ID)
//
//	if err = ms.escrow.AccountDeposit(ctx, eID, deposits); err != nil {
//		return &types.MsgDepositDeploymentResponse{}, err
//	}
//
//	return &types.MsgDepositDeploymentResponse{}, nil
//}

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

	if err := ms.escrow.AccountClose(ctx, deployment.ID.ToEscrowAccountID()); err != nil {
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

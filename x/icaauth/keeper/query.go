package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/cosmos/cosmos-sdk/types"
	icatypes "github.com/cosmos/ibc-go/v3/modules/apps/27-interchain-accounts/types"

	typesv1beta2 "github.com/ovrclk/akash/x/icaauth/types/v1beta2"
)

// InterchainAccountFromAddress implements the Query/InterchainAccountFromAddress gRPC method
func (k *Keeper) InterchainAccountFromAddress(goCtx context.Context, req *typesv1beta2.QueryInterchainAccountFromAddressRequest) (*typesv1beta2.QueryInterchainAccountFromAddressResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	portID, err := icatypes.NewControllerPortID(req.Owner)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "could not find account: %s", err)
	}

	addr, found := k.icaControllerKeeper.GetInterchainAccountAddress(ctx, req.ConnectionId, portID)
	if !found {
		return nil, status.Errorf(codes.NotFound, "no account found for portID %s", portID)
	}

	return &typesv1beta2.QueryInterchainAccountFromAddressResponse{
		InterchainAccountAddress: addr,
	}, nil
}

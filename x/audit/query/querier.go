package query

import (
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/audit/keeper"
	"github.com/ovrclk/akash/x/audit/types"
)

const (
	tokenList      = "list"
	tokenOwner     = "owner"
	tokenValidator = "validator"
)

// NewQuerier creates and returns a new market querier instance
func NewQuerier(keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[2] {
		case tokenList:
			return listAll(ctx, path[3:], req, keeper, legacyQuerierCdc)
		case tokenOwner:
			return ownerList(ctx, path[3:], req, keeper, legacyQuerierCdc)
		case tokenValidator:
			return validatorList(ctx, path[3:], req, keeper, legacyQuerierCdc)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func listAll(ctx sdk.Context, _ []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	var values types.Providers
	keeper.WithProviders(ctx, func(obj types.Provider) bool {
		values = append(values, obj)
		return false
	})

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func ownerList(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	if len(path) != 2 {
		return nil, sdkerrors.ErrInvalidRequest
	}

	owner, err := sdk.AccAddressFromBech32(path[0])
	if (err != nil) || (path[1] != tokenList) {
		return nil, types.ErrInvalidAddress
	}

	res, _ := keeper.GetProviderAttributes(ctx, owner)

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, res)
}

func validatorList(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	var values types.Providers

	if len(path) != 2 {
		return nil, sdkerrors.ErrInvalidRequest
	}

	validator, err := sdk.AccAddressFromBech32(path[0])
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	if path[1] == tokenList {
		keeper.WithProviders(ctx, func(obj types.Provider) bool {
			if path[0] == obj.Validator {
				values = append(values, obj)
			}
			return false
		})
	} else {
		var owner sdk.AccAddress
		if owner, err = sdk.AccAddressFromBech32(path[1]); err != nil {
			return nil, types.ErrInvalidAddress
		}

		res, exists := keeper.GetProviderByValidator(ctx, types.ProviderID{
			Owner:     owner,
			Validator: validator,
		})

		if exists {
			values = append(values, res)
		}
	}

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

package query

import (
	"math/big"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	abci "github.com/tendermint/tendermint/abci/types"

	"github.com/ovrclk/akash/sdkutil"
	"github.com/ovrclk/akash/x/cert/keeper"
	"github.com/ovrclk/akash/x/cert/types"
)

const (
	tokenList  = "list"
	tokenOwner = "owner"
	tokenState = "state"
)

// NewQuerier creates and returns a new market querier instance
func NewQuerier(keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) sdk.Querier {
	return func(ctx sdk.Context, path []string, req abci.RequestQuery) ([]byte, error) {
		switch path[2] {
		case tokenState:
			fallthrough
		case tokenList:
			return listAll(ctx, path[3:], req, keeper, legacyQuerierCdc)
		case tokenOwner:
			return ownerList(ctx, path[3:], req, keeper, legacyQuerierCdc)
		}
		return []byte{}, sdkerrors.ErrUnknownRequest
	}
}

func listAll(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	var values types.Certificates

	// nolint: gocritic
	if path[0] == tokenList {
		keeper.WithCertificates(ctx, func(obj types.Certificate) bool {
			values = append(values, obj)
			return false
		})
	} else if path[0] == tokenState && len(path) > 2 {
		state, err := validateState(path[1])
		if err != nil {
			return nil, err
		}

		keeper.WithCertificatesState(ctx, state, func(obj types.Certificate) bool {
			values = append(values, obj)
			return false
		})
	} else {
		return []byte{}, sdkerrors.ErrUnknownRequest
	}

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func ownerList(ctx sdk.Context, path []string, _ abci.RequestQuery, keeper keeper.Keeper, legacyQuerierCdc *codec.LegacyAmino) ([]byte, error) {
	if len(path) < 2 {
		return nil, sdkerrors.ErrInvalidRequest
	}

	owner, err := sdk.AccAddressFromBech32(path[0])
	if err != nil {
		return nil, types.ErrInvalidAddress
	}

	var values types.Certificates

	switch path[1] {
	case tokenList:
		keeper.WithOwner(ctx, owner, func(obj types.Certificate) bool {
			values = append(values, obj)
			return false
		})
	case tokenState:
		state, err := validateState(path[1])
		if err != nil {
			return nil, err
		}

		keeper.WithOwnerState(ctx, owner, state, func(obj types.Certificate) bool {
			values = append(values, obj)
			return false
		})
	default:
		serial, valid := new(big.Int).SetString(path[1], 10)
		if !valid {
			return nil, types.ErrInvalidSerialNumber
		}

		res, _ := keeper.GetCertificateByID(ctx, types.CertID{
			Owner:  owner,
			Serial: *serial,
		})
		values = append(values, res)
	}

	return sdkutil.RenderQueryResponse(legacyQuerierCdc, values)
}

func validateState(val string) (types.Certificate_State, error) {
	idx, exists := types.Certificate_State_value[val]

	if exists && types.Certificate_State(idx) != types.CertificateStateInvalid {
		return types.CertificateStateInvalid, types.ErrInvalidState
	}

	return types.Certificate_State(idx), nil
}

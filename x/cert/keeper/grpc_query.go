package keeper

import (
	"context"
	"math/big"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"
)

// Querier is used as Keeper will have duplicate methods if used directly, and gRPC names take precedence over keeper
type querier struct {
	keeper
}

var _ types.QueryServer = &querier{}

func (q querier) Certificates(c context.Context, req *types.QueryCertificatesRequest) (*types.QueryCertificatesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	var certificates types.CertificatesResponse
	var pageRes *sdkquery.PageResponse
	var err error

	ctx := sdk.UnwrapSDKContext(c)
	store := ctx.KVStore(q.skey)

	state := types.CertificateStateInvalid
	if req.Filter.State != "" {
		vl, exists := types.Certificate_State_value[req.Filter.State]

		if !exists {
			return nil, status.Error(codes.InvalidArgument, "invalid state value")
		}

		state = types.Certificate_State(vl)
	}

	if req.Filter.Owner != "" {
		var owner sdk.Address
		if owner, err = sdk.AccAddressFromBech32(req.Filter.Owner); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if req.Filter.Serial != "" {
			serial, valid := new(big.Int).SetString(req.Filter.Serial, 10)
			if !valid {
				return nil, status.Error(codes.InvalidArgument, "invalid serial number")
			}

			item, exists := q.GetCertificateByID(ctx, types.CertID{
				Owner:  owner,
				Serial: *serial,
			})

			if exists && filterCertByState(state, item.Certificate.State) {
				certificates = append(certificates, item)
			}
		} else {
			ownerStore := prefix.NewStore(store, certificatePrefix(owner))
			pageRes, err = sdkquery.FilteredPaginate(ownerStore, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
				// prefixed store returns key without prefix
				key = append(certificatePrefix(owner), key...)
				item, err := q.unmarshalIterator(key, value)
				if err != nil {
					return true, err
				}

				if filterCertByState(state, item.Certificate.State) {
					if accumulate {
						certificates = append(certificates, item)
					}
					return true, nil
				}

				return false, nil
			})
		}
	} else {
		pageRes, err = sdkquery.FilteredPaginate(store, req.Pagination, func(key []byte, value []byte, accumulate bool) (bool, error) {
			item, err := q.unmarshalIterator(key, value)
			if err != nil {
				return true, err
			}

			if filterCertByState(state, item.Certificate.State) {
				if accumulate {
					certificates = append(certificates, item)
				}
				return true, nil
			}

			return false, nil
		})
	}

	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &types.QueryCertificatesResponse{
		Certificates: certificates,
		Pagination:   pageRes,
	}, nil
}

func filterCertByState(state types.Certificate_State, cert types.Certificate_State) bool {
	return (state == types.CertificateStateInvalid) || (cert == state)
}

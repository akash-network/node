package keeper

import (
	"context"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

type Querier struct {
	vtypes.UnimplementedQueryServer
	Keeper Keeper
}

var _ vtypes.QueryServer = Querier{}

func (k Querier) Auditor(c context.Context, req *vtypes.QueryAuditorRequest) (*vtypes.QueryAuditorResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	auditor, err := sdk.AccAddressFromBech32(req.Auditor)
	if err != nil {
		return nil, err
	}

	record, found := k.Keeper.GetAuditor(sdk.UnwrapSDKContext(c), auditor)
	if !found {
		return nil, moduletypes.ErrAuditorNotFound
	}

	return &vtypes.QueryAuditorResponse{Auditor: record}, nil
}

func (k Querier) Auditors(c context.Context, req *vtypes.QueryAuditorsRequest) (*vtypes.QueryAuditorsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := prefix.NewStore(ctx.KVStore(k.Keeper.StoreKey()), singletonKey(prefixAuditor))

	var records []vtypes.AuditorRecord
	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(_ []byte, value []byte) error {
		var record vtypes.AuditorRecord
		if err := k.Keeper.Codec().Unmarshal(value, &record); err != nil {
			return err
		}
		if req.StatusFilter != vtypes.AuditorStatusUnspecified && record.Status != req.StatusFilter {
			return nil
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &vtypes.QueryAuditorsResponse{Auditors: records, Pagination: pageRes}, nil
}

func (k Querier) Attestation(c context.Context, req *vtypes.QueryAttestationRequest) (*vtypes.QueryAttestationResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, auditor, err := parseProviderAuditor(req.Provider, req.Auditor)
	if err != nil {
		return nil, err
	}

	record, found := k.Keeper.GetAttestation(sdk.UnwrapSDKContext(c), provider, auditor)
	if !found {
		return nil, moduletypes.ErrAttestationNotFound
	}

	return &vtypes.QueryAttestationResponse{Attestation: record}, nil
}

func (k Querier) ProviderAttestations(c context.Context, req *vtypes.QueryProviderAttestationsRequest) (*vtypes.QueryProviderAttestationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := prefix.NewStore(ctx.KVStore(k.Keeper.StoreKey()), addressKey(prefixAttestation, provider))

	var records []vtypes.AttestationRecord
	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(_ []byte, value []byte) error {
		var record vtypes.AttestationRecord
		if err := k.Keeper.Codec().Unmarshal(value, &record); err != nil {
			return err
		}
		if req.StatusFilter != vtypes.AttestationStatusUnspecified && record.Status != req.StatusFilter {
			return nil
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &vtypes.QueryProviderAttestationsResponse{Attestations: records, Pagination: pageRes}, nil
}

func (k Querier) AuditorAttestations(c context.Context, req *vtypes.QueryAuditorAttestationsRequest) (*vtypes.QueryAuditorAttestationsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	auditor, err := sdk.AccAddressFromBech32(req.Auditor)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := prefix.NewStore(ctx.KVStore(k.Keeper.StoreKey()), auditorAttestationPrefix(auditor))

	var records []vtypes.AttestationRecord
	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, _ []byte) error {
		record, found := k.Keeper.GetAttestation(ctx, sdk.AccAddress(key), auditor)
		if found {
			records = append(records, record)
		}
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &vtypes.QueryAuditorAttestationsResponse{Attestations: records, Pagination: pageRes}, nil
}

func (k Querier) Discrepancy(c context.Context, req *vtypes.QueryDiscrepancyRequest) (*vtypes.QueryDiscrepancyResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	record, found := k.Keeper.GetDiscrepancy(sdk.UnwrapSDKContext(c), req.Id)
	if !found {
		return nil, moduletypes.ErrDiscrepancyNotFound
	}

	return &vtypes.QueryDiscrepancyResponse{Discrepancy: record}, nil
}

func (k Querier) Discrepancies(c context.Context, req *vtypes.QueryDiscrepanciesRequest) (*vtypes.QueryDiscrepanciesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := prefix.NewStore(ctx.KVStore(k.Keeper.StoreKey()), singletonKey(prefixDiscrepancy))

	var records []vtypes.DiscrepancyEvent
	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(_ []byte, value []byte) error {
		var record vtypes.DiscrepancyEvent
		if err := k.Keeper.Codec().Unmarshal(value, &record); err != nil {
			return err
		}
		if req.StatusFilter != vtypes.DiscrepancyStatusUnspecified && record.ResolutionStatus != req.StatusFilter {
			return nil
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &vtypes.QueryDiscrepanciesResponse{Discrepancies: records, Pagination: pageRes}, nil
}

func (k Querier) AuditEscrow(c context.Context, req *vtypes.QueryAuditEscrowRequest) (*vtypes.QueryAuditEscrowResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	record, found := k.Keeper.GetAuditEscrow(sdk.UnwrapSDKContext(c), req.Id)
	if !found {
		return nil, moduletypes.ErrAuditEscrowNotFound
	}

	return &vtypes.QueryAuditEscrowResponse{Escrow: record}, nil
}

func (k Querier) ProviderAuditEscrows(c context.Context, req *vtypes.QueryProviderAuditEscrowsRequest) (*vtypes.QueryProviderAuditEscrowsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	store := prefix.NewStore(ctx.KVStore(k.Keeper.StoreKey()), providerAuditEscrowPrefix(provider))

	var records []vtypes.AuditEscrowRecord
	pageRes, err := sdkquery.Paginate(store, req.Pagination, func(key []byte, _ []byte) error {
		record, found := k.Keeper.GetAuditEscrow(ctx, readID(key))
		if !found {
			return nil
		}
		if req.StatusFilter != vtypes.AuditEscrowStatusUnspecified && record.Status != req.StatusFilter {
			return nil
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &vtypes.QueryProviderAuditEscrowsResponse{Escrows: records, Pagination: pageRes}, nil
}

func (k Querier) ProviderVerificationGrace(c context.Context, req *vtypes.QueryProviderVerificationGraceRequest) (*vtypes.QueryProviderVerificationGraceResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, err
	}

	record, found := k.Keeper.GetProviderVerificationGrace(sdk.UnwrapSDKContext(c), provider)
	if !found {
		return nil, status.Error(codes.NotFound, "provider verification grace not found")
	}

	return &vtypes.QueryProviderVerificationGraceResponse{Grace: record}, nil
}

func (k Querier) ProviderBond(c context.Context, req *vtypes.QueryProviderBondRequest) (*vtypes.QueryProviderBondResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, err
	}

	ctx := sdk.UnwrapSDKContext(c)
	record, found := k.Keeper.GetProviderBond(ctx, provider)
	if !found {
		return nil, moduletypes.ErrInsufficientProviderBond
	}

	snapshot, _ := k.Keeper.GetProviderSnapshot(ctx, provider)
	required := requiredProviderBond(k.Keeper.GetParams(ctx), k.currentProviderTier(ctx, provider), snapshot.ResourceSummary)
	return &vtypes.QueryProviderBondResponse{
		Bond:                   record,
		RequiredForCurrentTier: required,
	}, nil
}

func (k Querier) ProviderSnapshot(c context.Context, req *vtypes.QueryProviderSnapshotRequest) (*vtypes.QueryProviderSnapshotResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	provider, err := sdk.AccAddressFromBech32(req.Provider)
	if err != nil {
		return nil, err
	}

	record, found := k.Keeper.GetProviderSnapshot(sdk.UnwrapSDKContext(c), provider)
	if !found {
		return nil, moduletypes.ErrSnapshotNonCompliant
	}

	return &vtypes.QueryProviderSnapshotResponse{Snapshot: record}, nil
}

func (k Querier) Params(c context.Context, req *vtypes.QueryParamsRequest) (*vtypes.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	return &vtypes.QueryParamsResponse{Params: k.Keeper.GetParams(sdk.UnwrapSDKContext(c))}, nil
}

func (k Querier) currentProviderTier(ctx sdk.Context, provider sdk.AccAddress) vtypes.VerificationTier {
	tier := vtypes.TierUnspecified
	k.Keeper.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		if vtypes.TierBetter(record.Tier, tier) {
			tier = record.Tier
		}
		return false
	})
	return tier
}

func parseProviderAuditor(provider, auditor string) (sdk.AccAddress, sdk.AccAddress, error) {
	providerAddr, err := sdk.AccAddressFromBech32(provider)
	if err != nil {
		return nil, nil, err
	}
	auditorAddr, err := sdk.AccAddressFromBech32(auditor)
	if err != nil {
		return nil, nil, err
	}
	return providerAddr, auditorAddr, nil
}

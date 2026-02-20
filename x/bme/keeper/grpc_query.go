package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/bme/v1"
)

type Querier struct {
	*keeper
}

var _ types.QueryServer = &Querier{}

func (qs Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := qs.GetParams(sctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

func (qs Querier) VaultState(ctx context.Context, _ *types.QueryVaultStateRequest) (*types.QueryVaultStateResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	state, err := qs.GetState(sctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryVaultStateResponse{VaultState: state}, nil
}

func (qs Querier) Status(ctx context.Context, _ *types.QueryStatusRequest) (*types.QueryStatusResponse, error) {
	sctx := sdk.UnwrapSDKContext(ctx)

	params, err := qs.GetParams(sctx)
	if err != nil {
		return nil, err
	}

	status, err := qs.GetMintStatus(sctx)
	if err != nil {
		return nil, err
	}

	cr, _ := qs.GetCollateralRatio(sctx)

	warnThreshold := math.LegacyNewDec(int64(params.CircuitBreakerWarnThreshold)).Quo(math.LegacyNewDec(10000))
	haltThreshold := math.LegacyNewDec(int64(params.CircuitBreakerHaltThreshold)).Quo(math.LegacyNewDec(10000))

	return &types.QueryStatusResponse{
		Status:          status,
		CollateralRatio: cr,
		WarnThreshold:   warnThreshold,
		HaltThreshold:   haltThreshold,
		MintsAllowed:    status < types.MintStatusHaltCR,
		RefundsAllowed:  status < types.MintStatusHaltOracle,
	}, nil
}

func (qs Querier) LedgerRecords(ctx context.Context, req *types.QueryLedgerRecordsRequest) (*types.QueryLedgerRecordsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	limit := req.Pagination.GetLimit()
	if limit == 0 {
		limit = sdkquery.DefaultLimit
	}
	offset := req.Pagination.GetOffset()

	scanPending := true
	scanExecuted := true

	if req.Filters.Status != "" {
		statusVal := types.LedgerRecordStatus(types.LedgerRecordStatus_value[req.Filters.Status])
		switch statusVal {
		case types.LedgerRecordSatusPending:
			scanExecuted = false
		case types.LedgerRecordSatusExecuted:
			scanPending = false
		default:
			return nil, status.Error(codes.InvalidArgument, "invalid status filter value")
		}
	}

	var records []types.QueryLedgerRecordEntry
	total := uint64(0)
	skipped := uint64(0)

	if scanPending {
		err := qs.IterateLedgerPendingRecords(sctx, func(id types.LedgerRecordID, record types.LedgerPendingRecord) (bool, error) {
			if !req.Filters.AcceptPending(id, record) {
				return false, nil
			}

			total++

			if skipped < offset {
				skipped++
				return false, nil
			}

			if uint64(len(records)) >= limit {
				return true, nil
			}

			records = append(records, types.QueryLedgerRecordEntry{
				ID:     id,
				Status: types.LedgerRecordSatusPending,
				Record: &types.QueryLedgerRecordEntry_PendingRecord{
					PendingRecord: &record,
				},
			})

			return false, nil
		})
		if err != nil {
			sctx.Logger().Error("iterating pending ledger records", "error", err)
			return nil, status.Error(codes.Internal, "failed to query pending ledger records")
		}
	}

	if scanExecuted {
		remainingOffset := uint64(0)
		if offset > skipped {
			remainingOffset = offset - skipped
		}
		execSkipped := uint64(0)

		err := qs.IterateLedgerRecords(sctx, func(id types.LedgerRecordID, record types.LedgerRecord) (bool, error) {
			if !req.Filters.AcceptExecuted(id, record) {
				return false, nil
			}

			total++

			if execSkipped < remainingOffset {
				execSkipped++
				return false, nil
			}

			if uint64(len(records)) >= limit {
				return true, nil
			}

			records = append(records, types.QueryLedgerRecordEntry{
				ID:     id,
				Status: types.LedgerRecordSatusExecuted,
				Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
					ExecutedRecord: &record,
				},
			})

			return false, nil
		})
		if err != nil {
			sctx.Logger().Error("iterating executed ledger records", "error", err)
			return nil, status.Error(codes.Internal, "failed to query executed ledger records")
		}
	}

	return &types.QueryLedgerRecordsResponse{
		Records: records,
		Pagination: &sdkquery.PageResponse{
			Total: total,
		},
	}, nil
}

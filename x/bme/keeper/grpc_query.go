package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "pkg.akt.dev/go/node/bme/v1"

	"pkg.akt.dev/node/v2/util/query"
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

	if req.Pagination == nil {
		req.Pagination = &sdkquery.PageRequest{}
	} else if req.Pagination.Offset > 0 && req.Filters.Status == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid request parameters. if offset is set, filter.status must be provided")
	}

	if req.Pagination.Limit == 0 {
		req.Pagination.Limit = sdkquery.DefaultLimit
	}

	sctx := sdk.UnwrapSDKContext(ctx)

	// Step 1: Resolve states and resume key
	states := make([]byte, 0, 3)
	var resumeID *types.LedgerRecordID

	if len(req.Pagination.Key) > 0 {
		var pkBytes, unsolicited []byte
		var err error
		states, _, pkBytes, unsolicited, err = query.DecodePaginationKey(req.Pagination.Key)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		_, id, err := LedgerRecordIDKey.Decode(pkBytes)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resumeID = &id

		// Restore reverse flag from key — ignore req.Pagination.Reverse on resume
		req.Pagination.Reverse = len(unsolicited) > 0 && unsolicited[0] == 1
	} else {
		if req.Filters.Status != "" {
			statusVal := types.LedgerRecordStatus(types.LedgerRecordStatus_value[req.Filters.Status])
			switch statusVal {
			case types.LedgerRecordSatusPending,
				types.LedgerRecordSatusExecuted,
				types.LedgerRecordSatusCanceled:
				states = append(states, byte(statusVal))
			default:
				return nil, status.Error(codes.InvalidArgument, "invalid status filter value")
			}
		} else {
			states = append(states,
				byte(types.LedgerRecordSatusPending),
				byte(types.LedgerRecordSatusExecuted),
				byte(types.LedgerRecordSatusCanceled),
			)
		}
	}

	// Reverse states order when paginating in reverse without a resume key
	if len(req.Pagination.Key) == 0 && req.Pagination.Reverse {
		for i, j := 0, len(states)-1; i < j; i, j = i+1, j-1 {
			states[i], states[j] = states[j], states[i]
		}
	}

	// Step 2: Iterate status collections in order
	var records []types.QueryLedgerRecordEntry
	var nextKey []byte
	total := uint64(0)
	offset := req.Pagination.Offset
	var scanErr error

	filterRecord := func(id types.LedgerRecordID, status types.LedgerRecordStatus, idx int, record types.QueryLedgerRecordEntry) (bool, error) {
		if !req.Filters.Accept(id, status) {
			return false, nil
		}

		if offset > 0 {
			offset--
			return false, nil
		}

		if req.Pagination.Limit == 0 {
			nk, err := qs.encodeLedgerNextKey(states[idx:], states[idx], id, req.Pagination.Reverse)
			if err != nil {
				return true, err
			}
			nextKey = nk
			return true, nil
		}

		records = append(records, record)
		req.Pagination.Limit--
		total++

		return false, nil
	}

	for idx := range states {
		if req.Pagination.Limit == 0 && len(nextKey) > 0 {
			break
		}

		state := types.LedgerRecordStatus(states[idx])

		var r collections.Ranger[types.LedgerRecordID]
		if idx == 0 && resumeID != nil {
			if req.Pagination.Reverse {
				r = new(collections.Range[types.LedgerRecordID]).EndInclusive(*resumeID).Descending()
			} else {
				r = new(collections.Range[types.LedgerRecordID]).StartInclusive(*resumeID)
			}
		} else if req.Pagination.Reverse {
			r = new(collections.Range[types.LedgerRecordID]).Descending()
		}

		switch state {
		case types.LedgerRecordSatusPending:
			scanErr = qs.ledgerPending.Walk(sctx, r, func(id types.LedgerRecordID, record types.LedgerPendingRecord) (bool, error) {
				return filterRecord(id, state, idx, types.QueryLedgerRecordEntry{
					ID:     id,
					Status: state,
					Record: &types.QueryLedgerRecordEntry_PendingRecord{
						PendingRecord: &record,
					},
				})
			})
		case types.LedgerRecordSatusExecuted:
			scanErr = qs.ledger.Walk(sctx, r, func(id types.LedgerRecordID, record types.LedgerRecord) (bool, error) {
				return filterRecord(id, state, idx, types.QueryLedgerRecordEntry{
					ID:     id,
					Status: state,
					Record: &types.QueryLedgerRecordEntry_ExecutedRecord{
						ExecutedRecord: &record,
					},
				})
			})
		case types.LedgerRecordSatusCanceled:
			scanErr = qs.ledgerCanceled.Walk(sctx, r, func(id types.LedgerRecordID, record types.LedgerCanceledRecord) (bool, error) {
				return filterRecord(id, state, idx, types.QueryLedgerRecordEntry{
					ID:     id,
					Status: state,
					Record: &types.QueryLedgerRecordEntry_CanceledRecord{
						CanceledRecord: &record,
					},
				})
			})
		default:
			return nil, status.Error(codes.Internal, fmt.Sprintf("unknown ledger record status: %d", state))
		}

		if scanErr != nil {
			return nil, status.Error(codes.Internal, scanErr.Error())
		}
	}

	return &types.QueryLedgerRecordsResponse{
		Records: records,
		Pagination: &sdkquery.PageResponse{
			Total:   total,
			NextKey: nextKey,
		},
	}, nil
}

func (qs Querier) encodeLedgerNextKey(remainingStates []byte, currentState byte, id types.LedgerRecordID, reverse bool) ([]byte, error) {
	pkBuf := make([]byte, LedgerRecordIDKey.Size(id))
	if _, err := LedgerRecordIDKey.Encode(pkBuf, id); err != nil {
		return nil, err
	}
	var unsolicited []byte
	if reverse {
		unsolicited = []byte{1}
	}
	return query.EncodePaginationKey(remainingStates, []byte{currentState}, pkBuf, unsolicited)
}

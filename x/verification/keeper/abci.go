package keeper

import (
	"context"
	"time"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

func (k *keeper) EndBlocker(ctx context.Context) error {
	start := telemetry.Now()
	defer telemetry.ModuleMeasureSince(moduletypes.ModuleName, start, telemetry.MetricKeyEndBlocker)

	sctx := sdk.UnwrapSDKContext(ctx)
	params := k.GetParams(sctx)
	blockTime := sctx.BlockTime()

	if err := k.processAttestationExpiryQueue(sctx, blockTime, params.MaxEndblockerAttestationExpiries); err != nil {
		return err
	}
	if err := k.processAuditorRenewalQueue(sctx, blockTime, params.MaxEndblockerUnbondingCompletions); err != nil {
		return err
	}
	if err := k.processAuditorBondUnbondingQueue(sctx, blockTime, params.MaxEndblockerUnbondingCompletions); err != nil {
		return err
	}
	if err := k.processSnapshotComplianceQueue(sctx, blockTime, params.MaxEndblockerSnapshotSuspensions); err != nil {
		return err
	}
	if err := k.processProviderBondUnbondingQueue(sctx, blockTime, params.MaxEndblockerUnbondingCompletions); err != nil {
		return err
	}
	if err := k.processDiscrepancyTimeoutQueue(sctx, blockTime, params.MaxEndblockerDiscrepancyTimeouts); err != nil {
		return err
	}
	if err := k.processAuditEscrowExpiryQueue(sctx, blockTime, params.MaxEndblockerAuditEscrowExpiries); err != nil {
		return err
	}
	return k.processGraceExpiryQueue(sctx, blockTime, params.MaxEndblockerGraceExpiries)
}

func (k *keeper) processAttestationExpiryQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueAttestationExpiry, blockTime, limit, func(key []byte, expiresAt time.Time) error {
		provider, auditor, err := decodeAttestationExpiryQueueKey(key)
		if err != nil {
			return err
		}

		attestation, found := k.GetAttestation(ctx, provider, auditor)
		if !found || attestation.Status != vtypes.AttestationStatusValid || !attestation.ExpiresAt.Equal(expiresAt) {
			return nil
		}

		result, err := k.Settle(SettlementInput{
			Path:             SettlementPathAttestationExpired,
			FaultAttribution: vtypes.FaultAttributionNoFault,
		})
		if err != nil {
			return err
		}
		if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
			return err
		}

		attestation.Status = vtypes.AttestationStatusExpired
		attestation.FeeStatus = result.FeeStatus
		attestation.DepositStatus = result.DepositStatus
		attestation.FaultAttribution = vtypes.FaultAttributionNoFault
		if err = k.SetAttestation(ctx, attestation); err != nil {
			return err
		}
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationExpired{
			Provider: attestation.Provider,
			Auditor:  attestation.Auditor,
			Tier:     attestation.Tier,
		})
	})
}

func (k *keeper) processAuditorRenewalQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueAuditorRenewal, blockTime, limit, func(key []byte, deadline time.Time) error {
		auditor, err := decodeAddressQueueKey(key)
		if err != nil {
			return err
		}

		record, found := k.GetAuditor(ctx, auditor)
		if !found || record.Status != vtypes.AuditorStatusActive || !record.RenewalDeadline.Equal(deadline) {
			return nil
		}

		record.Status = vtypes.AuditorStatusLapsed
		return k.SetAuditor(ctx, record)
	})
}

func (k *keeper) processAuditorBondUnbondingQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueAuditorBondUnbonding, blockTime, limit, func(key []byte, completion time.Time) error {
		auditor, err := decodeAddressQueueKey(key)
		if err != nil {
			return err
		}

		record, found := k.GetAuditor(ctx, auditor)
		if !found ||
			record.BondStatus != vtypes.BondStatusUnbonding ||
			record.BondUnbondingCompletionTime == nil ||
			!record.BondUnbondingCompletionTime.Equal(completion) {
			return nil
		}

		if err = k.sendModuleToAccount(ctx, auditor, record.BondAmount); err != nil {
			return err
		}

		record.BondAmount = sdk.NewCoin(record.BondAmount.Denom, math.ZeroInt())
		record.BondStatus = vtypes.BondStatusNotBonded
		record.BondUnbondingCompletionTime = nil
		return k.SetAuditor(ctx, record)
	})
}

func (k *keeper) processSnapshotComplianceQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueSnapshotCompliance, blockTime, limit, func(key []byte, deadline time.Time) error {
		provider, err := decodeAddressQueueKey(key)
		if err != nil {
			return err
		}

		snapshot, found := k.GetProviderSnapshot(ctx, provider)
		if !found || snapshot.Suspended || !snapshot.ComplianceDeadline.Equal(deadline) {
			return nil
		}

		snapshot.Suspended = true
		if err = k.SetProviderSnapshot(ctx, snapshot); err != nil {
			return err
		}
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventSnapshotSuspended{
			Provider: snapshot.Provider,
		})
	})
}

func (k *keeper) processProviderBondUnbondingQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueProviderBondUnbonding, blockTime, limit, func(key []byte, completion time.Time) error {
		provider, err := decodeAddressQueueKey(key)
		if err != nil {
			return err
		}

		record, found := k.GetProviderBond(ctx, provider)
		if !found {
			return nil
		}

		remaining := make([]vtypes.UnbondingEntry, 0, len(record.UnbondingEntries))
		var completed sdk.Coin
		for _, entry := range record.UnbondingEntries {
			if entry.CompletionTime.After(completion) {
				remaining = append(remaining, entry)
				continue
			}
			completed, err = addCoin(completed, entry.Amount)
			if err != nil {
				return err
			}
		}
		if completed.IsNil() || completed.IsZero() {
			return nil
		}
		if err = k.sendModuleToAccount(ctx, provider, completed); err != nil {
			return err
		}

		record.UnbondingEntries = remaining
		return k.SetProviderBond(ctx, record)
	})
}

func (k *keeper) processDiscrepancyTimeoutQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueDiscrepancyTimeout, blockTime, limit, func(key []byte, timeout time.Time) error {
		id, err := decodeIDQueueKey(key)
		if err != nil {
			return err
		}

		discrepancy, found := k.GetDiscrepancy(ctx, id)
		if !found || discrepancy.ResolutionStatus != vtypes.DiscrepancyStatusPending {
			return nil
		}
		expectedTimeout := discrepancy.Timestamp.Add(k.GetParams(ctx).DiscrepancyResolutionTimeout)
		if !expectedTimeout.Equal(timeout) {
			return nil
		}

		result := SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusSlashed,
		}
		if err = k.timeoutDiscrepancyAttestation(ctx, discrepancy.Provider, discrepancy.AuditorA, result); err != nil {
			return err
		}
		if err = k.timeoutDiscrepancyAttestation(ctx, discrepancy.Provider, discrepancy.AuditorB, result); err != nil {
			return err
		}

		auditorA, err := sdk.AccAddressFromBech32(discrepancy.AuditorA)
		if err != nil {
			return err
		}
		auditorB, err := sdk.AccAddressFromBech32(discrepancy.AuditorB)
		if err != nil {
			return err
		}

		discrepancy.ResolutionStatus = vtypes.DiscrepancyStatusTimedOut
		discrepancy.FaultAttribution = vtypes.FaultAttributionInconclusive
		k.SetDiscrepancy(ctx, discrepancy)

		if err = k.resolveDiscrepancyAuditorBond(ctx, auditorA, discrepancy.ID, false); err != nil {
			return err
		}
		if err = k.resolveDiscrepancyAuditorBond(ctx, auditorB, discrepancy.ID, false); err != nil {
			return err
		}
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventDiscrepancyTimedOut{
			DiscrepancyID: discrepancy.ID,
			AuditorA:      discrepancy.AuditorA,
			AuditorB:      discrepancy.AuditorB,
		})
	})
}

func (k *keeper) timeoutDiscrepancyAttestation(ctx sdk.Context, providerString, auditorString string, result SettlementResult) error {
	provider, err := sdk.AccAddressFromBech32(providerString)
	if err != nil {
		return err
	}
	auditor, err := sdk.AccAddressFromBech32(auditorString)
	if err != nil {
		return err
	}

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}
	if err = k.settlePendingDiscrepancyAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return err
	}
	attestation.FeeStatus = result.FeeStatus
	attestation.DepositStatus = result.DepositStatus
	attestation.FaultAttribution = vtypes.FaultAttributionInconclusive
	return k.SetAttestation(ctx, attestation)
}

func (k *keeper) processAuditEscrowExpiryQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueAuditEscrowExpiry, blockTime, limit, func(key []byte, expiresAt time.Time) error {
		id, err := decodeIDQueueKey(key)
		if err != nil {
			return err
		}

		escrow, found := k.GetAuditEscrow(ctx, id)
		if !found ||
			escrow.Status != vtypes.AuditEscrowStatusOpen ||
			escrow.ConsumedByAuditor != "" ||
			escrow.ConsumedAt != nil ||
			!escrow.ExpiresAt.Equal(expiresAt) {
			return nil
		}
		if err = requireEscrowedAuditEscrowFunds(escrow); err != nil {
			return err
		}

		result, err := k.Settle(SettlementInput{
			Path:              SettlementPathAuditEscrow,
			AuditEscrowReason: vtypes.AuditEscrowSettlementReasonExpiredUnconsumed,
			FaultAttribution:  vtypes.FaultAttributionNoFault,
		})
		if err != nil {
			return err
		}

		provider, err := sdk.AccAddressFromBech32(escrow.Provider)
		if err != nil {
			return err
		}
		if err = k.settleAuditEscrowFunds(ctx, provider, escrow, result); err != nil {
			return err
		}

		escrow.Status = vtypes.AuditEscrowStatusExpired
		escrow.FeeStatus = result.FeeStatus
		escrow.ProviderDepositStatus = result.ProviderDepositStatus
		escrow.SettlementReason = vtypes.AuditEscrowSettlementReasonExpiredUnconsumed
		escrow.FaultAttribution = vtypes.FaultAttributionNoFault
		if err = k.SetAuditEscrow(ctx, escrow); err != nil {
			return err
		}
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditEscrowSettled{
			AuditEscrowID:    escrow.ID,
			Reason:           escrow.SettlementReason,
			FaultAttribution: escrow.FaultAttribution,
		})
	})
}

func (k *keeper) processGraceExpiryQueue(ctx sdk.Context, blockTime time.Time, limit uint32) error {
	return k.processDueQueue(ctx, prefixQueueGraceExpiry, blockTime, limit, func(key []byte, expiresAt time.Time) error {
		id, err := decodeIDQueueKey(key)
		if err != nil {
			return err
		}

		grace, found := k.getProviderVerificationGraceByID(ctx, id)
		if !found || grace.Status != vtypes.VerificationGraceStatusActive || !grace.ExpiresAt.Equal(expiresAt) {
			return nil
		}

		grace.Status = vtypes.VerificationGraceStatusExpired
		if err = k.SetProviderVerificationGrace(ctx, grace); err != nil {
			return err
		}
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventVerificationGraceEnded{
			GraceRecordID: grace.ID,
			Status:        grace.Status,
		})
	})
}

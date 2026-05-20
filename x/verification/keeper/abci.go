package keeper

import (
	"context"
	"time"

	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
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
	if err := k.processSnapshotComplianceQueue(sctx, blockTime, params.MaxEndblockerSnapshotSuspensions); err != nil {
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
		if err = k.settleAttestationFunds(ctx, auditor, attestation, result); err != nil {
			return err
		}

		attestation.Status = vtypes.AttestationStatusExpired
		attestation.FeeStatus = result.FeeStatus
		attestation.DepositStatus = result.DepositStatus
		attestation.FaultAttribution = vtypes.FaultAttributionNoFault
		return k.SetAttestation(ctx, attestation)
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
		return k.SetProviderSnapshot(ctx, snapshot)
	})
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
		return k.SetAuditEscrow(ctx, escrow)
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
		return k.SetProviderVerificationGrace(ctx, grace)
	})
}

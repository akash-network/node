package keeper

import (
	"crypto/sha256"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func (k *keeper) RegisterAuditor(ctx sdk.Context, authority string, auditor sdk.AccAddress, tier vtypes.VerificationTier, metadataHash []byte) error {
	if k.authority != "" && authority != k.authority {
		return errorsmod.Wrapf(moduletypes.ErrAuditorUnauthorizedTier, "invalid authority %s", authority)
	}
	if err := validateMsgTier(tier); err != nil {
		return err
	}
	if _, found := k.GetAuditor(ctx, auditor); found {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "auditor already registered")
	}

	params := k.GetParams(ctx)
	return k.SetAuditor(ctx, vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusActive,
		MaxAttestationTier: tier,
		BondAmount:         sdk.NewCoin(params.BondL1.Denom, math.ZeroInt()),
		BondStatus:         vtypes.BondStatusUnspecified,
		MetadataHash:       metadataHash,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(renewalPeriodForTier(params, tier)),
	})
}

func (k *keeper) PostAuditorBond(ctx sdk.Context, auditor sdk.AccAddress, amount sdk.Coin) error {
	if err := validatePositiveCoin(amount); err != nil {
		return err
	}

	record, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}
	if err := k.sendAccountToModule(ctx, auditor, amount); err != nil {
		return err
	}

	bond, err := addCoin(record.BondAmount, amount)
	if err != nil {
		return err
	}
	record.BondAmount = bond
	record.BondStatus = vtypes.BondStatusBonded
	return k.SetAuditor(ctx, record)
}

func (k *keeper) PostProviderBond(ctx sdk.Context, provider sdk.AccAddress, amount sdk.Coin) error {
	if err := validatePositiveCoin(amount); err != nil {
		return err
	}
	if !k.providerExists(ctx, provider) {
		return moduletypes.ErrProviderNotRegistered
	}
	if err := k.sendAccountToModule(ctx, provider, amount); err != nil {
		return err
	}

	record, found := k.GetProviderBond(ctx, provider)
	if !found {
		record = vtypes.ProviderBondRecord{
			Provider:     provider.String(),
			BondedAmount: sdk.NewCoin(amount.Denom, math.ZeroInt()),
		}
	}

	bond, err := addCoin(record.BondedAmount, amount)
	if err != nil {
		return err
	}
	record.BondedAmount = bond
	return k.SetProviderBond(ctx, record)
}

func (k *keeper) PostSnapshotHash(ctx sdk.Context, provider sdk.AccAddress, snapshotHash []byte, resources vtypes.ResourceSummary, snapshotTimestamp time.Time) error {
	if !k.providerExists(ctx, provider) {
		return moduletypes.ErrProviderNotRegistered
	}
	if err := validateHash(snapshotHash, "snapshot hash"); err != nil {
		return err
	}
	if snapshotTimestamp.IsZero() || snapshotTimestamp.After(ctx.BlockTime()) {
		return moduletypes.ErrSnapshotTooOld
	}

	params := k.GetParams(ctx)
	if ctx.BlockTime().Sub(snapshotTimestamp) > params.MaxSnapshotAge {
		return moduletypes.ErrSnapshotTooOld
	}

	return k.SetProviderSnapshot(ctx, vtypes.ProviderSnapshotRecord{
		Provider:           provider.String(),
		SnapshotHash:       snapshotHash,
		ResourceSummary:    resources,
		PostedAt:           ctx.BlockTime(),
		SnapshotTimestamp:  snapshotTimestamp,
		ComplianceDeadline: ctx.BlockTime().Add(params.SnapshotHashInterval),
		Suspended:          false,
	})
}

func (k *keeper) OpenAuditEscrow(
	ctx sdk.Context,
	provider sdk.AccAddress,
	tier vtypes.VerificationTier,
	capabilities []vtypes.CapabilityFlag,
	fee sdk.Coin,
	providerDeposit sdk.Coin,
	expiresAt time.Time,
	metadataHash []byte,
) (uint64, error) {
	if !k.providerExists(ctx, provider) {
		return 0, moduletypes.ErrProviderNotRegistered
	}
	if err := validateMsgTier(tier); err != nil {
		return 0, err
	}
	if err := validateCapabilities(capabilities); err != nil {
		return 0, err
	}
	params := k.GetParams(ctx)
	if err := requireCoinAtLeast(fee, minFeeForTier(params, tier), moduletypes.ErrInsufficientAuditFee); err != nil {
		return 0, err
	}
	if err := requireCoinAtLeast(providerDeposit, params.ProviderAuditDeposit, moduletypes.ErrInsufficientProviderDeposit); err != nil {
		return 0, err
	}
	if !expiresAt.After(ctx.BlockTime()) {
		return 0, errorsmod.Wrap(moduletypes.ErrInvalidReason, "audit escrow expiry must be in the future")
	}

	if err := k.sendAccountToModule(ctx, provider, fee); err != nil {
		return 0, err
	}
	if err := k.sendAccountToModule(ctx, provider, providerDeposit); err != nil {
		return 0, err
	}

	id := k.NextAuditEscrowID(ctx)
	return id, k.SetAuditEscrow(ctx, vtypes.AuditEscrowRecord{
		ID:                    id,
		Provider:              provider.String(),
		RequestedTier:         tier,
		RequestedCapabilities: capabilities,
		Fee:                   fee,
		FeeStatus:             vtypes.FeeStatusEscrowed,
		ProviderDeposit:       providerDeposit,
		ProviderDepositStatus: vtypes.ProviderDepositStatusEscrowed,
		Status:                vtypes.AuditEscrowStatusOpen,
		OpenedAt:              ctx.BlockTime(),
		ExpiresAt:             expiresAt,
		MetadataHash:          metadataHash,
	})
}

func (k *keeper) CancelAuditEscrow(ctx sdk.Context, provider sdk.AccAddress, auditEscrowID uint64) error {
	escrow, found := k.GetAuditEscrow(ctx, auditEscrowID)
	if !found {
		return moduletypes.ErrAuditEscrowNotFound
	}
	if escrow.Provider != provider.String() {
		return moduletypes.ErrUnauthorizedAuditEscrowSettlement
	}
	if escrow.Status != vtypes.AuditEscrowStatusOpen || escrow.ConsumedByAuditor != "" || escrow.ConsumedAt != nil || !escrow.ExpiresAt.After(ctx.BlockTime()) {
		return moduletypes.ErrAuditEscrowNotConsumable
	}
	if err := requireEscrowedAuditEscrowFunds(escrow); err != nil {
		return err
	}

	result, err := k.Settle(SettlementInput{
		Path:              SettlementPathAuditEscrow,
		AuditEscrowReason: vtypes.AuditEscrowSettlementReasonCancelledUnconsumed,
		FaultAttribution:  vtypes.FaultAttributionNoFault,
	})
	if err != nil {
		return err
	}
	if err = k.settleAuditEscrowFunds(ctx, provider, escrow, result); err != nil {
		return err
	}

	escrow.Status = vtypes.AuditEscrowStatusCancelled
	escrow.FeeStatus = result.FeeStatus
	escrow.ProviderDepositStatus = result.ProviderDepositStatus
	escrow.SettlementReason = vtypes.AuditEscrowSettlementReasonCancelledUnconsumed
	escrow.FaultAttribution = vtypes.FaultAttributionNoFault
	return k.SetAuditEscrow(ctx, escrow)
}

func (k *keeper) SettleAuditEscrow(
	ctx sdk.Context,
	authority string,
	auditEscrowID uint64,
	reason vtypes.AuditEscrowSettlementReason,
	fault vtypes.FaultAttribution,
	evidenceHash []byte,
) error {
	if k.authority != "" && authority != k.authority {
		return errorsmod.Wrapf(moduletypes.ErrUnauthorizedAuditEscrowSettlement, "invalid authority %s", authority)
	}
	if err := validateHash(evidenceHash, "evidence hash"); err != nil {
		return err
	}

	escrow, found := k.GetAuditEscrow(ctx, auditEscrowID)
	if !found {
		return moduletypes.ErrAuditEscrowNotFound
	}
	if escrow.Status != vtypes.AuditEscrowStatusOpen || escrow.ConsumedByAuditor != "" || escrow.ConsumedAt != nil {
		return moduletypes.ErrAuditEscrowNotConsumable
	}
	if reason != vtypes.AuditEscrowSettlementReasonProviderFault &&
		reason != vtypes.AuditEscrowSettlementReasonNoFault {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "governance audit escrow settlement requires provider fault or no fault")
	}
	if err := requireEscrowedAuditEscrowFunds(escrow); err != nil {
		return err
	}

	result, err := k.Settle(SettlementInput{
		Path:              SettlementPathAuditEscrow,
		AuditEscrowReason: reason,
		FaultAttribution:  fault,
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

	escrow.Status = auditEscrowStatusForSettlement(reason)
	escrow.FeeStatus = result.FeeStatus
	escrow.ProviderDepositStatus = result.ProviderDepositStatus
	escrow.SettlementReason = reason
	escrow.FaultAttribution = fault
	return k.SetAuditEscrow(ctx, escrow)
}

func (k *keeper) SubmitAttestation(
	ctx sdk.Context,
	provider sdk.AccAddress,
	auditor sdk.AccAddress,
	tier vtypes.VerificationTier,
	capabilities []vtypes.CapabilityFlag,
	evidenceHash []byte,
	fee sdk.Coin,
	deposit sdk.Coin,
	auditEscrowID uint64,
) error {
	if !k.providerExists(ctx, provider) {
		return moduletypes.ErrProviderNotRegistered
	}
	if provider.Equals(auditor) {
		return moduletypes.ErrSelfAttestation
	}
	if err := validateMsgTier(tier); err != nil {
		return err
	}
	if err := validateCapabilities(capabilities); err != nil {
		return err
	}
	if err := validateHash(evidenceHash, "evidence hash"); err != nil {
		return err
	}

	params := k.GetParams(ctx)
	auditorRecord, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}
	if auditorRecord.Status != vtypes.AuditorStatusActive {
		return moduletypes.ErrAuditorNotActive
	}
	if auditorRecord.BondStatus == vtypes.BondStatusFrozen {
		return moduletypes.ErrAuditorFrozen
	}
	if !vtypes.TierAtLeast(auditorRecord.MaxAttestationTier, tier) {
		return moduletypes.ErrAuditorUnauthorizedTier
	}
	if err := requireCoinAtLeast(auditorRecord.BondAmount, vtypes.MinBondForTier(params, tier), moduletypes.ErrInsufficientAuditorBond); err != nil {
		return err
	}
	if err := requireCoinAtLeast(deposit, params.AttestationDeposit, moduletypes.ErrInsufficientDeposit); err != nil {
		return err
	}

	escrow, found := k.GetAuditEscrow(ctx, auditEscrowID)
	if !found {
		return moduletypes.ErrAuditEscrowNotFound
	}
	if escrow.Status != vtypes.AuditEscrowStatusOpen || !escrow.ExpiresAt.After(ctx.BlockTime()) {
		return moduletypes.ErrAuditEscrowNotConsumable
	}
	if escrow.Provider != provider.String() || !vtypes.TierAtLeast(tier, escrow.RequestedTier) {
		return moduletypes.ErrAuditEscrowNotConsumable
	}
	if !capabilitiesInclude(capabilities, escrow.RequestedCapabilities) {
		return moduletypes.ErrMissingCapability
	}
	if !coinsEqual(fee, escrow.Fee) {
		return moduletypes.ErrInsufficientAuditFee
	}
	if err := k.validateProviderPrerequisites(ctx, provider, tier); err != nil {
		return err
	}
	if err := k.settleReplacedAttestation(ctx, provider, auditor); err != nil {
		return err
	}
	if err := k.sendAccountToModule(ctx, auditor, deposit); err != nil {
		return err
	}

	now := ctx.BlockTime()
	escrow.Status = vtypes.AuditEscrowStatusConsumed
	escrow.ConsumedByAuditor = auditor.String()
	escrow.ConsumedAt = &now
	if err := k.SetAuditEscrow(ctx, escrow); err != nil {
		return err
	}

	attestation := vtypes.AttestationRecord{
		Provider:      provider.String(),
		Auditor:       auditor.String(),
		Tier:          tier,
		Capabilities:  capabilities,
		EvidenceHash:  evidenceHash,
		Fee:           fee,
		FeeStatus:     vtypes.FeeStatusEscrowed,
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttlForTier(params, tier)),
		Status:        vtypes.AttestationStatusValid,
		Deposit:       deposit,
		DepositStatus: vtypes.DepositStatusEscrowed,
		AuditEscrowID: auditEscrowID,
	}

	return k.setAttestationWithDiscrepancyCheck(ctx, attestation, auditorRecord)
}

func (k *keeper) settleReplacedAttestation(ctx sdk.Context, provider, auditor sdk.AccAddress) error {
	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found || attestation.Status != vtypes.AttestationStatusValid {
		return nil
	}

	result, err := k.Settle(SettlementInput{
		Path:             SettlementPathReplacement,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})
	if err != nil {
		return err
	}
	return k.settleAttestationFunds(ctx, provider, auditor, attestation, result)
}

func (k *keeper) RevokeAttestation(
	ctx sdk.Context,
	provider sdk.AccAddress,
	auditor sdk.AccAddress,
	reason vtypes.AttestationRevocationReason,
	evidenceHash []byte,
) error {
	if err := validateHash(evidenceHash, "evidence hash"); err != nil {
		return err
	}

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}
	if attestation.Status != vtypes.AttestationStatusValid {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "attestation is not valid")
	}

	fault, err := revocationFaultAttribution(reason)
	if err != nil {
		return err
	}
	result, err := k.Settle(SettlementInput{
		Path:             SettlementPathAttestationRevoked,
		RevocationReason: reason,
		FaultAttribution: fault,
	})
	if err != nil {
		return err
	}
	if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return err
	}

	attestation.Status = vtypes.AttestationStatusRevoked
	attestation.FeeStatus = result.FeeStatus
	attestation.DepositStatus = result.DepositStatus
	attestation.FaultAttribution = fault
	return k.SetAttestation(ctx, attestation)
}

func (k *keeper) RemoveAttestation(ctx sdk.Context, provider sdk.AccAddress, auditor sdk.AccAddress) error {
	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}
	if attestation.Status != vtypes.AttestationStatusValid {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "attestation is not valid")
	}

	result, err := k.Settle(SettlementInput{
		Path:             SettlementPathAttestationRemoved,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})
	if err != nil {
		return err
	}
	if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return err
	}

	attestation.Status = vtypes.AttestationStatusRemoved
	attestation.FeeStatus = result.FeeStatus
	attestation.DepositStatus = result.DepositStatus
	attestation.FaultAttribution = vtypes.FaultAttributionNoFault
	return k.SetAttestation(ctx, attestation)
}

func revocationFaultAttribution(reason vtypes.AttestationRevocationReason) (vtypes.FaultAttribution, error) {
	switch reason {
	case vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
		vtypes.AttestationRevocationReasonSnapshotMismatch,
		vtypes.AttestationRevocationReasonSoftwareIdentityChanged,
		vtypes.AttestationRevocationReasonCapabilityMisrepresented,
		vtypes.AttestationRevocationReasonProviderNonResponsive:
		return vtypes.FaultAttributionProviderFault, nil
	case vtypes.AttestationRevocationReasonAuditorEvidenceError:
		return vtypes.FaultAttributionAuditorFault, nil
	case vtypes.AttestationRevocationReasonAuditorOperationalExit:
		return vtypes.FaultAttributionNoFault, nil
	case vtypes.AttestationRevocationReasonUnspecified:
		return vtypes.FaultAttributionUnspecified, errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified attestation revocation reason")
	default:
		return vtypes.FaultAttributionUnspecified, errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown attestation revocation reason %d", reason)
	}
}

func (k *keeper) setAttestationWithDiscrepancyCheck(ctx sdk.Context, attestation vtypes.AttestationRecord, auditorRecord vtypes.AuditorRecord) error {
	provider, err := sdk.AccAddressFromBech32(attestation.Provider)
	if err != nil {
		return err
	}
	if _, err = sdk.AccAddressFromBech32(attestation.Auditor); err != nil {
		return err
	}

	params := k.GetParams(ctx)
	conflicts := make([]vtypes.AttestationRecord, 0)
	bestBefore := vtypes.TierUnspecified
	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		if vtypes.TierBetter(record.Tier, bestBefore) {
			bestBefore = record.Tier
		}
		if record.Auditor == attestation.Auditor {
			return false
		}
		if tierDifference(record.Tier, attestation.Tier) > int32(params.DiscrepancyThreshold) {
			conflicts = append(conflicts, record)
		}
		return false
	})

	if len(conflicts) == 0 {
		return k.SetAttestation(ctx, attestation)
	}

	discrepancyIDs := make([]uint64, 0, len(conflicts))
	attestation.Status = vtypes.AttestationStatusVoided
	attestation.VoidedReason = vtypes.VoidedReasonDiscrepancy
	attestation.DepositStatus = vtypes.DepositStatusPendingDiscrepancy

	if err = k.SetAttestation(ctx, attestation); err != nil {
		return err
	}
	if err = k.freezeAuditorForDiscrepancy(ctx, auditorRecord); err != nil {
		return err
	}

	for _, conflict := range conflicts {
		conflictAuditor, err := sdk.AccAddressFromBech32(conflict.Auditor)
		if err != nil {
			return err
		}
		conflict.Status = vtypes.AttestationStatusVoided
		conflict.VoidedReason = vtypes.VoidedReasonDiscrepancy
		conflict.DepositStatus = vtypes.DepositStatusPendingDiscrepancy
		if err = k.SetAttestation(ctx, conflict); err != nil {
			return err
		}

		conflictAuditorRecord, found := k.GetAuditor(ctx, conflictAuditor)
		if !found {
			return moduletypes.ErrAuditorNotFound
		}
		if err = k.freezeAuditorForDiscrepancy(ctx, conflictAuditorRecord); err != nil {
			return err
		}

		id := k.NextDiscrepancyID(ctx)
		k.SetDiscrepancy(ctx, vtypes.DiscrepancyEvent{
			ID:               id,
			Provider:         attestation.Provider,
			AuditorA:         conflict.Auditor,
			AuditorATier:     conflict.Tier,
			AuditorB:         attestation.Auditor,
			AuditorBTier:     attestation.Tier,
			Timestamp:        ctx.BlockTime(),
			ResolutionStatus: vtypes.DiscrepancyStatusPending,
		})
		discrepancyIDs = append(discrepancyIDs, id)
	}

	bestAfter := vtypes.TierUnspecified
	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		if vtypes.TierBetter(record.Tier, bestAfter) {
			bestAfter = record.Tier
		}
		return false
	})

	if vtypes.TierBetter(bestBefore, bestAfter) {
		graceID, err := k.upsertProviderVerificationGrace(ctx, provider, bestBefore, discrepancyIDs)
		if err != nil {
			return err
		}
		for _, id := range discrepancyIDs {
			discrepancy, found := k.GetDiscrepancy(ctx, id)
			if !found {
				return moduletypes.ErrDiscrepancyNotFound
			}
			discrepancy.GraceRecordID = graceID
			k.SetDiscrepancy(ctx, discrepancy)
		}
	}
	return nil
}

func (k *keeper) freezeAuditorForDiscrepancy(ctx sdk.Context, record vtypes.AuditorRecord) error {
	record.BondStatus = vtypes.BondStatusFrozen
	record.DiscrepancyCount++
	return k.SetAuditor(ctx, record)
}

func (k *keeper) upsertProviderVerificationGrace(ctx sdk.Context, provider sdk.AccAddress, preservedTier vtypes.VerificationTier, discrepancyIDs []uint64) (uint64, error) {
	var active *vtypes.ProviderVerificationGraceRecord
	k.WithProviderVerificationGraces(ctx, provider, func(record vtypes.ProviderVerificationGraceRecord) bool {
		if record.Status == vtypes.VerificationGraceStatusActive {
			active = &record
			return true
		}
		return false
	})

	if active != nil {
		if vtypes.TierBetter(preservedTier, active.PreservedTier) {
			active.PreservedTier = preservedTier
		}
		active.SourceDiscrepancyIDs = append(active.SourceDiscrepancyIDs, discrepancyIDs...)
		return active.ID, k.SetProviderVerificationGrace(ctx, *active)
	}

	params := k.GetParams(ctx)
	id := k.NextGraceRecordID(ctx)
	return id, k.SetProviderVerificationGrace(ctx, vtypes.ProviderVerificationGraceRecord{
		ID:                   id,
		Provider:             provider.String(),
		PreservedTier:        preservedTier,
		SourceDiscrepancyIDs: discrepancyIDs,
		StartedAt:            ctx.BlockTime(),
		ExpiresAt:            ctx.BlockTime().Add(params.DiscrepancyGracePeriod),
		Status:               vtypes.VerificationGraceStatusActive,
	})
}

func tierDifference(a, b vtypes.VerificationTier) int32 {
	diff := int32(a) - int32(b)
	if diff < 0 {
		return -diff
	}
	return diff
}

func (k *keeper) ResolveDiscrepancy(
	ctx sdk.Context,
	authority string,
	discrepancyID uint64,
	vindicatedAuditor string,
	slashAuditorA bool,
	slashAuditorB bool,
	reason vtypes.DiscrepancyResolutionReason,
	fault vtypes.FaultAttribution,
	evidenceHash []byte,
) error {
	if k.authority != "" && authority != k.authority {
		return govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", k.authority, authority)
	}
	if err := validateHash(evidenceHash, "evidence hash"); err != nil {
		return err
	}

	discrepancy, found := k.GetDiscrepancy(ctx, discrepancyID)
	if !found {
		return moduletypes.ErrDiscrepancyNotFound
	}
	if discrepancy.ResolutionStatus != vtypes.DiscrepancyStatusPending && discrepancy.ResolutionStatus != vtypes.DiscrepancyStatusTimedOut {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "discrepancy is not pending or timed out")
	}
	if err := validateVindicatedAuditor(discrepancy, vindicatedAuditor, reason); err != nil {
		return err
	}
	if _, err := k.Settle(SettlementInput{
		Path:              SettlementPathDiscrepancyResolved,
		DiscrepancyReason: reason,
		FaultAttribution:  fault,
	}); err != nil {
		return err
	}

	if discrepancy.ResolutionStatus == vtypes.DiscrepancyStatusPending {
		if err := k.settleDiscrepancyAttestations(ctx, discrepancy, reason); err != nil {
			return err
		}
	}

	auditorA, err := sdk.AccAddressFromBech32(discrepancy.AuditorA)
	if err != nil {
		return err
	}
	auditorB, err := sdk.AccAddressFromBech32(discrepancy.AuditorB)
	if err != nil {
		return err
	}
	if err = k.resolveDiscrepancyAuditorBond(ctx, auditorA, discrepancy.ID, slashAuditorA); err != nil {
		return err
	}
	if err = k.resolveDiscrepancyAuditorBond(ctx, auditorB, discrepancy.ID, slashAuditorB); err != nil {
		return err
	}

	if discrepancy.GraceRecordID != 0 && fault == vtypes.FaultAttributionProviderFault {
		if grace, found := k.getProviderVerificationGraceByID(ctx, discrepancy.GraceRecordID); found && grace.Status == vtypes.VerificationGraceStatusActive {
			grace.Status = vtypes.VerificationGraceStatusTerminated
			if err = k.SetProviderVerificationGrace(ctx, grace); err != nil {
				return err
			}
		}
	}

	discrepancy.ResolutionStatus = vtypes.DiscrepancyStatusResolved
	discrepancy.ResolutionReason = reason
	discrepancy.FaultAttribution = fault
	discrepancy.ResolutionEvidenceHash = evidenceHash
	k.SetDiscrepancy(ctx, discrepancy)
	return nil
}

func (k *keeper) validateProviderPrerequisites(ctx sdk.Context, provider sdk.AccAddress, tier vtypes.VerificationTier) error {
	if !vtypes.TierRequiresSnapshot(tier) {
		return nil
	}

	params := k.GetParams(ctx)
	if err := k.validateProviderAge(ctx, provider, tier, params); err != nil {
		return err
	}

	snapshot, found := k.GetProviderSnapshot(ctx, provider)
	if !found {
		return moduletypes.ErrSnapshotNonCompliant
	}
	if snapshot.Suspended || snapshot.ComplianceDeadline.Before(ctx.BlockTime()) {
		return moduletypes.ErrProviderSnapshotSuspended
	}

	if vtypes.TierRequiresProviderBond(tier) {
		bond, found := k.GetProviderBond(ctx, provider)
		if !found {
			return moduletypes.ErrInsufficientProviderBond
		}
		required := requiredProviderBond(params, tier, snapshot.ResourceSummary)
		if err := requireCoinAtLeast(bond.BondedAmount, required, moduletypes.ErrInsufficientProviderBond); err != nil {
			return err
		}
	}

	return nil
}

func (k *keeper) validateProviderAge(ctx sdk.Context, provider sdk.AccAddress, tier vtypes.VerificationTier, params vtypes.Params) error {
	if k.provider == nil {
		return nil
	}

	registeredAt, found := k.provider.GetRegistrationTime(ctx, provider)
	if !found || registeredAt.IsZero() {
		return errorsmod.Wrap(moduletypes.ErrInsufficientProviderAge, "provider registration time not found")
	}

	minAge := minProviderAgeForTier(params, tier)
	if minAge == 0 {
		return nil
	}
	if registeredAt.After(ctx.BlockTime().Add(-minAge)) {
		return errorsmod.Wrapf(moduletypes.ErrInsufficientProviderAge, "registered_at %s requires minimum age %s", registeredAt.UTC().Format(time.RFC3339), minAge)
	}

	return nil
}

func (k *keeper) providerExists(ctx sdk.Context, provider sdk.AccAddress) bool {
	if k.provider == nil {
		return true
	}
	_, found := k.provider.Get(ctx, provider)
	return found
}

func (k *keeper) sendAccountToModule(ctx sdk.Context, addr sdk.AccAddress, coin sdk.Coin) error {
	if k.bank == nil || coin.IsZero() {
		return nil
	}
	return k.bank.SendCoinsFromAccountToModule(ctx, addr, moduletypes.ModuleName, sdk.NewCoins(coin))
}

func (k *keeper) sendModuleToAccount(ctx sdk.Context, addr sdk.AccAddress, coin sdk.Coin) error {
	if k.bank == nil || coin.IsZero() {
		return nil
	}
	return k.bank.SendCoinsFromModuleToAccount(ctx, moduletypes.ModuleName, addr, sdk.NewCoins(coin))
}

func (k *keeper) sendModuleToDistribution(ctx sdk.Context, coin sdk.Coin) error {
	if k.bank == nil || coin.IsZero() {
		return nil
	}
	return k.bank.SendCoinsFromModuleToModule(ctx, moduletypes.ModuleName, distrtypes.ModuleName, sdk.NewCoins(coin))
}

func (k *keeper) settleAuditEscrowFunds(ctx sdk.Context, provider sdk.AccAddress, escrow vtypes.AuditEscrowRecord, result SettlementResult) error {
	switch result.FeeStatus {
	case vtypes.FeeStatusReturnedToProvider:
		if err := k.sendModuleToAccount(ctx, provider, escrow.Fee); err != nil {
			return err
		}
	case vtypes.FeeStatusEscrowed, vtypes.FeeStatusReleasedToAuditor, vtypes.FeeStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported audit escrow fee settlement %s", result.FeeStatus)
	}

	switch result.ProviderDepositStatus {
	case vtypes.ProviderDepositStatusReturnedToProvider:
		return k.sendModuleToAccount(ctx, provider, escrow.ProviderDeposit)
	case vtypes.ProviderDepositStatusSlashed:
		return k.sendModuleToDistribution(ctx, escrow.ProviderDeposit)
	case vtypes.ProviderDepositStatusEscrowed, vtypes.ProviderDepositStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported audit escrow deposit settlement %s", result.ProviderDepositStatus)
	}

	return nil
}

func (k *keeper) settleAttestationFunds(ctx sdk.Context, provider, auditor sdk.AccAddress, attestation vtypes.AttestationRecord, result SettlementResult) error {
	if err := requireEscrowedAttestationFunds(attestation); err != nil {
		return err
	}

	switch result.FeeStatus {
	case vtypes.FeeStatusReleasedToAuditor:
		if err := k.sendModuleToAccount(ctx, auditor, attestation.Fee); err != nil {
			return err
		}
	case vtypes.FeeStatusReturnedToProvider:
		if err := k.sendModuleToAccount(ctx, provider, attestation.Fee); err != nil {
			return err
		}
	case vtypes.FeeStatusEscrowed, vtypes.FeeStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported attestation fee settlement %s", result.FeeStatus)
	}

	switch result.DepositStatus {
	case vtypes.DepositStatusReturnedToAuditor:
		return k.sendModuleToAccount(ctx, auditor, attestation.Deposit)
	case vtypes.DepositStatusSlashed:
		return k.sendModuleToDistribution(ctx, attestation.Deposit)
	case vtypes.DepositStatusEscrowed, vtypes.DepositStatusPendingDiscrepancy, vtypes.DepositStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported attestation deposit settlement %s", result.DepositStatus)
	}

	return nil
}

func (k *keeper) settlePendingDiscrepancyAttestationFunds(ctx sdk.Context, provider, auditor sdk.AccAddress, attestation vtypes.AttestationRecord, result SettlementResult) error {
	if attestation.FeeStatus != vtypes.FeeStatusEscrowed || attestation.DepositStatus != vtypes.DepositStatusPendingDiscrepancy {
		return moduletypes.ErrInvalidReason
	}

	switch result.FeeStatus {
	case vtypes.FeeStatusReleasedToAuditor:
		if err := k.sendModuleToAccount(ctx, auditor, attestation.Fee); err != nil {
			return err
		}
	case vtypes.FeeStatusReturnedToProvider:
		if err := k.sendModuleToAccount(ctx, provider, attestation.Fee); err != nil {
			return err
		}
	case vtypes.FeeStatusEscrowed, vtypes.FeeStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported pending discrepancy fee settlement %s", result.FeeStatus)
	}

	switch result.DepositStatus {
	case vtypes.DepositStatusReturnedToAuditor:
		return k.sendModuleToAccount(ctx, auditor, attestation.Deposit)
	case vtypes.DepositStatusSlashed:
		return k.sendModuleToDistribution(ctx, attestation.Deposit)
	case vtypes.DepositStatusEscrowed, vtypes.DepositStatusPendingDiscrepancy, vtypes.DepositStatusUnspecified:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported pending discrepancy deposit settlement %s", result.DepositStatus)
	}

	return nil
}

func (k *keeper) settleDiscrepancyAttestations(ctx sdk.Context, discrepancy vtypes.DiscrepancyEvent, reason vtypes.DiscrepancyResolutionReason) error {
	provider, err := sdk.AccAddressFromBech32(discrepancy.Provider)
	if err != nil {
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

	attestationA, found := k.GetAttestation(ctx, provider, auditorA)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}
	attestationB, found := k.GetAttestation(ctx, provider, auditorB)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}

	resultA, resultB, err := discrepancyAttestationSettlementResults(reason)
	if err != nil {
		return err
	}
	if err = k.settlePendingDiscrepancyAttestationFunds(ctx, provider, auditorA, attestationA, resultA); err != nil {
		return err
	}
	if err = k.settlePendingDiscrepancyAttestationFunds(ctx, provider, auditorB, attestationB, resultB); err != nil {
		return err
	}

	attestationA.FeeStatus = resultA.FeeStatus
	attestationA.DepositStatus = resultA.DepositStatus
	attestationB.FeeStatus = resultB.FeeStatus
	attestationB.DepositStatus = resultB.DepositStatus

	if err = k.SetAttestation(ctx, attestationA); err != nil {
		return err
	}
	return k.SetAttestation(ctx, attestationB)
}

func discrepancyAttestationSettlementResults(reason vtypes.DiscrepancyResolutionReason) (SettlementResult, SettlementResult, error) {
	released := SettlementResult{
		FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
		DepositStatus: vtypes.DepositStatusReturnedToAuditor,
	}
	slashed := SettlementResult{
		FeeStatus:     vtypes.FeeStatusReturnedToProvider,
		DepositStatus: vtypes.DepositStatusSlashed,
	}

	switch reason {
	case vtypes.DiscrepancyResolutionReasonAuditorACorrect:
		return released, slashed, nil
	case vtypes.DiscrepancyResolutionReasonAuditorBCorrect:
		return slashed, released, nil
	case vtypes.DiscrepancyResolutionReasonProviderFault,
		vtypes.DiscrepancyResolutionReasonEvidenceInconclusive:
		return released, released, nil
	case vtypes.DiscrepancyResolutionReasonBothAuditorsWrong,
		vtypes.DiscrepancyResolutionReasonSharedFault,
		vtypes.DiscrepancyResolutionReasonGovernanceTimeoutReview:
		return slashed, slashed, nil
	default:
		return SettlementResult{}, SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown discrepancy resolution reason %d", reason)
	}
}

func validateVindicatedAuditor(discrepancy vtypes.DiscrepancyEvent, vindicatedAuditor string, reason vtypes.DiscrepancyResolutionReason) error {
	if vindicatedAuditor != "" && vindicatedAuditor != discrepancy.AuditorA && vindicatedAuditor != discrepancy.AuditorB {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "vindicated auditor must be auditor A, auditor B, or empty")
	}

	switch reason {
	case vtypes.DiscrepancyResolutionReasonAuditorACorrect:
		if vindicatedAuditor != discrepancy.AuditorA {
			return errorsmod.Wrap(moduletypes.ErrInvalidReason, "auditor A resolution must vindicate auditor A")
		}
	case vtypes.DiscrepancyResolutionReasonAuditorBCorrect:
		if vindicatedAuditor != discrepancy.AuditorB {
			return errorsmod.Wrap(moduletypes.ErrInvalidReason, "auditor B resolution must vindicate auditor B")
		}
	case vtypes.DiscrepancyResolutionReasonBothAuditorsWrong,
		vtypes.DiscrepancyResolutionReasonSharedFault,
		vtypes.DiscrepancyResolutionReasonGovernanceTimeoutReview:
		if vindicatedAuditor != "" {
			return errorsmod.Wrap(moduletypes.ErrInvalidReason, "resolution reason cannot vindicate an auditor")
		}
	}
	return nil
}

func (k *keeper) resolveDiscrepancyAuditorBond(ctx sdk.Context, auditor sdk.AccAddress, discrepancyID uint64, slash bool) error {
	record, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}

	if slash {
		if err := k.sendModuleToDistribution(ctx, record.BondAmount); err != nil {
			return err
		}
		record.BondAmount = sdk.NewCoin(record.BondAmount.Denom, math.ZeroInt())
		record.BondStatus = vtypes.BondStatusUnspecified
		return k.SetAuditor(ctx, record)
	}

	if !k.auditorHasOtherPendingDiscrepancy(ctx, auditor.String(), discrepancyID) && record.BondStatus == vtypes.BondStatusFrozen {
		record.BondStatus = vtypes.BondStatusBonded
	}
	return k.SetAuditor(ctx, record)
}

func (k *keeper) auditorHasOtherPendingDiscrepancy(ctx sdk.Context, auditor string, currentID uint64) bool {
	found := false
	k.WithDiscrepancies(ctx, vtypes.DiscrepancyStatusPending, func(record vtypes.DiscrepancyEvent) bool {
		if record.ID != currentID && (record.AuditorA == auditor || record.AuditorB == auditor) {
			found = true
			return true
		}
		return false
	})
	return found
}

func requireEscrowedAuditEscrowFunds(escrow vtypes.AuditEscrowRecord) error {
	if escrow.FeeStatus != vtypes.FeeStatusEscrowed || escrow.ProviderDepositStatus != vtypes.ProviderDepositStatusEscrowed {
		return moduletypes.ErrAuditEscrowNotConsumable
	}
	return nil
}

func requireEscrowedAttestationFunds(attestation vtypes.AttestationRecord) error {
	if attestation.FeeStatus != vtypes.FeeStatusEscrowed || attestation.DepositStatus != vtypes.DepositStatusEscrowed {
		return moduletypes.ErrInvalidReason
	}
	return nil
}

func auditEscrowStatusForSettlement(reason vtypes.AuditEscrowSettlementReason) vtypes.AuditEscrowStatus {
	switch reason {
	case vtypes.AuditEscrowSettlementReasonCancelledUnconsumed:
		return vtypes.AuditEscrowStatusCancelled
	case vtypes.AuditEscrowSettlementReasonExpiredUnconsumed:
		return vtypes.AuditEscrowStatusExpired
	default:
		return vtypes.AuditEscrowStatusSettled
	}
}

func validatePositiveCoin(coin sdk.Coin) error {
	if !coin.IsValid() || !coin.IsPositive() {
		return moduletypes.ErrInsufficientDeposit
	}
	return nil
}

func validateMsgTier(tier vtypes.VerificationTier) error {
	switch tier {
	case vtypes.TierIdentified, vtypes.TierVerified, vtypes.TierEstablished, vtypes.TierTrusted:
		return nil
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "invalid tier %d", tier)
	}
}

func validateCapabilities(capabilities []vtypes.CapabilityFlag) error {
	seen := make(map[vtypes.CapabilityFlag]struct{}, len(capabilities))
	for _, capability := range capabilities {
		switch capability {
		case vtypes.CapabilityTEEHardwareAttestation,
			vtypes.CapabilityConfidentialComputing,
			vtypes.CapabilityPersistentStorage,
			vtypes.CapabilityBareMetal:
		default:
			return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "invalid capability %d", capability)
		}

		if _, found := seen[capability]; found {
			return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "duplicate capability %s", capability)
		}
		seen[capability] = struct{}{}
	}

	return nil
}

func validateHash(hash []byte, name string) error {
	if len(hash) != sha256.Size {
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "%s must be %d bytes", name, sha256.Size)
	}
	return nil
}

func addCoin(a, b sdk.Coin) (sdk.Coin, error) {
	if a.IsNil() || a.IsZero() {
		return b, nil
	}
	if !coinsSameDenom(a, b) {
		return sdk.Coin{}, errorsmod.Wrapf(moduletypes.ErrInvalidReason, "denom mismatch %s/%s", a.Denom, b.Denom)
	}
	return a.Add(b), nil
}

func requireCoinAtLeast(got, want sdk.Coin, err error) error {
	if !coinsSameDenom(got, want) || got.Amount.LT(want.Amount) {
		return err
	}
	return nil
}

func coinsSameDenom(a, b sdk.Coin) bool {
	return a.Denom == b.Denom && !a.IsNil() && !b.IsNil()
}

func coinsEqual(a, b sdk.Coin) bool {
	return coinsSameDenom(a, b) && a.Amount.Equal(b.Amount)
}

func minFeeForTier(params vtypes.Params, tier vtypes.VerificationTier) sdk.Coin {
	switch tier {
	case vtypes.TierIdentified:
		return params.MinFeeL1
	case vtypes.TierVerified:
		return params.MinFeeL2
	case vtypes.TierEstablished:
		return params.MinFeeL3
	case vtypes.TierTrusted:
		return params.MinFeeL4
	default:
		panic("verification: unknown tier")
	}
}

func ttlForTier(params vtypes.Params, tier vtypes.VerificationTier) time.Duration {
	switch tier {
	case vtypes.TierIdentified:
		return params.TtlL1
	case vtypes.TierVerified:
		return params.TtlL2
	case vtypes.TierEstablished:
		return params.TtlL3
	case vtypes.TierTrusted:
		return params.TtlL4
	default:
		panic("verification: unknown tier")
	}
}

func minProviderAgeForTier(params vtypes.Params, tier vtypes.VerificationTier) time.Duration {
	switch tier {
	case vtypes.TierIdentified:
		return 0
	case vtypes.TierVerified:
		return params.MinAgeL2
	case vtypes.TierEstablished:
		return params.MinAgeL3
	case vtypes.TierTrusted:
		return params.MinAgeL4
	default:
		panic("verification: unknown tier")
	}
}

func renewalPeriodForTier(params vtypes.Params, tier vtypes.VerificationTier) time.Duration {
	switch tier {
	case vtypes.TierIdentified:
		return params.RenewalPeriodL1
	case vtypes.TierVerified:
		return params.RenewalPeriodL2
	case vtypes.TierEstablished:
		return params.RenewalPeriodL3
	case vtypes.TierTrusted:
		return params.RenewalPeriodL4
	default:
		panic("verification: unknown tier")
	}
}

func capabilitiesInclude(got, required []vtypes.CapabilityFlag) bool {
	if len(required) == 0 {
		return true
	}
	present := make(map[vtypes.CapabilityFlag]struct{}, len(got))
	for _, capability := range got {
		present[capability] = struct{}{}
	}
	for _, capability := range required {
		if _, found := present[capability]; !found {
			return false
		}
	}
	return true
}

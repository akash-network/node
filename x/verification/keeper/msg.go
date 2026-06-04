package keeper

import (
	"crypto/sha256"
	"sort"
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
	record := vtypes.AuditorRecord{
		Address:            auditor.String(),
		Status:             vtypes.AuditorStatusPendingBond,
		MaxAttestationTier: tier,
		BondAmount:         sdk.NewCoin(params.BondL1.Denom, math.ZeroInt()),
		BondStatus:         vtypes.BondStatusNotBonded,
		MetadataHash:       metadataHash,
		RegisteredAt:       ctx.BlockTime(),
		RenewalDeadline:    ctx.BlockTime().Add(renewalPeriodForTier(params, tier)),
	}
	if err := k.SetAuditor(ctx, record); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditorRegistered{
		Auditor:            record.Address,
		MaxAttestationTier: record.MaxAttestationTier,
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
	if record.BondStatus != vtypes.BondStatusFrozen && record.BondStatus != vtypes.BondStatusUnbonding {
		record.BondStatus = vtypes.BondStatusNotBonded
		if coinAtLeast(bond, vtypes.MinBondForTier(k.GetParams(ctx), record.MaxAttestationTier)) {
			record.BondStatus = vtypes.BondStatusBonded
			if record.Status == vtypes.AuditorStatusPendingBond || record.Status == vtypes.AuditorStatusUnspecified {
				record.Status = vtypes.AuditorStatusActive
			}
		} else if record.Status == vtypes.AuditorStatusActive {
			record.Status = vtypes.AuditorStatusPendingBond
		}
	}
	if err := k.SetAuditor(ctx, record); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditorBondPosted{
		Auditor: auditor.String(),
		Amount:  amount,
	})
}

func (k *keeper) RenewAuditor(ctx sdk.Context, authority string, auditor sdk.AccAddress) error {
	if k.authority != "" && authority != k.authority {
		return errorsmod.Wrapf(moduletypes.ErrAuditorUnauthorizedTier, "invalid authority %s", authority)
	}

	record, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}
	if record.Status != vtypes.AuditorStatusActive && record.Status != vtypes.AuditorStatusLapsed {
		return moduletypes.ErrAuditorNotActive
	}

	params := k.GetParams(ctx)
	record.Status = vtypes.AuditorStatusActive
	record.RenewalDeadline = ctx.BlockTime().Add(renewalPeriodForTier(params, record.MaxAttestationTier))
	if err := k.SetAuditor(ctx, record); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditorRenewed{
		Auditor:     record.Address,
		NewDeadline: record.RenewalDeadline,
	})
}

func (k *keeper) RemoveAuditor(ctx sdk.Context, authority string, auditor sdk.AccAddress) error {
	if k.authority != "" && authority != k.authority {
		return errorsmod.Wrapf(moduletypes.ErrAuditorUnauthorizedTier, "invalid authority %s", authority)
	}

	record, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}
	return k.exitAuditor(ctx, record, vtypes.AuditorStatusRemoved)
}

func (k *keeper) ResignAuditor(ctx sdk.Context, auditor sdk.AccAddress) error {
	record, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}
	return k.exitAuditor(ctx, record, vtypes.AuditorStatusResigned)
}

func (k *keeper) exitAuditor(ctx sdk.Context, record vtypes.AuditorRecord, status vtypes.AuditorStatus) error {
	if record.Status != vtypes.AuditorStatusActive && record.Status != vtypes.AuditorStatusLapsed {
		return moduletypes.ErrAuditorNotActive
	}
	if record.BondStatus == vtypes.BondStatusFrozen {
		return moduletypes.ErrAuditorFrozen
	}

	record.Status = status
	record.RenewalDeadline = time.Time{}
	if record.BondAmount.IsNil() || record.BondAmount.IsZero() {
		record.BondStatus = vtypes.BondStatusNotBonded
		record.BondUnbondingCompletionTime = nil
	} else {
		completion := ctx.BlockTime().Add(k.GetParams(ctx).AuditorUnbondingPeriod)
		record.BondStatus = vtypes.BondStatusUnbonding
		record.BondUnbondingCompletionTime = &completion
	}

	if err := k.SetAuditor(ctx, record); err != nil {
		return err
	}
	switch status {
	case vtypes.AuditorStatusRemoved:
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditorRemoved{Auditor: record.Address})
	case vtypes.AuditorStatusResigned:
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditorResigned{Auditor: record.Address})
	default:
		return nil
	}
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
	if err = k.SetProviderBond(ctx, record); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventProviderBondPosted{
		Provider:    provider.String(),
		Amount:      amount,
		TotalBonded: bond,
	})
}

func (k *keeper) WithdrawProviderBond(ctx sdk.Context, provider sdk.AccAddress, amount sdk.Coin) error {
	if err := validatePositiveCoin(amount); err != nil {
		return err
	}
	if !k.providerExists(ctx, provider) {
		return moduletypes.ErrProviderNotRegistered
	}

	record, found := k.GetProviderBond(ctx, provider)
	if !found {
		return moduletypes.ErrInsufficientProviderBond
	}
	if !coinsSameDenom(record.BondedAmount, amount) || record.BondedAmount.Amount.LT(amount.Amount) {
		return moduletypes.ErrBondWithdrawalExceedsMinimum
	}

	remaining := sdk.NewCoin(record.BondedAmount.Denom, record.BondedAmount.Amount.Sub(amount.Amount))
	completion := ctx.BlockTime().Add(k.GetParams(ctx).ProviderBondUnbondingPeriod)
	record.BondedAmount = remaining
	record.UnbondingEntries = append(record.UnbondingEntries, vtypes.UnbondingEntry{
		Amount:         amount,
		CompletionTime: completion,
	})
	if err := k.SetProviderBond(ctx, record); err != nil {
		return err
	}

	ctx.KVStore(k.skey).Set(providerBondUnbondingQueueKey(completion, provider), []byte{})
	if err := k.voidUnsupportedProviderAttestations(ctx, provider, remaining, SettlementPathBondWithdrawn, vtypes.VoidedReasonBondWithdrawn); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventProviderBondWithdrawalInitiated{
		Provider:       provider.String(),
		Amount:         amount,
		CompletionTime: completion,
	})
}

func (k *keeper) SlashProviderBond(
	ctx sdk.Context,
	authority string,
	provider sdk.AccAddress,
	slashFraction math.LegacyDec,
	reason vtypes.ProviderBondSlashReason,
	evidenceHash []byte,
) error {
	if k.authority != "" && authority != k.authority {
		return govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", k.authority, authority)
	}
	if err := validateHash(evidenceHash, "evidence hash"); err != nil {
		return err
	}
	if slashFraction.IsNegative() || !slashFraction.IsPositive() || slashFraction.GT(math.LegacyNewDec(1)) {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "slash fraction must be in (0, 1]")
	}
	if !k.providerExists(ctx, provider) {
		return moduletypes.ErrProviderNotRegistered
	}

	record, found := k.GetProviderBond(ctx, provider)
	if !found {
		return moduletypes.ErrInsufficientProviderBond
	}
	if _, err := k.Settle(SettlementInput{
		Path:                    SettlementPathProviderBondSlash,
		ProviderBondSlashReason: reason,
		FaultAttribution:        vtypes.FaultAttributionProviderFault,
	}); err != nil {
		return err
	}

	slashAmount := slashFraction.MulInt(record.BondedAmount.Amount).TruncateInt()
	if !slashAmount.IsPositive() {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "slash amount must be positive")
	}

	slashedCoin := sdk.NewCoin(record.BondedAmount.Denom, slashAmount)
	remaining := sdk.NewCoin(record.BondedAmount.Denom, record.BondedAmount.Amount.Sub(slashAmount))
	if err := k.sendModuleToDistribution(ctx, slashedCoin); err != nil {
		return err
	}

	now := ctx.BlockTime()
	record.BondedAmount = remaining
	record.Slashed = true
	record.LastSlashTime = &now
	if err := k.SetProviderBond(ctx, record); err != nil {
		return err
	}
	if err := k.voidUnsupportedProviderAttestations(ctx, provider, remaining, SettlementPathBondSlashed, vtypes.VoidedReasonBondSlashed); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventProviderBondSlashed{
		Provider:      provider.String(),
		SlashedAmount: slashedCoin,
		Reason:        reason,
	})
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

	record := vtypes.ProviderSnapshotRecord{
		Provider:           provider.String(),
		SnapshotHash:       snapshotHash,
		ResourceSummary:    resources,
		PostedAt:           ctx.BlockTime(),
		SnapshotTimestamp:  snapshotTimestamp,
		ComplianceDeadline: ctx.BlockTime().Add(params.SnapshotHashInterval),
		Suspended:          false,
	}
	if err := k.SetProviderSnapshot(ctx, record); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventSnapshotHashPosted{
		Provider:           record.Provider,
		SnapshotHash:       record.SnapshotHash,
		ComplianceDeadline: record.ComplianceDeadline,
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
	record := vtypes.AuditEscrowRecord{
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
	}
	if err := k.SetAuditEscrow(ctx, record); err != nil {
		return 0, err
	}
	if err := ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditEscrowOpened{
		AuditEscrowID:   id,
		Provider:        provider.String(),
		Fee:             fee,
		ProviderDeposit: providerDeposit,
	}); err != nil {
		return 0, err
	}
	return id, nil
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
	if err = k.SetAuditEscrow(ctx, escrow); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    escrow.ID,
		Reason:           escrow.SettlementReason,
		FaultAttribution: escrow.FaultAttribution,
	})
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
	if err = k.SetAuditEscrow(ctx, escrow); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    escrow.ID,
		Reason:           reason,
		FaultAttribution: fault,
	})
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
	if auditorRecord.BondStatus != vtypes.BondStatusBonded {
		return moduletypes.ErrInsufficientAuditorBond
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
	if err := requireCoinAtLeast(fee, minFeeForTier(params, tier), moduletypes.ErrInsufficientAuditFee); err != nil {
		return err
	}
	if err := requireCoinAtLeast(escrow.Fee, fee, moduletypes.ErrInsufficientAuditFee); err != nil {
		return moduletypes.ErrInsufficientAuditFee
	}
	if err := k.validateProviderPrerequisites(ctx, provider, tier); err != nil {
		return err
	}
	replaced, hasReplacement, err := k.settleReplacedAttestation(ctx, provider, auditor)
	if err != nil {
		return err
	}
	if err := k.sendAccountToModule(ctx, auditor, deposit); err != nil {
		return err
	}

	now := ctx.BlockTime()
	if escrow.Fee.Amount.GT(fee.Amount) {
		refund := sdk.NewCoin(escrow.Fee.Denom, escrow.Fee.Amount.Sub(fee.Amount))
		if err := k.sendModuleToAccount(ctx, provider, refund); err != nil {
			return err
		}
		escrow.Fee = fee
	}
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

	if err = k.setAttestationWithDiscrepancyCheck(ctx, attestation, auditorRecord); err != nil {
		return err
	}
	if hasReplacement {
		if err = ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationReplaced{
			Provider:         attestation.Provider,
			Auditor:          attestation.Auditor,
			OldTier:          replaced.Tier,
			NewTier:          attestation.Tier,
			OldAuditEscrowID: replaced.AuditEscrowID,
			NewAuditEscrowID: attestation.AuditEscrowID,
		}); err != nil {
			return err
		}
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationSubmitted{
		Provider:      attestation.Provider,
		Auditor:       attestation.Auditor,
		Tier:          attestation.Tier,
		Capabilities:  attestation.Capabilities,
		ExpiresAt:     attestation.ExpiresAt,
		AuditEscrowID: attestation.AuditEscrowID,
	})
}

func (k *keeper) settleReplacedAttestation(ctx sdk.Context, provider, auditor sdk.AccAddress) (vtypes.AttestationRecord, bool, error) {
	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found || attestation.Status != vtypes.AttestationStatusValid {
		return vtypes.AttestationRecord{}, false, nil
	}

	result, err := k.Settle(SettlementInput{
		Path:             SettlementPathReplacement,
		FaultAttribution: vtypes.FaultAttributionNoFault,
	})
	if err != nil {
		return vtypes.AttestationRecord{}, false, err
	}
	if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return vtypes.AttestationRecord{}, false, err
	}
	return attestation, true, nil
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
	if err = k.SetAttestation(ctx, attestation); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationRevoked{
		Provider:  attestation.Provider,
		Auditor:   attestation.Auditor,
		Initiator: "auditor",
		Reason:    reason,
	})
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
	if err = requireEscrowedAttestationFunds(attestation); err != nil {
		return err
	}
	escrow, err := k.consumedAttestationEscrow(ctx, provider, auditor, attestation)
	if err != nil {
		return err
	}
	if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return err
	}
	if err = k.sendModuleToAccount(ctx, provider, escrow.ProviderDeposit); err != nil {
		return err
	}

	attestation.Status = vtypes.AttestationStatusRemoved
	attestation.FeeStatus = result.FeeStatus
	attestation.DepositStatus = result.DepositStatus
	attestation.FaultAttribution = vtypes.FaultAttributionNoFault
	escrow.Status = vtypes.AuditEscrowStatusSettled
	escrow.FeeStatus = result.FeeStatus
	escrow.ProviderDepositStatus = vtypes.ProviderDepositStatusReturnedToProvider
	escrow.SettlementReason = vtypes.AuditEscrowSettlementReasonNoFault
	escrow.FaultAttribution = vtypes.FaultAttributionNoFault
	if err = k.SetAuditEscrow(ctx, escrow); err != nil {
		return err
	}
	if err = k.SetAttestation(ctx, attestation); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAuditEscrowSettled{
		AuditEscrowID:    escrow.ID,
		Reason:           escrow.SettlementReason,
		FaultAttribution: escrow.FaultAttribution,
	})
}

func (k *keeper) RevokeProviderAttestation(
	ctx sdk.Context,
	authority string,
	provider sdk.AccAddress,
	auditor sdk.AccAddress,
	reason vtypes.GovernanceAttestationReason,
	fault vtypes.FaultAttribution,
	evidenceHash []byte,
) error {
	if err := k.validateGovernanceAttestationRevocation(authority, evidenceHash); err != nil {
		return err
	}

	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found {
		return moduletypes.ErrAttestationNotFound
	}
	if attestation.Status != vtypes.AttestationStatusValid {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "attestation is not valid")
	}

	result, err := k.Settle(SettlementInput{
		Path:                SettlementPathGovernanceAttestation,
		GovernanceReason:    reason,
		FaultAttribution:    fault,
		SlashAuditorDeposit: governanceRevocationSlashesAuditorDeposit(fault),
	})
	if err != nil {
		return err
	}
	return k.voidGovernanceAttestation(ctx, provider, auditor, attestation, result, fault)
}

func (k *keeper) RevokeAllProviderAttestations(
	ctx sdk.Context,
	authority string,
	provider sdk.AccAddress,
	reason vtypes.GovernanceAttestationReason,
	fault vtypes.FaultAttribution,
	evidenceHash []byte,
) error {
	if err := k.validateGovernanceAttestationRevocation(authority, evidenceHash); err != nil {
		return err
	}

	attestations := make([]vtypes.AttestationRecord, 0)
	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		attestations = append(attestations, record)
		return false
	})
	if len(attestations) == 0 {
		return moduletypes.ErrAttestationNotFound
	}

	result, err := k.Settle(SettlementInput{
		Path:                SettlementPathGovernanceAttestation,
		GovernanceReason:    reason,
		FaultAttribution:    fault,
		SlashAuditorDeposit: governanceRevocationSlashesAuditorDeposit(fault),
	})
	if err != nil {
		return err
	}

	for _, attestation := range attestations {
		auditor, err := sdk.AccAddressFromBech32(attestation.Auditor)
		if err != nil {
			return err
		}
		if err = k.voidGovernanceAttestation(ctx, provider, auditor, attestation, result, fault); err != nil {
			return err
		}
	}
	return nil
}

func (k *keeper) RevokeAuditorAttestations(
	ctx sdk.Context,
	authority string,
	auditor sdk.AccAddress,
	reason vtypes.GovernanceAttestationReason,
	fault vtypes.FaultAttribution,
	evidenceHash []byte,
) error {
	if err := k.validateGovernanceAttestationRevocation(authority, evidenceHash); err != nil {
		return err
	}

	auditorRecord, found := k.GetAuditor(ctx, auditor)
	if !found {
		return moduletypes.ErrAuditorNotFound
	}

	attestations := make([]vtypes.AttestationRecord, 0)
	k.WithAuditorAttestations(ctx, auditor, func(record vtypes.AttestationRecord) bool {
		if record.Status == vtypes.AttestationStatusValid {
			attestations = append(attestations, record)
		}
		return false
	})
	if len(attestations) == 0 {
		return moduletypes.ErrAttestationNotFound
	}

	result, err := k.Settle(SettlementInput{
		Path:                SettlementPathGovernanceAttestation,
		GovernanceReason:    reason,
		FaultAttribution:    fault,
		SlashAuditorDeposit: governanceRevocationSlashesAuditorDeposit(fault),
	})
	if err != nil {
		return err
	}

	for _, attestation := range attestations {
		provider, err := sdk.AccAddressFromBech32(attestation.Provider)
		if err != nil {
			return err
		}
		if err = k.voidGovernanceAttestation(ctx, provider, auditor, attestation, result, fault); err != nil {
			return err
		}
	}

	return k.slashAuditorBond(ctx, auditorRecord)
}

func (k *keeper) validateGovernanceAttestationRevocation(authority string, evidenceHash []byte) error {
	if k.authority != "" && authority != k.authority {
		return govtypes.ErrInvalidSigner.Wrapf("invalid authority; expected %s, got %s", k.authority, authority)
	}
	return validateHash(evidenceHash, "evidence hash")
}

func (k *keeper) voidGovernanceAttestation(
	ctx sdk.Context,
	provider sdk.AccAddress,
	auditor sdk.AccAddress,
	attestation vtypes.AttestationRecord,
	result SettlementResult,
	fault vtypes.FaultAttribution,
) error {
	if err := k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
		return err
	}

	attestation.Status = vtypes.AttestationStatusVoided
	attestation.VoidedReason = vtypes.VoidedReasonGovernance
	attestation.FeeStatus = result.FeeStatus
	attestation.DepositStatus = result.DepositStatus
	attestation.FaultAttribution = fault
	if err := k.SetAttestation(ctx, attestation); err != nil {
		return err
	}
	return ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationVoided{
		Provider: attestation.Provider,
		Auditor:  attestation.Auditor,
		Reason:   vtypes.VoidedReasonGovernance,
	})
}

func (k *keeper) slashAuditorBond(ctx sdk.Context, record vtypes.AuditorRecord) error {
	if !record.BondAmount.IsNil() && !record.BondAmount.IsZero() {
		if err := k.sendModuleToDistribution(ctx, record.BondAmount); err != nil {
			return err
		}
	}

	denom := k.GetParams(ctx).BondL1.Denom
	if !record.BondAmount.IsNil() {
		denom = record.BondAmount.Denom
	}
	record.BondAmount = sdk.NewCoin(denom, math.ZeroInt())
	record.BondStatus = vtypes.BondStatusNotBonded
	record.BondUnbondingCompletionTime = nil
	return k.SetAuditor(ctx, record)
}

func governanceRevocationSlashesAuditorDeposit(fault vtypes.FaultAttribution) bool {
	return fault == vtypes.FaultAttributionAuditorFault || fault == vtypes.FaultAttributionSharedFault
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
		discrepancy := vtypes.DiscrepancyEvent{
			ID:               id,
			Provider:         attestation.Provider,
			AuditorA:         conflict.Auditor,
			AuditorATier:     conflict.Tier,
			AuditorB:         attestation.Auditor,
			AuditorBTier:     attestation.Tier,
			Timestamp:        ctx.BlockTime(),
			ResolutionStatus: vtypes.DiscrepancyStatusPending,
		}
		k.SetDiscrepancy(ctx, discrepancy)
		if err = ctx.EventManager().EmitTypedEvent(&vtypes.EventDiscrepancyDetected{
			DiscrepancyID: id,
			Provider:      discrepancy.Provider,
			AuditorA:      discrepancy.AuditorA,
			TierA:         discrepancy.AuditorATier,
			AuditorB:      discrepancy.AuditorB,
			TierB:         discrepancy.AuditorBTier,
		}); err != nil {
			return err
		}
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
		graceID, started, err := k.upsertProviderVerificationGrace(ctx, provider, bestBefore, discrepancyIDs)
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
		if started {
			return ctx.EventManager().EmitTypedEvent(&vtypes.EventVerificationGraceStarted{
				GraceRecordID: graceID,
				Provider:      provider.String(),
				PreservedTier: bestBefore,
			})
		}
	}
	return nil
}

func (k *keeper) voidUnsupportedProviderAttestations(ctx sdk.Context, provider sdk.AccAddress, remaining sdk.Coin, path SettlementPath, reason vtypes.VoidedReason) error {
	attestations := make([]vtypes.AttestationRecord, 0)
	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(record vtypes.AttestationRecord) bool {
		if !k.providerBondSupportsAttestation(ctx, provider, remaining, record) {
			attestations = append(attestations, record)
		}
		return false
	})
	if len(attestations) == 0 {
		return nil
	}

	result, err := k.Settle(SettlementInput{
		Path:             path,
		FaultAttribution: vtypes.FaultAttributionProviderFault,
	})
	if err != nil {
		return err
	}

	for _, attestation := range attestations {
		auditor, err := sdk.AccAddressFromBech32(attestation.Auditor)
		if err != nil {
			return err
		}
		if err = k.settleAttestationFunds(ctx, provider, auditor, attestation, result); err != nil {
			return err
		}

		attestation.Status = vtypes.AttestationStatusVoided
		attestation.VoidedReason = reason
		attestation.FeeStatus = result.FeeStatus
		attestation.DepositStatus = result.DepositStatus
		attestation.FaultAttribution = vtypes.FaultAttributionProviderFault
		if err = k.SetAttestation(ctx, attestation); err != nil {
			return err
		}
		if err = ctx.EventManager().EmitTypedEvent(&vtypes.EventAttestationVoided{
			Provider: attestation.Provider,
			Auditor:  attestation.Auditor,
			Reason:   reason,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (k *keeper) providerBondSupportsAttestation(ctx sdk.Context, provider sdk.AccAddress, bonded sdk.Coin, attestation vtypes.AttestationRecord) bool {
	if !vtypes.TierRequiresProviderBond(attestation.Tier) {
		return true
	}

	snapshot, found := k.GetProviderSnapshot(ctx, provider)
	if !found {
		return false
	}

	required := requiredProviderBond(k.GetParams(ctx), attestation.Tier, snapshot.ResourceSummary)
	return requireCoinAtLeast(bonded, required, moduletypes.ErrInsufficientProviderBond) == nil
}

func (k *keeper) freezeAuditorForDiscrepancy(ctx sdk.Context, record vtypes.AuditorRecord) error {
	record.BondStatus = vtypes.BondStatusFrozen
	record.DiscrepancyCount++
	return k.SetAuditor(ctx, record)
}

func (k *keeper) upsertProviderVerificationGrace(ctx sdk.Context, provider sdk.AccAddress, preservedTier vtypes.VerificationTier, discrepancyIDs []uint64) (uint64, bool, error) {
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
		return active.ID, false, k.SetProviderVerificationGrace(ctx, *active)
	}

	params := k.GetParams(ctx)
	id := k.NextGraceRecordID(ctx)
	record := vtypes.ProviderVerificationGraceRecord{
		ID:                   id,
		Provider:             provider.String(),
		PreservedTier:        preservedTier,
		SourceDiscrepancyIDs: discrepancyIDs,
		StartedAt:            ctx.BlockTime(),
		ExpiresAt:            ctx.BlockTime().Add(params.DiscrepancyGracePeriod),
		Status:               vtypes.VerificationGraceStatusActive,
	}
	return id, true, k.SetProviderVerificationGrace(ctx, record)
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

	var endedGrace *vtypes.ProviderVerificationGraceRecord
	if discrepancy.GraceRecordID != 0 && fault == vtypes.FaultAttributionProviderFault {
		if grace, found := k.getProviderVerificationGraceByID(ctx, discrepancy.GraceRecordID); found && grace.Status == vtypes.VerificationGraceStatusActive {
			grace.Status = vtypes.VerificationGraceStatusTerminated
			if err = k.SetProviderVerificationGrace(ctx, grace); err != nil {
				return err
			}
			endedGrace = &grace
		}
	}

	discrepancy.ResolutionStatus = vtypes.DiscrepancyStatusResolved
	discrepancy.ResolutionReason = reason
	discrepancy.FaultAttribution = fault
	discrepancy.ResolutionEvidenceHash = evidenceHash
	k.SetDiscrepancy(ctx, discrepancy)
	if err = ctx.EventManager().EmitTypedEvent(&vtypes.EventDiscrepancyResolved{
		DiscrepancyID:     discrepancy.ID,
		VindicatedAuditor: vindicatedAuditor,
		Reason:            reason,
		FaultAttribution:  fault,
	}); err != nil {
		return err
	}
	if endedGrace != nil {
		return ctx.EventManager().EmitTypedEvent(&vtypes.EventVerificationGraceEnded{
			GraceRecordID: endedGrace.ID,
			Status:        endedGrace.Status,
		})
	}
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
	if err := k.validateProviderLeaseCompletion(ctx, provider, tier, params); err != nil {
		return err
	}

	snapshot, found := k.GetProviderSnapshot(ctx, provider)
	if !found {
		return moduletypes.ErrSnapshotNonCompliant
	}
	if snapshot.Suspended || snapshot.ComplianceDeadline.Before(ctx.BlockTime()) {
		return moduletypes.ErrProviderSnapshotSuspended
	}

	var bond vtypes.ProviderBondRecord
	if vtypes.TierRequiresProviderBond(tier) {
		var found bool
		bond, found = k.GetProviderBond(ctx, provider)
		if !found {
			return moduletypes.ErrInsufficientProviderBond
		}
		required := requiredProviderBond(params, tier, snapshot.ResourceSummary)
		if err := requireCoinAtLeast(bond.BondedAmount, required, moduletypes.ErrInsufficientProviderBond); err != nil {
			return err
		}
	}
	if err := k.validateProviderSlashingHistory(ctx, tier, params, bond); err != nil {
		return err
	}
	if vtypes.TierAtLeast(tier, vtypes.TierTrusted) && !k.hasContinuousL3History(ctx, provider, params.MinL3DurationForL4) {
		return moduletypes.ErrInsufficientL3History
	}

	return nil
}

func (k *keeper) validateProviderLeaseCompletion(ctx sdk.Context, provider sdk.AccAddress, tier vtypes.VerificationTier, params vtypes.Params) error {
	if k.market == nil || !vtypes.TierAtLeast(tier, vtypes.TierEstablished) {
		return nil
	}

	completed, failuresByReason, found := k.market.GetProviderLeaseStats(ctx, provider)
	var failed uint64
	for _, count := range failuresByReason {
		failed += count
	}

	total := completed + failed
	if !found || total < uint64(params.MinLeasesForCompletionRate) {
		return nil
	}

	minBps := params.MinLeaseCompletionBpsL3
	if vtypes.TierAtLeast(tier, vtypes.TierTrusted) {
		minBps = params.MinLeaseCompletionBpsL4
	}
	if completed*10000 < total*uint64(minBps) {
		return errorsmod.Wrapf(moduletypes.ErrInsufficientLeaseCompletionRate, "completed %d failed %d below %d bps", completed, failed, minBps)
	}

	return nil
}

func (k *keeper) validateProviderSlashingHistory(ctx sdk.Context, tier vtypes.VerificationTier, params vtypes.Params, bond vtypes.ProviderBondRecord) error {
	if !vtypes.TierAtLeast(tier, vtypes.TierEstablished) || bond.LastSlashTime == nil {
		return nil
	}

	window := cleanHistoryWindowForTier(params, tier)
	if window == 0 {
		return nil
	}
	if bond.LastSlashTime.After(ctx.BlockTime().Add(-window)) {
		return moduletypes.ErrSlashingHistoryViolation
	}
	return nil
}

type attestationInterval struct {
	start time.Time
	end   time.Time
}

func (k *keeper) hasContinuousL3History(ctx sdk.Context, provider sdk.AccAddress, duration time.Duration) bool {
	if duration == 0 {
		return true
	}

	start := ctx.BlockTime().Add(-duration)
	end := ctx.BlockTime()
	intervals := make([]attestationInterval, 0)
	k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusUnspecified, func(record vtypes.AttestationRecord) bool {
		if !vtypes.TierAtLeast(record.Tier, vtypes.TierEstablished) {
			return false
		}
		if record.Status != vtypes.AttestationStatusValid && record.Status != vtypes.AttestationStatusExpired {
			return false
		}
		if !record.ExpiresAt.After(start) || record.CreatedAt.After(end) {
			return false
		}
		intervals = append(intervals, attestationInterval{
			start: record.CreatedAt,
			end:   record.ExpiresAt,
		})
		return false
	})
	if len(intervals) == 0 {
		return false
	}

	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].start.Before(intervals[j].start)
	})

	coveredUntil := start
	for _, interval := range intervals {
		if !interval.end.After(coveredUntil) {
			continue
		}
		if interval.start.After(coveredUntil) {
			return false
		}
		coveredUntil = interval.end
		if !coveredUntil.Before(end) {
			return true
		}
	}
	return false
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

func (k *keeper) consumedAttestationEscrow(ctx sdk.Context, provider, auditor sdk.AccAddress, attestation vtypes.AttestationRecord) (vtypes.AuditEscrowRecord, error) {
	escrow, found := k.GetAuditEscrow(ctx, attestation.AuditEscrowID)
	if !found {
		return vtypes.AuditEscrowRecord{}, moduletypes.ErrAuditEscrowNotFound
	}
	if escrow.Status != vtypes.AuditEscrowStatusConsumed ||
		escrow.Provider != provider.String() ||
		escrow.ConsumedByAuditor != auditor.String() ||
		!coinsEqual(escrow.Fee, attestation.Fee) ||
		escrow.FeeStatus != vtypes.FeeStatusEscrowed ||
		escrow.ProviderDepositStatus != vtypes.ProviderDepositStatusEscrowed {
		return vtypes.AuditEscrowRecord{}, moduletypes.ErrAuditEscrowNotConsumable
	}
	return escrow, nil
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
	faultA, faultB, err := discrepancyAttestationFaultAttributions(reason)
	if err != nil {
		return err
	}
	attestationA.FaultAttribution = faultA
	attestationB.FaultAttribution = faultB

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

func discrepancyAttestationFaultAttributions(reason vtypes.DiscrepancyResolutionReason) (vtypes.FaultAttribution, vtypes.FaultAttribution, error) {
	switch reason {
	case vtypes.DiscrepancyResolutionReasonAuditorACorrect:
		return vtypes.FaultAttributionNoFault, vtypes.FaultAttributionAuditorFault, nil
	case vtypes.DiscrepancyResolutionReasonAuditorBCorrect:
		return vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionNoFault, nil
	case vtypes.DiscrepancyResolutionReasonProviderFault:
		return vtypes.FaultAttributionProviderFault, vtypes.FaultAttributionProviderFault, nil
	case vtypes.DiscrepancyResolutionReasonEvidenceInconclusive:
		return vtypes.FaultAttributionNoFault, vtypes.FaultAttributionNoFault, nil
	case vtypes.DiscrepancyResolutionReasonBothAuditorsWrong,
		vtypes.DiscrepancyResolutionReasonGovernanceTimeoutReview:
		return vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionAuditorFault, nil
	case vtypes.DiscrepancyResolutionReasonSharedFault:
		return vtypes.FaultAttributionSharedFault, vtypes.FaultAttributionSharedFault, nil
	default:
		return vtypes.FaultAttributionUnspecified, vtypes.FaultAttributionUnspecified, errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown discrepancy resolution reason %d", reason)
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
		record.BondStatus = vtypes.BondStatusNotBonded
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
	if !coinAtLeast(got, want) {
		return err
	}
	return nil
}

func coinAtLeast(got, want sdk.Coin) bool {
	return coinsSameDenom(got, want) && !got.Amount.LT(want.Amount)
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

func cleanHistoryWindowForTier(params vtypes.Params, tier vtypes.VerificationTier) time.Duration {
	switch tier {
	case vtypes.TierIdentified, vtypes.TierVerified:
		return 0
	case vtypes.TierEstablished:
		return params.CleanHistoryWindowL3
	case vtypes.TierTrusted:
		return params.CleanHistoryWindowL4
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

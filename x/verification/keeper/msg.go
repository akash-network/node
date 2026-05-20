package keeper

import (
	"crypto/sha256"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

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

	return k.SetAttestation(ctx, vtypes.AttestationRecord{
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
	})
}

func (k *keeper) validateProviderPrerequisites(ctx sdk.Context, provider sdk.AccAddress, tier vtypes.VerificationTier) error {
	if !vtypes.TierRequiresSnapshot(tier) {
		return nil
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
		required := requiredProviderBond(k.GetParams(ctx), tier, snapshot.ResourceSummary)
		if err := requireCoinAtLeast(bond.BondedAmount, required, moduletypes.ErrInsufficientProviderBond); err != nil {
			return err
		}
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

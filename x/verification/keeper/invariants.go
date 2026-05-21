package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

const (
	invariantEscrowBalance          = "escrow-balance"
	invariantAttestationValidity    = "attestation-validity"
	invariantFrozenAuditor          = "frozen-auditor"
	invariantDiscrepancyConsistency = "discrepancy-consistency"
	invariantSnapshotCompliance     = "snapshot-compliance"
	invariantProviderBond           = "provider-bond"
	invariantSelfAttestation        = "self-attestation"
	invariantAuditorAuthority       = "auditor-authority"
	invariantAuditEscrow            = "audit-escrow"
	invariantGraceRecord            = "grace-record"
)

func RegisterInvariants(ir sdk.InvariantRegistry, k Keeper) {
	ir.RegisterRoute(moduletypes.ModuleName, invariantEscrowBalance, EscrowBalanceInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantAttestationValidity, AttestationValidityInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantFrozenAuditor, FrozenAuditorInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantDiscrepancyConsistency, DiscrepancyConsistencyInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantSnapshotCompliance, SnapshotComplianceInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantProviderBond, ProviderBondInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantSelfAttestation, SelfAttestationInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantAuditorAuthority, AuditorAuthorityInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantAuditEscrow, AuditEscrowInvariant(k))
	ir.RegisterRoute(moduletypes.ModuleName, invariantGraceRecord, GraceRecordInvariant(k))
}

func EscrowBalanceInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		expected := sdk.NewCoins()
		k.WithAttestations(ctx, func(record vtypes.AttestationRecord) bool {
			if record.FeeStatus == vtypes.FeeStatusEscrowed {
				expected = expected.Add(record.Fee)
			}
			if record.DepositStatus == vtypes.DepositStatusEscrowed ||
				record.DepositStatus == vtypes.DepositStatusPendingDiscrepancy {
				expected = expected.Add(record.Deposit)
			}
			return false
		})
		k.WithAuditEscrows(ctx, func(record vtypes.AuditEscrowRecord) bool {
			if record.Status == vtypes.AuditEscrowStatusOpen &&
				record.FeeStatus == vtypes.FeeStatusEscrowed {
				expected = expected.Add(record.Fee)
			}
			if record.ProviderDepositStatus == vtypes.ProviderDepositStatusEscrowed {
				expected = expected.Add(record.ProviderDeposit)
			}
			return false
		})
		k.WithAuditors(ctx, func(record vtypes.AuditorRecord) bool {
			if record.BondStatus == vtypes.BondStatusBonded ||
				record.BondStatus == vtypes.BondStatusFrozen ||
				record.BondStatus == vtypes.BondStatusUnbonding {
				expected = expected.Add(record.BondAmount)
			}
			return false
		})
		k.WithProviderBonds(ctx, func(record vtypes.ProviderBondRecord) bool {
			expected = expected.Add(record.BondedAmount)
			for _, entry := range record.UnbondingEntries {
				expected = expected.Add(entry.Amount)
			}
			return false
		})

		for _, coin := range expected {
			if !k.ModuleBalance(ctx, coin.Denom).Amount.Equal(coin.Amount) {
				return sdk.FormatInvariant(
					moduletypes.ModuleName,
					invariantEscrowBalance,
					fmt.Sprintf("expected %s module balance, got %s", coin, k.ModuleBalance(ctx, coin.Denom)),
				), true
			}
		}
		return "", false
	}
}

func AttestationValidityInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithAttestations(ctx, func(record vtypes.AttestationRecord) bool {
			if record.Status == vtypes.AttestationStatusValid && !record.ExpiresAt.After(ctx.BlockTime()) {
				broken = fmt.Sprintf("valid attestation %s/%s expired at %s", record.Provider, record.Auditor, record.ExpiresAt)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantAttestationValidity, broken), true
		}
		return "", false
	}
}

func FrozenAuditorInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithAuditors(ctx, func(record vtypes.AuditorRecord) bool {
			if record.BondStatus != vtypes.BondStatusFrozen {
				return false
			}
			if !auditorHasPendingDiscrepancy(k, ctx, record.Address, 0) {
				broken = fmt.Sprintf("frozen auditor %s has no pending discrepancy", record.Address)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantFrozenAuditor, broken), true
		}
		return "", false
	}
}

func DiscrepancyConsistencyInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithDiscrepancies(ctx, vtypes.DiscrepancyStatusUnspecified, func(record vtypes.DiscrepancyEvent) bool {
			if record.ResolutionStatus != vtypes.DiscrepancyStatusPending &&
				record.ResolutionStatus != vtypes.DiscrepancyStatusTimedOut {
				return false
			}
			if err := checkDiscrepancyConsistency(k, ctx, record); err != nil {
				broken = err.Error()
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantDiscrepancyConsistency, broken), true
		}
		return "", false
	}
}

func SnapshotComplianceInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithProviderSnapshots(ctx, func(snapshot vtypes.ProviderSnapshotRecord) bool {
			if !snapshot.Suspended {
				return false
			}
			provider, err := sdk.AccAddressFromBech32(snapshot.Provider)
			if err != nil {
				broken = err.Error()
				return true
			}
			k.WithProviderAttestations(ctx, provider, vtypes.AttestationStatusValid, func(attestation vtypes.AttestationRecord) bool {
				if vtypes.TierRequiresSnapshot(attestation.Tier) {
					broken = fmt.Sprintf("snapshot-noncompliant provider %s has valid %s attestation", snapshot.Provider, attestation.Tier)
					return true
				}
				return false
			})
			return broken != ""
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantSnapshotCompliance, broken), true
		}
		return "", false
	}
}

func ProviderBondInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		params := k.GetParams(ctx)
		var broken string
		k.WithAttestations(ctx, func(attestation vtypes.AttestationRecord) bool {
			if attestation.Status != vtypes.AttestationStatusValid || !vtypes.TierRequiresProviderBond(attestation.Tier) {
				return false
			}
			provider, err := sdk.AccAddressFromBech32(attestation.Provider)
			if err != nil {
				broken = err.Error()
				return true
			}
			snapshot, found := k.GetProviderSnapshot(ctx, provider)
			if !found {
				broken = fmt.Sprintf("provider %s has no snapshot for %s attestation", attestation.Provider, attestation.Tier)
				return true
			}
			bond, found := k.GetProviderBond(ctx, provider)
			if !found {
				broken = fmt.Sprintf("provider %s has no bond for %s attestation", attestation.Provider, attestation.Tier)
				return true
			}
			required := requiredProviderBond(params, attestation.Tier, snapshot.ResourceSummary)
			if requireCoinAtLeast(bond.BondedAmount, required, moduletypes.ErrInsufficientProviderBond) != nil {
				broken = fmt.Sprintf("provider %s bond %s below required %s", attestation.Provider, bond.BondedAmount, required)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantProviderBond, broken), true
		}
		return "", false
	}
}

func SelfAttestationInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithAttestations(ctx, func(record vtypes.AttestationRecord) bool {
			if record.Provider == record.Auditor {
				broken = fmt.Sprintf("provider %s attested itself", record.Provider)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantSelfAttestation, broken), true
		}
		return "", false
	}
}

func AuditorAuthorityInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithAttestations(ctx, func(record vtypes.AttestationRecord) bool {
			if record.Status != vtypes.AttestationStatusValid {
				return false
			}
			auditor, err := sdk.AccAddressFromBech32(record.Auditor)
			if err != nil {
				broken = err.Error()
				return true
			}
			auditorRecord, found := k.GetAuditor(ctx, auditor)
			if !found || !vtypes.TierAtLeast(auditorRecord.MaxAttestationTier, record.Tier) {
				broken = fmt.Sprintf("auditor %s cannot issue %s attestation", record.Auditor, record.Tier)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantAuditorAuthority, broken), true
		}
		return "", false
	}
}

func AuditEscrowInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithAttestations(ctx, func(attestation vtypes.AttestationRecord) bool {
			if attestation.AuditEscrowID == 0 {
				return false
			}
			escrow, found := k.GetAuditEscrow(ctx, attestation.AuditEscrowID)
			if !found {
				broken = fmt.Sprintf("attestation %s/%s references missing escrow %d", attestation.Provider, attestation.Auditor, attestation.AuditEscrowID)
				return true
			}
			if escrow.Status != vtypes.AuditEscrowStatusConsumed ||
				escrow.Provider != attestation.Provider ||
				escrow.ConsumedByAuditor != attestation.Auditor ||
				requireCoinAtLeast(escrow.Fee, attestation.Fee, moduletypes.ErrInsufficientAuditFee) != nil {
				broken = fmt.Sprintf("attestation %s/%s has inconsistent escrow %d", attestation.Provider, attestation.Auditor, attestation.AuditEscrowID)
				return true
			}
			return false
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantAuditEscrow, broken), true
		}
		return "", false
	}
}

func GraceRecordInvariant(k Keeper) sdk.Invariant {
	return func(ctx sdk.Context) (string, bool) {
		var broken string
		k.WithVerificationGraces(ctx, func(record vtypes.ProviderVerificationGraceRecord) bool {
			if record.Status != vtypes.VerificationGraceStatusActive {
				return false
			}
			for _, id := range record.SourceDiscrepancyIDs {
				discrepancy, found := k.GetDiscrepancy(ctx, id)
				if found &&
					(discrepancy.ResolutionStatus == vtypes.DiscrepancyStatusPending ||
						discrepancy.ResolutionStatus == vtypes.DiscrepancyStatusTimedOut) {
					return false
				}
			}
			broken = fmt.Sprintf("active grace record %d has no pending or timed-out source discrepancy", record.ID)
			return true
		})
		if broken != "" {
			return sdk.FormatInvariant(moduletypes.ModuleName, invariantGraceRecord, broken), true
		}
		return "", false
	}
}

func checkDiscrepancyConsistency(k Keeper, ctx sdk.Context, record vtypes.DiscrepancyEvent) error {
	provider, err := sdk.AccAddressFromBech32(record.Provider)
	if err != nil {
		return err
	}
	auditorA, err := sdk.AccAddressFromBech32(record.AuditorA)
	if err != nil {
		return err
	}
	auditorB, err := sdk.AccAddressFromBech32(record.AuditorB)
	if err != nil {
		return err
	}
	if record.ResolutionStatus == vtypes.DiscrepancyStatusPending {
		if err := requireFrozenAuditor(k, ctx, auditorA); err != nil {
			return err
		}
		if err := requireFrozenAuditor(k, ctx, auditorB); err != nil {
			return err
		}
	}
	if err := requireDiscrepancyAttestation(k, ctx, provider, auditorA, record.ResolutionStatus); err != nil {
		return err
	}
	return requireDiscrepancyAttestation(k, ctx, provider, auditorB, record.ResolutionStatus)
}

func requireFrozenAuditor(k Keeper, ctx sdk.Context, auditor sdk.AccAddress) error {
	record, found := k.GetAuditor(ctx, auditor)
	if !found || record.BondStatus != vtypes.BondStatusFrozen {
		return fmt.Errorf("auditor %s is not frozen for pending discrepancy", auditor)
	}
	return nil
}

func requireDiscrepancyAttestation(k Keeper, ctx sdk.Context, provider, auditor sdk.AccAddress, status vtypes.DiscrepancyStatus) error {
	attestation, found := k.GetAttestation(ctx, provider, auditor)
	if !found {
		return fmt.Errorf("missing discrepancy attestation %s/%s", provider, auditor)
	}
	if attestation.Status != vtypes.AttestationStatusVoided || attestation.VoidedReason != vtypes.VoidedReasonDiscrepancy {
		return fmt.Errorf("attestation %s/%s is not voided for discrepancy", provider, auditor)
	}
	switch status {
	case vtypes.DiscrepancyStatusPending:
		if attestation.FeeStatus != vtypes.FeeStatusEscrowed ||
			attestation.DepositStatus != vtypes.DepositStatusPendingDiscrepancy {
			return fmt.Errorf("attestation %s/%s has invalid pending discrepancy settlement", provider, auditor)
		}
	case vtypes.DiscrepancyStatusTimedOut:
		if attestation.FeeStatus != vtypes.FeeStatusReturnedToProvider ||
			attestation.DepositStatus != vtypes.DepositStatusSlashed {
			return fmt.Errorf("attestation %s/%s has invalid timed-out discrepancy settlement", provider, auditor)
		}
	}
	return nil
}

func auditorHasPendingDiscrepancy(k Keeper, ctx sdk.Context, auditor string, skipID uint64) bool {
	found := false
	k.WithDiscrepancies(ctx, vtypes.DiscrepancyStatusPending, func(record vtypes.DiscrepancyEvent) bool {
		if record.ID != skipID && (record.AuditorA == auditor || record.AuditorB == auditor) {
			found = true
			return true
		}
		return false
	})
	return found
}

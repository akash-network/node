package keeper

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func ValidateReasonAttribution(reason any, fault vtypes.FaultAttribution) error {
	return validateReasonAttribution(reason, fault)
}

func validateReasonAttribution(reason any, fault vtypes.FaultAttribution) error {
	if fault == vtypes.FaultAttributionUnspecified || fault == vtypes.FaultAttributionInconclusive {
		return errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "invalid fault attribution %s", fault)
	}

	switch reason := reason.(type) {
	case vtypes.AttestationRevocationReason:
		return validateRevocationReasonAttribution(reason, fault)
	case vtypes.GovernanceAttestationReason:
		return validateGovernanceReasonAttribution(reason, fault)
	case vtypes.AuditEscrowSettlementReason:
		return validateAuditEscrowReasonAttribution(reason, fault)
	case vtypes.DiscrepancyResolutionReason:
		return validateDiscrepancyReasonAttribution(reason, fault)
	case vtypes.ProviderBondSlashReason:
		return validateProviderBondSlashReasonAttribution(reason, fault)
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unsupported reason type %T", reason)
	}
}

func validateRevocationReasonAttribution(reason vtypes.AttestationRevocationReason, fault vtypes.FaultAttribution) error {
	switch reason {
	case vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
		vtypes.AttestationRevocationReasonSnapshotMismatch,
		vtypes.AttestationRevocationReasonSoftwareIdentityChanged,
		vtypes.AttestationRevocationReasonCapabilityMisrepresented,
		vtypes.AttestationRevocationReasonProviderNonResponsive:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault, vtypes.FaultAttributionNoFault)
	case vtypes.AttestationRevocationReasonAuditorEvidenceError:
		return requireFault(reason, fault, vtypes.FaultAttributionAuditorFault)
	case vtypes.AttestationRevocationReasonAuditorOperationalExit:
		return requireFault(reason, fault, vtypes.FaultAttributionNoFault)
	case vtypes.AttestationRevocationReasonUnspecified:
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified attestation revocation reason")
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown attestation revocation reason %d", reason)
	}
}

func validateGovernanceReasonAttribution(reason vtypes.GovernanceAttestationReason, fault vtypes.FaultAttribution) error {
	switch reason {
	case vtypes.GovernanceAttestationReasonFraudulentProvider,
		vtypes.GovernanceAttestationReasonCompromisedProvider,
		vtypes.GovernanceAttestationReasonProviderNonCooperation:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault)
	case vtypes.GovernanceAttestationReasonFaultyAuditor,
		vtypes.GovernanceAttestationReasonNegligentAuditor:
		return requireFault(reason, fault, vtypes.FaultAttributionAuditorFault)
	case vtypes.GovernanceAttestationReasonEvidenceInsufficient:
		return requireFault(reason, fault, vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionNoFault)
	case vtypes.GovernanceAttestationReasonEmergencySafetyAction:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault, vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionSharedFault, vtypes.FaultAttributionNoFault)
	case vtypes.GovernanceAttestationReasonUnspecified:
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified governance attestation reason")
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown governance attestation reason %d", reason)
	}
}

func validateAuditEscrowReasonAttribution(reason vtypes.AuditEscrowSettlementReason, fault vtypes.FaultAttribution) error {
	switch reason {
	case vtypes.AuditEscrowSettlementReasonCancelledUnconsumed,
		vtypes.AuditEscrowSettlementReasonExpiredUnconsumed,
		vtypes.AuditEscrowSettlementReasonNoFault:
		return requireFault(reason, fault, vtypes.FaultAttributionNoFault)
	case vtypes.AuditEscrowSettlementReasonProviderFault:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault)
	case vtypes.AuditEscrowSettlementReasonUnspecified:
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified audit escrow settlement reason")
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown audit escrow settlement reason %d", reason)
	}
}

func validateDiscrepancyReasonAttribution(reason vtypes.DiscrepancyResolutionReason, fault vtypes.FaultAttribution) error {
	switch reason {
	case vtypes.DiscrepancyResolutionReasonAuditorACorrect,
		vtypes.DiscrepancyResolutionReasonAuditorBCorrect,
		vtypes.DiscrepancyResolutionReasonBothAuditorsWrong:
		return requireFault(reason, fault, vtypes.FaultAttributionAuditorFault)
	case vtypes.DiscrepancyResolutionReasonProviderFault:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault)
	case vtypes.DiscrepancyResolutionReasonSharedFault:
		return requireFault(reason, fault, vtypes.FaultAttributionSharedFault)
	case vtypes.DiscrepancyResolutionReasonEvidenceInconclusive:
		return requireFault(reason, fault, vtypes.FaultAttributionNoFault)
	case vtypes.DiscrepancyResolutionReasonGovernanceTimeoutReview:
		return requireFault(reason, fault, vtypes.FaultAttributionAuditorFault)
	case vtypes.DiscrepancyResolutionReasonUnspecified:
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified discrepancy resolution reason")
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown discrepancy resolution reason %d", reason)
	}
}

func validateProviderBondSlashReasonAttribution(reason vtypes.ProviderBondSlashReason, fault vtypes.FaultAttribution) error {
	switch reason {
	case vtypes.ProviderBondSlashReasonResourceMisrepresentation,
		vtypes.ProviderBondSlashReasonCapacityOverstatement,
		vtypes.ProviderBondSlashReasonFraudulentSnapshot,
		vtypes.ProviderBondSlashReasonProviderCompromise,
		vtypes.ProviderBondSlashReasonSLABreach,
		vtypes.ProviderBondSlashReasonNonCooperationDuringAudit:
		return requireFault(reason, fault, vtypes.FaultAttributionProviderFault)
	case vtypes.ProviderBondSlashReasonUnspecified:
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified provider bond slash reason")
	default:
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown provider bond slash reason %d", reason)
	}
}

func requireFault(reason fmt.Stringer, got vtypes.FaultAttribution, allowed ...vtypes.FaultAttribution) error {
	for _, want := range allowed {
		if got == want {
			return nil
		}
	}

	return errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "%s cannot use %s", reason, got)
}

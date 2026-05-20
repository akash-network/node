package keeper

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v2/x/verification/types"
)

func TestValidateReasonAttribution(t *testing.T) {
	tests := []struct {
		name    string
		reason  any
		fault   vtypes.FaultAttribution
		wantErr error
	}{
		{
			name:   "revocation provider monitoring may be provider fault",
			reason: vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			fault:  vtypes.FaultAttributionProviderFault,
		},
		{
			name:   "revocation provider monitoring may be no fault",
			reason: vtypes.AttestationRevocationReasonSnapshotMismatch,
			fault:  vtypes.FaultAttributionNoFault,
		},
		{
			name:   "auditor evidence error is auditor fault",
			reason: vtypes.AttestationRevocationReasonAuditorEvidenceError,
			fault:  vtypes.FaultAttributionAuditorFault,
		},
		{
			name:   "auditor operational exit is no fault",
			reason: vtypes.AttestationRevocationReasonAuditorOperationalExit,
			fault:  vtypes.FaultAttributionNoFault,
		},
		{
			name:   "audit escrow provider fault",
			reason: vtypes.AuditEscrowSettlementReasonProviderFault,
			fault:  vtypes.FaultAttributionProviderFault,
		},
		{
			name:   "audit escrow no fault",
			reason: vtypes.AuditEscrowSettlementReasonNoFault,
			fault:  vtypes.FaultAttributionNoFault,
		},
		{
			name:   "governance provider reason",
			reason: vtypes.GovernanceAttestationReasonFraudulentProvider,
			fault:  vtypes.FaultAttributionProviderFault,
		},
		{
			name:   "governance auditor reason",
			reason: vtypes.GovernanceAttestationReasonFaultyAuditor,
			fault:  vtypes.FaultAttributionAuditorFault,
		},
		{
			name:   "governance evidence insufficient may be no fault",
			reason: vtypes.GovernanceAttestationReasonEvidenceInsufficient,
			fault:  vtypes.FaultAttributionNoFault,
		},
		{
			name:   "governance emergency may be shared fault",
			reason: vtypes.GovernanceAttestationReasonEmergencySafetyAction,
			fault:  vtypes.FaultAttributionSharedFault,
		},
		{
			name:   "discrepancy auditor correct implies auditor fault for bad attestation",
			reason: vtypes.DiscrepancyResolutionReasonAuditorACorrect,
			fault:  vtypes.FaultAttributionAuditorFault,
		},
		{
			name:   "discrepancy evidence inconclusive settles no fault",
			reason: vtypes.DiscrepancyResolutionReasonEvidenceInconclusive,
			fault:  vtypes.FaultAttributionNoFault,
		},
		{
			name:   "discrepancy timeout review uses auditor fault timeout rule",
			reason: vtypes.DiscrepancyResolutionReasonGovernanceTimeoutReview,
			fault:  vtypes.FaultAttributionAuditorFault,
		},
		{
			name:   "provider bond slash is provider fault",
			reason: vtypes.ProviderBondSlashReasonResourceMisrepresentation,
			fault:  vtypes.FaultAttributionProviderFault,
		},
		{
			name:    "unspecified fault rejected",
			reason:  vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			fault:   vtypes.FaultAttributionUnspecified,
			wantErr: moduletypes.ErrInvalidFaultAttribution,
		},
		{
			name:    "inconclusive fault rejected",
			reason:  vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
			fault:   vtypes.FaultAttributionInconclusive,
			wantErr: moduletypes.ErrInvalidFaultAttribution,
		},
		{
			name:    "unspecified reason rejected",
			reason:  vtypes.AttestationRevocationReasonUnspecified,
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unknown revocation reason rejected",
			reason:  vtypes.AttestationRevocationReason(99),
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unknown governance reason rejected",
			reason:  vtypes.GovernanceAttestationReason(99),
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unknown audit escrow reason rejected",
			reason:  vtypes.AuditEscrowSettlementReason(99),
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unknown discrepancy reason rejected",
			reason:  vtypes.DiscrepancyResolutionReason(99),
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unknown provider bond slash reason rejected",
			reason:  vtypes.ProviderBondSlashReason(99),
			fault:   vtypes.FaultAttributionProviderFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
		{
			name:    "unsupported reason type rejected",
			reason:  "not-a-reason",
			fault:   vtypes.FaultAttributionNoFault,
			wantErr: moduletypes.ErrInvalidReason,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReasonAttribution(tc.reason, tc.fault)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.True(t, errors.Is(err, tc.wantErr), "got %v, want %v", err, tc.wantErr)
		})
	}
}

func TestValidateReasonAttributionBadPairs(t *testing.T) {
	allFaults := []vtypes.FaultAttribution{
		vtypes.FaultAttributionUnspecified,
		vtypes.FaultAttributionProviderFault,
		vtypes.FaultAttributionAuditorFault,
		vtypes.FaultAttributionSharedFault,
		vtypes.FaultAttributionNoFault,
		vtypes.FaultAttributionInconclusive,
	}

	valid := map[any]map[vtypes.FaultAttribution]bool{
		vtypes.AttestationRevocationReasonProviderNoLongerQualifies: {
			vtypes.FaultAttributionProviderFault: true,
			vtypes.FaultAttributionNoFault:       true,
		},
		vtypes.AttestationRevocationReasonAuditorEvidenceError: {
			vtypes.FaultAttributionAuditorFault: true,
		},
		vtypes.AttestationRevocationReasonAuditorOperationalExit: {
			vtypes.FaultAttributionNoFault: true,
		},
		vtypes.GovernanceAttestationReasonFraudulentProvider: {
			vtypes.FaultAttributionProviderFault: true,
		},
		vtypes.GovernanceAttestationReasonFaultyAuditor: {
			vtypes.FaultAttributionAuditorFault: true,
		},
		vtypes.AuditEscrowSettlementReasonProviderFault: {
			vtypes.FaultAttributionProviderFault: true,
		},
		vtypes.AuditEscrowSettlementReasonNoFault: {
			vtypes.FaultAttributionNoFault: true,
		},
		vtypes.DiscrepancyResolutionReasonProviderFault: {
			vtypes.FaultAttributionProviderFault: true,
		},
		vtypes.DiscrepancyResolutionReasonSharedFault: {
			vtypes.FaultAttributionSharedFault: true,
		},
		vtypes.ProviderBondSlashReasonFraudulentSnapshot: {
			vtypes.FaultAttributionProviderFault: true,
		},
	}

	for reason, validFaults := range valid {
		for _, fault := range allFaults {
			if validFaults[fault] {
				continue
			}

			t.Run(reasonName(reason)+"/"+fault.String(), func(t *testing.T) {
				err := ValidateReasonAttribution(reason, fault)
				require.Error(t, err)
				require.True(t, errors.Is(err, moduletypes.ErrInvalidFaultAttribution), "got %v", err)
			})
		}
	}
}

func reasonName(reason any) string {
	switch reason := reason.(type) {
	case vtypes.AttestationRevocationReason:
		return reason.String()
	case vtypes.GovernanceAttestationReason:
		return reason.String()
	case vtypes.AuditEscrowSettlementReason:
		return reason.String()
	case vtypes.DiscrepancyResolutionReason:
		return reason.String()
	case vtypes.ProviderBondSlashReason:
		return reason.String()
	default:
		return "unknown"
	}
}

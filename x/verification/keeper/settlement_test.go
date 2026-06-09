package keeper

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

func TestSettleDispositionMatrix(t *testing.T) {
	tests := []struct {
		name  string
		input SettlementInput
		want  SettlementResult
	}{
		{
			name: "attestation expired",
			input: SettlementInput{
				Path:             SettlementPathAttestationExpired,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "revoked provider monitoring provider fault",
			input: SettlementInput{
				Path:             SettlementPathAttestationRevoked,
				RevocationReason: vtypes.AttestationRevocationReasonProviderNoLongerQualifies,
				FaultAttribution: vtypes.FaultAttributionProviderFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "revoked provider monitoring no fault",
			input: SettlementInput{
				Path:             SettlementPathAttestationRevoked,
				RevocationReason: vtypes.AttestationRevocationReasonSnapshotMismatch,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "revoked auditor evidence error",
			input: SettlementInput{
				Path:             SettlementPathAttestationRevoked,
				RevocationReason: vtypes.AttestationRevocationReasonAuditorEvidenceError,
				FaultAttribution: vtypes.FaultAttributionAuditorFault,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "revoked auditor operational exit",
			input: SettlementInput{
				Path:             SettlementPathAttestationRevoked,
				RevocationReason: vtypes.AttestationRevocationReasonAuditorOperationalExit,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusReturnedToAuditor,
			},
		},
		{
			name: "removed by provider",
			input: SettlementInput{
				Path:             SettlementPathAttestationRemoved,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "pending discrepancy",
			input: SettlementInput{
				Path: SettlementPathAttestationPendingDiscrepancy,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusEscrowed,
				DepositStatus: vtypes.DepositStatusPendingDiscrepancy,
			},
		},
		{
			name: "discrepancy resolved auditor fault",
			input: SettlementInput{
				Path:              SettlementPathDiscrepancyResolved,
				DiscrepancyReason: vtypes.DiscrepancyResolutionReasonAuditorACorrect,
				FaultAttribution:  vtypes.FaultAttributionAuditorFault,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "discrepancy resolved provider fault for vindicated auditor",
			input: SettlementInput{
				Path:              SettlementPathDiscrepancyResolved,
				DiscrepancyReason: vtypes.DiscrepancyResolutionReasonProviderFault,
				FaultAttribution:  vtypes.FaultAttributionProviderFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "discrepancy resolved shared fault",
			input: SettlementInput{
				Path:              SettlementPathDiscrepancyResolved,
				DiscrepancyReason: vtypes.DiscrepancyResolutionReasonSharedFault,
				FaultAttribution:  vtypes.FaultAttributionSharedFault,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "discrepancy timed out",
			input: SettlementInput{
				Path: SettlementPathDiscrepancyTimedOut,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "governance provider fault",
			input: SettlementInput{
				Path:                SettlementPathGovernanceAttestation,
				GovernanceReason:    vtypes.GovernanceAttestationReasonFraudulentProvider,
				FaultAttribution:    vtypes.FaultAttributionProviderFault,
				SlashAuditorDeposit: true,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "governance no fault",
			input: SettlementInput{
				Path:             SettlementPathGovernanceAttestation,
				GovernanceReason: vtypes.GovernanceAttestationReasonEmergencySafetyAction,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "governance auditor fault names auditor",
			input: SettlementInput{
				Path:                SettlementPathGovernanceAttestation,
				GovernanceReason:    vtypes.GovernanceAttestationReasonFaultyAuditor,
				FaultAttribution:    vtypes.FaultAttributionAuditorFault,
				SlashAuditorDeposit: true,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "governance shared fault names auditor",
			input: SettlementInput{
				Path:                SettlementPathGovernanceAttestation,
				GovernanceReason:    vtypes.GovernanceAttestationReasonEmergencySafetyAction,
				FaultAttribution:    vtypes.FaultAttributionSharedFault,
				SlashAuditorDeposit: true,
			},
			want: SettlementResult{
				FeeStatus:     vtypes.FeeStatusReturnedToProvider,
				DepositStatus: vtypes.DepositStatusSlashed,
			},
		},
		{
			name: "provider bond withdrawn voids attestation",
			input: SettlementInput{
				Path:             SettlementPathBondWithdrawn,
				FaultAttribution: vtypes.FaultAttributionProviderFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "provider bond slashed voids attestation",
			input: SettlementInput{
				Path:             SettlementPathBondSlashed,
				FaultAttribution: vtypes.FaultAttributionProviderFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "replacement",
			input: SettlementInput{
				Path:             SettlementPathReplacement,
				FaultAttribution: vtypes.FaultAttributionNoFault,
			},
			want: feeToAuditorDepositReturned(),
		},
		{
			name: "audit escrow cancelled",
			input: SettlementInput{
				Path:              SettlementPathAuditEscrow,
				AuditEscrowReason: vtypes.AuditEscrowSettlementReasonCancelledUnconsumed,
				FaultAttribution:  vtypes.FaultAttributionNoFault,
			},
			want: escrowReturned(),
		},
		{
			name: "audit escrow expired",
			input: SettlementInput{
				Path:              SettlementPathAuditEscrow,
				AuditEscrowReason: vtypes.AuditEscrowSettlementReasonExpiredUnconsumed,
				FaultAttribution:  vtypes.FaultAttributionNoFault,
			},
			want: escrowReturned(),
		},
		{
			name: "audit escrow provider fault",
			input: SettlementInput{
				Path:              SettlementPathAuditEscrow,
				AuditEscrowReason: vtypes.AuditEscrowSettlementReasonProviderFault,
				FaultAttribution:  vtypes.FaultAttributionProviderFault,
			},
			want: SettlementResult{
				FeeStatus:             vtypes.FeeStatusReturnedToProvider,
				ProviderDepositStatus: vtypes.ProviderDepositStatusSlashed,
			},
		},
		{
			name: "audit escrow no fault",
			input: SettlementInput{
				Path:              SettlementPathAuditEscrow,
				AuditEscrowReason: vtypes.AuditEscrowSettlementReasonNoFault,
				FaultAttribution:  vtypes.FaultAttributionNoFault,
			},
			want: escrowReturned(),
		},
		{
			name: "provider bond slash",
			input: SettlementInput{
				Path:                    SettlementPathProviderBondSlash,
				ProviderBondSlashReason: vtypes.ProviderBondSlashReasonFraudulentSnapshot,
				FaultAttribution:        vtypes.FaultAttributionProviderFault,
			},
			want: SettlementResult{ProviderBondSlashed: true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Settle(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSettleRejectsInvalidFaultAttribution(t *testing.T) {
	tests := []SettlementInput{
		{
			Path:             SettlementPathAttestationExpired,
			FaultAttribution: vtypes.FaultAttributionProviderFault,
		},
		{
			Path:             SettlementPathAttestationRevoked,
			RevocationReason: vtypes.AttestationRevocationReasonAuditorEvidenceError,
			FaultAttribution: vtypes.FaultAttributionNoFault,
		},
		{
			Path:              SettlementPathAuditEscrow,
			AuditEscrowReason: vtypes.AuditEscrowSettlementReasonProviderFault,
			FaultAttribution:  vtypes.FaultAttributionNoFault,
		},
		{
			Path:                    SettlementPathProviderBondSlash,
			ProviderBondSlashReason: vtypes.ProviderBondSlashReasonSLABreach,
			FaultAttribution:        vtypes.FaultAttributionAuditorFault,
		},
	}

	for _, input := range tests {
		_, err := Settle(input)
		require.Error(t, err)
		require.True(t, errors.Is(err, moduletypes.ErrInvalidFaultAttribution), "got %v", err)
	}
}

func TestSettleRejectsInvalidReason(t *testing.T) {
	_, err := Settle(SettlementInput{Path: SettlementPathUnspecified})
	require.Error(t, err)
	require.True(t, errors.Is(err, moduletypes.ErrInvalidReason), "got %v", err)

	_, err = Settle(SettlementInput{Path: SettlementPath(99)})
	require.Error(t, err)
	require.True(t, errors.Is(err, moduletypes.ErrInvalidReason), "got %v", err)
}

func feeToAuditorDepositReturned() SettlementResult {
	return SettlementResult{
		FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
		DepositStatus: vtypes.DepositStatusReturnedToAuditor,
	}
}

func escrowReturned() SettlementResult {
	return SettlementResult{
		FeeStatus:             vtypes.FeeStatusReturnedToProvider,
		ProviderDepositStatus: vtypes.ProviderDepositStatusReturnedToProvider,
	}
}

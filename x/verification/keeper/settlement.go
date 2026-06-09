package keeper

import (
	errorsmod "cosmossdk.io/errors"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

type SettlementPath uint8

const (
	SettlementPathUnspecified SettlementPath = iota
	SettlementPathAttestationExpired
	SettlementPathAttestationRevoked
	SettlementPathAttestationRemoved
	SettlementPathAttestationPendingDiscrepancy
	SettlementPathDiscrepancyResolved
	SettlementPathDiscrepancyTimedOut
	SettlementPathGovernanceAttestation
	SettlementPathBondWithdrawn
	SettlementPathBondSlashed
	SettlementPathReplacement
	SettlementPathAuditEscrow
	SettlementPathProviderBondSlash
)

type SettlementInput struct {
	Path                    SettlementPath
	FaultAttribution        vtypes.FaultAttribution
	RevocationReason        vtypes.AttestationRevocationReason
	GovernanceReason        vtypes.GovernanceAttestationReason
	AuditEscrowReason       vtypes.AuditEscrowSettlementReason
	DiscrepancyReason       vtypes.DiscrepancyResolutionReason
	ProviderBondSlashReason vtypes.ProviderBondSlashReason
	SlashAuditorDeposit     bool
}

type SettlementResult struct {
	FeeStatus             vtypes.FeeStatus
	DepositStatus         vtypes.DepositStatus
	ProviderDepositStatus vtypes.ProviderDepositStatus
	ProviderBondSlashed   bool
}

func Settle(input SettlementInput) (SettlementResult, error) {
	switch input.Path {
	case SettlementPathAttestationExpired:
		return settleAttestationNoFault(input)
	case SettlementPathAttestationRevoked:
		return settleRevocation(input)
	case SettlementPathAttestationRemoved:
		return settleAttestationNoFault(input)
	case SettlementPathAttestationPendingDiscrepancy:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusEscrowed,
			DepositStatus: vtypes.DepositStatusPendingDiscrepancy,
		}, nil
	case SettlementPathDiscrepancyResolved:
		return settleDiscrepancy(input)
	case SettlementPathDiscrepancyTimedOut:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusSlashed,
		}, nil
	case SettlementPathGovernanceAttestation:
		return settleGovernanceAttestation(input)
	case SettlementPathBondWithdrawn, SettlementPathBondSlashed:
		return settleProviderFaultAttestation(input)
	case SettlementPathReplacement:
		return settleAttestationNoFault(input)
	case SettlementPathAuditEscrow:
		return settleAuditEscrow(input)
	case SettlementPathProviderBondSlash:
		return settleProviderBondSlash(input)
	case SettlementPathUnspecified:
		return SettlementResult{}, errorsmod.Wrap(moduletypes.ErrInvalidReason, "unspecified settlement path")
	default:
		return SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidReason, "unknown settlement path %d", input.Path)
	}
}

func settleAttestationNoFault(input SettlementInput) (SettlementResult, error) {
	if input.FaultAttribution != vtypes.FaultAttributionNoFault {
		return SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "expected no fault, got %s", input.FaultAttribution)
	}

	return SettlementResult{
		FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
		DepositStatus: vtypes.DepositStatusReturnedToAuditor,
	}, nil
}

func settleProviderFaultAttestation(input SettlementInput) (SettlementResult, error) {
	if input.FaultAttribution != vtypes.FaultAttributionProviderFault {
		return SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "expected provider fault, got %s", input.FaultAttribution)
	}

	return SettlementResult{
		FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
		DepositStatus: vtypes.DepositStatusReturnedToAuditor,
	}, nil
}

func settleRevocation(input SettlementInput) (SettlementResult, error) {
	if err := validateReasonAttribution(input.RevocationReason, input.FaultAttribution); err != nil {
		return SettlementResult{}, err
	}

	switch input.RevocationReason {
	case vtypes.AttestationRevocationReasonAuditorEvidenceError:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusSlashed,
		}, nil
	case vtypes.AttestationRevocationReasonAuditorOperationalExit:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusReturnedToAuditor,
		}, nil
	default:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			DepositStatus: vtypes.DepositStatusReturnedToAuditor,
		}, nil
	}
}

func settleDiscrepancy(input SettlementInput) (SettlementResult, error) {
	if err := validateReasonAttribution(input.DiscrepancyReason, input.FaultAttribution); err != nil {
		return SettlementResult{}, err
	}

	switch input.FaultAttribution {
	case vtypes.FaultAttributionProviderFault, vtypes.FaultAttributionNoFault:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			DepositStatus: vtypes.DepositStatusReturnedToAuditor,
		}, nil
	case vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionSharedFault:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusSlashed,
		}, nil
	default:
		return SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "unsupported discrepancy fault %s", input.FaultAttribution)
	}
}

func settleGovernanceAttestation(input SettlementInput) (SettlementResult, error) {
	if err := validateReasonAttribution(input.GovernanceReason, input.FaultAttribution); err != nil {
		return SettlementResult{}, err
	}

	switch input.FaultAttribution {
	case vtypes.FaultAttributionProviderFault, vtypes.FaultAttributionNoFault:
		return SettlementResult{
			FeeStatus:     vtypes.FeeStatusReleasedToAuditor,
			DepositStatus: vtypes.DepositStatusReturnedToAuditor,
		}, nil
	case vtypes.FaultAttributionAuditorFault, vtypes.FaultAttributionSharedFault:
		result := SettlementResult{
			FeeStatus:     vtypes.FeeStatusReturnedToProvider,
			DepositStatus: vtypes.DepositStatusReturnedToAuditor,
		}
		if input.SlashAuditorDeposit {
			result.DepositStatus = vtypes.DepositStatusSlashed
		}
		return result, nil
	default:
		return SettlementResult{}, errorsmod.Wrapf(moduletypes.ErrInvalidFaultAttribution, "unsupported governance fault %s", input.FaultAttribution)
	}
}

func settleAuditEscrow(input SettlementInput) (SettlementResult, error) {
	if err := validateReasonAttribution(input.AuditEscrowReason, input.FaultAttribution); err != nil {
		return SettlementResult{}, err
	}

	switch input.AuditEscrowReason {
	case vtypes.AuditEscrowSettlementReasonProviderFault:
		return SettlementResult{
			FeeStatus:             vtypes.FeeStatusReturnedToProvider,
			ProviderDepositStatus: vtypes.ProviderDepositStatusSlashed,
		}, nil
	default:
		return SettlementResult{
			FeeStatus:             vtypes.FeeStatusReturnedToProvider,
			ProviderDepositStatus: vtypes.ProviderDepositStatusReturnedToProvider,
		}, nil
	}
}

func settleProviderBondSlash(input SettlementInput) (SettlementResult, error) {
	if err := validateReasonAttribution(input.ProviderBondSlashReason, input.FaultAttribution); err != nil {
		return SettlementResult{}, err
	}

	return SettlementResult{ProviderBondSlashed: true}, nil
}

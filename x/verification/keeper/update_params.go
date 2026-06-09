package keeper

import (
	"time"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"

	moduletypes "pkg.akt.dev/node/v3/x/verification/types"
)

const maxBasisPoints = uint32(10000)

type coinParam struct {
	name string
	coin sdk.Coin
}

type durationParam struct {
	name  string
	value time.Duration
}

// ValidateParams validates a complete x/verification Params value before it is
// accepted through MsgUpdateParams.
func ValidateParams(params vtypes.Params) error {
	if err := validateParamCoins(params); err != nil {
		return err
	}
	if err := validateParamDurations(params); err != nil {
		return err
	}
	if err := validateParamScalars(params); err != nil {
		return err
	}
	if err := validateParamRelationships(params); err != nil {
		return err
	}

	return nil
}

func validateParamCoins(params vtypes.Params) error {
	coins := []coinParam{
		{name: "bond_l1", coin: params.BondL1},
		{name: "bond_l2", coin: params.BondL2},
		{name: "bond_l3", coin: params.BondL3},
		{name: "bond_l4", coin: params.BondL4},
		{name: "min_fee_l1", coin: params.MinFeeL1},
		{name: "min_fee_l2", coin: params.MinFeeL2},
		{name: "min_fee_l3", coin: params.MinFeeL3},
		{name: "min_fee_l4", coin: params.MinFeeL4},
		{name: "bond_gpu_l2", coin: params.BondGpuL2},
		{name: "bond_gpu_l3", coin: params.BondGpuL3},
		{name: "bond_gpu_l4", coin: params.BondGpuL4},
		{name: "bond_vcpu_l2", coin: params.BondVcpuL2},
		{name: "bond_vcpu_l3", coin: params.BondVcpuL3},
		{name: "bond_vcpu_l4", coin: params.BondVcpuL4},
		{name: "bond_mem_gb_l2", coin: params.BondMemGbL2},
		{name: "bond_mem_gb_l3", coin: params.BondMemGbL3},
		{name: "bond_mem_gb_l4", coin: params.BondMemGbL4},
		{name: "bond_storage_tb_l2", coin: params.BondStorageTbL2},
		{name: "bond_storage_tb_l3", coin: params.BondStorageTbL3},
		{name: "bond_storage_tb_l4", coin: params.BondStorageTbL4},
		{name: "attestation_deposit", coin: params.AttestationDeposit},
		{name: "provider_audit_deposit", coin: params.ProviderAuditDeposit},
	}

	denomName := ""
	denom := ""
	for _, param := range coins {
		if err := validatePositiveParamCoin(param); err != nil {
			return err
		}

		if denom == "" {
			denomName = param.name
			denom = param.coin.Denom
			continue
		}
		if param.coin.Denom != denom {
			return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s denom %q must match params.%s denom %q", param.name, param.coin.Denom, denomName, denom)
		}
	}

	return nil
}

func validatePositiveParamCoin(param coinParam) error {
	if !param.coin.IsValid() || !param.coin.IsPositive() {
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be a valid positive coin", param.name)
	}
	return nil
}

func validateParamDurations(params vtypes.Params) error {
	durations := []durationParam{
		{name: "ttl_l1", value: params.TtlL1},
		{name: "ttl_l2", value: params.TtlL2},
		{name: "ttl_l3", value: params.TtlL3},
		{name: "ttl_l4", value: params.TtlL4},
		{name: "auditor_unbonding_period", value: params.AuditorUnbondingPeriod},
		{name: "renewal_period_l1", value: params.RenewalPeriodL1},
		{name: "renewal_period_l2", value: params.RenewalPeriodL2},
		{name: "renewal_period_l3", value: params.RenewalPeriodL3},
		{name: "renewal_period_l4", value: params.RenewalPeriodL4},
		{name: "snapshot_hash_interval", value: params.SnapshotHashInterval},
		{name: "max_snapshot_age", value: params.MaxSnapshotAge},
		{name: "provider_bond_unbonding_period", value: params.ProviderBondUnbondingPeriod},
		{name: "min_age_l2", value: params.MinAgeL2},
		{name: "min_age_l3", value: params.MinAgeL3},
		{name: "min_age_l4", value: params.MinAgeL4},
		{name: "clean_history_window_l3", value: params.CleanHistoryWindowL3},
		{name: "clean_history_window_l4", value: params.CleanHistoryWindowL4},
		{name: "min_l3_duration_for_l4", value: params.MinL3DurationForL4},
		{name: "discrepancy_resolution_timeout", value: params.DiscrepancyResolutionTimeout},
		{name: "discrepancy_grace_period", value: params.DiscrepancyGracePeriod},
		{name: "contact_response_critical_l1", value: params.ContactResponseCriticalL1},
		{name: "contact_response_critical_l2", value: params.ContactResponseCriticalL2},
		{name: "contact_response_critical_l3", value: params.ContactResponseCriticalL3},
		{name: "contact_response_critical_l4", value: params.ContactResponseCriticalL4},
		{name: "contact_response_standard_l1", value: params.ContactResponseStandardL1},
		{name: "contact_response_standard_l2", value: params.ContactResponseStandardL2},
		{name: "contact_response_standard_l3", value: params.ContactResponseStandardL3},
		{name: "contact_response_standard_l4", value: params.ContactResponseStandardL4},
	}

	for _, param := range durations {
		if param.value <= 0 {
			return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be positive", param.name)
		}
	}

	return nil
}

func validateParamScalars(params vtypes.Params) error {
	if params.DiscrepancyThreshold == 0 {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "params.discrepancy_threshold must be positive")
	}
	if err := validateBasisPoints("min_lease_completion_bps_l3", params.MinLeaseCompletionBpsL3); err != nil {
		return err
	}
	if err := validateBasisPoints("min_lease_completion_bps_l4", params.MinLeaseCompletionBpsL4); err != nil {
		return err
	}
	if params.MinLeasesForCompletionRate == 0 {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "params.min_leases_for_completion_rate must be positive")
	}

	endBlockerCaps := []struct {
		name  string
		value uint32
	}{
		{name: "max_endblocker_attestation_expiries", value: params.MaxEndblockerAttestationExpiries},
		{name: "max_endblocker_snapshot_suspensions", value: params.MaxEndblockerSnapshotSuspensions},
		{name: "max_endblocker_unbonding_completions", value: params.MaxEndblockerUnbondingCompletions},
		{name: "max_endblocker_discrepancy_timeouts", value: params.MaxEndblockerDiscrepancyTimeouts},
		{name: "max_endblocker_audit_escrow_expiries", value: params.MaxEndblockerAuditEscrowExpiries},
		{name: "max_endblocker_grace_expiries", value: params.MaxEndblockerGraceExpiries},
	}
	for _, cap := range endBlockerCaps {
		if cap.value == 0 {
			return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be positive", cap.name)
		}
	}

	return nil
}

func validateBasisPoints(name string, value uint32) error {
	if value == 0 || value > maxBasisPoints {
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be between 1 and %d", name, maxBasisPoints)
	}
	return nil
}

func validateParamRelationships(params vtypes.Params) error {
	if err := validateCoinAscending("bond_l1", params.BondL1, "bond_l2", params.BondL2); err != nil {
		return err
	}
	if err := validateCoinAscending("bond_l2", params.BondL2, "bond_l3", params.BondL3); err != nil {
		return err
	}
	if err := validateCoinAscending("bond_l3", params.BondL3, "bond_l4", params.BondL4); err != nil {
		return err
	}

	if err := validateCoinAscending("min_fee_l1", params.MinFeeL1, "min_fee_l2", params.MinFeeL2); err != nil {
		return err
	}
	if err := validateCoinAscending("min_fee_l2", params.MinFeeL2, "min_fee_l3", params.MinFeeL3); err != nil {
		return err
	}
	if err := validateCoinAscending("min_fee_l3", params.MinFeeL3, "min_fee_l4", params.MinFeeL4); err != nil {
		return err
	}

	resourceBonds := [][3]coinParam{
		{
			{name: "bond_gpu_l2", coin: params.BondGpuL2},
			{name: "bond_gpu_l3", coin: params.BondGpuL3},
			{name: "bond_gpu_l4", coin: params.BondGpuL4},
		},
		{
			{name: "bond_vcpu_l2", coin: params.BondVcpuL2},
			{name: "bond_vcpu_l3", coin: params.BondVcpuL3},
			{name: "bond_vcpu_l4", coin: params.BondVcpuL4},
		},
		{
			{name: "bond_mem_gb_l2", coin: params.BondMemGbL2},
			{name: "bond_mem_gb_l3", coin: params.BondMemGbL3},
			{name: "bond_mem_gb_l4", coin: params.BondMemGbL4},
		},
		{
			{name: "bond_storage_tb_l2", coin: params.BondStorageTbL2},
			{name: "bond_storage_tb_l3", coin: params.BondStorageTbL3},
			{name: "bond_storage_tb_l4", coin: params.BondStorageTbL4},
		},
	}
	for _, tiered := range resourceBonds {
		if err := validateCoinAscending(tiered[0].name, tiered[0].coin, tiered[1].name, tiered[1].coin); err != nil {
			return err
		}
		if err := validateCoinAscending(tiered[1].name, tiered[1].coin, tiered[2].name, tiered[2].coin); err != nil {
			return err
		}
	}

	relationships := [][2]durationParam{
		{
			{name: "ttl_l2", value: params.TtlL2},
			{name: "ttl_l1", value: params.TtlL1},
		},
		{
			{name: "ttl_l3", value: params.TtlL3},
			{name: "ttl_l2", value: params.TtlL2},
		},
		{
			{name: "ttl_l4", value: params.TtlL4},
			{name: "ttl_l3", value: params.TtlL3},
		},
		{
			{name: "ttl_l1", value: params.TtlL1},
			{name: "renewal_period_l1", value: params.RenewalPeriodL1},
		},
		{
			{name: "ttl_l2", value: params.TtlL2},
			{name: "renewal_period_l2", value: params.RenewalPeriodL2},
		},
		{
			{name: "ttl_l3", value: params.TtlL3},
			{name: "renewal_period_l3", value: params.RenewalPeriodL3},
		},
		{
			{name: "ttl_l4", value: params.TtlL4},
			{name: "renewal_period_l4", value: params.RenewalPeriodL4},
		},
		{
			{name: "renewal_period_l2", value: params.RenewalPeriodL2},
			{name: "renewal_period_l1", value: params.RenewalPeriodL1},
		},
		{
			{name: "renewal_period_l3", value: params.RenewalPeriodL3},
			{name: "renewal_period_l2", value: params.RenewalPeriodL2},
		},
		{
			{name: "renewal_period_l4", value: params.RenewalPeriodL4},
			{name: "renewal_period_l3", value: params.RenewalPeriodL3},
		},
		{
			{name: "max_snapshot_age", value: params.MaxSnapshotAge},
			{name: "snapshot_hash_interval", value: params.SnapshotHashInterval},
		},
		{
			{name: "min_age_l2", value: params.MinAgeL2},
			{name: "min_age_l3", value: params.MinAgeL3},
		},
		{
			{name: "min_age_l3", value: params.MinAgeL3},
			{name: "min_age_l4", value: params.MinAgeL4},
		},
		{
			{name: "clean_history_window_l3", value: params.CleanHistoryWindowL3},
			{name: "clean_history_window_l4", value: params.CleanHistoryWindowL4},
		},
		{
			{name: "discrepancy_grace_period", value: params.DiscrepancyGracePeriod},
			{name: "discrepancy_resolution_timeout", value: params.DiscrepancyResolutionTimeout},
		},
		{
			{name: "contact_response_critical_l1", value: params.ContactResponseCriticalL1},
			{name: "contact_response_standard_l1", value: params.ContactResponseStandardL1},
		},
		{
			{name: "contact_response_critical_l2", value: params.ContactResponseCriticalL2},
			{name: "contact_response_standard_l2", value: params.ContactResponseStandardL2},
		},
		{
			{name: "contact_response_critical_l3", value: params.ContactResponseCriticalL3},
			{name: "contact_response_standard_l3", value: params.ContactResponseStandardL3},
		},
		{
			{name: "contact_response_critical_l4", value: params.ContactResponseCriticalL4},
			{name: "contact_response_standard_l4", value: params.ContactResponseStandardL4},
		},
		{
			{name: "contact_response_critical_l2", value: params.ContactResponseCriticalL2},
			{name: "contact_response_critical_l1", value: params.ContactResponseCriticalL1},
		},
		{
			{name: "contact_response_critical_l3", value: params.ContactResponseCriticalL3},
			{name: "contact_response_critical_l2", value: params.ContactResponseCriticalL2},
		},
		{
			{name: "contact_response_critical_l4", value: params.ContactResponseCriticalL4},
			{name: "contact_response_critical_l3", value: params.ContactResponseCriticalL3},
		},
		{
			{name: "contact_response_standard_l2", value: params.ContactResponseStandardL2},
			{name: "contact_response_standard_l1", value: params.ContactResponseStandardL1},
		},
		{
			{name: "contact_response_standard_l3", value: params.ContactResponseStandardL3},
			{name: "contact_response_standard_l2", value: params.ContactResponseStandardL2},
		},
		{
			{name: "contact_response_standard_l4", value: params.ContactResponseStandardL4},
			{name: "contact_response_standard_l3", value: params.ContactResponseStandardL3},
		},
	}
	for _, relationship := range relationships {
		if err := validateDurationAtMost(relationship[0], relationship[1]); err != nil {
			return err
		}
	}

	if params.MinLeaseCompletionBpsL4 < params.MinLeaseCompletionBpsL3 {
		return errorsmod.Wrap(moduletypes.ErrInvalidReason, "params.min_lease_completion_bps_l4 must be >= params.min_lease_completion_bps_l3")
	}

	return nil
}

func validateCoinAscending(lowName string, low sdk.Coin, highName string, high sdk.Coin) error {
	if low.Amount.GT(high.Amount) {
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be <= params.%s", lowName, highName)
	}
	return nil
}

func validateDurationAtMost(low durationParam, high durationParam) error {
	if low.value > high.value {
		return errorsmod.Wrapf(moduletypes.ErrInvalidReason, "params.%s must be <= params.%s", low.name, high.name)
	}
	return nil
}

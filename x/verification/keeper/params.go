package keeper

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"
)

const bondDenom = "uakt"

func DefaultParams() vtypes.Params {
	return vtypes.Params{
		BondL1:                            aktCoin(1000),
		BondL2:                            aktCoin(5000),
		BondL3:                            aktCoin(25000),
		BondL4:                            aktCoin(100000),
		TtlL1:                             365 * 24 * time.Hour,
		TtlL2:                             180 * 24 * time.Hour,
		TtlL3:                             90 * 24 * time.Hour,
		TtlL4:                             90 * 24 * time.Hour,
		MinFeeL1:                          aktCoin(10),
		MinFeeL2:                          aktCoin(50),
		MinFeeL3:                          aktCoin(200),
		MinFeeL4:                          aktCoin(1000),
		DiscrepancyThreshold:              1,
		AuditorUnbondingPeriod:            21 * 24 * time.Hour,
		RenewalPeriodL1:                   730 * 24 * time.Hour,
		RenewalPeriodL2:                   540 * 24 * time.Hour,
		RenewalPeriodL3:                   365 * 24 * time.Hour,
		RenewalPeriodL4:                   180 * 24 * time.Hour,
		SnapshotHashInterval:              24 * time.Hour,
		MaxSnapshotAge:                    time.Hour,
		BondGpuL2:                         aktCoin(50),
		BondGpuL3:                         aktCoin(100),
		BondGpuL4:                         aktCoin(200),
		BondVcpuL2:                        microAKT(500000),
		BondVcpuL3:                        aktCoin(1),
		BondVcpuL4:                        aktCoin(2),
		BondMemGbL2:                       microAKT(250000),
		BondMemGbL3:                       microAKT(500000),
		BondMemGbL4:                       aktCoin(1),
		BondStorageTbL2:                   aktCoin(2),
		BondStorageTbL3:                   aktCoin(4),
		BondStorageTbL4:                   aktCoin(8),
		ProviderBondUnbondingPeriod:       21 * 24 * time.Hour,
		MinAgeL2:                          30 * 24 * time.Hour,
		MinAgeL3:                          120 * 24 * time.Hour,
		MinAgeL4:                          300 * 24 * time.Hour,
		MinLeaseCompletionBpsL3:           9800,
		MinLeaseCompletionBpsL4:           9800,
		CleanHistoryWindowL3:              90 * 24 * time.Hour,
		CleanHistoryWindowL4:              180 * 24 * time.Hour,
		MinL3DurationForL4:                180 * 24 * time.Hour,
		MinLeasesForCompletionRate:        10,
		MaxEndblockerAttestationExpiries:  100,
		MaxEndblockerSnapshotSuspensions:  50,
		MaxEndblockerUnbondingCompletions: 50,
		MaxEndblockerDiscrepancyTimeouts:  10,
		MaxEndblockerAuditEscrowExpiries:  50,
		MaxEndblockerGraceExpiries:        50,
		DiscrepancyResolutionTimeout:      90 * 24 * time.Hour,
		AttestationDeposit:                aktCoin(100),
		DiscrepancyGracePeriod:            14 * 24 * time.Hour,
		ProviderAuditDeposit:              aktCoin(100),
		VerificationModuleActive:          false,
		ContactResponseCriticalL1:         72 * time.Hour,
		ContactResponseCriticalL2:         24 * time.Hour,
		ContactResponseCriticalL3:         4 * time.Hour,
		ContactResponseCriticalL4:         time.Hour,
		ContactResponseStandardL1:         7 * 24 * time.Hour,
		ContactResponseStandardL2:         72 * time.Hour,
		ContactResponseStandardL3:         24 * time.Hour,
		ContactResponseStandardL4:         4 * time.Hour,
	}
}

func aktCoin(amount int64) sdk.Coin {
	return microAKT(amount * 1000000)
}

func microAKT(amount int64) sdk.Coin {
	return sdk.NewInt64Coin(bondDenom, amount)
}

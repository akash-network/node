package keeper

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	vtypes "pkg.akt.dev/go/node/verification/v1"
)

func requiredProviderBond(params vtypes.Params, tier vtypes.VerificationTier, resources vtypes.ResourceSummary) sdk.Coin {
	switch tier {
	case vtypes.TierVerified:
		return sumResourceBond(resources, params.BondGpuL2, params.BondVcpuL2, params.BondMemGbL2, params.BondStorageTbL2)
	case vtypes.TierEstablished:
		return sumResourceBond(resources, params.BondGpuL3, params.BondVcpuL3, params.BondMemGbL3, params.BondStorageTbL3)
	case vtypes.TierTrusted:
		return sumResourceBond(resources, params.BondGpuL4, params.BondVcpuL4, params.BondMemGbL4, params.BondStorageTbL4)
	case vtypes.TierUnspecified, vtypes.TierIdentified:
		return sdk.NewCoin(params.BondL1.Denom, math.ZeroInt())
	default:
		panic("verification: unknown tier")
	}
}

func sumResourceBond(resources vtypes.ResourceSummary, gpu, vcpu, memGB, storageTB sdk.Coin) sdk.Coin {
	amount := math.ZeroInt()
	denom := gpu.Denom

	amount = amount.Add(gpu.Amount.MulRaw(int64(resources.TotalGPUs)))
	amount = amount.Add(vcpu.Amount.MulRaw(int64(resources.TotalVCPUs)))
	amount = amount.Add(memGB.Amount.MulRaw(int64(resources.TotalMemoryMB)).QuoRaw(1024))
	amount = amount.Add(storageTB.Amount.MulRaw(int64(resources.TotalStorageMB)).QuoRaw(1048576))

	return sdk.NewCoin(denom, amount)
}

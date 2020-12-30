package util

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/manifest"
	atypes "github.com/ovrclk/akash/types"
	"math"
)

func ShouldBeIngress(expose manifest.ServiceExpose) bool {
	return expose.Proto == manifest.TCP && expose.Global && 80 == ExposeExternalPort(expose)
}

func ExposeExternalPort(expose manifest.ServiceExpose) int32 {
	if expose.ExternalPort == 0 {
		return int32(expose.Port)
	}
	return int32(expose.ExternalPort)
}

func ComputeCommittedResources(factor float64, rv atypes.ResourceValue) atypes.ResourceValue {
	// If the value is less than 1, commit the original value. There is no concept of undercommit
	if factor <= 1.0 {
		return rv
	}

	v := rv.Val.Uint64()
	fraction := 1.0 / factor
	commitedValue := math.Round(float64(v) * fraction)

	// Don't return a value of zero, since this is used as a resource request
	if commitedValue <= 0 {
		commitedValue = 1
	}

	result := atypes.ResourceValue{
		Val: sdk.NewInt(int64(commitedValue)),
	}

	return result
}

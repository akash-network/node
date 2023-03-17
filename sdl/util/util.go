package util

import (
	"math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	atypes "github.com/akash-network/akash-api/go/node/types/v1beta3"
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
	committedValue := math.Round(float64(v) * fraction)

	// Don't return a value of zero, since this is used as a resource request
	if committedValue <= 0 {
		committedValue = 1
	}

	result := atypes.ResourceValue{
		Val: sdk.NewInt(int64(committedValue)),
	}

	return result
}

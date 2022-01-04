package util

import (
	"encoding/base32"
	"math"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	manifest "github.com/ovrclk/akash/manifest/v2beta1"

	atypes "github.com/ovrclk/akash/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	uuid "github.com/satori/go.uuid"
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

func AllHostnamesOfManifestGroup(mgroup manifest.Group) []string {
	allHostnames := make([]string, 0)
	for _, service := range mgroup.Services {
		for _, expose := range service.Expose {
			allHostnames = append(allHostnames, expose.Hosts...)
		}
	}

	return allHostnames
}

func IngressHost(lid mtypes.LeaseID, svcName string) string {
	uid := uuid.NewV5(uuid.NamespaceDNS, lid.String()+svcName).Bytes()
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(uid))
}

package util

import "github.com/ovrclk/akash/manifest"

func ShouldBeIngress(expose manifest.ServiceExpose) bool {
	return expose.Proto == manifest.TCP && expose.Global && 80 == ExposeExternalPort(expose)
}

func ExposeExternalPort(expose manifest.ServiceExpose) int32 {
	if expose.ExternalPort == 0 {
		return int32(expose.Port)
	}
	return int32(expose.ExternalPort)
}

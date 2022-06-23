package util

import (
	"context"
	"fmt"
	"net"
)

// A type that does nothing but return a result that is already existent
type staticServiceDiscoveryAgent net.SRV

func (staticServiceDiscoveryAgent) Stop()        {}
func (staticServiceDiscoveryAgent) DiscoverNow() {}
func (ssda staticServiceDiscoveryAgent) GetClient(ctx context.Context, isHTTPS, secure bool) (ServiceClient, error) {
	proto := "http"
	if isHTTPS {
		proto = "https"
	}
	url := fmt.Sprintf("%s://%v:%v", proto, ssda.Target, ssda.Port)
	return newHTTPWrapperServiceClient(isHTTPS, secure, url), nil
}

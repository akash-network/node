package cluster

import (
	"context"
	"fmt"
	lifecycle "github.com/boz/go-lifecycle"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

type reserveRequest struct {
	hostnames []string
	result    chan<- error
	doReserve bool
	dID       dtypes.DeploymentID
}

type HostnameServiceClient interface {
	ReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error
	ReleaseHostnames(hostnames []string)
	CanReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error
}

type SimpleHostnames struct {
	Hostnames map[string]dtypes.DeploymentID
	lock      sync.Mutex
} /* Used in test code */

func (sh *SimpleHostnames) ReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	ch := make(chan error, 1)
	for _, hostname := range hostnames {
		_, inUse := sh.Hostnames[hostname]
		if inUse {
			ch <- fmt.Errorf("%w: host %q in use", errHostnameNotAllowed, hostname)
			return ch
		}
		sh.Hostnames[hostname] = did
	}

	ch <- nil
	return ch
}

func (sh *SimpleHostnames) CanReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	ch := make(chan error, 1)

	for _, hostname := range hostnames {
		usedByDid, inUse := sh.Hostnames[hostname]
		if inUse && !usedByDid.Equals(did) {
			ch <- fmt.Errorf("%w: host %q in use", errHostnameNotAllowed, hostname)
			return ch
		}
	}

	ch <- nil
	return ch
}

func (sh *SimpleHostnames) ReleaseHostnames(hostnames []string) {
	sh.lock.Lock()
	defer sh.lock.Unlock()

	for _, hostname := range hostnames {
		delete(sh.Hostnames, hostname)
	}
}

type hostnameService struct {
	inUse map[string]dtypes.DeploymentID

	requests chan reserveRequest
	releases chan []string
	lc       lifecycle.Lifecycle

	blockedHostnames []string
	blockedDomains   []string
}

const HostnameSeparator = '.'

func newHostnameService(ctx context.Context, cfg Config) *hostnameService {

	blockedHostnames := make([]string, 0)
	blockedDomains := make([]string, 0)
	for _, name := range cfg.BlockedHostnames {

		if len(name) != 0 && name[0] == HostnameSeparator {
			blockedDomains = append(blockedDomains, name)
			blockedHostnames = append(blockedHostnames, name[1:])
		} else {
			blockedHostnames = append(blockedHostnames, name)
		}
	}

	hs := &hostnameService{
		inUse:            make(map[string]dtypes.DeploymentID),
		blockedHostnames: blockedHostnames,
		blockedDomains:   blockedDomains,
		requests:         make(chan reserveRequest),
		releases:         make(chan []string),
		lc:               lifecycle.New(),
	}

	go hs.lc.WatchContext(ctx)
	go hs.run()

	return hs
}

func (hs *hostnameService) run() {
	defer hs.lc.ShutdownCompleted()

loop:
	for {

		// Wait for any service to finish
		select {
		case <-hs.lc.ShutdownRequest():
			hs.lc.ShutdownInitiated(nil)
			break loop
		case rr := <-hs.requests:
			hs.doRequest(rr)
		case hostnames := <-hs.releases:
			hs.doRelease(hostnames)
		}
	}

}

var errHostnameNotAllowed = errors.New("hostname not allowed")

func (hs *hostnameService) isHostnameBlocked(hostname string) error {
	for _, blockedHostname := range hs.blockedHostnames {
		if blockedHostname == hostname {
			return fmt.Errorf("%w: %q is blocked by this provider", errHostnameNotAllowed, hostname)
		}
	}

	for _, blockedDomain := range hs.blockedDomains {
		if strings.HasSuffix(hostname, blockedDomain) {
			return fmt.Errorf("%w: domain %q is blocked by this provider", errHostnameNotAllowed, hostname)
		}
	}

	return nil
}

func (hs *hostnameService) doRequest(rr reserveRequest) {
	// check if hostname is blocked
	for _, hostname := range rr.hostnames {
		blockedErr := hs.isHostnameBlocked(hostname)
		if blockedErr != nil {
			rr.result <- blockedErr
			return
		}
		takenBy, hostnameTaken := hs.inUse[hostname]
		if hostnameTaken && !takenBy.Equals(rr.dID) {
			rr.result <- fmt.Errorf("%w: %q already in use", errHostnameNotAllowed, hostname)
			return
		}
	}

	if rr.doReserve {
		for _, hostname := range rr.hostnames {
			hs.inUse[hostname] = rr.dID
		}
	}

	rr.result <- nil // No error
}

func (hs *hostnameService) doRelease(hostnames []string) {
	for _, hostname := range hostnames {
		delete(hs.inUse, hostname)
	}
}

func (hs *hostnameService) ReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error {
	returnValue := make(chan error, 1) // Buffer of one so service does not block
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}
	request := reserveRequest{
		hostnames: lowercaseHostnames,
		result:    returnValue,
		doReserve: true, // reserve hostnames
		dID:       did,
	}

	select {
	case hs.requests <- request:

	case <-hs.lc.ShuttingDown():
		returnValue <- ErrNotRunning
	}

	return returnValue
}

func (hs *hostnameService) ReleaseHostnames(hostnames []string) {
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}
	select {
	case hs.releases <- lowercaseHostnames:
	case <-hs.lc.ShuttingDown():
		// service is shutting down, so release doesn't matter
	}
}

func (hs *hostnameService) CanReserveHostnames(hostnames []string, did dtypes.DeploymentID) <-chan error {
	returnValue := make(chan error, 1) // Buffer of one so service does not block
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}
	request := reserveRequest{
		hostnames: lowercaseHostnames,
		result:    returnValue,
		doReserve: false, // do not actually reserve hostnames
		dID:       did,
	}

	select {
	case hs.requests <- request:

	case <-hs.lc.ShuttingDown():
		returnValue <- ErrNotRunning
	}

	return returnValue
}

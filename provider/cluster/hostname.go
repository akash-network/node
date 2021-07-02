package cluster

import (
	"context"
	"fmt"
	lifecycle "github.com/boz/go-lifecycle"
	cosmostypes "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

type HostnameServiceClient interface {
	ReserveHostnames(hostnames []string, dID dtypes.DeploymentID) ReservationResult
	ReleaseHostnames(dID dtypes.DeploymentID)
	CanReserveHostnames(hostnames []string, ownerAddr cosmostypes.Address) <-chan error
}

type SimpleHostnames struct {
	Hostnames map[string]dtypes.DeploymentID
	lock      sync.Mutex
} /* Used in test code */

type ReplacedHostname struct {
	Hostname                   string
	PreviousDeploymentSequence uint64
}

type ReservationResult struct {
	ChErr               <-chan error
	ChReplacedHostnames <-chan []ReplacedHostname
}

func (rr ReservationResult) Wait(wait <-chan struct{}) ([]ReplacedHostname, error) {
	select {
	case err := <-rr.ChErr:
		if err != nil {
			return nil, err
		}
	case <-wait:
		return nil, errors.New("bob")
	}

	return <-rr.ChReplacedHostnames, nil
}

func (sh *SimpleHostnames) ReserveHostnames(hostnames []string, dID dtypes.DeploymentID) ReservationResult {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	errCh := make(chan error, 1)
	resultCh := make(chan []ReplacedHostname, 1)

	result := ReservationResult{
		ChErr:               errCh,
		ChReplacedHostnames: resultCh,
	}
	reserveHostnamesImpl(sh.Hostnames, hostnames, dID, errCh, resultCh)
	return result
}

func reserveHostnamesImpl(store map[string]dtypes.DeploymentID, hostnames []string, dID dtypes.DeploymentID, ch chan<- error, resultCh chan<- []ReplacedHostname) {
	reservingHostnameAddr, err := dID.GetOwnerAddress()
	if err != nil {
		ch <- err
		return
	}

	replacedHostnames := make([]ReplacedHostname, 0)
	for _, hostname := range hostnames {
		// Check if in use
		usedByDid, inUse := store[hostname]
		if inUse {
			// Check to see if the same address already is using this hostname
			existingOwnerAddr, err := usedByDid.GetOwnerAddress()
			if err != nil {
				ch <- err
				return
			}
			if !existingOwnerAddr.Equals(reservingHostnameAddr) {
				// The owner is not the same, this can't be done
				ch <- fmt.Errorf("%w: host %q in use", errHostnameNotAllowed, hostname)
				return
			}

			// Check for a deployment replacing another one
			if dID.DSeq != usedByDid.DSeq {
				// Record that the hostname is being replaced
				replacedHostnames = append(replacedHostnames, ReplacedHostname{
					Hostname:                   hostname,
					PreviousDeploymentSequence: usedByDid.DSeq,
				})
			}
		}
	}

	// There was no error, mark everything as in use
	for _, hostname := range hostnames {
		store[hostname] = dID
	}
	ch <- nil
	resultCh <- replacedHostnames
	return
}

func (sh *SimpleHostnames) CanReserveHostnames(hostnames []string, ownerAddr cosmostypes.Address) <-chan error {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	ch := make(chan error, 1)
	canReserveHostnamesImpl(sh.Hostnames, hostnames, ownerAddr, ch)
	return ch
}

func canReserveHostnamesImpl(store map[string]dtypes.DeploymentID, hostnames []string, ownerAddr cosmostypes.Address, chErr chan<- error) {
	for _, hostname := range hostnames {
		usedByDid, inUse := store[hostname]

		if inUse {
			existingOwnerAddr, err := usedByDid.GetOwnerAddress()
			if err != nil {
				chErr <- err
				return

			}
			if !existingOwnerAddr.Equals(ownerAddr) {
				chErr <- fmt.Errorf("%w: host %q in use", errHostnameNotAllowed, hostname)
				return
			}
		}
	}

	chErr <- nil
}

func (sh *SimpleHostnames) ReleaseHostnames(dID dtypes.DeploymentID) {
	sh.lock.Lock()
	defer sh.lock.Unlock()

	releaseHostnamesImpl(sh.Hostnames, dID)
}

func releaseHostnamesImpl(store map[string]dtypes.DeploymentID, dID dtypes.DeploymentID) {
	var toDelete []string
	for hostname, existingDeployment := range store {
		if existingDeployment.Equals(dID) {
			toDelete = append(toDelete, hostname)
		}
	}

	for _, hostname := range toDelete {
		delete(store, hostname)
	}
}

type reserveRequest struct {
	chErr               chan<- error
	chReplacedHostnames chan<- []ReplacedHostname
	hostnames           []string
	deploymentID        dtypes.DeploymentID
}

type canReserveRequest struct {
	hostnames []string
	result    chan<- error
	ownerAddr cosmostypes.Address
}

type hostnameService struct {
	inUse map[string]dtypes.DeploymentID

	requests   chan reserveRequest
	canRequest chan canReserveRequest
	releases   chan dtypes.DeploymentID
	lc         lifecycle.Lifecycle

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
		canRequest:       make(chan canReserveRequest),
		releases:         make(chan dtypes.DeploymentID),
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
		case crr := <-hs.canRequest:

			canReserveHostnamesImpl(hs.inUse, crr.hostnames, crr.ownerAddr, crr.result)
		case dID := <-hs.releases:
			releaseHostnamesImpl(hs.inUse, dID)
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
			rr.chErr <- blockedErr
			return
		}
	}

	reserveHostnamesImpl(hs.inUse, rr.hostnames, rr.deploymentID, rr.chErr, rr.chReplacedHostnames)
}

func (hs *hostnameService) ReserveHostnames(hostnames []string, dID dtypes.DeploymentID) ReservationResult {
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}

	chErr := make(chan error, 1)                            // Buffer of one so service does not block
	chReplacedHostnames := make(chan []ReplacedHostname, 1) // Buffer of one so service does not block

	request := reserveRequest{
		chErr:               chErr,
		chReplacedHostnames: chReplacedHostnames,
		hostnames:           lowercaseHostnames,
		deploymentID:        dID,
	}

	select {
	case hs.requests <- request:

	case <-hs.lc.ShuttingDown():
		chErr <- ErrNotRunning
	}

	return ReservationResult{
		ChErr:               chErr,
		ChReplacedHostnames: chReplacedHostnames,
	}
}

func (hs *hostnameService) ReleaseHostnames(dID dtypes.DeploymentID) {
	select {
	case hs.releases <- dID:
	case <-hs.lc.ShuttingDown():
		// service is shutting down, so release doesn't matter
	}
}

func (hs *hostnameService) CanReserveHostnames(hostnames []string, ownerAddr cosmostypes.Address) <-chan error {
	returnValue := make(chan error, 1) // Buffer of one so service does not block
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}
	request := canReserveRequest{ // do not actually reserve hostnames
		hostnames: lowercaseHostnames,
		result:    returnValue,
		ownerAddr: ownerAddr,
	}

	select {
	case hs.canRequest <- request:

	case <-hs.lc.ShuttingDown():
		returnValue <- ErrNotRunning
	}

	return returnValue
}

package cluster

import (
	"context"
	"fmt"
	lifecycle "github.com/boz/go-lifecycle"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

type HostnameServiceClient interface {
	ReserveHostnames(hostnames []string, dID dtypes.DeploymentID) ReservationResult
	ReleaseHostnames(dID dtypes.DeploymentID)
	CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) <-chan error
	PrepareHostnamesForTransfer(hostnames []string, dID dtypes.DeploymentID) <- chan error
}

type SimpleHostnames struct {
	Hostnames map[string]dtypes.DeploymentID
	lock      sync.Mutex
} /* Used in test code */

type ReservationResult struct {
	ChErr               <-chan error
	ChWithheldHostnames <-chan []string
}

func (rr ReservationResult) Wait(wait <-chan struct{}) ([]string, error) {
	select {
	case err := <-rr.ChErr:
		return nil, err
	case v := <-rr.ChWithheldHostnames:
		return v, nil
	case <-wait:
		return nil, errors.New("bob")
	}
}

func (sh *SimpleHostnames) PrepareHostnameForTransfer(hostnames []string, dID dtypes.DeploymentID) <- chan error{
	sh.lock.Lock()
	defer sh.lock.Unlock()
	errCh := make(chan error, 1)
	prepareHostnamesImpl(sh.Hostnames, hostnames, dID, errCh)
	return errCh
}

func prepareHostnamesImpl(store map[string]dtypes.DeploymentID, hostnames []string, dID dtypes.DeploymentID, errCh chan <- error){
	ownerAddr, err := dID.GetOwnerAddress()
	if err != nil {
		errCh <- err
		return
	}
	toChange := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		existingDID, ok := store[hostname]
		if ok {
			existingOwnerAddr, err := existingDID.GetOwnerAddress()
			if err != nil {
				errCh <- err
				return
			}
			if existingOwnerAddr.Equals(ownerAddr) {
				toChange = append(toChange, hostname)
			} else {
				errCh <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
			}
		}
	}

	for _, hostname := range toChange {
		store[hostname] = dID
	}
	close(errCh)
}

func (sh *SimpleHostnames) ReserveHostnames(hostnames []string, dID dtypes.DeploymentID) ReservationResult {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	errCh := make(chan error, 1)
	resultCh := make(chan []string, 1)

	result := ReservationResult{
		ChErr:               errCh,
		ChWithheldHostnames: resultCh,
	}
	reserveHostnamesImpl(sh.Hostnames, hostnames, dID, errCh, resultCh)
	return result
}

func reserveHostnamesImpl(store map[string]dtypes.DeploymentID, hostnames []string, dID dtypes.DeploymentID, ch chan<- error, resultCh chan<- []string) {
	reservingHostnameAddr, err := dID.GetOwnerAddress()
	if err != nil {
		ch <- err
		return
	}

	withheldHostnamesMap := make(map[string]struct{})
	withheldHostnames := make([]string, 0)
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
				ch <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
				return
			}

			// Check for a deployment replacing another one
			if dID.DSeq != usedByDid.DSeq {
				// Record that the hostname is being replaced
				withheldHostnames = append(withheldHostnames, hostname)
				withheldHostnamesMap[hostname] = struct{}{}
			}
		}
	}

	// There was no error, mark everything as in use that is not withheld
	for _, hostname := range hostnames {
		_, withheld := withheldHostnamesMap[hostname]
		if !withheld {
			store[hostname] = dID
		}

	}

	resultCh <- withheldHostnames
	return
}

func (sh *SimpleHostnames) CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) <-chan error {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	ch := make(chan error, 1)
	canReserveHostnamesImpl(sh.Hostnames, hostnames, ownerAddr, ch)
	return ch
}

func canReserveHostnamesImpl(store map[string]dtypes.DeploymentID, hostnames []string, ownerAddr sdktypes.Address, chErr chan<- error) {
	for _, hostname := range hostnames {
		usedByDid, inUse := store[hostname]

		if inUse {
			existingOwnerAddr, err := usedByDid.GetOwnerAddress()
			if err != nil {
				chErr <- err
				return

			}
			if !existingOwnerAddr.Equals(ownerAddr) {
				chErr <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
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
	chReplacedHostnames chan<- []string
	hostnames           []string
	deploymentID        dtypes.DeploymentID
}

type canReserveRequest struct {
	hostnames []string
	result    chan<- error
	ownerAddr sdktypes.Address
}

type prepareTransferRequest struct {
	hostnames []string
	deploymentID        dtypes.DeploymentID
	chErr chan<- error
}

type hostnameService struct {
	inUse map[string]dtypes.DeploymentID

	requests   chan reserveRequest
	canRequest chan canReserveRequest
	prepareRequest chan prepareTransferRequest
	releases   chan dtypes.DeploymentID
	lc         lifecycle.Lifecycle

	blockedHostnames []string
	blockedDomains   []string
}


const HostnameSeparator = '.'

func newHostnameService(ctx context.Context, cfg Config, initialData map[string]dtypes.DeploymentID) *hostnameService {
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
		inUse:            make(map[string]dtypes.DeploymentID, len(initialData)),
		blockedHostnames: blockedHostnames,
		blockedDomains:   blockedDomains,
		requests:         make(chan reserveRequest),
		canRequest:       make(chan canReserveRequest),
		releases:         make(chan dtypes.DeploymentID),
		lc:               lifecycle.New(),
		prepareRequest: make(chan prepareTransferRequest),
	}
	for k, v := range initialData {
		hs.inUse[k] = v
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
		case request := <-hs.prepareRequest:
			prepareHostnamesImpl(hs.inUse, request.hostnames, request.deploymentID, request.chErr)

		}
	}

}

var ErrHostnameNotAllowed = errors.New("hostname not allowed")

func (hs *hostnameService) PrepareHostnamesForTransfer(hostnames []string, dID dtypes.DeploymentID) <- chan error {
	chErr := make(chan error, 1)

	v:= prepareTransferRequest{
		hostnames: hostnames,
		deploymentID: dID,
		chErr:     chErr,
	}
	select {
		case hs.prepareRequest <- v:
	case <-hs.lc.ShuttingDown():
		chErr <- ErrNotRunning
	}

	return chErr
}

func (hs *hostnameService) isHostnameBlocked(hostname string) error {
	for _, blockedHostname := range hs.blockedHostnames {
		if blockedHostname == hostname {
			return fmt.Errorf("%w: %q is blocked by this provider", ErrHostnameNotAllowed, hostname)
		}
	}

	for _, blockedDomain := range hs.blockedDomains {
		if strings.HasSuffix(hostname, blockedDomain) {
			return fmt.Errorf("%w: domain %q is blocked by this provider", ErrHostnameNotAllowed, hostname)
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
	chWithheldHostnames := make(chan []string, 1) // Buffer of one so service does not block

	request := reserveRequest{
		chErr:               chErr,
		chReplacedHostnames: chWithheldHostnames,
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
		ChWithheldHostnames: chWithheldHostnames,
	}
}

func (hs *hostnameService) ReleaseHostnames(dID dtypes.DeploymentID) {
	select {
	case hs.releases <- dID:
	case <-hs.lc.ShuttingDown():
		// service is shutting down, so release doesn't matter
	}
}

func (hs *hostnameService) CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) <-chan error {
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

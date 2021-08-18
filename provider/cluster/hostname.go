package cluster

import (
	"context"
	"fmt"
	lifecycle "github.com/boz/go-lifecycle"
	sdktypes "github.com/cosmos/cosmos-sdk/types"

	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/pkg/errors"
	"strings"
	"sync"
)

type HostnameServiceClient interface {
	ReserveHostnames(hostnames []string, leaseID mtypes.LeaseID) ReservationResult
	ReleaseHostnames(leaseID mtypes.LeaseID) error
	CanReserveHostnames(hostnames []string, ownerAddr sdktypes.Address) <-chan error
	PrepareHostnamesForTransfer(hostnames []string, leaseID mtypes.LeaseID) <- chan error
}

/**
This type exists to identify the target of a reservation. The lease ID type is not used directly because
there is no need to consider order ID or provider ID for the purposes oft this
 */
type hostnameID struct {
	owner sdktypes.Address
	dseq uint64
	gseq uint32
}

func (hID hostnameID) Equals(other hostnameID) bool {
	return hID.gseq == other.gseq &&
		hID.dseq == other.dseq &&
		hID.owner.Equals(other.owner)
}

func hostnameIDFromLeaseID(lID mtypes.LeaseID) (hostnameID, error) {
	ownerAddr, err := lID.DeploymentID().GetOwnerAddress()
	if err != nil {
		return hostnameID{}, err
	}

	return hostnameID{
		owner: ownerAddr,
		dseq:  lID.GetDSeq(),
		gseq:  lID.GetGSeq(),
	}, nil
}

type SimpleHostnames struct {
	Hostnames map[string]hostnameID
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

func (sh *SimpleHostnames) PrepareHostnameForTransfer(hostnames []string, leaseID mtypes.LeaseID) <- chan error{
	sh.lock.Lock()
	defer sh.lock.Unlock()
	errCh := make(chan error, 1)
	hID, err := hostnameIDFromLeaseID(leaseID)
	if err != nil {
		errCh <- err
	} else {
		prepareHostnamesImpl(sh.Hostnames, hostnames, hID, errCh)
	}
	return errCh
}

func prepareHostnamesImpl(store map[string]hostnameID, hostnames []string, hID hostnameID, errCh chan <- error){
	toChange := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		existingID, ok := store[hostname]
		if ok {
			if existingID.owner.Equals(hID.owner) {
				toChange = append(toChange, hostname)
			} else {
				errCh <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
				return
			}
		}
	}

	// Swap over each hostname
	for _, hostname := range toChange {
		store[hostname] = hID
	}
	close(errCh)
}

func (sh *SimpleHostnames) ReserveHostnames(hostnames []string, leaseID mtypes.LeaseID) ReservationResult {
	sh.lock.Lock()
	defer sh.lock.Unlock()
	errCh := make(chan error, 1)
	resultCh := make(chan []string, 1)

	result := ReservationResult{
		ChErr:               errCh,
		ChWithheldHostnames: resultCh,
	}

	hID, err := hostnameIDFromLeaseID(leaseID)
	if err != nil {
		errCh <- err
	} else {
		reserveHostnamesImpl(sh.Hostnames, hostnames, hID, errCh, resultCh)
	}
	return result
}

func reserveHostnamesImpl(store map[string]hostnameID, hostnames []string, hID hostnameID, ch chan<- error, resultCh chan<- []string) {
	withheldHostnamesMap := make(map[string]struct{})
	withheldHostnames := make([]string, 0)
	for _, hostname := range hostnames {
		// Check if in use
		existingID, inUse := store[hostname]
		if inUse {
			// Check to see if the same address already is using this hostname
			if ! existingID.owner.Equals(hID.owner)  {
				// The owner is not the same, this can't be done
				ch <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
				return
			}

			// Check for a deployment replacing another one
			if ! existingID.Equals(hID) {
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
			store[hostname] = hID
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

func canReserveHostnamesImpl(store map[string]hostnameID, hostnames []string, ownerAddr sdktypes.Address, chErr chan<- error) {
	for _, hostname := range hostnames {
		existingID, inUse := store[hostname]

		if inUse {
			if !existingID.owner.Equals(ownerAddr) {
				chErr <- fmt.Errorf("%w: host %q in use", ErrHostnameNotAllowed, hostname)
				return
			}
		}
	}

	chErr <- nil
}

func (sh *SimpleHostnames) ReleaseHostnames(leaseID mtypes.LeaseID) error {
	sh.lock.Lock()
	defer sh.lock.Unlock()

	hID, err := hostnameIDFromLeaseID(leaseID)
	if err != nil {
		return err
	}

	releaseHostnamesImpl(sh.Hostnames, hID)
	return nil
}

func releaseHostnamesImpl(store map[string]hostnameID, hID hostnameID) {
	var toDelete []string
	for hostname, existing := range store {
		if existing.Equals(hID) {
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
	hID        hostnameID
}

type canReserveRequest struct {
	hostnames []string
	result    chan<- error
	ownerAddr sdktypes.Address
}

type prepareTransferRequest struct {
	hostnames []string
	hID        hostnameID
	chErr chan<- error
}

type hostnameService struct {
	inUse map[string]hostnameID

	requests   chan reserveRequest
	canRequest chan canReserveRequest
	prepareRequest chan prepareTransferRequest
	releases   chan hostnameID
	lc         lifecycle.Lifecycle

	blockedHostnames []string
	blockedDomains   []string
}


const HostnameSeparator = '.'

func newHostnameService(ctx context.Context, cfg Config, initialData map[string]mtypes.LeaseID) (*hostnameService, error) {
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
		inUse:            make(map[string]hostnameID, len(initialData)),
		blockedHostnames: blockedHostnames,
		blockedDomains:   blockedDomains,
		requests:         make(chan reserveRequest),
		canRequest:       make(chan canReserveRequest),
		releases:         make(chan hostnameID),
		lc:               lifecycle.New(),
		prepareRequest: make(chan prepareTransferRequest),
	}
	for k, v := range initialData {
		hID, err := hostnameIDFromLeaseID(v)
		if err != nil {
			return nil, err
		}
		hs.inUse[k] = hID
	}

	go hs.lc.WatchContext(ctx)
	go hs.run()

	return hs, nil
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
		case v := <-hs.releases:
			releaseHostnamesImpl(hs.inUse, v)
		case request := <-hs.prepareRequest:
			prepareHostnamesImpl(hs.inUse, request.hostnames, request.hID, request.chErr)

		}
	}

}

var ErrHostnameNotAllowed = errors.New("hostname not allowed")

func (hs *hostnameService) PrepareHostnamesForTransfer(hostnames []string, leaseID mtypes.LeaseID) <- chan error {
	chErr := make(chan error, 1)

	hID, err := hostnameIDFromLeaseID(leaseID)
	if err != nil {
		chErr <- err
		return chErr
	}

	v:= prepareTransferRequest{
		hostnames: hostnames,
		hID: hID,
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

	reserveHostnamesImpl(hs.inUse, rr.hostnames, rr.hID, rr.chErr, rr.chReplacedHostnames)
}

func (hs *hostnameService) ReserveHostnames(hostnames []string, leaseID mtypes.LeaseID) ReservationResult {
	lowercaseHostnames := make([]string, len(hostnames))
	for i, hostname := range hostnames {
		lowercaseHostnames[i] = strings.ToLower(hostname)
	}

	chErr := make(chan error, 1)                  // Buffer of one so service does not block
	chWithheldHostnames := make(chan []string, 1) // Buffer of one so service does not block

	hID, err := hostnameIDFromLeaseID(leaseID)

	if err != nil {
		chErr <- err
	} else {
		request := reserveRequest{
			chErr:               chErr,
			chReplacedHostnames: chWithheldHostnames,
			hostnames:           lowercaseHostnames,
			hID:                 hID,
		}

		select {
		case hs.requests <- request:

		case <-hs.lc.ShuttingDown():
			chErr <- ErrNotRunning
		}
	}

	return ReservationResult{
		ChErr:               chErr,
		ChWithheldHostnames: chWithheldHostnames,
	}
}

func (hs *hostnameService) ReleaseHostnames(leaseID mtypes.LeaseID) error {
	hID, err := hostnameIDFromLeaseID(leaseID)
	if err != nil {
		return err
	}
	select {
	case hs.releases <- hID:
	case <-hs.lc.ShuttingDown():
		// service is shutting down, so release doesn't matter
	}
	return nil
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

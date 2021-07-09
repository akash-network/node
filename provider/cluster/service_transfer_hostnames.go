package cluster

import (
	"context"
	"errors"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/pubsub"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/tendermint/tendermint/libs/log"
	"time"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

type transferHostnamesRequest struct {
	hostnames []string
	destination dtypes.DeploymentID
	errCh chan <- error
}

type transferHostnamesManager struct {
	hostnames []string
	ownerAddr sdktypes.Address
	destinationID dtypes.DeploymentID
	toPurge map[mtypes.LeaseID]*deploymentManager
	destination []*deploymentManager

	cancel context.CancelFunc

	log log.Logger

	client Client
}

var errDestintationDeploymentDoesNotExist = errors.New("destination deployment does not exist")
var errTransferRunning = errors.New("hostname transfer already running")

func (s *service) transferHostnames(req transferHostnamesRequest) error {
	ownerAddr, err := req.destination.GetOwnerAddress()
	if err != nil {
		return err
	}

	// One transfer running at any time per owner, to avoid any strange race conditions
	_, exists := s.xferHostnameManagers[ownerAddr.String()]
	if exists {
		return errTransferRunning
	}

	xferMgr := &transferHostnamesManager{
		hostnames: req.hostnames,
		ownerAddr: ownerAddr,
		destinationID: req.destination,
		toPurge: make(map[mtypes.LeaseID]*deploymentManager),
		log : s.log.With("owner", ownerAddr.String()),
		client: s.client,
	}
	s.xferHostnameManagers[ownerAddr.String()] = xferMgr

	// Find all deployment managers that need to be forced to update
	for managerLeaseID, mgr := range s.managers {
		leaseOwnerAddr, err := managerLeaseID.DeploymentID().GetOwnerAddress()
		if err != nil {
			return err
		}
		// sort into groups to either be purged or the destination groups
		if req.destination.Equals(managerLeaseID.DeploymentID()) {
			xferMgr.destination = append(xferMgr.destination, mgr)
		} else if ownerAddr.Equals(leaseOwnerAddr) {
			xferMgr.toPurge[managerLeaseID] = mgr
		}
	}

	// Check that the destination is running
	if len(xferMgr.destination) == 0 {
		xferMgr.log.Error("destination deployment does not exist", "dseq", req.destination.DSeq)
		return errDestintationDeploymentDoesNotExist
	}

	var ctx context.Context
	ctx, xferMgr.cancel = context.WithCancel(context.Background())
	go func() {
		select {
		case <- s.lc.ShuttingDown():
			xferMgr.cancel()
		case <- ctx.Done():
			return
		}
	}()

	var ready chan struct{}

	// If there is at least 1 source manager, then run the purge routines
	if len(xferMgr.toPurge) != 0 {
		sub, err := s.bus.Subscribe()
		if err != nil {
			xferMgr.cancel()
			return err
		}

		toCheck, toSkip := xferMgr.waitForPurge(ctx, sub)
		ready = make(chan struct{})
		xferMgr.waitForHostnamesAvailable(ctx, toCheck, toSkip, ready)
	}

	xferMgr.completeTransferWhenReady(ctx, ready, s.xferHostnameManagerCh)

	return nil
}

func checkForStringIntersection(lhs, rhs []string) bool{
	m := make(map[string]struct{})
	for _, v := range lhs {
		m[v] = struct{}{}
	}

	for _, v := range rhs {
		_, ok := m[v]
		if ok {
			return true
		}
	}

	return false
}
func (xferMgr *transferHostnamesManager) failNow(){
	xferMgr.cancel()
}

func (xferMgr *transferHostnamesManager) completeTransferWhenReady(ctx context.Context, ready <- chan struct{}, done chan <- *transferHostnamesManager) {
	go func (){
		defer func (){
			// Tell the parent we are complete
			done <- xferMgr
		}()

		if ready != nil {
			// Wait for everything to be ready or for something to fail
			select {
			case <-ready:
			case <-ctx.Done():
				return
			}
		}

		xferMgr.cancel() // Cancel everything
		xferMgr.log.Info("running final update to takeover hostnames")
		// Run the destination so it can pickup the hostnames
		for _, dst := range xferMgr.destination {
			err := dst.force()
			if err != nil {
				xferMgr.log.Error("failed final update", "err", err)
			}
		}
	}()
}

 func (xferMgr *transferHostnamesManager) waitForPurge(ctx context.Context, sub pubsub.Subscriber) (<- chan mtypes.LeaseID, <- chan mtypes.LeaseID){
	 toCheck := make(chan mtypes.LeaseID, len(xferMgr.toPurge))
	 toSkip := make(chan mtypes.LeaseID, len(xferMgr.toPurge))

	 go func() {
		 defer sub.Close()
		 sent := make(map[mtypes.LeaseID]struct{}, len(xferMgr.toPurge))

		 for k, mgr := range xferMgr.toPurge {
			 // query the cluster to determine what deployments need to
			 // actually change
			 hostnames, err := xferMgr.client.LeaseHostnames(ctx, k)
			 if err != nil {
			 	xferMgr.log.Error("failed checking for hostnames", "dseq", k.DSeq, "err", "err")
			 	xferMgr.failNow()
			 	return
			 }
			 usesAtLeastOneHostname := checkForStringIntersection(hostnames, xferMgr.hostnames)

			 if !usesAtLeastOneHostname {
			 	toSkip <- k
			 	sent[k] = struct{}{} // suppress sending this to the other channel
			 	continue
			 }

			 err = mgr.force()
			 if err != nil {
				 xferMgr.log.Error("failed forcing purge", "dseq", k.DSeq, "err", err)
				 xferMgr.failNow()
				 return
			 }
		 }

		 for {
			 var ev pubsub.Event
			 select {
			 case ev = <-sub.Events():

			 case <-ctx.Done():
				 return
			 }

			 deploymentEvent, ok := ev.(event.ClusterDeployment)
			 if !ok {
				 continue
			 }

			 if deploymentEvent.Status != event.ClusterDeploymentUpdated {
				 continue
			 }

			 _, isWaitingOn := xferMgr.toPurge[deploymentEvent.LeaseID]
			 if !isWaitingOn {
				 continue
			 }

			 _, alreadySent := sent[deploymentEvent.LeaseID]
			 if !alreadySent {
				 xferMgr.log.Info("Got deployment update event for", "lease", deploymentEvent.LeaseID)
				 toCheck <- deploymentEvent.LeaseID
				 sent[deploymentEvent.LeaseID] = struct{}{}
			 }

			 if len(sent) == len(xferMgr.toPurge) {
			 	break
			 }
		 }
	 }()

	 return toCheck, toSkip
 }

 func (xferMgr *transferHostnamesManager) waitForHostnamesAvailable(ctx context.Context, toCheck <- chan mtypes.LeaseID, toSkip <- chan mtypes.LeaseID, ready chan struct{}) {
 	go func() {
		checking := make([]mtypes.LeaseID, 0, len(xferMgr.toPurge))
		skipping := make([]mtypes.LeaseID, 0, len(xferMgr.toPurge))
		releasedHostnames := make(map[mtypes.LeaseID]struct{})
		const pollingPeriod = 10*time.Second
		// TODO - this polling behavior seems to cause a period where the ingress returns 404
		// because it briefly doesn't have a configured ingress for the hostname. This can probably be solved
		// by more agressive polling
		polling := time.NewTicker(pollingPeriod)
		for {
			select {
			case lID := <-toCheck:
				checking = append(checking, lID)
				continue
			case lID := <- toSkip:
				skipping = append(skipping, lID)
				continue
			case <-polling.C:
				polling.Stop()

			case <-ctx.Done():
				return
			}

			stillChecking := false
			for _, lID := range checking {
				_, done := releasedHostnames[lID]
				if done {
					continue
				}

				xferMgr.log.Info("Checking hostnames of", "lease", lID)
				hostnames, err := xferMgr.client.LeaseHostnames(ctx, lID)
				if err != nil {
					xferMgr.log.Error("failed checking hostnames of", "lease", lID, "err", err)
					xferMgr.failNow()
					return
				}

				inUse := checkForStringIntersection(xferMgr.hostnames, hostnames)
				if inUse {
					stillChecking = true
				} else {
					releasedHostnames[lID] = struct{}{}
				}

			}

			if !stillChecking && len(checking) + len(skipping) == len(xferMgr.toPurge) {
				break
			}
			polling.Reset(pollingPeriod)
		}
		close(ready)
	}()
}
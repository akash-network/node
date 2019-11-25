package deployment

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/cmd/common/sdutil"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
	tmctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type deployEvent string

const (
	ttl = int64(5)

	eventDeployBegin              deployEvent = "eventDeployBegin"
	eventBroadcastBegin           deployEvent = "eventBroadcastBegin"
	eventBroadcastDone            deployEvent = "eventBroadcastDone"
	eventReceiveOrdersBegin       deployEvent = "eventReceiveOrdersBegin"
	eventReceiveOrder             deployEvent = "eventReceiveOrder"
	eventReceiveOrdersDone        deployEvent = "eventReceiveOrdersDone"
	eventReceiveFulfillmentsBegin deployEvent = "eventReceiveFulfillmentsBegin"
	eventReceiveFulfillment       deployEvent = "eventReceiveFulfillment"
	eventReceiveFulfillmentsDone  deployEvent = "eventReceiveFulfillmentsDone"
	eventReceiveLeaseBegin        deployEvent = "eventReceiveLeaseBegin"
	eventReceiveLease             deployEvent = "eventReceiveLease"
	eventSendManifest             deployEvent = "eventSendManifest"
	eventSendManifestDone         deployEvent = "eventSendManifestDone"
	eventLeaseStatusFetch         deployEvent = "eventLeaseStatusFetch"
	eventReceiveLeaseDone         deployEvent = "eventReceiveLeaseDone"
	eventDeployDone               deployEvent = "eventDeployDone"
)

type deployState struct {
	id                  string
	fulfilments         []*types.TxCreateFulfillment
	leases              []*types.TxCreateLease
	providerLeaseStatus map[string]*types.LeaseStatusResponse
	groups              []*types.GroupSpec
	orders              []*types.TxCreateOrder
	mani                *types.Manifest
	hash                []byte
}

var (
	state = &deployState{
		providerLeaseStatus: make(map[string]*types.LeaseStatusResponse),
	}
	mtx sync.Mutex
)

type deployStatus struct {
	Event   deployEvent
	Message string
	Error   error
	Result  interface{}
}

func CreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <deployment-file>",
		Short: "create a deployment",
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(create))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	session.AddFlagWait(cmd, cmd.Flags())
	return cmd
}

func create(ses session.Session, cmd *cobra.Command, args []string) error {
	var filePath string
	if len(args) == 1 {
		filePath = args[0]
	}
	filePath = ses.Mode().Ask().StringVar(filePath, "Deployment File Path (required): ", true)

	statusChan := make(chan *deployStatus)
	go func() {
		var status *deployStatus
		// step 1: parse SDL to extract deployment groups, manifest and hash
		status = &deployStatus{Event: eventDeployBegin}
		if state.groups, state.mani, state.hash, status.Error = parseSDL(filePath); status.Error != nil {
			statusChan <- status
			return
		}
		status.Result = state.groups
		statusChan <- status

		// step 2: broadcast the deployment
		status = &deployStatus{Event: eventBroadcastBegin}
		txclient, err := ses.TxClient()
		if err != nil {
			status.Error = err
			statusChan <- status
			return
		}
		statusChan <- status

		status = &deployStatus{Event: eventBroadcastDone}

		var bcResult *tmctypes.ResultBroadcastTxCommit
		bcResult, status.Error = createBroadcast(txclient, ttl, state.groups, state.mani, state.hash)
		if status.Error != nil {
			statusChan <- status
			return
		}
		address := bcResult.DeliverTx.Data
		status.Message = X(address)
		state.id = status.Message
		statusChan <- status

		statusChan <- &deployStatus{Event: eventReceiveOrdersBegin}

		// step 3: listen for buy orders
		// step 4: listen for fullfillments on orders
		// step 5: listen for leases on fulfillments
		handler := marketplace.NewBuilder().
			OnTxCreateOrder(func(tx *types.TxCreateOrder) {
				if bytes.Equal(tx.Deployment, address) {
					mtx.Lock()
					state.orders = append(state.orders, tx)
					mtx.Unlock()
					statusChan <- &deployStatus{Event: eventReceiveOrder, Result: tx}
				}
				// filfillments should begin when there is atleast 1 order
				if len(state.orders) == 1 {
					statusChan <- &deployStatus{Event: eventReceiveFulfillmentsBegin}
				}
			}).
			OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
				if bytes.Equal(tx.Deployment, address) {
					mtx.Lock()
					state.fulfilments = append(state.fulfilments, tx)
					mtx.Unlock()
					statusChan <- &deployStatus{Event: eventReceiveFulfillment, Result: tx}
				}
				// receving leases begin when at alteast one fulfillment exists
				if len(state.fulfilments) == 1 {
					statusChan <- &deployStatus{Event: eventReceiveLeaseBegin, Message: "OK"}
				}
			}).
			OnTxCreateLease(func(tx *types.TxCreateLease) {
				if bytes.Equal(tx.Deployment, address) {
					mtx.Lock()
					state.leases = append(state.leases, tx)
					mtx.Unlock()
					// fulfilments are complete when a lease is created
					if len(state.leases) == 1 {
						statusChan <- &deployStatus{Event: eventReceiveFulfillmentsDone}
					}

					status = &deployStatus{Event: eventReceiveLease, Message: tx.LeaseID.String(), Result: tx}
					// get provider on the lease
					var provider *types.Provider

					if provider, status.Error = ses.QueryClient().Provider(ses.Ctx(), tx.Provider); status.Error != nil {
						statusChan <- status
						return
					}
					statusChan <- status

					// send manifest over http to provider uri
					statusChan <- &deployStatus{Event: eventSendManifest, Message: provider.HostURI}

					status = &deployStatus{Event: eventSendManifestDone, Message: provider.HostURI}
					if status.Error = http.SendManifest(ses.Ctx(), state.mani, txclient.Signer(), provider, tx.Deployment); status.Error != nil {
						statusChan <- status
						return
					}
					statusChan <- status

					const (
						statusRetryCount = 5
						statusRetryDelay = time.Second / statusRetryCount
					)

					// get lease status with deployment addresses (ips and hostnames) for the provider in lease.
					var leaseStatus *types.LeaseStatusResponse
					for i := 0; i < statusRetryCount; i++ {

						status = &deployStatus{Event: eventLeaseStatusFetch, Message: fmt.Sprintf("Fetching lease status [attempt %v/%v]", i+1, statusRetryCount)}

						leaseStatus, err = http.LeaseStatus(ses.Ctx(), provider, tx.LeaseID)

						if err != nil {
							status.Message += fmt.Sprintf(": error %v", err)
							if i == statusRetryCount-1 {
								status.Error = err
							}
						} else {
							status.Message += ": ok"
						}

						statusChan <- status

						if err == nil {
							break
						}

						time.Sleep(statusRetryDelay)
					}

					mtx.Lock()
					state.providerLeaseStatus[tx.LeaseID.String()] = leaseStatus
					mtx.Unlock()

					// when there is a lease created for each order, the deploy is complete
					if len(state.groups) == len(state.leases) {
						statusChan <- &deployStatus{Event: eventReceiveLeaseDone}
						statusChan <- &deployStatus{Event: eventDeployDone}
					}
				}
			}).
			Create()

		if err := common.MonitorMarketplace(ses.Ctx(), ses.Log(), ses.Client(), handler); err != nil {
			status.Error = err
			statusChan <- status
			return
		}
	}()
	return processStages(statusChan, ses)
}

func processStages(statusChan chan *deployStatus, s session.Session) error {
	log := s.Mode().Printer().Log().WithModule("deploy")
	for {
		status := <-statusChan
		if err := status.Error; err != nil {
			fmt.Println("(error) event: ", status.Event)
			return err
		}
		switch status.Event {
		case eventDeployBegin:
			logWait(log, "deploy", "begin deployment from config: (...)")
		case eventBroadcastBegin:
			var names []string
			for _, g := range state.groups {
				names = append(names, g.Name)
			}
			logWait(log, "broadcast", fmt.Sprintf("request deployment for group(s): %s", strings.Join(names, ",")))
		case eventBroadcastDone:
			msg := "request accepted, deployment created with id: " + status.Message
			logDone(log, "broadcast", msg)
		case eventReceiveOrdersBegin:
			msg := fmt.Sprintf("waiting to create buy orders(s) for %d deployment groups(s)", len(state.groups))
			logWait(log, "auction", msg)
		case eventReceiveOrder:
			if tx, ok := status.Result.(*types.TxCreateOrder); ok {
				msg := fmt.Sprintf("buy order (%d) created with id: %s", len(state.orders), tx.OrderID.String())
				logWait(log, "auction", msg)
			}
		case eventReceiveOrdersDone:
			msg := fmt.Sprintf("%d order(s) created", len(state.orders))
			logDone(log, "auction", msg)
		case eventReceiveFulfillmentsBegin:
			msg := fmt.Sprintf("waiting on fulfillment(s)")
			logWait(log, "auction", msg)
		case eventReceiveFulfillment:
			if tx, ok := status.Result.(*types.TxCreateFulfillment); ok {
				msg := fmt.Sprintf("received fulfillment (%d/%d) with id: %s", len(state.fulfilments), len(state.orders), tx.FulfillmentID.String())
				logWait(log, "auction", msg)
			}
		case eventReceiveFulfillmentsDone:
			msg := fmt.Sprintf("complete; received %d fulfillment(s) for %d order(s)", len(state.fulfilments), len(state.orders))
			logDone(log, "auction", msg)
		case eventReceiveLeaseBegin:
			msg := fmt.Sprintf("waiting on lease(s)")
			logWait(log, "lease", msg)
		case eventReceiveLease:
			if tx, ok := status.Result.(*types.TxCreateLease); ok {
				msg := fmt.Sprintf("received lease (%d) for group (%v/%v) [price %v] [id %s]", len(state.leases), tx.Group, len(state.groups), tx.Price, tx.LeaseID)
				logWait(log, "lease", msg)
			}
		case eventReceiveLeaseDone:
			msg := fmt.Sprintf("complete; received %d lease(s) for %d groups(s)", len(state.leases), len(state.groups))
			logDone(log, "lease", msg)
		case eventSendManifest:
			msg := fmt.Sprintf("send manifest to provider at %s", status.Message)
			logWait(log, "lease", msg)
		case eventSendManifestDone:
			msg := fmt.Sprintf("manifest accepted by provider at %s", status.Message)
			logDone(log, "lease", msg)
		case eventLeaseStatusFetch:
			logWait(log, "lease", status.Message)
		case eventDeployDone:
			msg := "deployment complete"
			logDone(log, "deploy", msg)
			printer := s.Mode().Printer()

			data := printer.NewSection("Deployment").NewData().Add("Deployment ID", state.id)

			// add groups
			gd := dsky.NewSectionData(" ")
			if len(state.groups) > 1 {
				gd.AsList()
			}
			sdutil.AppendGroupSpec(state.groups, gd)
			data.Add("Deployment Groups", gd).WithLabel("Deployment Groups", "Deployment Groups(s)")

			// add fulfillments
			fd := dsky.NewSectionData(" ")
			if len(state.fulfilments) > 1 {
				fd.AsList()
			}
			sdutil.AppendTxCreateFulfilment(state.fulfilments, fd)
			data.Add("Fulfillments", fd).WithLabel("Fulfillments", "Fulfillment(s)")

			// add services
			data = printer.NewSection("Leases").WithLabel("Lease(s)").NewData().AsPane()
			for lid, v := range state.providerLeaseStatus {
				data.Add("Lease ID", lid)
				if v == nil {
					continue
				}
				sd := dsky.NewSectionData(" ").AsList()
				sdutil.AppendLeaseStatus(v, sd)
				data.Add("Services", sd).WithLabel("Services", "Services(s)")
			}
			return printer.Flush()
		}
	}
}

func parseSDL(filePath string) (groups []*types.GroupSpec, mani *types.Manifest, hash []byte, err error) {
	sdl, err := sdl.ReadFile(filePath)
	if err != nil {
		return
	}
	if groups, err = sdl.DeploymentGroups(); err != nil {
		return
	}
	if mani, err = sdl.Manifest(); err != nil {
		return
	}
	if hash, err = manifest.Hash(mani); err != nil {
		return
	}
	return
}

func createBroadcast(client txutil.Client, ttl int64, groups []*types.GroupSpec, mani *types.Manifest, hash []byte) (*tmctypes.ResultBroadcastTxCommit, error) {
	nonce, err := client.Nonce()
	if err != nil {
		return nil, err
	}
	return client.BroadcastTxCommit(&types.TxCreateDeployment{
		Tenant:   client.Key().GetPubKey().Address().Bytes(),
		Nonce:    nonce,
		OrderTTL: ttl,
		Groups:   groups,
		Version:  hash,
	})
}

func logDone(log dsky.Logger, module, msg string) {
	log.WithAction(dsky.LogActionDone).WithModule(module).Info(msg)
}

func logWait(log dsky.Logger, module, msg string) {
	log.WithAction(dsky.LogActionWait).WithModule(module).Warn(msg)
}

func logError(log dsky.Logger, module, msg string) {
	log.WithAction(dsky.LogActionFail).WithModule(module).Error(msg)
}

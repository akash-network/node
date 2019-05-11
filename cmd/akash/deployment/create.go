package deployment

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
	tmctypes "github.com/tendermint/tendermint/rpc/core/types"
)

type deployEvent uint

const (
	ttl = int64(5)

	eventDeployBegin deployEvent = iota + 1
	eventBroadcastBegin
	eventBroadcastDone
	eventReceiveOrdersBegin
	eventReceiveOrder
	eventReceiveOrdersDone
	eventReceiveFulfillmentsBegin
	eventReceiveFulfillment
	eventReceiveFulfillmentsDone
	eventReceiveLeaseBegin
	eventReceiveLease
	eventSendManifest
	eventSendManifestDone
	eventReceiveLeaseDone
	eventDeployDone
)

type deployState struct {
	fulfilments         []*types.TxCreateFulfillment
	leases              []*types.TxCreateLease
	providerLeaseStatus map[*types.Provider]*types.LeaseStatusResponse
	groups              []*types.GroupSpec
	orders              []*types.TxCreateOrder
	mani                *types.Manifest
	hash                []byte
}

var (
	state = &deployState{
		providerLeaseStatus: make(map[*types.Provider]*types.LeaseStatusResponse),
	}
	mtx sync.Mutex
)

type deployStatus struct {
	Event   deployEvent
	Message string
	Error   error
	Result  interface{}
}

func createCmd() *cobra.Command {
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

					if provider, status.Error = ses.QueryClient().Provider(ses.Ctx(), tx.Provider); err != nil {
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

					// get lease status with deployment addresses (ips and hostnames) for the provider in lease.
					var leaseStatus *types.LeaseStatusResponse
					if leaseStatus, status.Error = http.LeaseStatus(ses.Ctx(), provider, tx.LeaseID); err != nil {
						statusChan <- status
						return
					}
					mtx.Lock()
					state.providerLeaseStatus[provider] = leaseStatus
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
	return processStages(statusChan)
}

func processStages(statusChan chan *deployStatus) error {
	writer := os.Stdout
	for {
		status := <-statusChan
		if err := status.Error; err != nil {
			fmt.Println("(error) event: ", status.Event)
			return err
		}
		switch status.Event {
		case eventDeployBegin:
			logWait("[deploy] begin deployment from config: (...)")
		case eventBroadcastBegin:
			var names []string
			for _, g := range state.groups {
				names = append(names, g.Name)
			}
			logWait(fmt.Sprintf("[broadcast] request deployment for group(s): %s", strings.Join(names, ",")))
		case eventBroadcastDone:
			logDone("[broadcast] request accepted, deployment created with id: " + status.Message)
		case eventReceiveOrdersBegin:
			logWait(fmt.Sprintf("[auction] waiting to create buy orders(s) for %d deployment groups(s)", len(state.groups)))
		case eventReceiveOrder:
			if tx, ok := status.Result.(*types.TxCreateOrder); ok {
				logWait(fmt.Sprintf("[auction] buy order (%d) created with id: %s", len(state.orders), tx.OrderID.String()))
			}
		case eventReceiveOrdersDone:
			logDone(fmt.Sprintf("[auction] %d order(s) created", len(state.orders)))
		case eventReceiveFulfillmentsBegin:
			logWait(fmt.Sprintf("[auction] waiting on fulfillment(s)"))
		case eventReceiveFulfillment:
			if tx, ok := status.Result.(*types.TxCreateFulfillment); ok {
				logWait(fmt.Sprintf("[auction] received fulfillment (%d/%d) with id: %s", len(state.fulfilments), len(state.orders), tx.FulfillmentID.String()))
			}
		case eventReceiveFulfillmentsDone:
			logDone(fmt.Sprintf("[auction] complete; received %d fulfillment(s) for %d order(s)", len(state.fulfilments), len(state.orders)))
		case eventReceiveLeaseBegin:
			logWait(fmt.Sprintf("[lease] waiting on lease(s)"))
		case eventReceiveLease:
			if tx, ok := status.Result.(*types.TxCreateLease); ok {
				logWait(fmt.Sprintf("[lease] received lease (%d) for group (%v/%v) [price %v] [id %s]", len(state.leases), tx.Group, len(state.groups), tx.Price, tx.LeaseID))
			}
		case eventReceiveLeaseDone:
			logDone(fmt.Sprintf("[lease] complete; received %d lease(s) for %d groups(s)", len(state.leases), len(state.groups)))
		case eventSendManifest:
			logWait(fmt.Sprintf("[lease] send manifest to provider at %s", status.Message))
		case eventSendManifestDone:
			logDone(fmt.Sprintf("[lease] manifest accepted by provider at %s", status.Message))
		case eventDeployDone:
			logDone("[deploy] deployment complete")
			session.NewIPrinter(writer).
				AddText("").
				AddTitle("Deployment Group(s)").
				Add(groupsUITable(state.groups)).
				AddText("").
				AddTitle("Fulfillment(s)").
				Add(fulfilmentsUITable(state.fulfilments)).
				AddText("").
				AddTitle("Lease(s)").
				Add(tableLeases(state.leases)).
				Flush()
			return nil
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

func fulfilmentsUITable(f []*types.TxCreateFulfillment) *uitable.Table {
	t := uitable.New()
	t.AddRow("GROUP", "PRICE", "PROVIDER")
	for _, tx := range f {
		t.AddRow(tx.Group, tx.Price, tx.Provider.String())
	}
	return t
}

func groupsUITable(groups []*types.GroupSpec) *uitable.Table {
	t := uitable.New()
	t.Wrap = true
	t.AddRow("GROUP", "REQUIREMENTS", "RESOURCES")
	for _, g := range groups {
		var reqs []string
		for _, r := range g.Requirements {
			reqs = append(reqs, fmt.Sprintf("%s:%s", r.Name, r.Value))
		}
		var resources []string
		for _, r := range g.Resources {
			rg := fmt.Sprintf("Count: %d, Price %d, CPU: %d, Memory: %d, Disk: %d", r.Count, r.Price, r.Unit.CPU, r.Unit.Memory, r.Unit.Disk)
			resources = append(resources, rg)
		}
		t.AddRow(g.Name, strings.Join(reqs, "\n"), strings.Join(resources, "\n"))
	}
	return t
}

func tableLeases(leases []*types.TxCreateLease) *uitable.Table {
	t := uitable.New().AddRow("LEASE ID", "PRICE")
	for _, tx := range leases {
		t.AddRow(tx.LeaseID.String(), tx.Price)
	}
	return t
}

func tableSummary(ls map[*types.Provider]*types.LeaseStatusResponse) {
	ptable := uitable.New()
	ptable.Wrap = true
	ptable.MaxColWidth = 300
	ptable.AddRow("SERVICE", "PROVIDER", "URI")
	for provider, status := range ls {
		for _, service := range status.Services {
			for _, uri := range service.URIs {
				ptable.AddRow(service.Name, provider.Address, uri)
			}
		}
	}

}

func logDone(msg string) {
	fmt.Println("(done)", msg)
}

func logWait(msg string) {
	fmt.Println("[/] (wait)", msg)
}

func logError(msg string) {
	fmt.Println("(error)", msg)
}

// func depSummaryTable(tx

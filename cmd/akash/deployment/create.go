package deployment

import (
	"bytes"
	"fmt"
	"os"
	"strings"

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
	eventReceiveFulfillmentsBegin
	eventReceiveFulfillment
	eventReceiveFulfillmentsDone
	eventReceiveLeaseBegin
	eventReceiveLease
	eventReceiveLeaseDone
	eventDeployDone
)

type deployState struct {
	fulfilments         []*types.TxCreateFulfillment
	leases              []*types.TxCreateLease
	providerLeaseStatus map[*types.Provider]*types.LeaseStatusResponse
	groups              []*types.GroupSpec
	mani                *types.Manifest
	hash                []byte
}

var (
	state = &deployState{
		providerLeaseStatus: make(map[*types.Provider]*types.LeaseStatusResponse),
	}
)

type deployStatus struct {
	Event   deployEvent
	Message string
	Error   error
	Result  interface{}

	bcResult    *tmctypes.ResultBroadcastTxCommit
	fulfilment  *types.TxCreateFulfillment
	provider    *types.Provider
	lease       *types.TxCreateLease
	leaseStatus *types.LeaseStatusResponse
}

func createCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <deployment-file>",
		Short: "create a deployment",
		// Args:  cobra.ExactArgs(1),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(create))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
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
		status.bcResult, status.Error = createBroadcast(txclient, ttl, state.groups, state.mani, state.hash)
		if status.Error != nil {
			return
		}
		address := status.bcResult.DeliverTx.Data
		statusChan <- status

		// step 3: listen for fullfillments and leases
		statusChan <- &deployStatus{Event: eventReceiveFulfillmentsBegin, Message: "OK"}
		statusChan <- &deployStatus{Event: eventReceiveLeaseBegin, Message: "OK"}

		handler := marketplace.NewBuilder().
			OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
				if bytes.Equal(tx.Deployment, address) {
					state.fulfilments = append(state.fulfilments, tx)
					statusChan <- &deployStatus{Event: eventReceiveFulfillment, fulfilment: tx}
				}
			}).
			OnTxCreateLease(func(tx *types.TxCreateLease) {
				if bytes.Equal(tx.Deployment, address) {
					state.leases = append(state.leases, tx)
					// fulfilments are complete when a lease is created
					if len(state.leases) == 1 {
						statusChan <- &deployStatus{Event: eventReceiveFulfillmentsDone}
					}

					status = &deployStatus{Event: eventReceiveLease, Message: "OK", lease: tx}

					// get provider on the lease
					if status.provider, status.Error = ses.QueryClient().Provider(ses.Ctx(), tx.Provider); err != nil {
						statusChan <- status
						return
					}

					// send manifest over http to provider uri
					if status.Error = http.SendManifest(ses.Ctx(), state.mani, txclient.Signer(), status.provider, tx.Deployment); status.Error != nil {
						statusChan <- status
						return
					}

					// get lease status with deployment addresses (ips and hostnames) for the provider in lease.
					if status.leaseStatus, status.Error = http.LeaseStatus(ses.Ctx(), status.provider, tx.LeaseID); err != nil {
						statusChan <- status
						return
					}
					state.providerLeaseStatus[status.provider] = status.leaseStatus

					statusChan <- status

					// when there is a lease created for each deployment group, the deploy is complete
					if len(state.groups) == len(state.leases) {
						statusChan <- &deployStatus{Event: eventDeployDone, Message: "OK"}
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
	processStages(statusChan)
	return nil
}

func processStages(statusChan chan *deployStatus) {
	writer := os.Stdout
	for {
		status := <-statusChan
		switch status.Event {
		case eventDeployBegin:
			fmt.Println("DEPLOY BEGIN")
			fmt.Println(status.Event, status.Message)
			if groups, ok := status.Result.([]*types.GroupSpec); ok {
				session.NewIPrinter(writer).
					AddText("(begin) deployment for groups(s)").
					AddTitle("Groups").
					AddText("").
					Add(groupsUITable(groups)).
					Flush()
			}
		case eventBroadcastBegin:
			fmt.Println("BROADCAST BEGIN")
			fmt.Println(status.Event, status.Message)
		case eventBroadcastDone:
			fmt.Println("BROADCAST DONE")
			address := status.bcResult.DeliverTx.Data
			fmt.Fprintln(writer, "(success) deployment posted with id:", X(address))
			fmt.Println(status.Event, status.Message)
		case eventReceiveFulfillmentsBegin:
			fmt.Println("FULFIL BEGIN")
			fmt.Println(status.Event, status.Message)
		case eventReceiveFulfillmentsDone:
			fmt.Println("FULFIL DONE")
			fmt.Println(status.Event, status.Message)
		case eventReceiveFulfillment:
			tx := status.fulfilment
			fmt.Println("GOT --> FULFIL", tx)
			fmt.Println(status.Event, status.Message)
		case eventReceiveLease:
			fmt.Println(status.Event, status.Message)
			tx := status.lease
			fmt.Println("GOT --> LEASE", tx, "PROVIDER", status.provider, "LEASE STATUS", status.leaseStatus)
		case eventDeployDone:
			fmt.Println("DEPLOY DONE")
			session.NewIPrinter(writer).
				AddText("").
				AddTitle("Fulfillments").
				Add(fulfilmentsUITable(state.fulfilments)).
				Flush()
			return
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

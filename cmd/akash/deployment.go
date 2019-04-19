package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/gosuri/uilive"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/marketplace"
	"github.com/ovrclk/akash/provider/http"
	"github.com/ovrclk/akash/sdl"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/validation"
	"github.com/spf13/cobra"
	tmctypes "github.com/tendermint/tendermint/rpc/core/types"
)

func deploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "Manage deployments",
	}

	cmd.AddCommand(createDeploymentCommand())
	cmd.AddCommand(updateDeploymentCommand())
	cmd.AddCommand(closeDeploymentCommand())
	cmd.AddCommand(statusDeploymentCommand())
	cmd.AddCommand(sendManifestCommand())
	cmd.AddCommand(validateDeploymentCommand())

	return cmd
}

func createDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <deployment-file>",
		Short: "create a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(createDeployment))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	session.AddFlagWait(cmd, cmd.Flags())

	return cmd
}

func createDeployment(ses session.Session, cmd *cobra.Command, args []string) error {
	writer := uilive.New()
	//writer.RefreshInterval = 100 * time.Millisecond

	// start listening for updates and render
	writer.Start()
	defer writer.Stop() // flush and stop rendering

	var file string
	if len(args) == 1 {
		file = args[0]
	}
	file = ses.Mode().Ask().StringVar(file, "Deployment File Path (required): ", true)

	const ttl = int64(5)

	txclient, err := ses.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(file)
	if err != nil {
		return err
	}

	groups, err := sdl.DeploymentGroups()
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	hash, err := manifest.Hash(mani)
	if err != nil {
		return err
	}

	resChan := make(chan *tmctypes.ResultBroadcastTxCommit, 1)
	errChan := make(chan error)
	errC := make(chan error)
	var res *tmctypes.ResultBroadcastTxCommit
	var address []byte
	var start time.Time
	var elapsed time.Duration
	blue := color.New(color.FgCyan, color.Bold)

	go func() {
		res, err := txclient.BroadcastTxCommit(&types.TxCreateDeployment{
			Tenant:   txclient.Key().GetPubKey().Address().Bytes(),
			Nonce:    nonce,
			OrderTTL: ttl,
			Groups:   groups,
			Version:  hash,
		})
		if err != nil {
			ses.Log().Error("error sending tx", "error", err)
			errChan <- err
			return
		}
		resChan <- res
	}()

	lines := []rune{'—', '\\', '|', '/'}
	start = time.Now()
outloop:
	for {
		select {
		case res = <-resChan:
			address = res.DeliverTx.Data
			fmt.Fprintln(writer.Bypass(), "(success) deployment posted with id:", X(address))
			break outloop
		case err := <-errChan:
			ses.Log().Error("error sending tx", "error", err)
			return err
		default:
			elapsed = time.Now().Sub(start)
			m := math.Mod(float64(elapsed), float64(len(lines)))
			spinner := blue.Sprintf("[%s]", string(lines[int(m)]))
			fmt.Fprintf(writer, "%s (send) upload deployment manifent (elapsed: %v)\n", spinner, elapsed)
			time.Sleep(250 * time.Millisecond)
		}
	}

	if ses.NoWait() {
		return nil
	}

	err = ses.Mode().When(session.ModeTypeInteractive, func() error {
		return nil
	}).Run()

	fmt.Fprintln(writer, blue.Sprintf("[/] (wait) waiting on bids for %d deployment group(s)", len(groups)))
	expected := len(groups)
	providers := make(map[*types.Provider]types.LeaseID)

	outChan := make(chan string)
	closeChan := make(chan int)
	//bidsChan := make(chan *types.TxCreateFulfillment)
	//deployChan := make(chan *types.TxCreateLease)
	var bidsCount int
	fulfilmentsPinted := false

	bidsTab := uitable.New()
	bidsTab.AddRow("GROUP", "PRICE", "PROVIDER")

	handler := marketplace.NewBuilder().
		OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
			if bytes.Equal(tx.Deployment, address) {
				bidsCount++
				writer.Flush()
				fmt.Fprintln(writer, blue.Sprintf("(receive) bid (%d) received for group (%d) from provider (%s) for %d AKASH", bidsCount, tx.Group, tx.Provider.String(), tx.Price))
				bidsTab.AddRow(tx.Group, tx.Price, tx.Provider.String())
				//outChan <- fmt.Sprintf("Group %v/%v Fulfillment: %v [price=%v]", tx.Group, len(groups), tx.FulfillmentID, tx.Price)
				time.Sleep(2 * time.Second)
			}
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			if bytes.Equal(tx.Deployment, address) {
				if !fulfilmentsPinted {
					session.NewIPrinter(writer.Bypass()).AddText("").AddTitle("Fulfillments").Add(bidsTab).Flush()
				}
				fulfilmentsPinted = true

				// get lease provider
				prov, err := ses.QueryClient().Provider(ses.Ctx(), tx.Provider)
				if err != nil {
					errC <- err
				}

				// send manifest over http to provider uri
				writer.Flush()
				fmt.Fprintln(writer, blue.Sprintf("[/] (send) upload manifest to provider (%s) at %s", prov.Address, prov.HostURI))
				//time.Sleep(1 * time.Second)
				err = http.SendManifest(ses.Ctx(), mani, txclient.Signer(), prov, tx.Deployment)
				if err != nil {
					errC <- err
				} else {
					providers[prov] = tx.LeaseID
				}
				writer.Flush()
				fmt.Fprintln(writer, blue.Sprintf("[/] (success) manifest accepted by provider (%s) at %s", prov.Address, prov.HostURI))
				//time.Sleep(1 * time.Second)
				expected--
			}
			if expected == 0 {
				writer.Flush()
				ptable := uitable.New()
				ptable.Wrap = true
				ptable.MaxColWidth = 300
				ptable.AddRow("SERVICE", "PROVIDER", "URI")
				// get deployment addresses for each provider in lease.
				for provider, leaseID := range providers {
					writer.Flush()
					fmt.Fprintln(writer, blue.Sprintf("[/] (wait) requesting service URIs from provider (%s)", provider.Address))
					status, err := http.LeaseStatus(ses.Ctx(), provider, leaseID)
					if err != nil {
						errC <- err
						return
					} else {
						writer.Flush()
						fmt.Fprintln(writer, blue.Sprintf("(success) received service URIs from provider (%s)", provider.Address))
						for _, service := range status.Services {
							for _, uri := range service.URIs {
								ptable.AddRow(service.Name, provider.Address, uri)
							}
						}
						//printLeaseStatus(status)
					}
				}

				dtable := uitable.New()
				dtable.MaxColWidth = 400
				dtable.Wrap = true
				dtable.
					AddRow("Group:", tx.Group).
					AddRow("Deployment ID:", tx.Deployment).
					AddRow("Lease ID:", tx.LeaseID).
					AddRow("Price:", tx.Price).AddRow("Service URI(s):", ptable.String())

				session.NewIPrinter(writer.Bypass()).
					AddTitle("Deployment Info").
					Add(dtable).
					// AddText("").
					// AddTitle("Service URI(s)").
					// Add(ptable).
					Flush()

				closeChan <- 1
			}
		}).Create()

	go func(errC chan error) {
		if err = common.MonitorMarketplace(ses.Ctx(), ses.Log(), ses.Client(), handler); err != nil {
			errC <- err
		}
	}(errC)

	for {
		select {
		case <-outChan:
			// if bidsCount == 0 {
			// }
			//fmt.Println(out)
		// case <-deployChan:
		// 	fmt.Fprintln(writer.Bypass(), bidsTab)
		// case tx := <-bidsChan:
		// 	bidsCount++
		// 	fmt.Fprintf(writer, "Bids Recieved (%d): %s (%d AKASH) \n", bidsCount, tx.Provider.String(), tx.Price)
		// 	writer.Flush()
		// 	bidsTab.AddRow(tx.Group, tx.Price, tx.Provider.String())
		case err := <-errC:
			return err
		case <-closeChan:
			os.Exit(0)
			return nil
		}
	}
	//return
}

func updateDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "update <manifest> <deployment-id>",
		Short: "update a deployment (*EXPERIMENTAL*)",
		Args:  cobra.ExactArgs(2),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(updateDeployment))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func updateDeployment(session session.Session, cmd *cobra.Command, args []string) error {

	fmt.Println(`WARNING: this command is experimental and limited.

	It is currently only possible to make small changes to your deployment.

	Resources within a datacenter must remain the same.  You can change ports
	and images;add and remove services; etc... so long as the overall
	infrastructure requirements do not change.
	`)

	signer, _, err := session.Signer()
	if err != nil {
		return err
	}

	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	daddr, err := keys.ParseDeploymentPath(args[1])
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	if err := manifestValidateResources(session, mani, daddr); err != nil {
		return err
	}

	hash, err := manifest.Hash(mani)
	if err != nil {
		return err
	}

	fmt.Println("updating deployment...")

	_, err = txclient.BroadcastTxCommit(&types.TxUpdateDeployment{
		Deployment: daddr.ID(),
		Version:    hash,
	})
	if err != nil {
		session.Log().Error("error sending tx", "error", err)
		return err
	}

	return doSendManifest(session, signer, daddr.ID(), mani)
}

func closeDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "close <deployment-id>",
		Short: "close a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(closeDeployment))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())

	return cmd
}

func closeDeployment(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	deployment, err := keys.ParseDeploymentPath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseDeployment{
		Deployment: deployment.ID(),
		Reason:     types.TxCloseDeployment_TENANT_CLOSE,
	})

	if err != nil {
		session.Log().Error("error sending tx", "error", err)
		return err
	}

	fmt.Println("Closing deployment")
	return nil
}

func statusDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "status <deployment-id>",
		Short: "get deployment status",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(statusDeployment)),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	return cmd
}

func statusDeployment(session session.Session, cmd *cobra.Command, args []string) error {

	deployment, err := keys.ParseDeploymentPath(args[0])
	if err != nil {
		return err
	}

	leases, err := session.QueryClient().DeploymentLeases(session.Ctx(), deployment.ID())
	if err != nil {
		return err
	}

	var exitErr error

	for _, lease := range leases.Items {
		if lease.State != types.Lease_ACTIVE {
			continue
		}

		provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
		if err != nil {
			session.Log().Error("error fetching provider", "err", err, "lease", lease.LeaseID)
			exitErr = err
			continue
		}

		status, err := http.LeaseStatus(session.Ctx(), provider, lease.LeaseID)
		if err != nil {
			session.Log().Error("error fetching status ", "err", err, "lease", lease.LeaseID)
			exitErr = err
			continue
		}
		fmt.Printf("lease: %s\n", lease.LeaseID)
		printLeaseStatus(status)
	}

	return exitErr
}

func sendManifestCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "sendmani <manifest> <deployment>",
		Short: "send manifest to all deployment providers",
		Args:  cobra.ExactArgs(2),
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(sendManifest))),
	}

	session.AddFlagNode(cmd, cmd.Flags())
	session.AddFlagKey(cmd, cmd.Flags())

	return cmd
}

func sendManifest(session session.Session, cmd *cobra.Command, args []string) error {
	signer, _, err := session.Signer()
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}

	depAddr, err := keys.ParseDeploymentPath(args[1])
	if err != nil {
		return err
	}

	if err := manifestValidateResources(session, mani, depAddr); err != nil {
		return err
	}

	return doSendManifest(session, signer, depAddr.ID(), mani)
}

func validateDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <deployment-file>",
		Short: "validate deployment file",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(doValidateDeploymentCommand),
	}

	return cmd
}

func doValidateDeploymentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	_, err := sdl.ReadFile(args[0])
	if err != nil {
		return err
	}
	fmt.Println("ok")
	return nil
}

func manifestValidateResources(session session.Session, mani *types.Manifest, daddr []byte) error {
	dgroups, err := session.QueryClient().DeploymentGroupsForDeployment(session.Ctx(), daddr)
	if err != nil {
		return err
	}
	return validation.ValidateManifestWithDeployment(mani, dgroups.Items)
}

func doSendManifest(session session.Session, signer txutil.Signer, daddr []byte, mani *types.Manifest) error {
	leases, err := session.QueryClient().DeploymentLeases(session.Ctx(), daddr)
	if err != nil {
		return err
	}

	for _, lease := range leases.Items {
		if lease.State != types.Lease_ACTIVE {
			continue
		}
		provider, err := session.QueryClient().Provider(session.Ctx(), lease.Provider)
		if err != nil {
			return err
		}
		err = http.SendManifest(session.Ctx(), mani, signer, provider, lease.Deployment)
		if err != nil {
			return err
		}
	}
	return nil
}

func printLeaseStatus(status *types.LeaseStatusResponse) {
	for _, service := range status.Services {
		for _, uri := range service.URIs {
			fmt.Printf("\t%v: %v\n", service.Name, uri)
		}
	}
}

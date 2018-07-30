package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

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
)

func deploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "deployment",
		Short: "manage deployments",
	}

	cmd.AddCommand(createDeploymentCommand())
	cmd.AddCommand(updateDeploymentCommand())
	cmd.AddCommand(closeDeploymentCommand())
	cmd.AddCommand(statusDeploymentCommand())
	cmd.AddCommand(sendManifestCommand())

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

func createDeployment(session session.Session, cmd *cobra.Command, args []string) error {

	const ttl = int64(5)

	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(args[0])
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

	res, err := txclient.BroadcastTxCommit(&types.TxCreateDeployment{
		Tenant:   txclient.Key().Address(),
		Nonce:    nonce,
		OrderTTL: ttl,
		Groups:   groups,
		Version:  hash,
	})

	if err != nil {
		session.Log().Error("error sending tx", "error", err)
		return err
	}

	address := res.DeliverTx.Data

	fmt.Println(X(address))

	if session.NoWait() {
		return nil
	}

	fmt.Printf("Waiting...\n")
	expected := len(groups)
	providers := make(map[*types.Provider]types.LeaseID)
	handler := marketplace.NewBuilder().
		OnTxCreateFulfillment(func(tx *types.TxCreateFulfillment) {
			if bytes.Equal(tx.Deployment, address) {
				fmt.Printf("Group %v/%v Fulfillment: %v [price=%v]\n", tx.Group, len(groups), tx.FulfillmentID, tx.Price)
			}
		}).
		OnTxCreateLease(func(tx *types.TxCreateLease) {
			if bytes.Equal(tx.Deployment, address) {
				fmt.Printf("Group %v/%v Lease: %v [price=%v]\n", tx.Group, len(groups), tx.LeaseID, tx.Price)
				// get lease provider
				prov, err := session.QueryClient().Provider(session.Ctx(), tx.Provider)
				if err != nil {
					fmt.Printf("ERROR: %v", err)
				}

				// send manifest over http to provider uri
				fmt.Printf("Sending manifest to %v...\n", prov.HostURI)
				err = http.SendManifest(mani, txclient.Signer(), prov, tx.Deployment)
				if err != nil {
					fmt.Printf("ERROR: %v", err)
				} else {
					providers[prov] = tx.LeaseID
				}
				expected--
			}
			if expected == 0 {
				// get deployment addresses for each provider in lease.
				for provider, leaseID := range providers {
					fmt.Printf("Service URIs for provider: %s\n", provider.Address)
					status, err := http.LeaseStatus(provider, leaseID)
					if err != nil {
						fmt.Printf("ERROR: %v", err)
					} else {
						printLeaseStatus(status)
					}
				}
				os.Exit(0)
			}
		}).Create()

	return common.RunForever(func(ctx context.Context) error {
		return common.MonitorMarketplace(ctx, session.Log(), session.Client(), handler)
	})
}

func updateDeploymentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "update <manifest> <deployment>",
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

		status, err := http.LeaseStatus(provider, lease.LeaseID)
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
		err = http.SendManifest(mani, signer, provider, lease.Deployment)
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

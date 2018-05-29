package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/event"
	psession "github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	ptype "github.com/ovrclk/akash/types/provider"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func providerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "provider",
		Short: "manage provider",
		Args:  cobra.ExactArgs(1),
	}

	session.AddFlagNode(cmd, cmd.PersistentFlags())
	session.AddFlagKey(cmd, cmd.PersistentFlags())
	session.AddFlagNonce(cmd, cmd.PersistentFlags())

	cmd.AddCommand(createProviderCommand())
	cmd.AddCommand(runCommand())
	cmd.AddCommand(closeFulfillmentCommand())
	cmd.AddCommand(closeLeaseCommand())

	return cmd
}

func createProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <file>",
		Short: "create a provider",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doCreateProviderCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCreateProviderCommand(session session.Session, cmd *cobra.Command, args []string) error {
	kmgr, err := session.KeyManager()
	if err != nil {
		return err
	}

	// XXX generate key for provider if doens't exist
	key, err := session.Key()
	if err != nil {
		kname := session.KeyName()
		ktype, err := session.KeyType()
		if err != nil {
			return err
		}

		info, _, err := kmgr.Create(kname, constants.Password, ktype)
		if err != nil {
			return err
		}

		key, err = kmgr.Get(kname)
		if err != nil {
			return err
		}

		fmt.Printf("Key created: %v\n", X(info.Address()))
	}

	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	prov := &ptype.Provider{}
	err = prov.Parse(args[0])
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxCreateProvider{
		Owner:      key.Address(),
		HostURI:    prov.HostURI,
		Attributes: prov.Attributes,
		Nonce:      nonce,
	})

	if err != nil {
		return err
	}

	fmt.Println(X(result.DeliverTx.Data))

	return nil
}

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <provider>",
		Short: "respond to chain events",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doProviderRunCommand)),
	}
	return cmd
}

func doProviderRunCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseProviderPath(args[0])
	if err != nil {
		return err
	}

	pobj, err := session.QueryClient().Provider(session.Ctx(), key.ID())
	if err != nil {
		return err
	}

	if bytes.Compare(pobj.Owner, txclient.Key().Address()) != 0 {
		return fmt.Errorf("invalid key for provider (owner: %v, key: %v)",
			pobj.Owner.EncodeString(), X(txclient.Key().Address()))
	}

	return common.RunForever(func(ctx context.Context) error {
		ctx, cancel := context.WithCancel(ctx)

		psession := psession.New(session.Log(), pobj, txclient, session.QueryClient())

		bus := event.NewBus()
		defer bus.Close()

		errch := make(chan error, 3)

		go func() {
			defer cancel()
			mclient := session.Client()
			mlog := session.Log()
			mhandler := event.MarketplaceTxHandler(bus)
			errch <- common.MonitorMarketplace(ctx, mlog, mclient, mhandler)
		}()

		service, err := provider.NewService(ctx, psession, bus)
		if err != nil {
			cancel()
			<-errch
			return err
		}

		go func() {
			defer cancel()
			<-service.Done()
			errch <- nil
		}()

		go func() {
			defer cancel()
			errch <- manifest.RunServer(ctx, session.Log(), "3001", service.ManifestHandler())
		}()

		var reterr error
		for i := 0; i < 3; i++ {
			if err := <-errch; err != nil {
				session.Log().Error("error", "err", err)
				reterr = err
			}
		}

		return reterr
	})
}

func closeFulfillmentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "closef",
		Short: "close an open fulfillment",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doCloseFulfillmentCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseFulfillmentCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseFulfillmentPath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseFulfillment{
		FulfillmentID: key.ID(),
	})
	return err
}

func closeLeaseCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "closel <deployment> <group> <order> <provider>",
		Short: "close an active lease",
		Args:  cobra.ExactArgs(4),
		RunE:  session.WithSession(session.RequireNode(doCloseLeaseCommand)),
	}

	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCloseLeaseCommand(session session.Session, cmd *cobra.Command, args []string) error {
	txclient, err := session.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseLeasePath(args[0])
	if err != nil {
		return err
	}

	_, err = txclient.BroadcastTxCommit(&types.TxCloseLease{
		LeaseID: key.ID(),
	})

	return err
}

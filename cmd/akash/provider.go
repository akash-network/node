package main

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/dustin/go-humanize"
	"github.com/gosuri/uitable/util/strutil"
	"github.com/gosuri/uitable/util/wordwrap"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/event"
	"github.com/ovrclk/akash/provider/grpc"
	akash_json "github.com/ovrclk/akash/provider/grpc/json"
	"github.com/ovrclk/akash/provider/http"
	psession "github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	ptype "github.com/ovrclk/akash/types/provider"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/dsky"
	"github.com/spf13/cobra"
)

func providerCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage provider",
		Args:  cobra.ExactArgs(1),
	}

	session.AddFlagNode(cmd, cmd.PersistentFlags())

	cmd.AddCommand(createProviderCommand())
	cmd.AddCommand(runCommand())
	cmd.AddCommand(providerStatusCommand())
	cmd.AddCommand(closeFulfillmentCommand())
	cmd.AddCommand(closeLeaseCommand())

	return cmd
}

func createProviderCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "create <file>",
		Short: "create a provider",
		Long:  "create a provider with the provided config file",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doCreateProviderCommand)),
	}

	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	session.AddFlagKeyType(cmd, cmd.Flags())

	return cmd
}

func doCreateProviderCommand(session session.Session, cmd *cobra.Command, args []string) error {
	kmgr, err := session.KeyManager()
	if err != nil {
		return err
	}

	printer := session.Mode().Printer()
	// XXX generate key for provider if doens't exist
	key, err := session.Key()
	if err != nil {
		kname := session.KeyName()
		ktype, err := session.KeyType()
		if err != nil {
			return err
		}

		password, err := session.Password()
		if err != nil {
			return err
		}

		info, seed, err := kmgr.CreateMnemonic(kname, common.DefaultCodec, password, ktype)
		if err != nil {
			return err
		}

		key, err = kmgr.Get(kname)
		if err != nil {
			return err
		}

		printer.Log().WithModule("key").Info("key created")
		data := printer.NewSection("Create Key").NewData()
		data.
			WithTag("raw", info).
			Add("Name", kname).
			Add("Public Key", X(info.GetPubKey().Address())).
			Add("Recovery Codes", seed)
		printer.Flush()
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
		Owner:      key.GetPubKey().Address().Bytes(),
		HostURI:    prov.HostURI,
		Attributes: prov.Attributes,
		Nonce:      nonce,
	})
	if err != nil {
		return err
	}

	printer.Log().WithModule("provider").Info("provider added")
	data := printer.NewSection("Add Provider").NewData()
	data.Add("Data", X(result.DeliverTx.Data))
	return printer.Flush()
}

func runCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <provider>",
		Short: "respond to chain events",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(session.RequireHost(doProviderRunCommand))),
	}

	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	session.AddFlagHost(cmd, cmd.PersistentFlags())

	cmd.Flags().Bool("kube", false, "use kubernetes cluster")
	cmd.Flags().String("manifest-ns", "lease", "set manifest namespace")
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

	if !bytes.Equal(pobj.Owner, txclient.Key().GetPubKey().Address()) {
		return fmt.Errorf("invalid key for provider (owner: %v, key: %v)",
			pobj.Owner.EncodeString(), X(txclient.Key().GetPubKey().Address()))
	}

	var cclient cluster.Client

	if ok, _ := cmd.Flags().GetBool("kube"); ok {
		session.Log().Debug("using kube client")
		ns, err := cmd.Flags().GetString("manifest-ns")
		if err != nil {
			return err
		}
		cclient, err = kube.NewClient(session.Log().With("cmp", "cluster-client"), session.Host(), ns)
		if err != nil {
			return err
		}
	} else {
		cclient = cluster.NullClient()
	}

	ctx, cancel := context.WithCancel(session.Ctx())

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

	service, err := provider.NewService(ctx, psession, bus, cclient)
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
		errch <- grpc.Run(ctx, ":9090", psession, cclient, service, service.ManifestHandler())
	}()

	go func() {
		defer cancel()
		errch <- akash_json.Run(ctx, session.Log(), ":3001", "localhost:9090")
	}()

	var reterr error
	for i := 0; i < 3; i++ {
		if err := <-errch; err != nil {
			session.Log().Error("error", "err", err)
			reterr = err
		}
	}

	return reterr
}

func providerStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [<provider-id> ...]",
		Short: "print status of (given) providers",
		RunE:  session.WithSession(session.RequireNode(doProviderStatusCommand)),
	}
	return cmd
}

func doProviderStatusCommand(session session.Session, cmd *cobra.Command, args []string) error {
	plist, err := session.QueryClient().Providers(session.Ctx())
	if err != nil {
		return err
	}
	var providers []*types.Provider

	if len(args) == 0 {
		providers = plist.Providers
	} else {
		for _, arg := range args {
			pkey, err := keys.ParseAddressPath(arg)
			if err != nil {
				return err
			}
			pid := pkey.ID()
			for _, provider := range plist.Providers {
				if bytes.Equal(provider.Address, pid) {
					providers = append(providers, provider)
				}
			}
		}
	}

	output := []*outputItem{}

	for _, provider := range providers {
		status, err := http.Status(session.Ctx(), provider)
		if err != nil {
			output = append(output, &outputItem{Provider: provider, Error: err.Error()})
			continue
		}

		if !bytes.Equal(status.Provider, provider.Address) {
			output = append(output, &outputItem{
				Provider: provider,
				Status:   status,
				Error:    "Status received from incorrect provider",
			})
			continue
		}

		output = append(output, &outputItem{
			Provider: provider,
			Status:   status,
		})
	}

	printer := session.Mode().Printer()

	var active, passive []*outputItem

	fmt.Println("got entries:", len(output))

	for _, o := range output {
		if len(o.Error) == 0 {
			fmt.Println("found active")
			active = append(active, o)
			continue
		}
		passive = append(passive, o)
	}
	activedat := printer.NewSection("Active Providers").WithLabel("Active Provider(s) Status").NewData().WithTag("raw", active)
	passivedat := printer.NewSection("Passive Providers").WithLabel("Passive Provider(s) Status").NewData().WithTag("raw", passive)
	if len(output) > 1 {
		activedat.AsList()
		passivedat.AsList()
	}

	applySectionData(active, activedat)
	applySectionData(passive, passivedat)
	return printer.Flush()
}

type outputItem struct {
	Provider *types.Provider              `json:"provider,omitempty"`
	Status   *types.ServerStatusParseable `json:"status,omitempty"`
	Error    string                       `json:"error,omitempty"`
}

func applySectionData(output []*outputItem, data *dsky.SectionData) {
	for _, result := range output {
		var msg string
		if len(result.Error) > 0 {
			msg = fmt.Sprintf("error=%v", result.Error)
		}

		if provider := result.Provider; provider != nil {
			data.Add("Provider", X(result.Provider.Address))
			if len(result.Provider.Attributes) > 0 {
				attrs := make(map[string]string)
				for _, a := range result.Provider.Attributes {
					attrs[a.Name] = a.Value
				}
				data.Add("Attributes", attrs)
			}

			if s := result.Status; s != nil {
				ver := make(map[string]string)
				ver["version"] = s.Version.Version
				// ver["date"] = s.Version.Date
				if len(s.Version.Commit) > 1 {
					ver["commit"] = strutil.Resize(s.Version.Commit, 10, false)
				}
				data.Add("Version", ver)
				msg = msg + fmt.Sprintf(" code=%v", s.Code)
				cluster := s.Status.Cluster
				if cluster == nil {
					continue
				}
				data.Add("Leases", cluster.Leases)
				data.Add("Deployments", s.Status.Manifest.Deployments)
				data.Add("Orders", s.Status.Bidengine.Orders)

				cir := cluster.Inventory

				acunits := make(map[string]string)
				peunits := make(map[string]string)
				avunits := make(map[string]string)
				for _, r := range cir.Reservations.Active {
					m, _ := strconv.Atoi(r.Memory)
					d, _ := strconv.Atoi(r.Disk)
					acunits["cpu"] = fmt.Sprint(r.CPU)
					acunits["mem"] = humanize.Bytes(uint64(m))
					acunits["disk"] = humanize.Bytes(uint64(d))
				}
				data.Add("Active", acunits)
				for _, r := range cir.Reservations.Pending {
					m, _ := strconv.Atoi(r.Memory)
					d, _ := strconv.Atoi(r.Disk)
					peunits["cpu"] = fmt.Sprint(r.CPU)
					peunits["mem"] = humanize.Bytes(uint64(m))
					peunits["disk"] = humanize.Bytes(uint64(d))
				}
				data.Add("Pending", peunits)

				for _, r := range cir.Available {
					m, _ := strconv.Atoi(r.Memory)
					d, _ := strconv.Atoi(r.Disk)
					avunits["cpu"] = fmt.Sprint(r.CPU)
					avunits["mem"] = humanize.Bytes(uint64(m))
					avunits["disk"] = humanize.Bytes(uint64(d))

				}
				data.Add("Available", avunits)
				if len(s.Message) > 0 {
					msg = msg + fmt.Sprintf(" msg=%v", s.Message)
				}
			} else {
				// Add empty rows
				data.Add("Version", "").Add("Leases", "").Add("Deployments", "").Add("Orders", "").Add("Version", "").Add("Available", "")
			}
			data.Add("Message(s)", wordwrap.WrapString(msg, 25))
		}
	}
}

func closeFulfillmentCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "closef",
		Short: "close an open fulfillment",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireNode(doCloseFulfillmentCommand)),
	}

	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())

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

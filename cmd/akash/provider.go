package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/dustin/go-humanize"
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
		Use:   "add <config>",
		Short: "add a new provider",
		Long:  "register a provider with the provided config file",
		RunE: session.WithSession(
			session.RequireKey(session.RequireNode(doCreateProviderCommand))),
	}

	session.AddFlagKey(cmd, cmd.Flags())
	session.AddFlagNonce(cmd, cmd.Flags())
	session.AddFlagKeyType(cmd, cmd.Flags())
	return cmd
}

func doCreateProviderCommand(ses session.Session, cmd *cobra.Command, args []string) error {
	var config string
	if len(args) == 1 {
		config = args[0]
	}
	config = ses.Mode().Ask().StringVar(config, "Config Path (required): ", true)
	if len(config) == 0 {
		return fmt.Errorf("required argument missing: config")
	}

	printer := ses.Mode().Printer()
	txclient, err := ses.TxClient()
	if err != nil {
		return err
	}

	nonce, err := txclient.Nonce()
	if err != nil {
		return err
	}

	prov := &ptype.Provider{}
	err = prov.Parse(config)
	if err != nil {
		return err
	}

	result, err := txclient.BroadcastTxCommit(&types.TxCreateProvider{
		Owner:      txclient.Key().GetPubKey().Address().Bytes(),
		HostURI:    prov.HostURI,
		Attributes: prov.Attributes,
		Nonce:      nonce,
	})
	if err != nil {
		return err
	}

	printer.Log().WithModule("provider").Info("provider added")
	data := printer.NewSection("Add Provider").NewData()
	data.Add("Key", X(result.DeliverTx.Data))
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

	cmd.Flags().String("private-key", "", "import private key")
	cmd.Flags().Bool("kube", false, "use kubernetes cluster")
	cmd.Flags().String("manifest-ns", "lease", "set manifest namespace")
	return cmd
}

func doProviderRunCommand(ses session.Session, cmd *cobra.Command, args []string) error {
	log := ses.Mode().Printer().Log().WithModule("provider")
	if pk, err := cmd.Flags().GetString("private-key"); len(pk) > 0 && err == nil {
		log.Info(fmt.Sprintf("Import private key from: %v", pk))
		b, err := ioutil.ReadFile(pk)
		if err != nil {
			return err
		}

		kmgr, err := ses.KeyManager()
		if err != nil {
			return err
		}

		err = kmgr.Import(ses.KeyName(), string(b))
		if err != nil {
			return err
		}
	}

	txclient, err := ses.TxClient()
	if err != nil {
		return err
	}

	key, err := keys.ParseProviderPath(args[0])
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("Staring provider with address: %v", args[0]))

	pobj, err := ses.QueryClient().Provider(ses.Ctx(), key.ID())
	if err != nil {
		ses.Mode().Printer().Log().WithModule("provider").Error(fmt.Sprintf("unable to query with key %v", args[0]))
		return err
	}

	if !bytes.Equal(pobj.Owner, txclient.Key().GetPubKey().Address()) {
		return fmt.Errorf("invalid key for provider (owner: %v, key: %v)",
			pobj.Owner.EncodeString(), X(txclient.Key().GetPubKey().Address()))
	}

	var clusterClient cluster.Client

	// use kubeclient as cluster client when kube is enabled
	if ok, _ := cmd.Flags().GetBool("kube"); ok {
		ses.Log().Debug("using kube client")
		ns, err := cmd.Flags().GetString("manifest-ns")
		if err != nil {
			return err
		}
		clusterClient, err = kube.NewClient(ses.Log().With("cmp", "cluster-client"), ses.Host(), ns)
		if err != nil {
			ses.Log().Error("error creating kubeClient", err)
			return err
		}
	}

	// fall back on null client if no client is specified
	// used for testing

	if clusterClient == nil {
		clusterClient = cluster.NullClient()
	}

	ctx, cancel := context.WithCancel(ses.Ctx())
	psession := psession.New(ses.Log(), pobj, txclient, ses.QueryClient())

	bus := event.NewBus()
	defer bus.Close()

	errch := make(chan error, 3)

	go func() {
		defer cancel()
		mclient := ses.Client()
		mlog := ses.Log()
		mhandler := event.MarketplaceTxHandler(bus)
		errch <- common.MonitorMarketplace(ctx, mlog, mclient, mhandler)
	}()

	service, err := provider.NewService(ctx, psession, bus, clusterClient)
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
		errch <- grpc.Run(ctx, ":9090", psession, clusterClient, service, service.ManifestHandler())
	}()

	go func() {
		defer cancel()
		errch <- akash_json.Run(ctx, ses.Log(), ":3001", "localhost:9090")
	}()

	var reterr error
	for i := 0; i < 3; i++ {
		if err := <-errch; err != nil {
			ses.Log().Error("error", "err", err)
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

	cmd.Flags().String("state", "active", "Query providers with state")

	return cmd
}

func doProviderStatusCommand(session session.Session, cmd *cobra.Command, args []string) error {
	providerState, _ := cmd.Flags().GetString("state")
	plist, err := session.QueryClient().Providers(session.Ctx())

	if err != nil {
		return err
	}

	// determine providers to check status for
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
	outChan := make(chan *outputItem)
	for _, provider := range providers {
		go func(provider *types.Provider, outChan chan *outputItem) {
			status, err := http.Status(session.Ctx(), provider)
			var op *outputItem
			switch {
			case err != nil:
				op = &outputItem{Provider: provider, Error: err.Error()}
			case !bytes.Equal(status.Provider, provider.Address):
				op = &outputItem{
					Provider: provider,
					Status:   status,
					Error:    "Status received from incorrect provider",
				}
			default:
				op = &outputItem{
					Provider: provider,
					Status:   status,
				}
			}
			outChan <- op
		}(provider, outChan)
	}

	for i := 0; i < len(providers); i++ {
		output = append(output, <-outChan)
	}

	printer := session.Mode().Printer()
	var active, passive []*outputItem
	for _, o := range output {
		if len(o.Error) == 0 {
			if providerState != "passive" {
				active = append(active, o)
			}

			continue
		}

		if providerState != "active" {
			passive = append(passive, o)
		}
	}

	if len(active) > 0 {
		activedat := printer.NewSection("Active Providers").WithLabel("Active Provider(s) Status").NewData().WithTag("raw", active)
		applySectionData(active, activedat, session)
		if len(active) > 1 && session.Mode().IsInteractive() {
			activedat.AsList()
			activedat.Hide("Active", "Pending", "Available")
		}
	}
	if len(passive) > 0 {
		passivedat := printer.NewSection("Passive Providers").WithLabel("Passive Provider(s) Status").NewData().WithTag("raw", passive)
		applySectionData(passive, passivedat, session)
		if len(passive) > 1 && session.Mode().IsInteractive() {
			passivedat.AsList()
			passivedat.Hide("Active", "Pending", "Available")
		}
	}

	return printer.Flush()
}

type outputItem struct {
	Provider *types.Provider              `json:"provider,omitempty"`
	Status   *types.ServerStatusParseable `json:"status,omitempty"`
	Error    string                       `json:"error,omitempty"`
}

func applySectionData(output []*outputItem, data dsky.SectionData, session session.Session) {
	for _, result := range output {
		var msg []string
		if len(result.Error) > 0 {
			msg = append(msg, fmt.Sprintf("error=%v", result.Error))
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
				// add the full version when there is a single item (pane mode),
				// else just show the main version
				if len(output) > 1 && session.Mode().IsInteractive() {
					data.Add("Version", s.Version.Version)
				} else {
					ver := make(map[string]string)
					ver["version"] = s.Version.Version
					ver["date"] = s.Version.Date
					if len(s.Version.Commit) > 1 {
						ver["commit"] = s.Version.Commit
					}
					data.Add("Version", ver)
				}
				msg = append(msg, fmt.Sprintf("code=%v", s.Code))
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
					msg = append(msg, fmt.Sprintf(" msg=%v", s.Message))
				}
			} else {
				// Add empty rows
				data.Add("Version", "").Add("Leases", "").Add("Deployments", "").Add("Orders", "").Add("Version", "").Add("Available", "")
			}
			data.Add("Message(s)", wordwrap.WrapString(strings.Join(msg, " "), 25))
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

package hostnameoperator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"github.com/ovrclk/akash/provider/cluster"
	clusterClient "github.com/ovrclk/akash/provider/cluster/kube"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/cluster/util"
	provider_flags "github.com/ovrclk/akash/provider/cmd/flags"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"strings"
	"time"

	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/provider/operator/operatorcommon"
)

var (
	errExpectedResourceNotFound = fmt.Errorf("%w: resource not found", operatorcommon.ErrObservationStopped)
)

type hostnameOperator struct {
	hostnames map[string]managedHostname

	leasesIgnored operatorcommon.IgnoreList

	client cluster.Client

	log log.Logger

	cfg    hostnameOperatorConfig
	server operatorcommon.OperatorHTTP

	flagHostnamesData  operatorcommon.PrepareFlagFn
	flagIgnoreListData operatorcommon.PrepareFlagFn
}

func (op *hostnameOperator) run(parentCtx context.Context) error {
	op.log.Debug("hostname operator start")

	for {
		lastAttempt := time.Now()
		err := op.monitorUntilError(parentCtx)
		if errors.Is(err, context.Canceled) {
			op.log.Debug("hostname operator terminate")
			return err
		}

		op.log.Error("observation stopped", "err", err)

		// don't spin if there is a condition causing fast failure
		elapsed := time.Since(lastAttempt)
		if elapsed < op.cfg.retryDelay {
			op.log.Info("delaying")
			select {
			case <-parentCtx.Done():
				return parentCtx.Err()
			case <-time.After(op.cfg.retryDelay):
				// delay complete
			}
		}
	}
}

func (op *hostnameOperator) monitorUntilError(parentCtx context.Context) error {
	/*
		Note - the only possible enhancement here would be to enumerate all
		Ingress objects in the kube cluster not managed by Akash & then
		avoid trying to create Ingress objects with those names. This isn't really
		needed at this time.
	*/
	op.hostnames = make(map[string]managedHostname)
	ctx, cancel := context.WithCancel(parentCtx)
	op.log.Info("starting observation")

	connections, err := op.client.GetHostnameDeploymentConnections(ctx)
	if err != nil {
		cancel()
		return err
	}

	for _, conn := range connections {
		leaseID := conn.GetLeaseID()
		hostname := conn.GetHostname()
		entry := managedHostname{
			lastEvent:           nil,
			presentLease:        leaseID,
			presentServiceName:  conn.GetServiceName(),
			presentExternalPort: uint32(conn.GetExternalPort()),
		}

		op.hostnames[hostname] = entry
		op.log.Debug("identified existing hostname connection",
			"hostname", hostname,
			"lease", entry.presentLease,
			"service", entry.presentServiceName,
			"port", entry.presentExternalPort)
	}
	op.flagHostnamesData()

	events, err := op.client.ObserveHostnameState(ctx)
	if err != nil {
		cancel()
		return err
	}

	pruneTicker := time.NewTicker(op.cfg.pruneInterval)
	defer pruneTicker.Stop()
	prepareTicker := time.NewTicker(op.cfg.webRefreshInterval)
	defer prepareTicker.Stop()

	var exitError error
loop:
	for {
		select {
		case <-ctx.Done():
			exitError = ctx.Err()
			break loop

		case ev, ok := <-events:
			if !ok {
				exitError = operatorcommon.ErrObservationStopped
				break loop
			}
			err = op.applyEvent(ctx, ev)
			if err != nil {
				op.log.Error("failed applying event", "err", err)
				exitError = err
				break loop
			}
		case <-pruneTicker.C:
			op.prune()
		case <-prepareTicker.C:
			if err := op.server.PrepareAll(); err != nil {
				op.log.Error("preparing web data failed", "err", err)
			}

		}
	}

	cancel()
	op.log.Debug("hostname operator done")
	return exitError
}

func (op *hostnameOperator) prepareIgnoreListData(pd operatorcommon.PreparedResult) error {
	op.log.Debug("preparing ignore-list")
	return op.leasesIgnored.Prepare(pd)
}

func (op *hostnameOperator) prepareHostnamesData(pd operatorcommon.PreparedResult) error {
	op.log.Debug("preparing managed-hostnames")
	buf := &bytes.Buffer{}
	data := make(map[string]interface{})

	for hostname, entry := range op.hostnames {
		preparedEntry := struct {
			LeaseID      mtypes.LeaseID
			Namespace    string
			ExternalPort uint32
			ServiceName  string
			LastUpdate   string
		}{
			LeaseID:      entry.presentLease,
			Namespace:    clusterutil.LeaseIDToNamespace(entry.presentLease),
			ExternalPort: entry.presentExternalPort,
			ServiceName:  entry.presentServiceName,
			LastUpdate:   entry.lastChangeAt.String(),
		}
		data[hostname] = preparedEntry
	}

	enc := json.NewEncoder(buf)
	err := enc.Encode(data)
	if err != nil {
		return err
	}

	pd.Set(buf.Bytes())
	return nil
}

func (op *hostnameOperator) prune() {
	if op.leasesIgnored.Prune() {
		op.flagIgnoreListData()
	}
}

func errorIsKubernetesResourceNotFound(failure error) bool {
	// check the error, only consider errors that are obviously
	// indicating a missing resource
	// otherwise simple errors like network issues could wind up with all CRDs
	// being ignored

	if kubeErrors.IsNotFound(failure) {
		return true
	}

	if errors.Is(failure, errExpectedResourceNotFound) {
		return true
	}

	errStr := failure.Error()
	// unless the error indicates a resource was not found, no action
	return strings.Contains(errStr, "not found")
}

func (op *hostnameOperator) recordEventError(ev ctypes.HostnameResourceEvent, failure error) {
	// no error, no action
	if failure == nil {
		return
	}

	mark := errorIsKubernetesResourceNotFound(failure)

	if !mark {
		return
	}

	op.log.Info("recording error for", "lease", ev.GetLeaseID().String(), "err", failure)

	op.leasesIgnored.AddError(ev.GetLeaseID(), failure, ev.GetHostname())
	op.flagIgnoreListData()
}

func (op *hostnameOperator) isEventIgnored(ev ctypes.HostnameResourceEvent) bool {
	return op.leasesIgnored.IsFlagged(ev.GetLeaseID())
}

func (op *hostnameOperator) applyEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	op.log.Debug("apply event", "event-type", ev.GetEventType(), "hostname", ev.GetHostname())
	switch ev.GetEventType() {
	case ctypes.ProviderResourceDelete:
		// note that on delete the resource might be gone anyways because the namespace is deleted
		return op.applyDeleteEvent(ctx, ev)
	case ctypes.ProviderResourceAdd, ctypes.ProviderResourceUpdate:
		if op.isEventIgnored(ev) {
			op.log.Info("ignoring event for", "lease", ev.GetLeaseID().String())
			return nil
		}
		err := op.applyAddOrUpdateEvent(ctx, ev)
		op.recordEventError(ev, err)
		return err
	default:
		return fmt.Errorf("%w: unknown event type %v", operatorcommon.ErrObservationStopped, ev.GetEventType())
	}

}

func (op *hostnameOperator) applyDeleteEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	leaseID := ev.GetLeaseID()
	err := op.client.RemoveHostnameFromDeployment(ctx, ev.GetHostname(), leaseID, true)

	if err == nil {
		delete(op.hostnames, ev.GetHostname())
		op.flagHostnamesData()
	}

	return err
}

func buildDirective(ev ctypes.HostnameResourceEvent, serviceExpose crd.ManifestServiceExpose) ctypes.ConnectHostnameToDeploymentDirective {
	// Build the directive based off the event
	directive := ctypes.ConnectHostnameToDeploymentDirective{
		Hostname:    ev.GetHostname(),
		LeaseID:     ev.GetLeaseID(),
		ServiceName: ev.GetServiceName(),
		ServicePort: int32(ev.GetExternalPort()),
	}
	/*
		Populate the configuration options
		selectedExpose.HttpOptions has zero values if this is from an earlier CRD. Just insert
		defaults and move on
	*/
	if serviceExpose.HTTPOptions.MaxBodySize == 0 {
		directive.ReadTimeout = 60000
		directive.SendTimeout = 60000
		directive.NextTimeout = 60000
		directive.MaxBodySize = 1048576
		directive.NextTries = 3
		directive.NextCases = []string{"error", "timeout"}
	} else {
		directive.ReadTimeout = serviceExpose.HTTPOptions.ReadTimeout
		directive.SendTimeout = serviceExpose.HTTPOptions.SendTimeout
		directive.NextTimeout = serviceExpose.HTTPOptions.NextTimeout
		directive.MaxBodySize = serviceExpose.HTTPOptions.MaxBodySize
		directive.NextTries = serviceExpose.HTTPOptions.NextTries
		directive.NextCases = serviceExpose.HTTPOptions.NextCases
	}

	return directive
}

func locateServiceFromManifest(ctx context.Context, client cluster.Client, leaseID mtypes.LeaseID, serviceName string, externalPort uint32) (crd.ManifestServiceExpose, error) {

	// Locate the matchin service name & expose directive in the manifest CRD
	found, manifestGroup, err := client.GetManifestGroup(ctx, leaseID)
	if err != nil {
		return crd.ManifestServiceExpose{}, err
	}
	if !found {
		/*
			It's possible this code could race to read the CRD, although unlikely. If this fails the operator
			restarts and should work the attempt anyways. If this becomes a pain point then the operator
			can be rewritten to watch for CRD events on the manifest as well, then avoid running this code
			until the manifest exists.
		*/
		return crd.ManifestServiceExpose{}, fmt.Errorf("%w: manifest for %v", errExpectedResourceNotFound, leaseID)
	}

	var selectedService crd.ManifestService
	for _, service := range manifestGroup.Services {
		if service.Name == serviceName {
			selectedService = service
			break
		}
	}

	if selectedService.Count == 0 {
		return crd.ManifestServiceExpose{}, fmt.Errorf("%w: service for %v - %v", errExpectedResourceNotFound, leaseID, serviceName)
	}

	var selectedExpose crd.ManifestServiceExpose
	for _, expose := range selectedService.Expose {
		if !expose.Global {
			continue
		}

		if externalPort == uint32(util.ExposeExternalPort(manifest.ServiceExpose{
			Port:         expose.Port,
			ExternalPort: expose.ExternalPort,
		})) {
			selectedExpose = expose
			break
		}
	}

	if selectedExpose.Port == 0 {
		return crd.ManifestServiceExpose{}, fmt.Errorf("%w: service expose for %v - %v - %v", errExpectedResourceNotFound, leaseID, serviceName, externalPort)
	}

	return selectedExpose, nil
}

func (op *hostnameOperator) applyAddOrUpdateEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	selectedExpose, err := locateServiceFromManifest(ctx, op.client, ev.GetLeaseID(), ev.GetServiceName(), ev.GetExternalPort())
	if err != nil {
		return err
	}

	leaseID := ev.GetLeaseID()

	op.log.Debug("connecting",
		"hostname", ev.GetHostname(),
		"lease", leaseID,
		"service", ev.GetServiceName(),
		"externalPort", ev.GetExternalPort())
	entry, exists := op.hostnames[ev.GetHostname()]

	isSameLease := false
	if exists {
		isSameLease = entry.presentLease.Equals(leaseID)
	} else {
		isSameLease = true
	}

	directive := buildDirective(ev, selectedExpose)

	if isSameLease {
		shouldConnect := false

		if !exists {
			shouldConnect = true
			op.log.Debug("hostname target is new, applying")
			// Check to see if port or service name is different
		} else if entry.presentExternalPort != ev.GetExternalPort() || entry.presentServiceName != ev.GetServiceName() {
			shouldConnect = true
			op.log.Debug("hostname target has changed, applying")
		}

		if shouldConnect {
			op.log.Debug("Updating ingress")
			// Update or create the existing ingress
			err = op.client.ConnectHostnameToDeployment(ctx, directive)
		}
	} else {
		op.log.Debug("Swapping ingress to new deployment")
		//  Delete the ingress in one namespace and recreate it in the correct one
		err = op.client.RemoveHostnameFromDeployment(ctx, ev.GetHostname(), entry.presentLease, false)
		// TODO - remove entry
		if err == nil {
			err = op.client.ConnectHostnameToDeployment(ctx, directive)
		}
	}

	if err == nil { // Update stored entry if everything went OK
		entry.presentExternalPort = ev.GetExternalPort()
		entry.presentServiceName = ev.GetServiceName()
		entry.presentLease = leaseID
		entry.lastEvent = ev
		entry.lastChangeAt = time.Now()
		op.hostnames[ev.GetHostname()] = entry
		op.flagHostnamesData()
	}

	return err
}

func newHostnameOperator(logger log.Logger, client cluster.Client, config hostnameOperatorConfig, ilc operatorcommon.IgnoreListConfig) (*hostnameOperator, error) {
	opHTTP, err := operatorcommon.NewOperatorHTTP()
	if err != nil {
		return nil, err
	}
	op := &hostnameOperator{
		hostnames:     make(map[string]managedHostname),
		client:        client,
		log:           logger,
		cfg:           config,
		server:        opHTTP,
		leasesIgnored: operatorcommon.NewIgnoreList(ilc),
	}

	op.flagIgnoreListData = op.server.AddPreparedEndpoint("/ignore-list", op.prepareIgnoreListData)
	op.flagHostnamesData = op.server.AddPreparedEndpoint("/managed-hostnames", op.prepareHostnamesData)

	return op, nil
}

func doHostnameOperator(cmd *cobra.Command) error {
	ns := viper.GetString(provider_flags.FlagK8sManifestNS)

	listenAddr := viper.GetString(provider_flags.FlagListenAddress)
	config := hostnameOperatorConfig{
		pruneInterval:      viper.GetDuration(provider_flags.FlagPruneInterval),
		webRefreshInterval: viper.GetDuration(provider_flags.FlagWebRefreshInterval),
		retryDelay:         viper.GetDuration(provider_flags.FlagRetryDelay),
	}

	logger := operatorcommon.OpenLogger().With("op", "hostname")
	logger.Info("HTTP listening", "address", listenAddr)

	// Config path not provided because the authorization comes from the role assigned to the deployment
	// and provided by kubernetes
	client, err := clusterClient.NewClient(logger, ns, "")
	if err != nil {
		return err
	}

	op, err := newHostnameOperator(logger, client, config, operatorcommon.IgnoreListConfigFromViper())
	if err != nil {
		return err
	}

	router := op.server.GetRouter()
	group, ctx := errgroup.WithContext(cmd.Context())

	group.Go(func() error {
		srv := http.Server{Addr: listenAddr, Handler: router}
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})

	group.Go(func() error {
		return op.run(ctx)
	})

	err = group.Wait()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "hostname-operator",
		Short:        "kubernetes operator interfacing with k8s nginx ingress",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doHostnameOperator(cmd)
		},
	}
	operatorcommon.AddOperatorFlags(cmd, "0.0.0.0:8085")
	operatorcommon.AddIgnoreListFlags(cmd)

	return cmd
}

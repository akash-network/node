package cmd

import (
	"context"
	"errors"
	"fmt"
	"github.com/ovrclk/akash/manifest"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/provider/cluster"
	clusterClient "github.com/ovrclk/akash/provider/cluster/kube"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	"time"
)

type managedHostname struct {
	lastEvent    ctypes.HostnameResourceEvent
	presentLease mtypes.LeaseID

	presentServiceName  string
	presentExternalPort int32
}

type hostnameOperator struct {
	hostnames map[string]managedHostname

	client cluster.Client

	log log.Logger
}

func (op *hostnameOperator) run(parentCtx context.Context) error {
	const threshold = 3 * time.Second

	for {
		lastAttempt := time.Now()
		err := op.monitorUntilError(parentCtx)
		if errors.Is(err, context.Canceled) {
			return err
		}

		op.log.Error("observation stopped", "err", err)

		// don't spin if there is a condition causing fast failure
		elapsed := time.Now().Sub(lastAttempt)
		if elapsed < threshold {
			op.log.Info("delaying")
			select {
			case <-parentCtx.Done():
				return parentCtx.Err()
			case <-time.After(threshold):
				// delay complete
			}
		}
	}
}

var errObservationStopped = errors.New("observation stopped")

func (op *hostnameOperator) monitorUntilError(parentCtx context.Context) error {
	/*
		Note - the only possible enhancement here would be to enumerate all
		Ingress objects in the kube cluster not managed by Akash & then
		avoid trying to create Ingress objects with those names. This isn't really
		needed at this time.
	*/
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
			presentExternalPort: conn.GetExternalPort(),
		}

		op.hostnames[hostname] = entry
		op.log.Debug("identified existing hostname connection",
			"hostname", hostname,
			"lease", entry.presentLease,
			"service", entry.presentServiceName,
			"port", entry.presentExternalPort)
	}

	events, err := op.client.ObserveHostnameState(ctx)
	if err != nil {
		cancel()
		return err
	}

	var exitError error
loop:
	for {
		select {
		case <-ctx.Done():
			exitError = ctx.Err()
			break loop

		case ev, ok := <-events:
			if !ok {
				exitError = errObservationStopped
				break loop
			}
			err = op.applyEvent(ctx, ev)
			if err != nil {
				op.log.Error("failed applying event", "err", err)
				exitError = err
				break loop
			}
		}
	}

	cancel()
	return exitError
}

func (op *hostnameOperator) applyEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	op.log.Debug("apply event", "event-type", ev.GetEventType(), "hostname", ev.GetHostname())
	switch ev.GetEventType() {
	case ctypes.ProviderResourceDelete:
		// note that on delete the resource might be gone anyways because the namespace is deleted
		return op.applyDeleteEvent(ctx, ev)
	case ctypes.ProviderResourceAdd, ctypes.ProviderResourceUpdate:
		return op.applyAddOrUpdateEvent(ctx, ev)
	default:
		return fmt.Errorf("%w: unknown event type %v", errObservationStopped, ev.GetEventType())
	}

}

func (op *hostnameOperator) applyDeleteEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	leaseID := ev.GetLeaseID()
	err := op.client.RemoveHostnameFromDeployment(ctx, ev.GetHostname(), leaseID, true)

	if err == nil {
		delete(op.hostnames, ev.GetHostname())
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
	if serviceExpose.HttpOptions.MaxBodySize == 0 {
		directive.ReadTimeout = 60000
		directive.SendTimeout = 60000
		directive.NextTimeout = 60000
		directive.MaxBodySize = 1048576
		directive.NextTries = 3
		directive.NextCases = []string{"error", "timeout"}
	} else {
		directive.ReadTimeout = serviceExpose.HttpOptions.ReadTimeout
		directive.SendTimeout = serviceExpose.HttpOptions.SendTimeout
		directive.NextTimeout = serviceExpose.HttpOptions.NextTimeout
		directive.MaxBodySize = serviceExpose.HttpOptions.MaxBodySize
		directive.NextTries = serviceExpose.HttpOptions.NextTries
		directive.NextCases = serviceExpose.HttpOptions.NextCases
	}

	return directive
}

func (op *hostnameOperator) applyAddOrUpdateEvent(ctx context.Context, ev ctypes.HostnameResourceEvent) error {
	// Locate the matchin service name & expose directive in the manifest CRD
	found, manifestGroup, err := op.client.GetManifestGroup(ctx, ev.GetLeaseID())
	if err != nil {
		return err
	}
	if !found {
		/*
			It's possible this code could race to read the CRD, although unlikely. If this fails the operator
			restarts and should work the attempt anyways. If this becomes a pain point then the operator
			can be rewritten to watch for CRD events on the manifest as well, then avoid running this code
			until the manifest exists.
		*/
		return fmt.Errorf("%w: no manifest found for %v", errObservationStopped, ev.GetLeaseID())
	}

	var selectedService crd.ManifestService
	for _, service := range manifestGroup.Services {
		if service.Name == ev.GetServiceName() {
			selectedService = service
			break
		}
	}

	if selectedService.Count == 0 {
		return fmt.Errorf("%w: no service found for %v - %v", errObservationStopped, ev.GetLeaseID(), ev.GetServiceName())
	}

	var selectedExpose crd.ManifestServiceExpose
	for _, expose := range selectedService.Expose {
		if !expose.Global {
			continue
		}

		if ev.GetExternalPort() == uint32(util.ExposeExternalPort(manifest.ServiceExpose{
			Port:         expose.Port,
			ExternalPort: expose.ExternalPort,
		})) {
			selectedExpose = expose
			break
		}
	}

	if selectedExpose.Port == 0 {
		return fmt.Errorf("%w: no service expose found for %v - %v - %v", errObservationStopped, ev.GetLeaseID(), ev.GetServiceName(), ev.GetExternalPort())
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
		// Check to see if port or service name is different
		changed := !exists || uint32(entry.presentExternalPort) != ev.GetExternalPort() || entry.presentServiceName != ev.GetServiceName()
		if changed {
			op.log.Debug("Updating ingress")
			// Update or create the existing ingress
			err = op.client.ConnectHostnameToDeployment(ctx, directive)
		}
	} else {
		op.log.Debug("Swapping ingress to new deployment")
		//  Delete the ingress in one namespace and recreate it in the correct one
		err = op.client.RemoveHostnameFromDeployment(ctx, ev.GetHostname(), entry.presentLease, false)
		if err == nil {
			err = op.client.ConnectHostnameToDeployment(ctx, directive)
		}
	}

	if err == nil { // Update sored entry if everything went OK
		entry.presentLease = leaseID
		entry.lastEvent = ev
		op.hostnames[ev.GetHostname()] = entry
	}

	return err
}

func doHostnameOperator(cmd *cobra.Command) error {
	ns := viper.GetString(FlagK8sManifestNS)
	log := openLogger()
	// TODO - figure out a way to split out the client object. At this time there is no real need
	// to do anything with 'settings' in this context, but the object needs to be passed in anyways
	// TODO - make sure the client can pickup the in-cluster authorization to support running as a kubernetes
	// deployment
	client, err := clusterClient.NewClient(log, ns, clusterClient.Settings{})
	if err != nil {
		return err
	}

	op := hostnameOperator{
		hostnames: make(map[string]managedHostname),
		client:    client,
		log:       log,
	}

	return op.run(cmd.Context())
}

func hostnameOperatorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "hostname-operator",
		Short:        "",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doHostnameOperator(cmd)
		},
	}

	cmd.Flags().String(FlagK8sManifestNS, "lease", "Cluster manifest namespace")
	if err := viper.BindPFlag(FlagK8sManifestNS, cmd.Flags().Lookup(FlagK8sManifestNS)); err != nil {
		return nil
	}

	return cmd
}

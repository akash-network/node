package kube

import (
	"context"
	"fmt"
	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"strings"

	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/flowcontrol"
	"os"
	"path"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/cluster/util"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/tendermint/tendermint/libs/log"

	"github.com/ovrclk/akash/manifest"
	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"k8s.io/client-go/tools/pager"

	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"
)

var (
	ErrLeaseNotFound             = errors.New("kube: lease not found")
	ErrNoDeploymentForLease      = errors.New("kube: no deployments for lease")
	ErrInternalError             = errors.New("kube: internal error")
	ErrNoServiceForLease         = errors.New("no service for that lease")
	ErrInvalidHostnameConnection = errors.New("kube: invalid hostname connection")
	ErrMissingLabel              = errors.New("kube: missing label")
	ErrInvalidLabelValue         = errors.New("kube: invalid label value")

	errNotConfiguredWithSettings = errors.New("not configured with settings in the context passed to function")
)

var (
	kubeCallsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "provider_kube_calls",
	}, []string{"action", "result"})
)

// Client interface includes cluster client
type Client interface {
	cluster.Client
}

var _ Client = (*client)(nil)

type client struct {
	kc                kubernetes.Interface
	ac                akashclient.Interface
	metc              metricsclient.Interface
	ns                string
	log               log.Logger
	kubeContentConfig *restclient.Config
}

// NewClient returns new Kubernetes Client instance with provided logger, host and ns. Returns error incase of failure
// configPath may be the empty string
func NewClient(log log.Logger, ns string, configPath string) (Client, error) {
	return newClientWithSettings(log, ns, configPath, false)
}

func NewPreparedClient(log log.Logger, ns string, configPath string) (Client, error) {
	return newClientWithSettings(log, ns, configPath, true)
}

func newClientWithSettings(log log.Logger, ns string, configPath string, prepare bool) (Client, error) {
	ctx := context.Background()

	config, err := openKubeConfig(configPath, log)
	if err != nil {
		return nil, errors.Wrap(err, "kube: error building config flags")
	}
	config.RateLimiter = flowcontrol.NewFakeAlwaysRateLimiter()

	kc, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "kube: error creating kubernetes client")
	}

	mc, err := akashclient.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "kube: error creating manifest client")
	}

	metc, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "kube: error creating metrics client")
	}

	if prepare {
		if err := prepareEnvironment(ctx, kc, ns); err != nil {
			return nil, errors.Wrap(err, "kube: error preparing environment")
		}

		if _, err := kc.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
			return nil, errors.Wrap(err, "kube: error connecting to kubernetes")
		}
	}

	return &client{
		kc:                kc,
		ac:                mc,
		metc:              metc,
		ns:                ns,
		log:               log.With("module", "provider-cluster-kube"),
		kubeContentConfig: config,
	}, nil

}

func openKubeConfig(cfgPath string, log log.Logger) (*rest.Config, error) {
	// If no value is specified, use a default
	if len(cfgPath) == 0 {
		cfgPath = path.Join(homedir.HomeDir(), ".kube", "config")
	}

	if _, err := os.Stat(cfgPath); err == nil {
		log.Info("using kube config file", "path", cfgPath)
		return clientcmd.BuildConfigFromFlags("", cfgPath)
	}

	log.Info("using in cluster kube config")
	return rest.InClusterConfig()
}

func (c *client) GetDeployments(ctx context.Context, dID dtypes.DeploymentID) ([]ctypes.Deployment, error) {
	labelSelectors := &strings.Builder{}
	_, _ = fmt.Fprintf(labelSelectors, "%s=%d", akashLeaseDSeqLabelName, dID.DSeq)
	_, _ = fmt.Fprintf(labelSelectors, ",%s=%s", akashLeaseOwnerLabelName, dID.Owner)

	manifests, err := c.ac.AkashV1().Manifests(c.ns).List(ctx, metav1.ListOptions{
		TypeMeta:             metav1.TypeMeta{},
		LabelSelector:        labelSelectors.String(),
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	})

	if err != nil {
		return nil, err
	}

	result := make([]ctypes.Deployment, len(manifests.Items))
	for i, manifest := range manifests.Items {
		result[i], err = manifest.Deployment()
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (c *client) GetManifestGroup(ctx context.Context, lID mtypes.LeaseID) (bool, akashv1.ManifestGroup, error) {
	leaseNamespace := lidNS(lID)

	obj, err := c.ac.AkashV1().Manifests(c.ns).Get(ctx, leaseNamespace, metav1.GetOptions{})
	if err != nil {
		if kubeErrors.IsNotFound(err) {
			c.log.Info("CRD manifest not found", "lease-ns", leaseNamespace)
			return false, akashv1.ManifestGroup{}, nil
		}

		return false, akashv1.ManifestGroup{}, err
	}

	return true, obj.Spec.Group, nil
}

func (c *client) Deployments(ctx context.Context) ([]ctypes.Deployment, error) {
	manifests, err := c.ac.AkashV1().Manifests(c.ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	deployments := make([]ctypes.Deployment, 0, len(manifests.Items))
	for _, manifest := range manifests.Items {
		deployment, err := manifest.Deployment()
		if err != nil {
			return deployments, err
		}
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (c *client) Deploy(ctx context.Context, lid mtypes.LeaseID, group *manifest.Group) error {
	settingsI := ctx.Value(SettingsKey)
	if nil == settingsI {
		return errNotConfiguredWithSettings
	}
	settings := settingsI.(Settings)
	if err := ValidateSettings(settings); err != nil {
		return err
	}

	if err := applyNS(ctx, c.kc, newNSBuilder(settings, lid, group)); err != nil {
		c.log.Error("applying namespace", "err", err, "lease", lid)
		return err
	}

	if err := applyNetPolicies(ctx, c.kc, newNetPolBuilder(settings, lid, group)); err != nil {
		c.log.Error("applying namespace network policies", "err", err, "lease", lid)
		return err
	}

	if err := applyManifest(ctx, c.ac, newManifestBuilder(c.log, settings, c.ns, lid, group)); err != nil {
		c.log.Error("applying manifest", "err", err, "lease", lid)
		return err
	}

	if err := cleanupStaleResources(ctx, c.kc, lid, group); err != nil {
		c.log.Error("cleaning stale resources", "err", err, "lease", lid)
		return err
	}

	for svcIdx := range group.Services {
		service := &group.Services[svcIdx]
		if err := applyDeployment(ctx, c.kc, newDeploymentBuilder(c.log, settings, lid, group, service)); err != nil {
			c.log.Error("applying deployment", "err", err, "lease", lid, "service", service.Name)
			return err
		}

		if len(service.Expose) == 0 {
			c.log.Debug("no services", "lease", lid, "service", service.Name)
			continue
		}

		serviceBuilderLocal := newServiceBuilder(c.log, settings, lid, group, service, false)
		if serviceBuilderLocal.any() {
			if err := applyService(ctx, c.kc, serviceBuilderLocal); err != nil {
				c.log.Error("applying local service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		serviceBuilderGlobal := newServiceBuilder(c.log, settings, lid, group, service, true)
		if serviceBuilderGlobal.any() {
			if err := applyService(ctx, c.kc, serviceBuilderGlobal); err != nil {
				c.log.Error("applying global service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

	}

	return nil
}

func (c *client) TeardownLease(ctx context.Context, lid mtypes.LeaseID) error {
	leaseNamespace := lidNS(lid)
	result := c.kc.CoreV1().Namespaces().Delete(ctx, leaseNamespace, metav1.DeleteOptions{})

	label := metricsutils.SuccessLabel
	if result != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("namespaces-delete", label).Inc()

	return result
}
func kubeSelectorForLease(dst *strings.Builder, lID mtypes.LeaseID) {
	_, _ = fmt.Fprintf(dst, "%s=%s", akashLeaseOwnerLabelName, lID.Owner)
	_, _ = fmt.Fprintf(dst, ",%s=%d", akashLeaseDSeqLabelName, lID.DSeq)
	_, _ = fmt.Fprintf(dst, ",%s=%d", akashLeaseGSeqLabelName, lID.GSeq)
	_, _ = fmt.Fprintf(dst, ",%s=%d", akashLeaseOSeqLabelName, lID.OSeq)
}

func newEventsFeedList(ctx context.Context, events []eventsv1.Event) ctypes.EventsWatcher {
	wtch := ctypes.NewEventsFeed(ctx)

	go func() {
		defer wtch.Shutdown()

	done:
		for _, evt := range events {
			evt := evt
			if !wtch.SendEvent(&evt) {
				break done
			}
		}
	}()

	return wtch
}

func newEventsFeedWatch(ctx context.Context, events watch.Interface) ctypes.EventsWatcher {
	wtch := ctypes.NewEventsFeed(ctx)

	go func() {
		defer func() {
			events.Stop()
			wtch.Shutdown()
		}()

	done:
		for {
			select {
			case obj, ok := <-events.ResultChan():
				if !ok {
					break done
				}
				evt := obj.Object.(*eventsv1.Event)
				if !wtch.SendEvent(evt) {
					break done
				}
			case <-wtch.Done():
				break done
			}
		}
	}()

	return wtch
}

func (c *client) LeaseEvents(ctx context.Context, lid mtypes.LeaseID, services string, follow bool) (ctypes.EventsWatcher, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	listOpts := metav1.ListOptions{}
	if len(services) != 0 {
		listOpts.LabelSelector = fmt.Sprintf(akashManifestServiceLabelName+" in (%s)", services)
	}

	var wtch ctypes.EventsWatcher
	if follow {
		watcher, err := c.kc.EventsV1().Events(lidNS(lid)).Watch(ctx, listOpts)
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("events-follow", label).Inc()
		if err != nil {
			return nil, err
		}

		wtch = newEventsFeedWatch(ctx, watcher)
	} else {
		list, err := c.kc.EventsV1().Events(lidNS(lid)).List(ctx, listOpts)
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("events-list", label).Inc()
		if err != nil {
			return nil, err
		}

		wtch = newEventsFeedList(ctx, list.Items)
	}

	return wtch, nil
}

func (c *client) LeaseLogs(ctx context.Context, lid mtypes.LeaseID,
	services string, follow bool, tailLines *int64) ([]*ctypes.ServiceLog, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	listOpts := metav1.ListOptions{}
	if len(services) != 0 {
		listOpts.LabelSelector = fmt.Sprintf(akashManifestServiceLabelName+" in (%s)", services)
	}

	c.log.Error("filtering pods", "labelSelector", listOpts.LabelSelector)

	pods, err := c.kc.CoreV1().Pods(lidNS(lid)).List(ctx, listOpts)
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("pods-list", label).Inc()
	if err != nil {
		c.log.Error("listing pods", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	streams := make([]*ctypes.ServiceLog, len(pods.Items))
	for i, pod := range pods.Items {
		stream, err := c.kc.CoreV1().Pods(lidNS(lid)).GetLogs(pod.Name, &corev1.PodLogOptions{
			Follow:     follow,
			TailLines:  tailLines,
			Timestamps: false,
		}).Stream(ctx)
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("pods-getlogs", label).Inc()
		if err != nil {
			c.log.Error("get pod logs", "err", err)
			return nil, errors.Wrap(err, ErrInternalError.Error())
		}
		streams[i] = cluster.NewServiceLog(pod.Name, stream)
	}
	return streams, nil
}

// todo: limit number of results and do pagination / streaming
func (c *client) LeaseStatus(ctx context.Context, lid mtypes.LeaseID) (*ctypes.LeaseStatus, error) {
	settingsI := ctx.Value(SettingsKey)
	if nil == settingsI {
		return nil, errNotConfiguredWithSettings
	}
	settings := settingsI.(Settings)
	if err := ValidateSettings(settings); err != nil {
		return nil, err
	}

	deployments, err := c.deploymentsForLease(ctx, lid)
	if err != nil {
		return nil, err
	}
	labelSelector := &strings.Builder{}
	kubeSelectorForLease(labelSelector, lid)
	phResult, err := c.ac.AkashV1().ProviderHosts(c.ns).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})

	if err != nil {
		return nil, err
	}

	serviceStatus := make(map[string]*ctypes.ServiceStatus, len(deployments))
	forwardedPorts := make(map[string][]ctypes.ForwardedPortStatus, len(deployments))
	for _, deployment := range deployments {
		status := &ctypes.ServiceStatus{
			Name:               deployment.Name,
			Available:          deployment.Status.AvailableReplicas,
			Total:              deployment.Status.Replicas,
			ObservedGeneration: deployment.Status.ObservedGeneration,
			Replicas:           deployment.Status.Replicas,
			UpdatedReplicas:    deployment.Status.UpdatedReplicas,
			ReadyReplicas:      deployment.Status.ReadyReplicas,
			AvailableReplicas:  deployment.Status.AvailableReplicas,
		}
		serviceStatus[deployment.Name] = status
	}

	// For each provider host entry, update the status of each service to indicate
	// the presently assigned hostnames
	for _, ph := range phResult.Items {
		entry, ok := serviceStatus[ph.Spec.ServiceName]
		if ok {
			entry.URIs = append(entry.URIs, ph.Spec.Hostname)
		}
	}

	services, err := c.kc.CoreV1().Services(lidNS(lid)).List(ctx, metav1.ListOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("services-list", label).Inc()
	if err != nil {
		c.log.Error("list services", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	// Search for a Kubernetes service declared as nodeport
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeNodePort {
			serviceName := service.Name // Always suffixed during creation, so chop it off
			deploymentName := serviceName[0 : len(serviceName)-len(suffixForNodePortServiceName)]
			deployment, ok := serviceStatus[deploymentName]
			if ok && 0 != len(service.Spec.Ports) {
				portsForDeployment := make([]ctypes.ForwardedPortStatus, 0, len(service.Spec.Ports))
				for _, port := range service.Spec.Ports {
					// Check if the service is exposed via NodePort mechanism in the cluster
					// This is a random port chosen by the cluster when the deployment is created
					nodePort := port.NodePort
					if nodePort > 0 {
						// Record the actual port inside the container that is exposed
						v := ctypes.ForwardedPortStatus{
							Host:         settings.ClusterPublicHostname,
							Port:         uint16(port.TargetPort.IntVal),
							ExternalPort: uint16(nodePort),
							Available:    deployment.Available,
							Name:         deploymentName,
						}

						isValid := true
						switch port.Protocol {
						case corev1.ProtocolTCP:
							v.Proto = manifest.TCP
						case corev1.ProtocolUDP:
							v.Proto = manifest.UDP
						default:
							isValid = false // Skip this, since the Protocol is set to something not supported by Akash
						}
						if isValid {
							portsForDeployment = append(portsForDeployment, v)
						}
					}
				}
				forwardedPorts[deploymentName] = portsForDeployment
			}
		}
	}

	response := &ctypes.LeaseStatus{
		Services:       serviceStatus,
		ForwardedPorts: forwardedPorts,
	}

	return response, nil
}

func (c *client) ServiceStatus(ctx context.Context, lid mtypes.LeaseID, name string) (*ctypes.ServiceStatus, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	c.log.Debug("get deployment", "lease-ns", lidNS(lid), "name", name)
	deployment, err := c.kc.AppsV1().Deployments(lidNS(lid)).Get(ctx, name, metav1.GetOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("deployments-get", label).Inc()

	if err != nil {
		c.log.Error("deployment get", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	if deployment == nil {
		c.log.Error("no deployment found", "name", name)
		return nil, ErrNoDeploymentForLease
	}

	hasHostnames := false
	// Get manifest definition from CRD
	c.log.Debug("Pulling manifest from CRD", "lease-ns", lidNS(lid))
	obj, err := c.ac.AkashV1().Manifests(c.ns).Get(ctx, lidNS(lid), metav1.GetOptions{})
	if err != nil {
		c.log.Error("CRD manifest not found", "lease-ns", lidNS(lid), "name", name)
		return nil, err
	}

	found := false
exposeCheckLoop:
	for _, service := range obj.Spec.Group.Services {
		if service.Name == name {
			found = true
			for _, expose := range service.Expose {

				proto, err := manifest.ParseServiceProtocol(expose.Proto)
				if err != nil {
					return nil, err
				}
				mse := manifest.ServiceExpose{
					Port:         expose.Port,
					ExternalPort: expose.ExternalPort,
					Proto:        proto,
					Service:      expose.Service,
					Global:       expose.Global,
					Hosts:        expose.Hosts,
				}
				if util.ShouldBeIngress(mse) {
					hasHostnames = true
					break exposeCheckLoop
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: service %q", ErrNoServiceForLease, name)
	}

	c.log.Debug("service result", "lease-ns", lidNS(lid), "has-hostnames", hasHostnames)

	result := &ctypes.ServiceStatus{
		Name:               deployment.Name,
		Available:          deployment.Status.AvailableReplicas,
		Total:              deployment.Status.Replicas,
		ObservedGeneration: deployment.Status.ObservedGeneration,
		Replicas:           deployment.Status.Replicas,
		UpdatedReplicas:    deployment.Status.UpdatedReplicas,
		ReadyReplicas:      deployment.Status.ReadyReplicas,
		AvailableReplicas:  deployment.Status.AvailableReplicas,
	}

	if hasHostnames {
		labelSelector := &strings.Builder{}
		kubeSelectorForLease(labelSelector, lid)

		phs, err := c.ac.AkashV1().ProviderHosts(c.ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("provider-hosts", label).Inc()
		if err != nil {
			c.log.Error("provider hosts get", "err", err)
			return nil, errors.Wrap(err, ErrInternalError.Error())
		}

		hosts := make([]string, 0, len(phs.Items))
		for _, ph := range phs.Items {
			hosts = append(hosts, ph.Spec.Hostname)
		}

		result.URIs = hosts
	}

	return result, nil
}

func (c *client) Inventory(ctx context.Context) ([]ctypes.Node, error) {
	// Load all the nodes
	knodes, err := c.activeNodes(ctx)
	if err != nil {
		return nil, err
	}

	nodes := make([]ctypes.Node, 0, len(knodes))
	// Iterate over the node metrics
	for nodeName, knode := range knodes {

		// Get the amount of available CPU, then subtract that in use
		var tmp resource.Quantity

		tmp = knode.cpu.allocatable
		cpuTotal := (&tmp).MilliValue()

		tmp = knode.memory.allocatable
		memoryTotal := (&tmp).Value()

		tmp = knode.storage.allocatable
		storageTotal := (&tmp).Value()

		tmp = knode.cpu.available()
		cpuAvailable := (&tmp).MilliValue()
		if cpuAvailable < 0 {
			cpuAvailable = 0
		}

		tmp = knode.memory.available()
		memoryAvailable := (&tmp).Value()
		if memoryAvailable < 0 {
			memoryAvailable = 0
		}

		tmp = knode.storage.available()
		storageAvailable := (&tmp).Value()
		if storageAvailable < 0 {
			storageAvailable = 0
		}

		resources := types.ResourceUnits{
			CPU: &types.CPU{
				Units: types.NewResourceValue(uint64(cpuAvailable)),
				Attributes: []types.Attribute{
					{
						Key:   "arch",
						Value: knode.arch,
					},
					// todo (#788) other node attributes ?
				},
			},
			Memory: &types.Memory{
				Quantity: types.NewResourceValue(uint64(memoryAvailable)),
				// todo (#788) memory attributes ?
			},
			Storage: &types.Storage{
				Quantity: types.NewResourceValue(uint64(storageAvailable)),
				// todo (#788) storage attributes like class and iops?
			},
		}

		allocateable := types.ResourceUnits{
			CPU: &types.CPU{
				Units: types.NewResourceValue(uint64(cpuTotal)),
				Attributes: []types.Attribute{
					{
						Key:   "arch",
						Value: knode.arch,
					},
					// todo (#788) other node attributes ?
				},
			},
			Memory: &types.Memory{
				Quantity: types.NewResourceValue(uint64(memoryTotal)),
				// todo (#788) memory attributes ?
			},
			Storage: &types.Storage{
				Quantity: types.NewResourceValue(uint64(storageTotal)),
				// todo (#788) storage attributes like class and iops?
			},
		}

		nodes = append(nodes, cluster.NewNode(nodeName, allocateable, resources))
	}

	return nodes, nil
}

type resourcePair struct {
	allocatable resource.Quantity
	allocated   resource.Quantity
}

func (rp resourcePair) available() resource.Quantity {
	result := rp.allocatable.DeepCopy()
	// Modifies the value in place
	(&result).Sub(rp.allocated)
	return result
}

type nodeResources struct {
	cpu     resourcePair
	memory  resourcePair
	storage resourcePair
	arch    string
}

func (c *client) activeNodes(ctx context.Context) (map[string]nodeResources, error) {
	knodes, err := c.kc.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("nodes-list", label).Inc()
	if err != nil {
		return nil, err
	}

	podListOptions := metav1.ListOptions{
		FieldSelector: "status.phase!=Failed,status.phase!=Succeeded",
	}
	podsClient := c.kc.CoreV1().Pods(metav1.NamespaceAll)
	podsPager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return podsClient.List(ctx, opts)
	})
	zero := resource.NewMilliQuantity(0, "m")

	retnodes := make(map[string]nodeResources)
	for _, knode := range knodes.Items {

		if !c.nodeIsActive(knode) {
			continue
		}

		// Create an entry with the allocatable amount for the node
		cpu := knode.Status.Allocatable.Cpu().DeepCopy()
		memory := knode.Status.Allocatable.Memory().DeepCopy()
		storage := knode.Status.Allocatable.StorageEphemeral().DeepCopy()

		entry := nodeResources{
			arch: knode.Status.NodeInfo.Architecture,
			cpu: resourcePair{
				allocatable: cpu,
			},
			memory: resourcePair{
				allocatable: memory,
			},
			storage: resourcePair{
				allocatable: storage,
			},
		}

		// Initialize the allocated amount to for each node
		zero.DeepCopyInto(&entry.cpu.allocated)
		zero.DeepCopyInto(&entry.memory.allocated)
		zero.DeepCopyInto(&entry.storage.allocated)

		retnodes[knode.Name] = entry
	}

	// Go over each pod and sum the resources for it into the value for the pod it lives on
	err = podsPager.EachListItem(ctx, podListOptions, func(obj runtime.Object) error {
		pod := obj.(*corev1.Pod)
		nodeName := pod.Spec.NodeName

		entry := retnodes[nodeName]
		cpuAllocated := &entry.cpu.allocated
		memoryAllocated := &entry.memory.allocated
		storageAllocated := &entry.storage.allocated
		for _, container := range pod.Spec.Containers {
			// Per the documentation Limits > Requests for each pod. But stuff in the kube-system
			// namespace doesn't follow this. The requests is always summed here since it is what
			// the cluster considers a dedicated resource

			cpuAllocated.Add(*container.Resources.Requests.Cpu())
			memoryAllocated.Add(*container.Resources.Requests.Memory())
			storageAllocated.Add(*container.Resources.Requests.StorageEphemeral())
		}

		retnodes[nodeName] = entry // Map is by value, so store the copy back into the map
		return nil
	})
	if err != nil {
		return nil, err
	}

	return retnodes, nil
}

func (c *client) nodeIsActive(node corev1.Node) bool {
	ready := false
	issues := 0

	for _, cond := range node.Status.Conditions {
		switch cond.Type {

		case corev1.NodeReady:

			if cond.Status == corev1.ConditionTrue {
				ready = true
			}

		case corev1.NodeMemoryPressure:
			fallthrough
		case corev1.NodeDiskPressure:
			fallthrough
		case corev1.NodePIDPressure:
			fallthrough
		case corev1.NodeNetworkUnavailable:

			if cond.Status != corev1.ConditionFalse {

				c.log.Error("node in poor condition",
					"node", node.Name,
					"condition", cond.Type,
					"status", cond.Status)

				issues++
			}
		}
	}

	return ready && issues == 0
}

func (c *client) leaseExists(ctx context.Context, lid mtypes.LeaseID) error {
	_, err := c.kc.CoreV1().Namespaces().Get(ctx, lidNS(lid), metav1.GetOptions{})
	label := metricsutils.SuccessLabel
	if err != nil && !kubeErrors.IsNotFound(err) {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("namespace-get", label).Inc()
	if err != nil {
		if kubeErrors.IsNotFound(err) {
			return ErrLeaseNotFound
		}

		c.log.Error("namespaces get", "err", err)
		return errors.Wrap(err, ErrInternalError.Error())
	}

	return nil
}

func (c *client) deploymentsForLease(ctx context.Context, lid mtypes.LeaseID) ([]appsv1.Deployment, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	deployments, err := c.kc.AppsV1().Deployments(lidNS(lid)).List(ctx, metav1.ListOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("deployments-list", label).Inc()

	if err != nil {
		c.log.Error("deployments list", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	if deployments == nil || 0 == len(deployments.Items) {
		c.log.Info("No deployments found for", "lease namespace", lidNS(lid))
		return nil, ErrNoDeploymentForLease
	}

	return deployments.Items, nil
}

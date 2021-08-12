package kube

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	"github.com/ovrclk/akash/types"

	metricsutils "github.com/ovrclk/akash/util/metrics"

	"k8s.io/client-go/util/flowcontrol"

	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"

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

	"k8s.io/client-go/tools/pager"

	"github.com/ovrclk/akash/manifest"
	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/provider/cluster"
	mtypes "github.com/ovrclk/akash/x/market/types"

	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"
)

var (
	ErrLeaseNotFound            = errors.New("kube: lease not found")
	ErrNoDeploymentForLease     = errors.New("kube: no deployments for lease")
	ErrNoGlobalServicesForLease = errors.New("kube: no global services for lease")
	ErrInternalError            = errors.New("kube: internal error")
	ErrNoServiceForLease        = errors.New("no service for that lease")
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
	settings          builder.Settings
	log               log.Logger
	kubeContentConfig *restclient.Config
}

// NewClient returns new Kubernetes Client instance with provided logger, host and ns. Returns error incase of failure
func NewClient(log log.Logger, ns string, settings builder.Settings) (Client, error) {
	if err := builder.ValidateSettings(settings); err != nil {
		return nil, err
	}
	return newClientWithSettings(log, ns, settings)
}

func newClientWithSettings(log log.Logger, ns string, settings builder.Settings) (Client, error) {
	ctx := context.Background()

	config, err := openKubeConfig(settings.ConfigPath, log)
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

	if err := prepareEnvironment(ctx, kc, ns); err != nil {
		return nil, errors.Wrap(err, "kube: error preparing environment")
	}

	if _, err := kc.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		return nil, errors.Wrap(err, "kube: error connecting to kubernetes")
	}

	return &client{
		settings:          settings,
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

func (c *client) ActiveStorageClasses(ctx context.Context) ([]string, error) {
	return c.storageClasses(ctx)
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
	if err := applyNS(ctx, c.kc, builder.NewNS(c.settings, lid, group)); err != nil {
		c.log.Error("applying namespace", "err", err, "lease", lid)
		return err
	}

	// TODO: re-enable.  see #946
	// pspRestrictedBuilder produces restrictive PodSecurityPolicies for tenant Namespaces.
	// Restricted PSP source: https://raw.githubusercontent.com/kubernetes/website/master/content/en/examples/policy/restricted-psp.yaml
	// if err := applyRestrictivePodSecPoliciesToNS(ctx, c.kc, builder.NewPsp(c.settings, lid, group)); err != nil {
	// 	c.log.Error("applying pod security policies", "err", err, "lease", lid)
	// 	return err
	// }

	if err := applyNetPolicies(ctx, c.kc, builder.NewNetPol(c.settings, lid, group)); err != nil {
		c.log.Error("applying namespace network policies", "err", err, "lease", lid)
		return err
	}

	if err := applyManifest(ctx, c.ac, builder.NewManifest(c.log, c.settings, lid, group)); err != nil {
		c.log.Error("applying manifest", "err", err, "lease", lid)
		return err
	}

	if err := cleanupStaleResources(ctx, c.kc, lid, group); err != nil {
		c.log.Error("cleaning stale resources", "err", err, "lease", lid)
		return err
	}

	for svcIdx := range group.Services {
		service := &group.Services[svcIdx]

		persistent := false
		for i := range service.Resources.Storage {
			attrVal := service.Resources.Storage[i].Attributes.Find(builder.StorageAttributePersistent)
			if persistent, _ = attrVal.AsBool(); persistent {
				break
			}
		}

		if persistent {
			if err := applyStatefulSet(ctx, c.kc, builder.NewStatefulSet(c.log, c.settings, lid, group, service)); err != nil {
				c.log.Error("applying statefulSet", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		} else {
			if err := applyDeployment(ctx, c.kc, builder.NewDeployment(c.log, c.settings, lid, group, service)); err != nil {
				c.log.Error("applying deployment", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		if len(service.Expose) == 0 {
			c.log.Debug("no services", "lease", lid, "service", service.Name)
			continue
		}

		serviceBuilderLocal := builder.NewService(c.log, c.settings, lid, group, service, false)
		if serviceBuilderLocal.Any() {
			if err := applyService(ctx, c.kc, serviceBuilderLocal); err != nil {
				c.log.Error("applying local service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		serviceBuilderGlobal := builder.NewService(c.log, c.settings, lid, group, service, true)
		if serviceBuilderGlobal.Any() {
			if err := applyService(ctx, c.kc, serviceBuilderGlobal); err != nil {
				c.log.Error("applying global service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		for expIdx := range service.Expose {
			expose := service.Expose[expIdx]
			if !util.ShouldBeIngress(expose) {
				continue
			}
			if err := applyIngress(ctx, c.kc, builder.NewIngress(c.log, c.settings, lid, group, service, &service.Expose[expIdx])); err != nil {
				c.log.Error("applying ingress", "err", err, "lease", lid, "service", service.Name, "expose", expose)
				return err
			}
		}
	}

	return nil
}

func (c *client) TeardownLease(ctx context.Context, lid mtypes.LeaseID) error {
	result := c.kc.CoreV1().Namespaces().Delete(ctx, builder.LidNS(lid), metav1.DeleteOptions{})

	label := metricsutils.SuccessLabel
	if result != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("namespaces-delete", label).Inc()

	result = c.ac.AkashV1().Manifests("lease").Delete(ctx, builder.LidNS(lid), metav1.DeleteOptions{})

	return result
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
			case obj := <-events.ResultChan():
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
		listOpts.LabelSelector = fmt.Sprintf(builder.AkashManifestServiceLabelName+" in (%s)", services)
	}

	var wtch ctypes.EventsWatcher
	if follow {
		watcher, err := c.kc.EventsV1().Events(builder.LidNS(lid)).Watch(ctx, listOpts)
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
		list, err := c.kc.EventsV1().Events(builder.LidNS(lid)).List(ctx, listOpts)
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
		listOpts.LabelSelector = fmt.Sprintf(builder.AkashManifestServiceLabelName+" in (%s)", services)
	}

	c.log.Error("filtering pods", "labelSelector", listOpts.LabelSelector)

	pods, err := c.kc.CoreV1().Pods(builder.LidNS(lid)).List(ctx, listOpts)
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
		stream, err := c.kc.CoreV1().Pods(builder.LidNS(lid)).GetLogs(pod.Name, &corev1.PodLogOptions{
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
	deployments, err := c.deploymentsForLease(ctx, lid)
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

	ingress, err := c.kc.NetworkingV1().Ingresses(builder.LidNS(lid)).List(ctx, metav1.ListOptions{})
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("ingresses-list", label).Inc()
	if err != nil {
		c.log.Error("list ingresses", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	services, err := c.kc.CoreV1().Services(builder.LidNS(lid)).List(ctx, metav1.ListOptions{})
	label = metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("services-list", label).Inc()
	if err != nil {
		c.log.Error("list services", "err", err)
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	foundCnt := 0
	for _, ing := range ingress.Items {
		service, found := serviceStatus[ing.Name]
		if !found {
			continue
		}

		hosts := make([]string, 0, len(ing.Spec.Rules)+(len(ing.Status.LoadBalancer.Ingress)*2))

		for _, rule := range ing.Spec.Rules {
			hosts = append(hosts, rule.Host)
		}

		if c.settings.DeploymentIngressExposeLBHosts {
			for _, lbing := range ing.Status.LoadBalancer.Ingress {
				if val := lbing.IP; val != "" {
					hosts = append(hosts, val)
				}
				if val := lbing.Hostname; val != "" {
					hosts = append(hosts, val)
				}
			}
		}

		service.URIs = hosts
		foundCnt++
	}

	// Search for a Kubernetes service declared as nodeport
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeNodePort {
			serviceName := service.Name // Always suffixed during creation, so chop it off
			deploymentName := serviceName[0 : len(serviceName)-len(builder.SuffixForNodePortServiceName)]
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
							Host:         c.exposedHostForPort(),
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
							foundCnt++
							portsForDeployment = append(portsForDeployment, v)
						}
					}
				}
				forwardedPorts[deploymentName] = portsForDeployment
			}
		}
	}

	// If no ingress are found and at least 1 NodePort is not found, that is an error
	if 0 == foundCnt {
		return nil, ErrNoGlobalServicesForLease
	}

	response := &ctypes.LeaseStatus{
		Services:       serviceStatus,
		ForwardedPorts: forwardedPorts,
	}

	return response, nil
}

func (c *client) exposedHostForPort() string {
	return c.settings.ClusterPublicHostname
}

func (c *client) ServiceStatus(ctx context.Context, lid mtypes.LeaseID, name string) (*ctypes.ServiceStatus, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	c.log.Debug("get deployment", "lease-ns", builder.LidNS(lid), "name", name)
	deployment, err := c.kc.AppsV1().Deployments(builder.LidNS(lid)).Get(ctx, name, metav1.GetOptions{})
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

	hasIngress := false
	// Get manifest definition from CRD
	c.log.Debug("Pulling manifest from CRD", "lease-ns", builder.LidNS(lid))
	obj, err := c.ac.AkashV1().Manifests(c.ns).Get(ctx, builder.LidNS(lid), metav1.GetOptions{})
	if err != nil {
		c.log.Error("CRD manifest not found", "lease-ns", builder.LidNS(lid), "name", name)
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
					hasIngress = true
					break exposeCheckLoop
				}
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: service %q", ErrNoServiceForLease, name)
	}

	c.log.Debug("service result", "lease-ns", builder.LidNS(lid), "hasIngress", hasIngress)

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

	if hasIngress {
		ingress, err := c.kc.NetworkingV1().Ingresses(builder.LidNS(lid)).Get(ctx, name, metav1.GetOptions{})
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("networking-ingresses", label).Inc()
		if err != nil {
			c.log.Error("ingresses get", "err", err)
			return nil, errors.Wrap(err, ErrInternalError.Error())
		}

		hosts := make([]string, 0, len(ingress.Spec.Rules))
		for _, rule := range ingress.Spec.Rules {
			hosts = append(hosts, rule.Host)
		}

		if c.settings.DeploymentIngressExposeLBHosts {
			for _, lbing := range ingress.Status.LoadBalancer.Ingress {
				if val := lbing.IP; val != "" {
					hosts = append(hosts, val)
				}
				if val := lbing.Hostname; val != "" {
					hosts = append(hosts, val)
				}
			}
		}

		result.URIs = hosts
	}

	return result, nil
}

func (c *client) Inventory(ctx context.Context) (ctypes.Inventory, error) {
	sclasses, err := c.storageClasses(ctx)
	if err != nil {
		return nil, err
	}

	// Load all the nodes
	knodes, err := c.activeNodes(ctx)
	if err != nil {
		return nil, err
	}

	inventory := cluster.NewInventory()

	// Iterate over the node metrics
	for nodeName, knode := range knodes {
		// Get the amount of available CPU, then subtract that in use
		var tmp resource.Quantity

		tmp = knode.cpu.allocatable
		cpuTotal := (&tmp).MilliValue()

		tmp = knode.memory.allocatable
		memoryTotal := (&tmp).Value()

		tmp = knode.ephemeralStorage.allocatable
		ephemeralStorageTotal := (&tmp).Value()

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

		tmp = knode.ephemeralStorage.available()
		ephemeralStorageAvailable := (&tmp).Value()
		if ephemeralStorageAvailable < 0 {
			ephemeralStorageAvailable = 0
		}

		resources := ctypes.ResourceUnits{
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
			Storage: make(map[string]types.Storage),
		}

		resources.Storage["ephemeral"] = types.Storage{
			Quantity: types.NewResourceValue(uint64(ephemeralStorageAvailable)),
		}

		allocateable := ctypes.ResourceUnits{
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
			Storage: make(map[string]types.Storage),
		}

		allocateable.Storage["ephemeral"] = types.Storage{
			Quantity: types.NewResourceValue(uint64(ephemeralStorageTotal)),
		}

		inventory.AddNode(nodeName, allocateable, resources)
	}

	return inventory, nil
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
	cpu                       resourcePair
	memory                    resourcePair
	ephemeralStorage          resourcePair
	persistentVolumes         resourcePair
	arch                      string
	volumesAttached           resourcePair
	persistentVolumesAttached resourcePair
	volumesMounted            resourcePair
	persistentVolumesMounted  resourcePair
	storageclasses            map[string]bool
}

type NodeStorage struct {
}

type ActiveClusterStorage struct {
}

func (c *client) countKubeCall(err error, name string) {
	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues(name, label).Inc()
}

func (c *client) storageClasses(ctx context.Context) (map[string]resourcePair, error) {
	classes := make(map[string]bool)

	sc, err := c.kc.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", builder.AkashManagedLabelName),
	})
	if err != nil {
		return nil, err
	}

	for _, ksclass := range sc.Items {
		if !isSupportedStorageClass(ksclass.Name) {
			continue
		}

		classes[ksclass.Name] = true
	}

	storageStates, err := c.ac.AkashV1().StorageClassStates().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	storageResources := make(map[string]resourcePair)
	for _, state := range storageStates.Items {
		storageResources[state.Name] = resourcePair{
			allocatable: *resource.NewQuantity(int64(state.Spec.Capacity), resource.DecimalSI),
			allocated:   *resource.NewQuantity(int64(state.Spec.Capacity-state.Spec.Available), resource.DecimalSI),
		}
	}

	for class := range classes {
		if _, exists := storageResources[class]; !exists {
			c.log.Error(fmt.Sprintf("could not find CRD:StorageClassState:%[1]s for class %[1]s", class))
			delete(classes, class)
		}
	}

	for class := range storageResources {
		if _, exists := classes[class]; !exists {
			c.log.Error(fmt.Sprintf("CRD:StorageClassState:%[1]s represents non existing storage class %[1]s", class))
			delete(storageResources, class)
		}
	}

	return storageResources, nil
}

func isSupportedStorageClass(name string) bool {
	switch name {
	case "standard":
		fallthrough
	case "beta1":
		fallthrough
	case "beta2":
		fallthrough
	case "beta3":
		return true
	default:
		return false
	}
}

func (c *client) activeNodes(ctx context.Context) (map[string]nodeResources, error) {
	// todo filter nodes by akash.network label
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
			ephemeralStorage: resourcePair{
				allocatable: storage,
			},
			volumesAttached: resourcePair{
				allocated: *resource.NewQuantity(int64(len(knode.Status.VolumesAttached)), resource.DecimalSI),
			},
			storageclasses: make(map[string]bool),
		}

		if value, defined := knode.Labels[builder.AkashNetworkStorageClasses]; defined {
			for _, class := range strings.Split(value, ",") {
				if isSupportedStorageClass(class) {
					entry.storageclasses[class] = true
				}
			}
		}

		// Initialize the allocated amount to for each node
		zero.DeepCopyInto(&entry.cpu.allocated)
		zero.DeepCopyInto(&entry.memory.allocated)
		zero.DeepCopyInto(&entry.ephemeralStorage.allocated)

		retnodes[knode.Name] = entry
	}

	// Go over each pod and sum the resources for it into the value for the pod it lives on
	err = podsPager.EachListItem(ctx, podListOptions, func(obj runtime.Object) error {
		pod := obj.(*corev1.Pod)
		nodeName := pod.Spec.NodeName

		entry := retnodes[nodeName]

		for _, container := range pod.Spec.Containers {
			entry.addAllocatedResources(container.Resources.Requests)
		}

		// Add overhead for running a pod to the sum of requests
		// https://kubernetes.io/docs/concepts/scheduling-eviction/pod-overhead/
		entry.addAllocatedResources(pod.Spec.Overhead)

		retnodes[nodeName] = entry // Map is by value, so store the copy back into the map
		return nil
	})

	if err != nil {
		return nil, err
	}

	return retnodes, nil
}

func (nr *nodeResources) addAllocatedResources(rl corev1.ResourceList) {
	for name, quantity := range rl {
		switch name {
		case corev1.ResourceCPU:
			nr.cpu.allocated.Add(quantity)
		case corev1.ResourceMemory:
			nr.memory.allocated.Add(quantity)
		case corev1.ResourceEphemeralStorage:
			nr.ephemeralStorage.allocated.Add(quantity)
		}
	}
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
	_, err := c.kc.CoreV1().Namespaces().Get(ctx, builder.LidNS(lid), metav1.GetOptions{})
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

	deployments, err := c.kc.AppsV1().Deployments(builder.LidNS(lid)).List(ctx, metav1.ListOptions{})
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
		c.log.Info("No deployments found for", "lease namespace", builder.LidNS(lid))
		return nil, ErrNoDeploymentForLease
	}

	return deployments.Items, nil
}

package kube

import (
	"context"
	"os"
	"path"

	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	"github.com/ovrclk/akash/provider/cluster/util"

	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
)

var (
	ErrNoDeploymentForLease     = errors.New("kube: no deployments for lease")
	ErrNoGlobalServicesForLease = errors.New("kube: no global services for lease")
	ErrInternalError            = errors.New("kube: internal error")
)

// Client interface includes cluster client
type Client interface {
	cluster.Client
}

var _ Client = (*client)(nil)

type client struct {
	kc       kubernetes.Interface
	ac       akashclient.Interface
	metc     metricsclient.Interface
	ns       string
	settings Settings
	log      log.Logger
}

// NewClient returns new Kubernetes Client instance with provided logger, host and ns. Returns error incase of failure
func NewClient(log log.Logger, ns string, settings Settings) (Client, error) {
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	return newClientWithSettings(log, ns, settings)
}

func newClientWithSettings(log log.Logger, ns string, settings Settings) (Client, error) {
	ctx := context.Background()

	config, err := openKubeConfig(log)
	if err != nil {
		return nil, errors.Wrap(err, "kube: error building config flags")
	}

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
		settings: settings,
		kc:       kc,
		ac:       mc,
		metc:     metc,
		ns:       ns,
		log:      log.With("module", "provider-cluster-kube"),
	}, nil

}

func openKubeConfig(log log.Logger) (*rest.Config, error) {
	cfgpath := path.Join(homedir.HomeDir(), ".kube", "config")

	if _, err := os.Stat(cfgpath); err == nil {
		log.Debug("reading kube config", "path", cfgpath)
		return clientcmd.BuildConfigFromFlags("", cfgpath)
	}

	log.Info("using in-cluster config")
	return rest.InClusterConfig()
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
	if err := applyNS(ctx, c.kc, newNSBuilder(c.settings, lid, group)); err != nil {
		c.log.Error("applying namespace", "err", err, "lease", lid)
		return err
	}

	// TODO: re-enable.  see #946
	// if err := applyRestrictivePodSecPoliciesToNS(ctx, c.kc, newPspBuilder(c.settings, lid, group)); err != nil {
	// 	c.log.Error("applying pod security policies", "err", err, "lease", lid)
	// 	return err
	// }

	if err := applyNetPolicies(ctx, c.kc, newNetPolBuilder(c.settings, lid, group)); err != nil {
		c.log.Error("applying namespace network policies", "err", err, "lease", lid)
		return err
	}

	if err := applyManifest(ctx, c.ac, newManifestBuilder(c.log, c.settings, c.ns, lid, group)); err != nil {
		c.log.Error("applying manifest", "err", err, "lease", lid)
		return err
	}

	if err := cleanupStaleResources(ctx, c.kc, lid, group); err != nil {
		c.log.Error("cleaning stale resources", "err", err, "lease", lid)
		return err
	}

	for svcIdx := range group.Services {
		service := &group.Services[svcIdx]
		if err := applyDeployment(ctx, c.kc, newDeploymentBuilder(c.log, c.settings, lid, group, service)); err != nil {
			c.log.Error("applying deployment", "err", err, "lease", lid, "service", service.Name)
			return err
		}

		if len(service.Expose) == 0 {
			c.log.Debug("no services", "lease", lid, "service", service.Name)
			continue
		}

		serviceBuilderLocal := newServiceBuilder(c.log, c.settings, lid, group, service, false)
		if serviceBuilderLocal.any() {
			if err := applyService(ctx, c.kc, serviceBuilderLocal); err != nil {
				c.log.Error("applying local service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		serviceBuilderGlobal := newServiceBuilder(c.log, c.settings, lid, group, service, true)
		if serviceBuilderGlobal.any() {
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
			if err := applyIngress(ctx, c.kc, newIngressBuilder(c.log, c.settings, lid, group, service, &service.Expose[expIdx])); err != nil {
				c.log.Error("applying ingress", "err", err, "lease", lid, "service", service.Name, "expose", expose)
				return err
			}
		}
	}

	return nil
}

func (c *client) TeardownLease(ctx context.Context, lid mtypes.LeaseID) error {
	return c.kc.CoreV1().Namespaces().Delete(ctx, lidNS(lid), metav1.DeleteOptions{})
}

func (c *client) ServiceLogs(ctx context.Context, lid mtypes.LeaseID,
	_ string, follow bool, tailLines *int64) ([]*ctypes.ServiceLog, error) {
	pods, err := c.kc.CoreV1().Pods(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	streams := make([]*ctypes.ServiceLog, len(pods.Items))
	for i, pod := range pods.Items {
		stream, err := c.kc.CoreV1().Pods(lidNS(lid)).GetLogs(pod.Name, &corev1.PodLogOptions{
			Follow:     follow,
			TailLines:  tailLines,
			Timestamps: false,
		}).Stream(ctx)
		if err != nil {
			c.log.Error(err.Error())
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
		c.log.Error(err.Error())
		return nil, err
	}

	serviceStatus := make(map[string]*ctypes.ServiceStatus, len(deployments))
	forwardedPorts := make(map[string][]ctypes.ForwardedPortStatus, len(deployments))
	for _, deployment := range deployments {
		status := &ctypes.ServiceStatus{
			Name:      deployment.Name,
			Available: deployment.Status.AvailableReplicas,
			Total:     deployment.Status.Replicas,
		}
		serviceStatus[deployment.Name] = status

	}

	ingress, err := c.kc.NetworkingV1().Ingresses(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	services, err := c.kc.CoreV1().Services(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	foundCnt := 0
	for _, ing := range ingress.Items {
		service, found := serviceStatus[ing.Name]
		if !found {
			continue
		}
		hosts := []string{}

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
	deployment, err := c.kc.AppsV1().Deployments(lidNS(lid)).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	if deployment == nil {
		return nil, ErrNoDeploymentForLease
	}
	return &ctypes.ServiceStatus{
		ObservedGeneration: deployment.Status.ObservedGeneration,
		Replicas:           deployment.Status.Replicas,
		UpdatedReplicas:    deployment.Status.UpdatedReplicas,
		ReadyReplicas:      deployment.Status.ReadyReplicas,
		AvailableReplicas:  deployment.Status.AvailableReplicas,
	}, nil
}

func (c *client) Inventory(ctx context.Context) ([]ctypes.Node, error) {
	// Load all the nodes
	knodes, err := c.activeNodes(ctx)
	if err != nil {
		return nil, err
	}

	// Load all the node metrics
	nodeMetrics, err := c.metc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	nodes := make([]ctypes.Node, 0, len(nodeMetrics.Items))
	// Iterate over the node metrics
	for _, mnode := range nodeMetrics.Items {
		// Lookup the node
		knode, ok := knodes[mnode.Name]
		if !ok {
			continue
		}

		// Get the amount of available CPU, then subtract that in use
		cpu := knode.Status.Allocatable.Cpu().MilliValue()
		cpu -= mnode.Usage.Cpu().MilliValue()
		if cpu < 0 {
			cpu = 0
		}

		// Get the amount of memory, then subtract that in use
		memory := knode.Status.Allocatable.Memory().Value()
		memory -= mnode.Usage.Memory().Value()
		if memory < 0 {
			memory = 0
		}

		// Get the amount of storage, then subtract that in use
		storage := knode.Status.Allocatable.StorageEphemeral().Value()
		storage -= mnode.Usage.StorageEphemeral().Value()
		if storage < 0 {
			storage = 0
		}

		resources := types.ResourceUnits{
			CPU: &types.CPU{
				Units: types.NewResourceValue(uint64(cpu)),
				Attributes: []types.Attribute{
					{
						Key:   "arch",
						Value: knode.Status.NodeInfo.Architecture,
						// Value: types.NewAttributeValue(
						// 	[]string{
						// 		knode.Status.NodeInfo.Architecture,
						// 	}),
					},
					// todo (#788) other node attributes ?
				},
			},
			Memory: &types.Memory{
				Quantity: types.NewResourceValue(uint64(memory)),
				// todo (#788) memory attributes ?
			},
			Storage: &types.Storage{
				Quantity: types.NewResourceValue(uint64(storage)),
				// todo (#788) storage attributes like class and iops?
			},
		}

		nodes = append(nodes, cluster.NewNode(knode.Name, resources))
	}

	return nodes, nil
}

func (c *client) activeNodes(ctx context.Context) (map[string]*corev1.Node, error) {
	knodes, err := c.kc.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	retnodes := make(map[string]*corev1.Node)

	for i := range knodes.Items {
		knode := &knodes.Items[i]
		if !c.nodeIsActive(knode) {
			continue
		}
		retnodes[knode.Name] = knode
	}

	return retnodes, nil
}

func (c *client) nodeIsActive(node *corev1.Node) bool {
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

func (c *client) deploymentsForLease(ctx context.Context, lid mtypes.LeaseID) ([]appsv1.Deployment, error) {
	deployments, err := c.kc.AppsV1().Deployments(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}

	if deployments == nil || 0 == len(deployments.Items) {
		return nil, ErrNoDeploymentForLease
	}
	return deployments.Items, nil
}

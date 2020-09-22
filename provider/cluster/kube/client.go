package kube

import (
	"context"
	"os"
	"path"

	"github.com/caarlos0/env"
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
	"github.com/ovrclk/akash/validation"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

var (
	ErrNoDeploymentForLease = errors.New("kube: no deployments for lease")
	ErrNoIngressForLease    = errors.New("kube: no ingress for lease")
	ErrInternalError        = errors.New("kube: internal error")
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
	host     string
	settings settings
	log      log.Logger
}

// NewClient returns new Kubernetes Client instance with provided logger, host and ns. Returns error incase of failure
func NewClient(log log.Logger, host, ns string) (Client, error) {
	var settings settings
	if err := env.Parse(&settings); err != nil {
		return nil, err
	}
	if err := validateSettings(settings); err != nil {
		return nil, err
	}
	return newClientWithSettings(log, host, ns, settings)
}

func newClientWithSettings(log log.Logger, host, ns string, settings settings) (Client, error) {
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
		host:     host,
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

func (c *client) shouldExpose(expose *manifest.ServiceExpose) bool {
	return expose.Global &&
		(expose.ExternalPort == 80 ||
			(expose.ExternalPort == 0 && expose.Port == 80))
}

func (c *client) Deployments(ctx context.Context) ([]cluster.Deployment, error) {
	manifests, err := c.ac.AkashV1().Manifests(c.ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	deployments := make([]cluster.Deployment, 0, len(manifests.Items))
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

		if err := applyService(ctx, c.kc, newServiceBuilder(c.log, c.settings, lid, group, service)); err != nil {
			c.log.Error("applying service", "err", err, "lease", lid, "service", service.Name)
			return err
		}

		for expIdx := range service.Expose {
			expose := &service.Expose[expIdx]
			if !c.shouldExpose(expose) {
				continue
			}
			if err := applyIngress(ctx, c.kc, newIngressBuilder(c.log, c.settings, c.host, lid, group, service, expose)); err != nil {
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
	_ string, follow bool, tailLines *int64) ([]*cluster.ServiceLog, error) {
	pods, err := c.kc.CoreV1().Pods(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	streams := make([]*cluster.ServiceLog, len(pods.Items))
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
func (c *client) LeaseStatus(ctx context.Context, lid mtypes.LeaseID) (*cluster.LeaseStatus, error) {
	deployments, err := c.deploymentsForLease(ctx, lid)
	if err != nil {
		c.log.Error(err.Error())
		return nil, err
	}
	if len(deployments) == 0 {
		return nil, cluster.ErrNoDeployments
	}
	serviceStatus := make(map[string]*cluster.ServiceStatus, len(deployments))
	for _, deployment := range deployments {
		status := &cluster.ServiceStatus{
			Name:      deployment.Name,
			Available: deployment.Status.AvailableReplicas,
			Total:     deployment.Status.Replicas,
		}
		serviceStatus[deployment.Name] = status
	}
	ingress, err := c.kc.ExtensionsV1beta1().Ingresses(lidNS(lid)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	if ingress == nil || len(ingress.Items) == 0 {
		return nil, ErrNoIngressForLease
	}
	for _, ing := range ingress.Items {
		service := serviceStatus[ing.Name]
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
	}
	response := &cluster.LeaseStatus{}
	for _, status := range serviceStatus {
		response.Services = append(response.Services, status)
	}
	return response, nil
}

func (c *client) ServiceStatus(ctx context.Context, lid mtypes.LeaseID, name string) (*cluster.ServiceStatus, error) {
	deployment, err := c.kc.AppsV1().Deployments(lidNS(lid)).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		c.log.Error(err.Error())
		return nil, errors.Wrap(err, ErrInternalError.Error())
	}
	if deployment == nil {
		return nil, ErrNoDeploymentForLease
	}
	return &cluster.ServiceStatus{
		ObservedGeneration: deployment.Status.ObservedGeneration,
		Replicas:           deployment.Status.Replicas,
		UpdatedReplicas:    deployment.Status.UpdatedReplicas,
		ReadyReplicas:      deployment.Status.ReadyReplicas,
		AvailableReplicas:  deployment.Status.AvailableReplicas,
	}, nil
}

func (c *client) Inventory(ctx context.Context) ([]cluster.Node, error) {
	knodes, err := c.activeNodes(ctx)
	if err != nil {
		return nil, err
	}

	nodeMetrics, err := c.metc.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	nodes := make([]cluster.Node, 0, len(nodeMetrics.Items))
	for _, mnode := range nodeMetrics.Items {
		knode, ok := knodes[mnode.Name]
		if !ok {
			continue
		}

		cpu := knode.Status.Allocatable.Cpu().MilliValue()
		cpu -= mnode.Usage.Cpu().MilliValue()
		if cpu < 0 {
			cpu = 0
		}

		memory := knode.Status.Allocatable.Memory().Value()
		memory -= mnode.Usage.Memory().Value()
		if memory < 0 {
			memory = 0
		}

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

	if os.Getenv("AKASH_PROVIDER_FAKE_CAPACITY") == "true" {
		cfg := validation.Config()
		return []cluster.Node{
			cluster.NewNode("minikube", types.ResourceUnits{
				CPU: &types.CPU{
					Units: types.NewResourceValue(uint64(cfg.MaxUnitCPU * 100)),
				},
				Memory: &types.Memory{
					Quantity: types.NewResourceValue(uint64(cfg.MaxUnitMemory * 100)),
				},
				Storage: &types.Storage{
					Quantity: types.NewResourceValue(uint64(cfg.MaxUnitStorage * 100)),
				},
			}),
		}, nil
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
	if deployments == nil {
		return nil, ErrNoDeploymentForLease
	}
	return deployments.Items, nil
}

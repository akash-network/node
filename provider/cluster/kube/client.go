package kube

import (
	"context"
	"fmt"
	"os"
	"path"

	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	manifestclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/types"
	"github.com/tendermint/tmlibs/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextcs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client interface {
	cluster.Client
}

type client struct {
	kc   kubernetes.Interface
	mc   *manifestclient.Clientset
	ns   string
	host string
	log  log.Logger
}

func NewClient(log log.Logger, host, ns string) (Client, error) {

	config, err := openKubeConfig(log)
	if err != nil {
		return nil, fmt.Errorf("error building config flags: %v", err)
	}

	kc, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating kubernetes client: %v", err)
	}

	mc, err := manifestclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating manifest client: %v", err)
	}

	mcr, err := apiextcs.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating apiextcs client: %v", err)
	}

	err = akashv1.CreateCRD(mcr)
	if err != nil {
		return nil, fmt.Errorf("error creating akashv1 CRD: %v", err)
	}

	err = prepareEnvironment(kc, ns)
	if err != nil {
		return nil, fmt.Errorf("error preparing environment %v", err)
	}

	_, err = kc.CoreV1().Namespaces().List(metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, fmt.Errorf("error connecting to kubernetes: %v", err)
	}

	return &client{
		kc:   kc,
		mc:   mc,
		ns:   ns,
		host: host,
		log:  log,
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

func (c *client) shouldExpose(expose *types.ManifestServiceExpose) bool {
	return expose.Global &&
		(expose.ExternalPort == 80 ||
			(expose.ExternalPort == 0 && expose.Port == 80))
}

func (c *client) Deployments() ([]cluster.Deployment, error) {

	manifests, err := c.mc.AkashV1().Manifests(c.ns).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var deployments []cluster.Deployment

	for _, manifest := range manifests.Items {
		deployments = append(deployments, manifest)
	}

	return deployments, nil
}

func (c *client) Deploy(lid types.LeaseID, group *types.ManifestGroup) error {
	if err := applyNS(c.kc, newNSBuilder(lid, group)); err != nil {
		c.log.Error("applying namespace", "err", err, "lease", lid)
		return err
	}

	if err := applyManifest(c.mc, newManifestBuilder(c.ns, lid, group)); err != nil {
		c.log.Error("applying manifest", "err", err, "lease", lid)
		return err
	}

	for _, service := range group.Services {
		if err := applyDeployment(c.kc, newDeploymentBuilder(lid, group, service)); err != nil {
			c.log.Error("applying deployment", "err", err, "lease", lid, "service", service.Name)
			return err
		}

		if len(service.Expose) == 0 {
			c.log.Debug("no services", "lease", lid, "service", service.Name)
			continue
		}

		if err := applyService(c.kc, newServiceBuilder(lid, group, service)); err != nil {
			c.log.Error("applying service", "err", err, "lease", lid, "service", service.Name)
			return err
		}

		for _, expose := range service.Expose {
			if !c.shouldExpose(expose) {
				continue
			}
			if err := applyIngress(c.kc, newIngressBuilder(c.host, lid, group, service, expose)); err != nil {
				c.log.Error("applying ingress", "err", err, "lease", lid, "service", service.Name, "expose", expose)
				return err
			}
		}
	}

	return nil
}

func (c *client) TeardownLease(lid types.LeaseID) error {
	return c.kc.CoreV1().Namespaces().Delete(lidNS(lid), &metav1.DeleteOptions{})
}

func (c *client) ServiceLogs(ctx context.Context, lid types.LeaseID,
	tailLines int64, follow bool) ([]*cluster.ServiceLog, error) {
	pods, err := c.kc.CoreV1().Pods(lidNS(lid)).List(metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	streams := make([]*cluster.ServiceLog, len(pods.Items))
	for i, pod := range pods.Items {
		stream, err := c.kc.CoreV1().Pods(lidNS(lid)).GetLogs(pod.Name, &corev1.PodLogOptions{
			Follow:     follow,
			TailLines:  &tailLines,
			Timestamps: true,
		}).Context(ctx).Stream()
		if err != nil {
			c.log.Error(err.Error())
			return nil, types.ErrInternalError{Message: "internal error"}
		}
		streams[i] = cluster.NewServiceLog(pod.Name, stream)
	}
	return streams, nil
}

// todo: limit number of results and do pagination / streaming
func (c *client) LeaseStatus(lid types.LeaseID) (*types.LeaseStatusResponse, error) {
	deployments, err := c.deploymentsForLease(lid)
	if err != nil {
		c.log.Error(err.Error())
		return nil, err
	}
	if len(deployments) == 0 {
		return nil, cluster.ErrNoDeployments
	}
	serviceStatus := make(map[string]*types.ServiceStatus, len(deployments))
	for _, deployment := range deployments {
		status := &types.ServiceStatus{
			Name:      deployment.Name,
			Available: deployment.Status.AvailableReplicas,
			Total:     deployment.Status.Replicas,
		}
		serviceStatus[deployment.Name] = status
	}
	ingress, err := c.kc.ExtensionsV1beta1().Ingresses(lidNS(lid)).List(metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if ingress == nil || len(ingress.Items) == 0 {
		return nil, types.ErrResourceNotFound{Message: "no ingress for lease"}
	}
	for _, ing := range ingress.Items {
		service := serviceStatus[ing.Name]
		hosts := []string{}
		for _, rule := range ing.Spec.Rules {
			hosts = append(hosts, rule.Host)
		}
		service.URIs = hosts
	}
	response := &types.LeaseStatusResponse{}
	for _, status := range serviceStatus {
		response.Services = append(response.Services, status)
	}
	return response, nil
}

func (c *client) ServiceStatus(lid types.LeaseID, name string) (*types.ServiceStatusResponse, error) {
	deployment, err := c.kc.AppsV1().Deployments(lidNS(lid)).Get(name, metav1.GetOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if deployment == nil {
		return nil, types.ErrResourceNotFound{Message: "no deployment for lease"}
	}
	return &types.ServiceStatusResponse{
		ObservedGeneration: deployment.Status.ObservedGeneration,
		Replicas:           deployment.Status.Replicas,
		UpdatedReplicas:    deployment.Status.UpdatedReplicas,
		ReadyReplicas:      deployment.Status.ReadyReplicas,
		AvailableReplicas:  deployment.Status.AvailableReplicas,
	}, nil
}

func (c *client) Inventory() ([]cluster.Node, error) {
	var nodes []cluster.Node

	knodes, err := c.kc.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, knode := range knodes.Items {
		if !c.nodeIsActive(&knode) {
			continue
		}

		unit := types.ResourceUnit{
			CPU:    uint32(knode.Status.Allocatable.Cpu().Value()),
			Memory: uint64(knode.Status.Capacity.Memory().Value()),
			Disk:   uint64(knode.Status.Capacity.StorageEphemeral().Value()),
		}

		nodes = append(nodes, cluster.NewNode(knode.Name, unit))
	}

	return nodes, nil
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

		case corev1.NodeOutOfDisk:
			fallthrough
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

		case corev1.NodeKubeletConfigOk:
			// ignored
		}
	}

	return ready && issues == 0
}

func (c *client) deploymentsForLease(lid types.LeaseID) ([]appsv1.Deployment, error) {
	deployments, err := c.kc.AppsV1().Deployments(lidNS(lid)).List(metav1.ListOptions{})
	if err != nil {
		c.log.Error(err.Error())
		return nil, types.ErrInternalError{Message: "internal error"}
	}
	if deployments == nil {
		return nil, types.ErrResourceNotFound{Message: "no deployments for lease"}
	}
	return deployments.Items, nil
}

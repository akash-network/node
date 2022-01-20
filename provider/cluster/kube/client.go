package kube

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"

	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	crd "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	akashclient "github.com/ovrclk/akash/pkg/client/clientset/versioned"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	"github.com/ovrclk/akash/provider/cluster/util"
	"github.com/ovrclk/akash/sdl"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	"github.com/ovrclk/akash/provider/cluster/kube/clientcommon"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	kubeclienterrors "github.com/ovrclk/akash/provider/cluster/kube/errors"
)

var (
	ErrLeaseNotFound             = errors.New("kube: lease not found")
	ErrNoDeploymentForLease      = errors.New("kube: no deployments for lease")
	ErrNoManifestForLease        = errors.New("kube: no manifest for lease")
	ErrNoServiceForLease         = errors.New("no service for that lease")
	ErrInvalidHostnameConnection = errors.New("kube: invalid hostname connection")
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

func (c *client) String() string {
	return fmt.Sprintf("kube client %p ns=%s", c, c.ns)
}

// NewClient returns new Kubernetes Client instance with provided logger, host and ns. Returns error incase of failure
// configPath may be the empty string
func NewClient(log log.Logger, ns string, configPath string) (Client, error) {
	config, err := clientcommon.OpenKubeConfig(configPath, log)
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

	return &client{
		kc:                kc,
		ac:                mc,
		metc:              metc,
		ns:                ns,
		log:               log.With("client", "kube"),
		kubeContentConfig: config,
	}, nil
}

func (c *client) GetDeployments(ctx context.Context, dID dtypes.DeploymentID) ([]ctypes.Deployment, error) {
	labelSelectors := &strings.Builder{}
	_, _ = fmt.Fprintf(labelSelectors, "%s=%d", builder.AkashLeaseDSeqLabelName, dID.DSeq)
	_, _ = fmt.Fprintf(labelSelectors, ",%s=%s", builder.AkashLeaseOwnerLabelName, dID.Owner)

	manifests, err := c.ac.AkashV2beta1().Manifests(c.ns).List(ctx, metav1.ListOptions{
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

func (c *client) GetManifestGroup(ctx context.Context, lID mtypes.LeaseID) (bool, crd.ManifestGroup, error) {
	leaseNamespace := builder.LidNS(lID)

	obj, err := c.ac.AkashV2beta1().Manifests(c.ns).Get(ctx, leaseNamespace, metav1.GetOptions{})
	if err != nil {
		if kubeErrors.IsNotFound(err) {
			c.log.Info("CRD manifest not found", "lease-ns", leaseNamespace)
			return false, crd.ManifestGroup{}, nil
		}

		return false, crd.ManifestGroup{}, err
	}

	return true, obj.Spec.Group, nil
}

func (c *client) Deployments(ctx context.Context) ([]ctypes.Deployment, error) {
	manifests, err := c.ac.AkashV2beta1().Manifests(c.ns).List(ctx, metav1.ListOptions{})
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
	settingsI := ctx.Value(builder.SettingsKey)
	if nil == settingsI {
		return errNotConfiguredWithSettings
	}
	settings := settingsI.(builder.Settings)
	if err := builder.ValidateSettings(settings); err != nil {
		return err
	}

	if err := applyNS(ctx, c.kc, builder.BuildNS(settings, lid, group)); err != nil {
		c.log.Error("applying namespace", "err", err, "lease", lid)
		return err
	}

	if err := applyNetPolicies(ctx, c.kc, builder.BuildNetPol(settings, lid, group)); err != nil { //
		c.log.Error("applying namespace network policies", "err", err, "lease", lid)
		return err
	}

	if err := applyManifest(ctx, c.ac, builder.BuildManifest(c.log, settings, c.ns, lid, group)); err != nil {
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
			attrVal := service.Resources.Storage[i].Attributes.Find(sdl.StorageAttributePersistent)
			if persistent, _ = attrVal.AsBool(); persistent {
				break
			}
		}

		if persistent {
			if err := applyStatefulSet(ctx, c.kc, builder.BuildStatefulSet(c.log, settings, lid, group, service)); err != nil {
				c.log.Error("applying statefulSet", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		} else {
			if err := applyDeployment(ctx, c.kc, builder.NewDeployment(c.log, settings, lid, group, service)); err != nil {
				c.log.Error("applying deployment", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		if len(service.Expose) == 0 {
			c.log.Debug("no services", "lease", lid, "service", service.Name)
			continue
		}

		serviceBuilderLocal := builder.BuildService(c.log, settings, lid, group, service, false)
		if serviceBuilderLocal.Any() {
			if err := applyService(ctx, c.kc, serviceBuilderLocal); err != nil {
				c.log.Error("applying local service", "err", err, "lease", lid, "service", service.Name)
				return err
			}
		}

		serviceBuilderGlobal := builder.BuildService(c.log, settings, lid, group, service, true)
		if serviceBuilderGlobal.Any() {
			if err := applyService(ctx, c.kc, serviceBuilderGlobal); err != nil {
				c.log.Error("applying global service", "err", err, "lease", lid, "service", service.Name)
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

	err := c.ac.AkashV2beta1().Manifests(c.ns).Delete(ctx, builder.LidNS(lid), metav1.DeleteOptions{})
	if err != nil {
		c.log.Error("teardown lease: unable to delete manifest", "ns", builder.LidNS(lid), "error", err)
	}

	return result
}

func kubeSelectorForLease(dst *strings.Builder, lID mtypes.LeaseID) {
	_, _ = fmt.Fprintf(dst, "%s=%s", builder.AkashLeaseOwnerLabelName, lID.Owner)
	_, _ = fmt.Fprintf(dst, ",%s=%d", builder.AkashLeaseDSeqLabelName, lID.DSeq)
	_, _ = fmt.Fprintf(dst, ",%s=%d", builder.AkashLeaseGSeqLabelName, lID.GSeq)
	_, _ = fmt.Fprintf(dst, ",%s=%d", builder.AkashLeaseOSeqLabelName, lID.OSeq)
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
		return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
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
			return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
		}
		streams[i] = cluster.NewServiceLog(pod.Name, stream)
	}
	return streams, nil
}

func (c *client) ForwardedPortStatus(ctx context.Context, leaseID mtypes.LeaseID) (map[string][]ctypes.ForwardedPortStatus, error) {
	settingsI := ctx.Value(builder.SettingsKey)
	if nil == settingsI {
		return nil, errNotConfiguredWithSettings
	}
	settings := settingsI.(builder.Settings)
	if err := builder.ValidateSettings(settings); err != nil {
		return nil, err
	}

	services, err := c.kc.CoreV1().Services(builder.LidNS(leaseID)).List(ctx, metav1.ListOptions{})

	label := metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("services-list", label).Inc()
	if err != nil {
		c.log.Error("list services", "err", err)
		return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
	}

	forwardedPorts := make(map[string][]ctypes.ForwardedPortStatus)

	// Search for a Kubernetes service declared as nodeport
	for _, service := range services.Items {
		if service.Spec.Type == corev1.ServiceTypeNodePort {
			serviceName := service.Name // Always suffixed during creation, so chop it off
			deploymentName := serviceName[0 : len(serviceName)-len(builder.SuffixForNodePortServiceName)]

			if 0 != len(service.Spec.Ports) {
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

	return forwardedPorts, nil
}

// todo: limit number of results and do pagination / streaming
func (c *client) LeaseStatus(ctx context.Context, lid mtypes.LeaseID) (map[string]*ctypes.ServiceStatus, error) {
	settingsI := ctx.Value(builder.SettingsKey)
	if nil == settingsI {
		return nil, errNotConfiguredWithSettings
	}
	settings := settingsI.(builder.Settings)
	if err := builder.ValidateSettings(settings); err != nil {
		return nil, err
	}

	serviceStatus, err := c.deploymentsForLease(ctx, lid)
	if err != nil {
		return nil, err
	}
	labelSelector := &strings.Builder{}
	kubeSelectorForLease(labelSelector, lid)
	// Note: this is a separate call to the Kubernetes API to get this data. It could
	// be a separate method on the interface entirely
	phResult, err := c.ac.AkashV2beta1().ProviderHosts(c.ns).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})

	if err != nil {
		return nil, err
	}

	// For each provider host entry, update the status of each service to indicate
	// the presently assigned hostnames
	for _, ph := range phResult.Items {
		entry, ok := serviceStatus[ph.Spec.ServiceName]
		if ok {
			entry.URIs = append(entry.URIs, ph.Spec.Hostname)
		}
	}

	return serviceStatus, nil
}

func (c *client) ServiceStatus(ctx context.Context, lid mtypes.LeaseID, name string) (*ctypes.ServiceStatus, error) {
	if err := c.leaseExists(ctx, lid); err != nil {
		return nil, err
	}

	// Get manifest definition from CRD
	c.log.Debug("Pulling manifest from CRD", "lease-ns", builder.LidNS(lid))
	mani, err := c.ac.AkashV2beta1().Manifests(c.ns).Get(ctx, builder.LidNS(lid), metav1.GetOptions{})
	if err != nil {
		c.log.Error("CRD manifest not found", "lease-ns", builder.LidNS(lid), "name", name)
		return nil, ErrNoManifestForLease
	}

	var result *ctypes.ServiceStatus
	isDeployment := true

	for _, svc := range mani.Spec.Group.Services {
		if svc.Name == name {
			if params := svc.Params; params != nil {
				for _, param := range params.Storage {
					if param.Mount != "" {
						isDeployment = false
					}
				}
			}

			break
		}
	}

	if isDeployment {
		c.log.Debug("get deployment", "lease-ns", builder.LidNS(lid), "name", name)
		deployment, err := c.kc.AppsV1().Deployments(builder.LidNS(lid)).Get(ctx, name, metav1.GetOptions{})
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("deployments-get", label).Inc()

		if err != nil {
			c.log.Error("deployment get", "err", err)
			return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
		}
		if deployment == nil {
			c.log.Error("no deployment found", "name", name)
			return nil, ErrNoDeploymentForLease
		}

		result = &ctypes.ServiceStatus{
			Name:               deployment.Name,
			Available:          deployment.Status.AvailableReplicas,
			Total:              deployment.Status.Replicas,
			ObservedGeneration: deployment.Status.ObservedGeneration,
			Replicas:           deployment.Status.Replicas,
			UpdatedReplicas:    deployment.Status.UpdatedReplicas,
			ReadyReplicas:      deployment.Status.ReadyReplicas,
			AvailableReplicas:  deployment.Status.AvailableReplicas,
		}
	} else {
		c.log.Debug("get statefulsets", "lease-ns", builder.LidNS(lid), "name", name)
		statefulset, err := c.kc.AppsV1().StatefulSets(builder.LidNS(lid)).Get(ctx, name, metav1.GetOptions{})
		label := metricsutils.SuccessLabel
		if err != nil {
			label = metricsutils.FailLabel
		}
		kubeCallsCounter.WithLabelValues("statefulsets-get", label).Inc()

		if err != nil {
			c.log.Error("statefulsets get", "err", err)
			return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
		}
		if statefulset == nil {
			c.log.Error("no statefulsets found", "name", name)
			return nil, ErrNoDeploymentForLease
		}

		result = &ctypes.ServiceStatus{
			Name:               statefulset.Name,
			Available:          statefulset.Status.CurrentReplicas,
			Total:              statefulset.Status.Replicas,
			ObservedGeneration: statefulset.Status.ObservedGeneration,
			Replicas:           statefulset.Status.Replicas,
			UpdatedReplicas:    statefulset.Status.UpdatedReplicas,
			ReadyReplicas:      statefulset.Status.ReadyReplicas,
			AvailableReplicas:  statefulset.Status.CurrentReplicas,
		}
	}

	hasHostnames := false

	found := false
exposeCheckLoop:
	for _, service := range mani.Spec.Group.Services {
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

	c.log.Debug("service result", "lease-ns", builder.LidNS(lid), "has-hostnames", hasHostnames)

	if hasHostnames {
		labelSelector := &strings.Builder{}
		kubeSelectorForLease(labelSelector, lid)

		phs, err := c.ac.AkashV2beta1().ProviderHosts(c.ns).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector.String(),
		})

		if hasHostnames {
			label := metricsutils.SuccessLabel
			if err != nil {
				label = metricsutils.FailLabel
			}
			kubeCallsCounter.WithLabelValues("provider-hosts", label).Inc()
			if err != nil {
				c.log.Error("provider hosts get", "err", err)
				return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
			}

			hosts := make([]string, 0, len(phs.Items))
			for _, ph := range phs.Items {
				hosts = append(hosts, ph.Spec.Hostname)
			}

			result.URIs = hosts
		}
	}

	return result, nil
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
		return errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
	}

	return nil
}

func (c *client) deploymentsForLease(ctx context.Context, lid mtypes.LeaseID) (map[string]*ctypes.ServiceStatus, error) {
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
		return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
	}

	statefulsets, err := c.kc.AppsV1().StatefulSets(builder.LidNS(lid)).List(ctx, metav1.ListOptions{})
	label = metricsutils.SuccessLabel
	if err != nil {
		label = metricsutils.FailLabel
	}
	kubeCallsCounter.WithLabelValues("statefulsets-list", label).Inc()
	if err != nil {
		c.log.Error("statefulsets list", "err", err)
		return nil, errors.Wrap(err, kubeclienterrors.ErrInternalError.Error())
	}

	serviceStatus := make(map[string]*ctypes.ServiceStatus)

	if deployments != nil {
		for _, deployment := range deployments.Items {
			serviceStatus[deployment.Name] = &ctypes.ServiceStatus{
				Name:               deployment.Name,
				Available:          deployment.Status.AvailableReplicas,
				Total:              deployment.Status.Replicas,
				ObservedGeneration: deployment.Status.ObservedGeneration,
				Replicas:           deployment.Status.Replicas,
				UpdatedReplicas:    deployment.Status.UpdatedReplicas,
				ReadyReplicas:      deployment.Status.ReadyReplicas,
				AvailableReplicas:  deployment.Status.AvailableReplicas,
			}
		}
	}

	if statefulsets != nil {
		for _, statefulset := range statefulsets.Items {
			serviceStatus[statefulset.Name] = &ctypes.ServiceStatus{
				Name:               statefulset.Name,
				Available:          statefulset.Status.CurrentReplicas,
				Total:              statefulset.Status.Replicas,
				ObservedGeneration: statefulset.Status.ObservedGeneration,
				Replicas:           statefulset.Status.Replicas,
				UpdatedReplicas:    statefulset.Status.UpdatedReplicas,
				ReadyReplicas:      statefulset.Status.ReadyReplicas,
				AvailableReplicas:  statefulset.Status.CurrentReplicas,
			}
		}
	}

	if len(serviceStatus) == 0 {
		c.log.Info("No deployments found for", "lease namespace", builder.LidNS(lid))
		return nil, ErrNoDeploymentForLease
	}

	return serviceStatus, nil
}

func (c *client) KubeVersion() (*version.Info, error) {
	return c.kc.Discovery().ServerVersion()
}

package kube

// nolint:deadcode,golint

import (
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ovrclk/akash/provider/cluster/util"
	uuid "github.com/satori/go.uuid"

	"github.com/tendermint/tendermint/libs/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"

	// TODO: re-enable.  see #946
	// "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/ovrclk/akash/manifest"
	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	clusterUtil "github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	akashManagedLabelName         = "akash.network"
	akashNetworkNamespace         = "akash.network/namespace"
	akashManifestServiceLabelName = "akash.network/manifest-service"

	akashLeaseOwnerLabelName    = "akash.network/lease.id.owner"
	akashLeaseDSeqLabelName     = "akash.network/lease.id.dseq"
	akashLeaseGSeqLabelName     = "akash.network/lease.id.gseq"
	akashLeaseOSeqLabelName     = "akash.network/lease.id.oseq"
	akashLeaseProviderLabelName = "akash.network/lease.id.provider"

	netPolDefaultDenyIngress = "default-deny-ingress"
	netPolDefaultDenyEgress  = "default-deny-egress"

	netPolIngressInternalAllow = "ingress-allow-internal"
	netPolIngressAllowIngCtrl  = "ingress-allow-controller"

	netPolEgressInternalAllow     = "egress-allow-internal"
	netPolEgressAllowExternalCidr = "egress-allow-cidr"
	netPolEgressAllowKubeDNS      = "egress-allow-kube-dns"
)

var (
	dnsPort     = intstr.FromInt(53)
	dnsProtocol = corev1.Protocol("UDP")
)

type builder struct {
	log      log.Logger
	settings Settings
	lid      mtypes.LeaseID
	group    *manifest.Group
}

func (b *builder) ns() string {
	return lidNS(b.lid)
}

func (b *builder) labels() map[string]string {
	return map[string]string{
		akashManagedLabelName: "true",
		akashNetworkNamespace: lidNS(b.lid),
	}
}

type nsBuilder struct {
	builder
}

func newNSBuilder(settings Settings, lid mtypes.LeaseID, group *manifest.Group) *nsBuilder {
	return &nsBuilder{builder: builder{settings: settings, lid: lid, group: group}}
}

func (b *nsBuilder) name() string {
	return b.ns()
}

func (b *nsBuilder) labels() map[string]string {
	return appendLeaseLabels(b.lid, b.builder.labels())
}

func (b *nsBuilder) create() (*corev1.Namespace, error) { // nolint:golint,unparam
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.ns(),
			Labels: b.labels(),
		},
	}, nil
}

func (b *nsBuilder) update(obj *corev1.Namespace) (*corev1.Namespace, error) { // nolint:golint,unparam
	obj.Name = b.ns()
	obj.Labels = b.labels()
	return obj, nil
}

// TODO: re-enable.  see #946
// pspRestrictedBuilder produces restrictive PodSecurityPolicies for tenant Namespaces.
// Restricted PSP source: https://raw.githubusercontent.com/kubernetes/website/master/content/en/examples/policy/restricted-psp.yaml
// type pspRestrictedBuilder struct {
// 	builder
// }
//
// func newPspBuilder(settings Settings, lid mtypes.LeaseID, group *manifest.Group) *pspRestrictedBuilder { // nolint:golint,unparam
// 	return &pspRestrictedBuilder{builder: builder{settings: settings, lid: lid, group: group}}
// }
//
// func (p *pspRestrictedBuilder) name() string {
// 	return p.ns()
// }
//
// func (p *pspRestrictedBuilder) create() (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
// 	falseVal := false
// 	return &v1beta1.PodSecurityPolicy{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      p.name(),
// 			Namespace: p.name(),
// 			Labels:    p.labels(),
// 			Annotations: map[string]string{
// 				"seccomp.security.alpha.kubernetes.io/allowedProfileNames": "docker/default,runtime/default",
// 				"apparmor.security.beta.kubernetes.io/allowedProfileNames": "runtime/default",
// 				"seccomp.security.alpha.kubernetes.io/defaultProfileName":  "runtime/default",
// 				"apparmor.security.beta.kubernetes.io/defaultProfileName":  "runtime/default",
// 			},
// 		},
// 		Spec: v1beta1.PodSecurityPolicySpec{
// 			Privileged:               false,
// 			AllowPrivilegeEscalation: &falseVal,
// 			RequiredDropCapabilities: []corev1.Capability{
// 				"ALL",
// 			},
// 			Volumes: []v1beta1.FSType{
// 				v1beta1.EmptyDir,
// 				v1beta1.PersistentVolumeClaim, // evaluate necessity later
// 			},
// 			HostNetwork: false,
// 			HostIPC:     false,
// 			HostPID:     false,
// 			RunAsUser: v1beta1.RunAsUserStrategyOptions{
// 				// fixme(#946): previous value RunAsUserStrategyMustRunAsNonRoot was interfering with
// 				// (b *deploymentBuilder) create() RunAsNonRoot: false
// 				// allow any user at this moment till revise all security debris of kube api
// 				Rule: v1beta1.RunAsUserStrategyRunAsAny,
// 			},
// 			SELinux: v1beta1.SELinuxStrategyOptions{
// 				Rule: v1beta1.SELinuxStrategyRunAsAny,
// 			},
// 			SupplementalGroups: v1beta1.SupplementalGroupsStrategyOptions{
// 				Rule: v1beta1.SupplementalGroupsStrategyRunAsAny,
// 			},
// 			FSGroup: v1beta1.FSGroupStrategyOptions{
// 				Rule: v1beta1.FSGroupStrategyMustRunAs,
// 				Ranges: []v1beta1.IDRange{
// 					{
// 						Min: int64(1),
// 						Max: int64(65535),
// 					},
// 				},
// 			},
// 			ReadOnlyRootFilesystem: false,
// 		},
// 	}, nil
// }
//
// func (p *pspRestrictedBuilder) update(obj *v1beta1.PodSecurityPolicy) (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
// 	obj.Name = p.ns()
// 	obj.Labels = p.labels()
// 	return obj, nil
// }

// deployment
type deploymentBuilder struct {
	builder
	service *manifest.Service
}

func newDeploymentBuilder(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service) *deploymentBuilder {
	return &deploymentBuilder{
		builder: builder{
			settings: settings,
			log:      log.With("module", "kube-builder"),
			lid:      lid,
			group:    group,
		},
		service: service,
	}
}

func (b *deploymentBuilder) name() string {
	return b.service.Name
}

func (b *deploymentBuilder) labels() map[string]string {
	obj := b.builder.labels()
	obj[akashManifestServiceLabelName] = b.service.Name
	return obj
}

func (b *deploymentBuilder) create() (*appsv1.Deployment, error) { // nolint:golint,unparam
	replicas := int32(b.service.Count)
	falseValue := false

	kdeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: b.labels(),
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: b.labels(),
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &falseValue,
					},
					AutomountServiceAccountToken: &falseValue,
					Containers:                   []corev1.Container{b.container()},
				},
			},
		},
	}

	return kdeployment, nil
}

func (b *deploymentBuilder) update(obj *appsv1.Deployment) (*appsv1.Deployment, error) { // nolint:golint,unparam
	replicas := int32(b.service.Count)
	obj.Labels = b.labels()
	obj.Spec.Selector.MatchLabels = b.labels()
	obj.Spec.Replicas = &replicas
	obj.Spec.Template.Labels = b.labels()
	obj.Spec.Template.Spec.Containers = []corev1.Container{b.container()}
	return obj, nil
}

func (b *deploymentBuilder) container() corev1.Container {
	falseValue := false

	kcontainer := corev1.Container{
		Name:    b.service.Name,
		Image:   b.service.Image,
		Command: b.service.Command,
		Args:    b.service.Args,
		Resources: corev1.ResourceRequirements{
			Limits:   make(corev1.ResourceList),
			Requests: make(corev1.ResourceList),
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &falseValue,
			Privileged:               &falseValue,
			AllowPrivilegeEscalation: &falseValue,
		},
	}

	if cpu := b.service.Resources.CPU; cpu != nil {
		requestedCPU := clusterUtil.ComputeCommittedResources(b.settings.CPUCommitLevel, cpu.Units)
		kcontainer.Resources.Requests[corev1.ResourceCPU] = resource.NewScaledQuantity(int64(requestedCPU.Value()), resource.Milli).DeepCopy()
		kcontainer.Resources.Limits[corev1.ResourceCPU] = resource.NewScaledQuantity(int64(cpu.Units.Value()), resource.Milli).DeepCopy()
	}

	if mem := b.service.Resources.Memory; mem != nil {
		requestedMem := clusterUtil.ComputeCommittedResources(b.settings.MemoryCommitLevel, mem.Quantity)
		kcontainer.Resources.Requests[corev1.ResourceMemory] = resource.NewQuantity(int64(requestedMem.Value()), resource.DecimalSI).DeepCopy()
		kcontainer.Resources.Limits[corev1.ResourceMemory] = resource.NewQuantity(int64(mem.Quantity.Value()), resource.DecimalSI).DeepCopy()
	}

	if storage := b.service.Resources.Storage; storage != nil {
		requestedStorage := clusterUtil.ComputeCommittedResources(b.settings.StorageCommitLevel, storage.Quantity)
		kcontainer.Resources.Requests[corev1.ResourceEphemeralStorage] = resource.NewQuantity(int64(requestedStorage.Value()), resource.DecimalSI).DeepCopy()
		kcontainer.Resources.Limits[corev1.ResourceEphemeralStorage] = resource.NewQuantity(int64(storage.Quantity.Value()), resource.DecimalSI).DeepCopy()
	}

	// TODO: this prevents over-subscription.  skip for now.

	envVarsAdded := make(map[string]int)
	for _, env := range b.service.Env {
		parts := strings.SplitN(env, "=", 2)
		switch len(parts) {
		case 2:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0], Value: parts[1]})
		case 1:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0]})
		}
		envVarsAdded[parts[0]] = 0
	}
	kcontainer.Env = b.addEnvVarsForDeployment(envVarsAdded, kcontainer.Env)

	for _, expose := range b.service.Expose {
		kcontainer.Ports = append(kcontainer.Ports, corev1.ContainerPort{
			ContainerPort: int32(expose.Port),
		})
	}

	return kcontainer
}

const (
	envVarAkashGroupSequence         = "AKASH_GROUP_SEQUENCE"
	envVarAkashDeploymentSequence    = "AKASH_DEPLOYMENT_SEQUENCE"
	envVarAkashOrderSequence         = "AKASH_ORDER_SEQUENCE"
	envVarAkashOwner                 = "AKASH_OWDER"
	envVarAkashProvider              = "AKASH_PROVIDER"
	envVarAkashClusterPublicHostname = "AKASH_CLUSTER_PUBLIC_HOSTNAME"
)

func addIfNotPresent(envVarsAlreadyAdded map[string]int, env []corev1.EnvVar, key string, value interface{}) []corev1.EnvVar {
	_, exists := envVarsAlreadyAdded[key]
	if exists {
		return env
	}

	env = append(env, corev1.EnvVar{Name: key, Value: fmt.Sprintf("%v", value)})
	return env
}

func (b *deploymentBuilder) addEnvVarsForDeployment(envVarsAlreadyAdded map[string]int, env []corev1.EnvVar) []corev1.EnvVar {
	// Add each env. var. if it is not already set by the SDL
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashGroupSequence, b.lid.GetGSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashDeploymentSequence, b.lid.GetDSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashOrderSequence, b.lid.GetOSeq())
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashOwner, b.lid.Owner)
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashProvider, b.lid.Provider)
	env = addIfNotPresent(envVarsAlreadyAdded, env, envVarAkashClusterPublicHostname, b.settings.ClusterPublicHostname)
	return env
}

// service
type serviceBuilder struct {
	deploymentBuilder
	requireNodePort bool
}

func newServiceBuilder(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service, requireNodePort bool) *serviceBuilder {
	return &serviceBuilder{
		deploymentBuilder: deploymentBuilder{
			builder: builder{
				log:      log.With("module", "kube-builder"),
				settings: settings,
				lid:      lid,
				group:    group,
			},
			service: service,
		},
		requireNodePort: requireNodePort,
	}
}

const suffixForNodePortServiceName = "-np"

func makeGlobalServiceNameFromBasename(basename string) string {
	return fmt.Sprintf("%s%s", basename, suffixForNodePortServiceName)
}

func (b *serviceBuilder) name() string {
	basename := b.deploymentBuilder.name()
	if b.requireNodePort {
		return makeGlobalServiceNameFromBasename(basename)
	}
	return basename
}

func (b *serviceBuilder) deploymentServiceType() corev1.ServiceType {
	if b.requireNodePort {
		return corev1.ServiceTypeNodePort
	}
	return corev1.ServiceTypeClusterIP
}

func (b *serviceBuilder) create() (*corev1.Service, error) { // nolint:golint,unparam
	ports, err := b.ports()
	if err != nil {
		return nil, err
	}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: corev1.ServiceSpec{
			Type:     b.deploymentServiceType(),
			Selector: b.labels(),
			Ports:    ports,
		},
	}
	b.log.Debug("provider/cluster/kube/builder: created service", "service", service)

	return service, nil
}

func (b *serviceBuilder) update(obj *corev1.Service) (*corev1.Service, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Selector = b.labels()
	ports, err := b.ports()
	if err != nil {
		return nil, err
	}
	obj.Spec.Ports = ports
	return obj, nil
}

func (b *serviceBuilder) any() bool {
	for _, expose := range b.service.Expose {
		exposeIsIngress := util.ShouldBeIngress(expose)
		if b.requireNodePort && exposeIsIngress {
			continue
		}

		if !b.requireNodePort && exposeIsIngress {
			return true
		}

		if expose.Global == b.requireNodePort {
			return true
		}
	}
	return false
}

var errUnsupportedProtocol = errors.New("Unsupported protocol for service")
var errInvalidServiceBuilder = errors.New("service builder invalid")

func (b *serviceBuilder) ports() ([]corev1.ServicePort, error) {
	ports := make([]corev1.ServicePort, 0, len(b.service.Expose))
	for i, expose := range b.service.Expose {
		shouldBeIngress := util.ShouldBeIngress(expose)
		if expose.Global == b.requireNodePort || (!b.requireNodePort && shouldBeIngress) {
			if b.requireNodePort && shouldBeIngress {
				continue
			}

			var exposeProtocol corev1.Protocol
			switch expose.Proto {
			case manifest.TCP:
				exposeProtocol = corev1.ProtocolTCP
			case manifest.UDP:
				exposeProtocol = corev1.ProtocolUDP
			default:
				return nil, errUnsupportedProtocol
			}
			externalPort := util.ExposeExternalPort(b.service.Expose[i])
			ports = append(ports, corev1.ServicePort{
				Name:       fmt.Sprintf("%d-%d", i, int(externalPort)),
				Port:       externalPort,
				TargetPort: intstr.FromInt(int(expose.Port)),
				Protocol:   exposeProtocol,
			})
		}
	}

	if len(ports) == 0 {
		b.log.Debug("provider/cluster/kube/builder: created 0 ports", "requireNodePort", b.requireNodePort, "serviceExpose", b.service.Expose)
		return nil, errInvalidServiceBuilder
	}
	return ports, nil
}

type netPolBuilder struct {
	builder
}

func newNetPolBuilder(settings Settings, lid mtypes.LeaseID, group *manifest.Group) *netPolBuilder {
	return &netPolBuilder{builder: builder{settings: settings, lid: lid, group: group}}
}

// Create a set of NetworkPolicies to restrict the ingress traffic to a Tenant's
// Deployment namespace.
func (b *netPolBuilder) create() ([]*netv1.NetworkPolicy, error) { // nolint:golint,unparam

	if !b.settings.NetworkPoliciesEnabled {
		return []*netv1.NetworkPolicy{}, nil
	}

	return []*netv1.NetworkPolicy{
		// INGRESS ---------------------------------------------------------------
		{
			// Deny all ingress to tenant namespace. Default rule which is opened up
			// by subsequent rules.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolDefaultDenyIngress,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeIngress,
				},
			},
		},

		{
			// Allow ingress between services within namespace.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolIngressInternalAllow,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				Ingress: []netv1.NetworkPolicyIngressRule{
					{ // Allow Network Connections from same Namespace
						From: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										akashNetworkNamespace: lidNS(b.lid),
									},
								},
							},
						},
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeIngress,
				},
			},
		},

		{
			// Allow valid ingress to the tentant namespace from ingress controller, by default ingress-nginx
			// TODO: a generic selector should be used since some Providers may choose
			// a different Ingress Controller.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolIngressAllowIngCtrl,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				Ingress: []netv1.NetworkPolicyIngressRule{
					{ // Allow Network Connections ingress-nginx Namespace
						From: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"name": "ingress-nginx",
									},
								},
							},
						},
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeIngress,
				},
			},
		},

		// EGRESS -----------------------------------------------------------------
		{
			// Deny all egress from tenant namespace. Default rule which is opened up
			// by subsequent rules.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolDefaultDenyEgress,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeEgress,
				},
			},
		},

		{
			// Allow egress between services within the namespace.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolEgressInternalAllow,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				Ingress: []netv1.NetworkPolicyIngressRule{
					{ // Allow Network Connections from same Namespace
						From: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										akashNetworkNamespace: lidNS(b.lid),
									},
								},
							},
						},
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeEgress,
				},
			},
		},

		{ // Allow egress to all IPs, EXCEPT local cluster.
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolEgressAllowExternalCidr,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				Egress: []netv1.NetworkPolicyEgressRule{
					{ // Allow Network Connections to Internet, block access to internal IPs
						To: []netv1.NetworkPolicyPeer{
							{
								IPBlock: &netv1.IPBlock{
									CIDR: "0.0.0.0/0",
									Except: []string{
										// TODO: Full validation and correction required.
										// Initial testing indicates that this exception is being ignored;
										// eg: Internal k8s API is accessible from containers, but
										// open Internet is made accessible by rule.
										"10.0.0.0/8",
									},
								},
							},
						},
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeEgress,
				},
			},
		},

		{
			// Allow egress to Kubernetes internal subnet for DNS
			ObjectMeta: metav1.ObjectMeta{
				Name:   netPolEgressAllowKubeDNS,
				Labels: b.labels(),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						akashNetworkNamespace: lidNS(b.lid),
					},
				},
				Egress: []netv1.NetworkPolicyEgressRule{
					{ // Allow Network Connections from same Namespace
						Ports: []netv1.NetworkPolicyPort{
							{
								Protocol: &dnsProtocol,
								Port:     &dnsPort,
							},
						},
						To: []netv1.NetworkPolicyPeer{
							{
								IPBlock: &netv1.IPBlock{
									CIDR: "10.0.0.0/8",
								},
							},
						},
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeEgress,
				},
			},
		},
	}, nil
}

// Update a single NetworkPolicy with correct labels.
func (b *netPolBuilder) update(obj *netv1.NetworkPolicy) (*netv1.NetworkPolicy, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	return obj, nil
}

// ingress
type ingressBuilder struct {
	deploymentBuilder
	expose *manifest.ServiceExpose
	hosts  []string
}

func newIngressBuilder(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service, expose *manifest.ServiceExpose) *ingressBuilder {

	builder := &ingressBuilder{
		deploymentBuilder: deploymentBuilder{
			builder: builder{
				log:      log.With("module", "kube-builder"),
				settings: settings,
				lid:      lid,
				group:    group,
			},
			service: service,
		},
		expose: expose,
		hosts:  make([]string, len(expose.Hosts), len(expose.Hosts)+1),
	}

	copy(builder.hosts, expose.Hosts)

	if settings.DeploymentIngressStaticHosts {
		uid := ingressHost(lid, service)
		host := fmt.Sprintf("%s.%s", uid, settings.DeploymentIngressDomain)
		builder.hosts = append(builder.hosts, host)
	}

	return builder
}

func ingressHost(lid mtypes.LeaseID, svc *manifest.Service) string {
	uid := uuid.NewV5(uuid.NamespaceDNS, lid.String()+svc.Name).Bytes()
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(uid))
}

func (b *ingressBuilder) create() (*netv1.Ingress, error) { // nolint:golint,unparam
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: netv1.IngressSpec{
			Rules: b.rules(),
		},
	}, nil
}

func (b *ingressBuilder) update(obj *netv1.Ingress) (*netv1.Ingress, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Rules = b.rules()
	return obj, nil
}

func (b *ingressBuilder) rules() []netv1.IngressRule {
	// for some reason we need top pass a pointer to this
	pathTypeForAll := netv1.PathTypePrefix

	rules := make([]netv1.IngressRule, 0, len(b.expose.Hosts))
	httpRule := &netv1.HTTPIngressRuleValue{
		Paths: []netv1.HTTPIngressPath{{
			Path:     "/",
			PathType: &pathTypeForAll,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: b.name(),
					Port: netv1.ServiceBackendPort{
						Number: util.ExposeExternalPort(*b.expose),
					},
				},
			}},
		},
	}

	for _, host := range b.hosts {
		rules = append(rules, netv1.IngressRule{
			Host:             host,
			IngressRuleValue: netv1.IngressRuleValue{HTTP: httpRule},
		})
	}
	b.log.Debug("provider/cluster/kube/builder: created rules", "rules", rules)
	return rules
}

// lidNS generates a unique sha256 sum for identifying a provider's object name.
func lidNS(lid mtypes.LeaseID) string {
	path := lid.String()
	// DNS-1123 label must consist of lower case alphanumeric characters or '-',
	// and must start and end with an alphanumeric character
	// (e.g. 'my-name',  or '123-abc', regex used for validation
	// is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')
	sha := sha256.Sum224([]byte(path))
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(sha[:]))
}

// manifestBuilder composes the k8s akashv1.Manifest type from LeaseID and
// manifest.Group data.
type manifestBuilder struct {
	builder
	mns string // Q: is this supposed to be the k8s Namespace? It's the Object name now.
}

func newManifestBuilder(log log.Logger, settings Settings, ns string, lid mtypes.LeaseID, group *manifest.Group) *manifestBuilder {
	return &manifestBuilder{
		builder: builder{
			log:      log.With("module", "kube-builder"),
			settings: settings,
			lid:      lid,
			group:    group,
		},
		mns: ns,
	}
}

func (b *manifestBuilder) labels() map[string]string {
	return appendLeaseLabels(b.lid, b.builder.labels())
}

func (b *manifestBuilder) ns() string {
	return b.mns
}

func (b *manifestBuilder) create() (*akashv1.Manifest, error) {
	obj, err := akashv1.NewManifest(b.name(), b.lid, b.group)
	if err != nil {
		return nil, err
	}
	obj.Labels = b.labels()
	return obj, nil
}

func (b *manifestBuilder) update(obj *akashv1.Manifest) (*akashv1.Manifest, error) {
	m, err := akashv1.NewManifest(b.name(), b.lid, b.group)
	if err != nil {
		return nil, err
	}
	obj.Spec = m.Spec
	obj.Labels = b.labels()
	return obj, nil
}

func (b *manifestBuilder) name() string {
	return lidNS(b.lid)
}

func appendLeaseLabels(lid mtypes.LeaseID, labels map[string]string) map[string]string {
	labels[akashLeaseOwnerLabelName] = lid.Owner
	labels[akashLeaseDSeqLabelName] = strconv.FormatUint(lid.DSeq, 10)
	labels[akashLeaseGSeqLabelName] = strconv.FormatUint(uint64(lid.GSeq), 10)
	labels[akashLeaseOSeqLabelName] = strconv.FormatUint(uint64(lid.OSeq), 10)
	labels[akashLeaseProviderLabelName] = lid.Provider
	return labels
}

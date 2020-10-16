package kube

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"

	"github.com/lithammer/shortuuid"
	"github.com/tendermint/tendermint/libs/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/ovrclk/akash/manifest"
	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	akashManagedLabelName         = "akash.network"
	akashNetworkNamespace         = "akash.network/namespace"
	akashManifestServiceLabelName = "akash.network/manifest-service"

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
	settings settings
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

func newNSBuilder(settings settings, lid mtypes.LeaseID, group *manifest.Group) *nsBuilder {
	return &nsBuilder{builder: builder{settings: settings, lid: lid, group: group}}
}

func (b *nsBuilder) name() string {
	return b.ns()
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

// pspRestrictedBuilder produces restrictive PodSecurityPolicies for tenant Namespaces.
// Restricted PSP source: https://raw.githubusercontent.com/kubernetes/website/master/content/en/examples/policy/restricted-psp.yaml
type pspRestrictedBuilder struct {
	builder
}

func newPspBuilder(settings settings, lid mtypes.LeaseID, group *manifest.Group) *pspRestrictedBuilder { // nolint:golint,unparam
	return &pspRestrictedBuilder{builder: builder{settings: settings, lid: lid, group: group}}
}

func (p *pspRestrictedBuilder) name() string {
	return p.ns()
}

func (p *pspRestrictedBuilder) create() (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
	falseVal := false
	return &v1beta1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.name(),
			Namespace: p.name(),
			Labels:    p.labels(),
			Annotations: map[string]string{
				"seccomp.security.alpha.kubernetes.io/allowedProfileNames": "docker/default,runtime/default",
				"apparmor.security.beta.kubernetes.io/allowedProfileNames": "runtime/default",
				"seccomp.security.alpha.kubernetes.io/defaultProfileName":  "runtime/default",
				"apparmor.security.beta.kubernetes.io/defaultProfileName":  "runtime/default",
			},
		},
		Spec: v1beta1.PodSecurityPolicySpec{
			Privileged:               false,
			AllowPrivilegeEscalation: &falseVal,
			RequiredDropCapabilities: []corev1.Capability{
				"ALL",
			},
			Volumes: []v1beta1.FSType{
				v1beta1.EmptyDir,
				v1beta1.PersistentVolumeClaim, // evaluate necessity later
			},
			HostNetwork: false,
			HostIPC:     false,
			HostPID:     false,
			RunAsUser: v1beta1.RunAsUserStrategyOptions{
				Rule: v1beta1.RunAsUserStrategyMustRunAsNonRoot,
			},
			SELinux: v1beta1.SELinuxStrategyOptions{
				Rule: v1beta1.SELinuxStrategyRunAsAny,
			},
			SupplementalGroups: v1beta1.SupplementalGroupsStrategyOptions{
				Rule: v1beta1.SupplementalGroupsStrategyMustRunAs,
				Ranges: []v1beta1.IDRange{
					{
						Min: int64(1),
						Max: int64(65535),
					},
				},
			},
			FSGroup: v1beta1.FSGroupStrategyOptions{
				Rule: v1beta1.FSGroupStrategyMustRunAs,
				Ranges: []v1beta1.IDRange{
					{
						Min: int64(1),
						Max: int64(65535),
					},
				},
			},
			ReadOnlyRootFilesystem: false,
		},
	}, nil
}

func (p *pspRestrictedBuilder) update(obj *v1beta1.PodSecurityPolicy) (*v1beta1.PodSecurityPolicy, error) { // nolint:golint,unparam
	obj.Name = p.ns()
	obj.Labels = p.labels()
	return obj, nil
}

// deployment
type deploymentBuilder struct {
	builder
	service *manifest.Service
}

func newDeploymentBuilder(log log.Logger, settings settings, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service) *deploymentBuilder {
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
			Limits: make(corev1.ResourceList),
			// TODO: this prevents over-subscription.  skip for now.
			// Requests: make(corev1.ResourceList),
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             &falseValue,
			Privileged:               &falseValue,
			AllowPrivilegeEscalation: &falseValue,
		},
	}

	if cpu := b.service.Resources.CPU; cpu != nil {
		kcontainer.Resources.Limits[corev1.ResourceCPU] = resource.NewScaledQuantity(int64(cpu.Units.Value()), resource.Milli).DeepCopy()
	}

	if mem := b.service.Resources.Memory; mem != nil {
		kcontainer.Resources.Limits[corev1.ResourceMemory] = resource.NewQuantity(int64(mem.Quantity.Value()), resource.DecimalSI).DeepCopy()
	}

	// TODO: this prevents over-subscription.  skip for now.

	for _, env := range b.service.Env {
		parts := strings.Split(env, "=")
		switch len(parts) {
		case 2:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0], Value: parts[1]})
		case 1:
			kcontainer.Env = append(kcontainer.Env, corev1.EnvVar{Name: parts[0]})
		}
	}

	for _, expose := range b.service.Expose {
		kcontainer.Ports = append(kcontainer.Ports, corev1.ContainerPort{
			ContainerPort: int32(expose.Port),
		})
	}

	return kcontainer
}

// service
type serviceBuilder struct {
	deploymentBuilder
}

func newServiceBuilder(log log.Logger, settings settings, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service) *serviceBuilder {
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
	}
}

func (b *serviceBuilder) create() (*corev1.Service, error) { // nolint:golint,unparam
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: corev1.ServiceSpec{
			// use NodePort to support GCP. GCP provides a new IP address for every ingress
			// and requires the service type to be either NodePort or LoadBalancer
			Type:     b.settings.DeploymentServiceType,
			Selector: b.labels(),
			Ports:    b.ports(),
		},
	}, nil
}

func (b *serviceBuilder) update(obj *corev1.Service) (*corev1.Service, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Selector = b.labels()
	obj.Spec.Ports = b.ports()
	return obj, nil
}

func (b *serviceBuilder) ports() []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(b.service.Expose))
	for i, expose := range b.service.Expose {
		ports = append(ports, corev1.ServicePort{
			Name:       strconv.Itoa(int(expose.Port)),
			Port:       exposeExternalPort(&b.service.Expose[i]),
			TargetPort: intstr.FromInt(int(expose.Port)),
		})
	}
	return ports
}

type netPolBuilder struct {
	builder
}

func newNetPolBuilder(settings settings, lid mtypes.LeaseID, group *manifest.Group) *netPolBuilder {
	return &netPolBuilder{builder: builder{settings: settings, lid: lid, group: group}}
}

// Create a set of NetworkPolicies to restrict the ingress traffic to a Tenant's
// Deployment namespace.
func (b *netPolBuilder) create() ([]*netv1.NetworkPolicy, error) { // nolint:golint,unparam
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
										"app.kubernetes.io/name": "ingress-nginx",
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
}

func newIngressBuilder(log log.Logger, settings settings, host string, lid mtypes.LeaseID, group *manifest.Group, service *manifest.Service, expose *manifest.ServiceExpose) *ingressBuilder {
	if settings.DeploymentIngressStaticHosts {
		uid := strings.ToLower(shortuuid.New())
		h := fmt.Sprintf("%s.%s", uid, settings.DeploymentIngressDomain)
		log.Debug("IngressBuilder: map ", h, " host ", host)
		expose.Hosts = append(expose.Hosts, h)
	}
	return &ingressBuilder{
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
	}
}

func (b *ingressBuilder) create() (*extv1.Ingress, error) { // nolint:golint,unparam
	return &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: extv1.IngressSpec{
			Rules: b.rules(),
		},
	}, nil
}

func (b *ingressBuilder) update(obj *extv1.Ingress) (*extv1.Ingress, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Rules = b.rules()
	return obj, nil
}

func (b *ingressBuilder) rules() []extv1.IngressRule {
	rules := make([]extv1.IngressRule, 0, len(b.expose.Hosts))
	httpRule := &extv1.HTTPIngressRuleValue{
		Paths: []extv1.HTTPIngressPath{{
			Backend: extv1.IngressBackend{
				ServiceName: b.name(),
				ServicePort: intstr.FromInt(int(exposeExternalPort(b.expose))),
			}},
		},
	}

	for _, host := range b.expose.Hosts {
		rules = append(rules, extv1.IngressRule{
			Host:             host,
			IngressRuleValue: extv1.IngressRuleValue{HTTP: httpRule},
		})
	}
	b.log.Debug("provider/cluster/kube/builder: created rules", "rules", rules)
	return rules
}

func exposeExternalPort(expose *manifest.ServiceExpose) int32 {
	if expose.ExternalPort == 0 {
		return int32(expose.Port)
	}
	return int32(expose.ExternalPort)
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

func newManifestBuilder(log log.Logger, settings settings, ns string, lid mtypes.LeaseID, group *manifest.Group) *manifestBuilder {
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

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/ovrclk/akash/manifest"
	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

const (
	akashManagedLabelName         = "akash.network"
	akashManifestServiceLabelName = "akash.network/manifest-service"
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

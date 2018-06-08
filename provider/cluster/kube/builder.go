package kube

import (
	"crypto/sha1"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/ovrclk/akash/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	akashServiceLabelName = "akash.network/service"
)

type builder struct {
	oid   types.OrderID
	group *types.ManifestGroup
}

func (b *builder) ns() string {
	return oidNS(b.oid)
}

func (b *builder) labels() map[string]string {
	return map[string]string{}
}

type nsBuilder struct {
	builder
}

func newNSBuilder(oid types.OrderID, group *types.ManifestGroup) *nsBuilder {
	return &nsBuilder{builder: builder{oid, group}}
}

func (b *nsBuilder) name() string {
	return b.ns()
}

func (b *nsBuilder) create() *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.ns(),
			Labels: b.labels(),
		},
	}
}

func (b *nsBuilder) update(prev *corev1.Namespace) *corev1.Namespace {
	prev.Name = b.ns()
	prev.Labels = b.labels()
	return prev
}

// deployment
type deploymentBuilder struct {
	builder
	service *types.ManifestService
}

func newDeploymentBuilder(oid types.OrderID, group *types.ManifestGroup, service *types.ManifestService) *deploymentBuilder {
	return &deploymentBuilder{builder: builder{oid, group}, service: service}
}

func (b *deploymentBuilder) name() string {
	return b.service.Name
}

func (b *deploymentBuilder) labels() map[string]string {
	obj := b.builder.labels()
	obj[akashServiceLabelName] = b.service.Name
	return obj
}

func (b *deploymentBuilder) create() *appsv1.Deployment {
	replicas := int32(b.service.Count)

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
					Containers: []corev1.Container{b.container()},
				},
			},
		},
	}

	return kdeployment
}

func (b *deploymentBuilder) update(obj *appsv1.Deployment) *appsv1.Deployment {
	replicas := int32(b.service.Count)

	obj.Labels = b.labels()
	obj.Spec.Selector.MatchLabels = b.labels()
	obj.Spec.Replicas = &replicas
	obj.Spec.Template.Labels = b.labels()
	obj.Spec.Template.Spec.Containers = []corev1.Container{b.container()}
	return obj
}

func (b *deploymentBuilder) container() corev1.Container {

	qcpu := resource.NewQuantity(int64(b.service.Unit.Cpu), resource.DecimalSI)
	qmem := resource.NewScaledQuantity(int64(b.service.Unit.Memory), resource.Mega)

	kcontainer := corev1.Container{
		Name:  b.service.Name,
		Image: b.service.Image,
		Args:  b.service.Args,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    qcpu.DeepCopy(),
				corev1.ResourceMemory: qmem.DeepCopy(),
			},
			// TODO: this prevents over-subscription.  skip for now.
			// Requests: corev1.ResourceList{
			// 	corev1.ResourceCPU:    qcpu.DeepCopy(),
			// 	corev1.ResourceMemory: qmem.DeepCopy(),
			// },
		},
	}

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

func newServiceBuilder(oid types.OrderID, group *types.ManifestGroup, service *types.ManifestService) *serviceBuilder {
	return &serviceBuilder{
		deploymentBuilder: deploymentBuilder{builder: builder{oid, group}, service: service},
	}
}

func (b *serviceBuilder) create() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: corev1.ServiceSpec{
			Selector: b.labels(),
			Ports:    b.ports(),
		},
	}
}

func (b *serviceBuilder) update(obj *corev1.Service) *corev1.Service {
	obj.Labels = b.labels()
	obj.Spec.Selector = b.labels()
	obj.Spec.Ports = b.ports()
	return obj
}

func (b *serviceBuilder) ports() []corev1.ServicePort {
	ports := make([]corev1.ServicePort, 0, len(b.service.Expose))
	for _, expose := range b.service.Expose {
		ports = append(ports, corev1.ServicePort{
			Name:       strconv.Itoa(int(expose.Port)),
			Port:       exposeExternalPort(expose),
			TargetPort: intstr.FromInt(int(expose.Port)),
		})
	}
	return ports
}

// ingress
type ingressBuilder struct {
	deploymentBuilder
	expose *types.ManifestServiceExpose
}

func newIngressBuilder(oid types.OrderID, group *types.ManifestGroup, service *types.ManifestService, expose *types.ManifestServiceExpose) *ingressBuilder {
	return &ingressBuilder{
		deploymentBuilder: deploymentBuilder{builder: builder{oid, group}, service: service},
		expose:            expose,
	}
}

func (b *ingressBuilder) create() *extv1.Ingress {
	return &extv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.name(),
			Labels: b.labels(),
		},
		Spec: extv1.IngressSpec{
			Backend: &extv1.IngressBackend{
				ServiceName: b.name(),
				ServicePort: intstr.FromInt(int(exposeExternalPort(b.expose))),
			},
			Rules: b.rules(),
		},
	}
}

func (b *ingressBuilder) update(obj *extv1.Ingress) *extv1.Ingress {
	obj.Labels = b.labels()
	obj.Spec.Backend.ServicePort = intstr.FromInt(int(exposeExternalPort(b.expose)))
	obj.Spec.Rules = b.rules()
	return obj
}

func (b *ingressBuilder) rules() []extv1.IngressRule {
	rules := make([]extv1.IngressRule, 0, len(b.expose.Hosts))
	for _, host := range b.expose.Hosts {
		rules = append(rules, extv1.IngressRule{Host: host})
	}
	return rules
}

func exposeExternalPort(expose *types.ManifestServiceExpose) int32 {
	if expose.ExternalPort == 0 {
		return int32(expose.Port)
	}
	return int32(expose.ExternalPort)
}

func oidNS(oid types.OrderID) string {
	path := oid.String()
	sha := sha1.Sum([]byte(path))
	return hex.EncodeToString(sha[:])
}

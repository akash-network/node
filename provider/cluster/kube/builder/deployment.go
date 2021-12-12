package builder

import (
	"github.com/tendermint/tendermint/libs/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	manitypes "github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type Deployment interface {
	workloadBase
	Create() (*appsv1.Deployment, error)
	Update(obj *appsv1.Deployment) (*appsv1.Deployment, error)
}

type deployment struct {
	workload
}

var _ Deployment = (*deployment)(nil)

func NewDeployment(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manitypes.Group, service *manitypes.Service) Deployment {
	return &deployment{
		workload: newWorkloadBuilder(log, settings, lid, group, service),
	}
}

func (b *deployment) Create() (*appsv1.Deployment, error) { // nolint:golint,unparam
	replicas := int32(b.service.Count)
	falseValue := false

	var effectiveRuntimeClassName *string
	if len(b.runtimeClassName) != 0 && b.runtimeClassName != runtimeClassNoneValue {
		effectiveRuntimeClassName = &b.runtimeClassName
	}

	kdeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.Name(),
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
					RuntimeClassName: effectiveRuntimeClassName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &falseValue,
					},
					AutomountServiceAccountToken: &falseValue,
					Containers:                   []corev1.Container{b.container()},
					ImagePullSecrets:             b.imagePullSecrets(),
				},
			},
		},
	}

	return kdeployment, nil
}

func (b *deployment) Update(obj *appsv1.Deployment) (*appsv1.Deployment, error) { // nolint:golint,unparam
	replicas := int32(b.service.Count)
	obj.Labels = b.labels()
	obj.Spec.Selector.MatchLabels = b.labels()
	obj.Spec.Replicas = &replicas
	obj.Spec.Template.Labels = b.labels()
	obj.Spec.Template.Spec.Containers = []corev1.Container{b.container()}
	obj.Spec.Template.Spec.ImagePullSecrets = b.imagePullSecrets()

	return obj, nil
}

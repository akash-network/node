package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	manitypes "github.com/ovrclk/akash/manifest/v2beta1"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
)

type NS interface {
	builderBase
	Create() (*corev1.Namespace, error)
	Update(obj *corev1.Namespace) (*corev1.Namespace, error)
}

type ns struct {
	builder
}

var _ NS = (*ns)(nil)

func BuildNS(settings Settings, lid mtypes.LeaseID, group *manitypes.Group) NS {
	return &ns{builder: builder{settings: settings, lid: lid, group: group}}
}

func (b *ns) labels() map[string]string {
	return AppendLeaseLabels(b.lid, b.builder.labels())
}

func (b *ns) Create() (*corev1.Namespace, error) { // nolint:golint,unparam
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.NS(),
			Labels: b.labels(),
		},
	}, nil
}

func (b *ns) Update(obj *corev1.Namespace) (*corev1.Namespace, error) { // nolint:golint,unparam
	obj.Name = b.NS()
	obj.Labels = b.labels()
	return obj, nil
}

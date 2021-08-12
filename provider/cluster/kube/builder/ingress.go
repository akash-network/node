package builder

import (
	"encoding/base32"
	"fmt"
	"strings"

	uuid "github.com/satori/go.uuid"
	"github.com/tendermint/tendermint/libs/log"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	manitypes "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type Ingress interface {
	workloadBase
	Create() (*netv1.Ingress, error)
	Update(obj *netv1.Ingress) (*netv1.Ingress, error)
}

type ingress struct {
	workload
	expose *manitypes.ServiceExpose
	hosts  []string
}

var _ Ingress = (*ingress)(nil)

func BuildIngress(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manitypes.Group, service *manitypes.Service, expose *manitypes.ServiceExpose) Ingress {
	builder := &ingress{
		workload: newWorkloadBuilder(log, settings, lid, group, service),
		expose:   expose,
		hosts:    make([]string, len(expose.Hosts), len(expose.Hosts)+1),
	}

	copy(builder.hosts, expose.Hosts)

	if settings.DeploymentIngressStaticHosts {
		uid := ingressHost(lid, service)
		host := fmt.Sprintf("%s.%s", uid, settings.DeploymentIngressDomain)
		builder.hosts = append(builder.hosts, host)
	}

	return builder
}

func ingressHost(lid mtypes.LeaseID, svc *manitypes.Service) string {
	uid := uuid.NewV5(uuid.NamespaceDNS, lid.String()+svc.Name).Bytes()
	return strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(uid))
}

func (b *ingress) Create() (*netv1.Ingress, error) { // nolint:golint,unparam
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.Name(),
			Labels: b.labels(),
		},
		Spec: netv1.IngressSpec{
			Rules: b.rules(),
		},
	}, nil
}

func (b *ingress) Update(obj *netv1.Ingress) (*netv1.Ingress, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Rules = b.rules()
	return obj, nil
}

func (b *ingress) rules() []netv1.IngressRule {
	// for some reason we need top pass a pointer to this
	pathTypeForAll := netv1.PathTypePrefix

	rules := make([]netv1.IngressRule, 0, len(b.expose.Hosts))
	httpRule := &netv1.HTTPIngressRuleValue{
		Paths: []netv1.HTTPIngressPath{{
			Path:     "/",
			PathType: &pathTypeForAll,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: b.Name(),
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

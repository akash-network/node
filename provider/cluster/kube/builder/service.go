package builder

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	manitypes "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type Service interface {
	workloadBase
	Create() (*corev1.Service, error)
	Update(obj *corev1.Service) (*corev1.Service, error)
	Any() bool
}

type service struct {
	workload
	requireNodePort bool
}

var _ Service = (*service)(nil)

func BuildService(log log.Logger, settings Settings, lid mtypes.LeaseID, group *manitypes.Group, mservice *manitypes.Service, requireNodePort bool) Service {
	return &service{
		workload:        newWorkloadBuilder(log, settings, lid, group, mservice),
		requireNodePort: requireNodePort,
	}
}

func (b *service) Name() string {
	basename := b.workload.Name()
	if b.requireNodePort {
		return makeGlobalServiceNameFromBasename(basename)
	}
	return basename
}

func (b *service) workloadServiceType() corev1.ServiceType {
	if b.requireNodePort {
		return corev1.ServiceTypeNodePort
	}
	return corev1.ServiceTypeClusterIP
}

func (b *service) Create() (*corev1.Service, error) { // nolint:golint,unparam
	ports, err := b.ports()
	if err != nil {
		return nil, err
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:   b.Name(),
			Labels: b.labels(),
		},
		Spec: corev1.ServiceSpec{
			Type:     b.workloadServiceType(),
			Selector: b.labels(),
			Ports:    ports,
		},
	}
	// b.log.Debug("provider/cluster/kube/builder: created service", "service", svc)

	return svc, nil
}

func (b *service) Update(obj *corev1.Service) (*corev1.Service, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	obj.Spec.Selector = b.labels()
	ports, err := b.ports()
	if err != nil {
		return nil, err
	}

	// retain provisioned NodePort values
	if b.requireNodePort {

		// for each newly-calculated port
		for i, port := range ports {

			// if there is a current (in-kube) port defined
			// with the same specified values
			for _, curport := range obj.Spec.Ports {
				if curport.Name == port.Name &&
					curport.Port == port.Port &&
					curport.TargetPort.IntValue() == port.TargetPort.IntValue() &&
					curport.Protocol == port.Protocol {

					// re-use current port
					ports[i] = curport
				}
			}
		}
	}

	obj.Spec.Ports = ports
	return obj, nil
}

func (b *service) Any() bool {
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

func (b *service) ports() ([]corev1.ServicePort, error) {
	ports := make([]corev1.ServicePort, 0, len(b.service.Expose))
	portsAdded := make(map[int32]struct{})
	for i, expose := range b.service.Expose {
		shouldBeIngress := util.ShouldBeIngress(expose)
		if expose.Global == b.requireNodePort || (!b.requireNodePort && shouldBeIngress) {
			if b.requireNodePort && shouldBeIngress {
				continue
			}

			var exposeProtocol corev1.Protocol
			switch expose.Proto {
			case manitypes.TCP:
				exposeProtocol = corev1.ProtocolTCP
			case manitypes.UDP:
				exposeProtocol = corev1.ProtocolUDP
			default:
				return nil, errUnsupportedProtocol
			}
			externalPort := util.ExposeExternalPort(b.service.Expose[i])
			_, added := portsAdded[externalPort]
			if !added {
				portsAdded[externalPort] = struct{}{}
				ports = append(ports, corev1.ServicePort{
					Name:       fmt.Sprintf("%d-%d", i, int(externalPort)),
					Port:       externalPort,
					TargetPort: intstr.FromInt(int(expose.Port)),
					Protocol:   exposeProtocol,
				})
			}
		}
	}

	if len(ports) == 0 {
		b.log.Debug("provider/cluster/kube/builder: created 0 ports", "requireNodePort", b.requireNodePort, "serviceExpose", b.service.Expose)
		return nil, errInvalidServiceBuilder
	}

	return ports, nil
}

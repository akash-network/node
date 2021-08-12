package builder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	manitypes "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type NetPol interface {
	builderBase
	Create() ([]*netv1.NetworkPolicy, error)
	Update(obj *netv1.NetworkPolicy) (*netv1.NetworkPolicy, error)
}

type netPol struct {
	builder
}

var _ NetPol = (*netPol)(nil)

func BuildNetPol(settings Settings, lid mtypes.LeaseID, group *manitypes.Group) NetPol {
	return &netPol{builder: builder{settings: settings, lid: lid, group: group}}
}

// Create a set of NetworkPolicies to restrict the ingress traffic to a Tenant's
// Deployment namespace.
func (b *netPol) Create() ([]*netv1.NetworkPolicy, error) { // nolint:golint,unparam
	if !b.settings.NetworkPoliciesEnabled {
		return []*netv1.NetworkPolicy{}, nil
	}

	const ingressLabelName = "app.kubernetes.io/name"
	const ingressLabelValue = "ingress-nginx"

	result := []*netv1.NetworkPolicy{
		{

			ObjectMeta: metav1.ObjectMeta{
				Name:      akashDeploymentPolicyName,
				Labels:    b.labels(),
				Namespace: LidNS(b.lid),
			},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeIngress,
					netv1.PolicyTypeEgress,
				},
				Ingress: []netv1.NetworkPolicyIngressRule{
					{ // Allow Network Connections from same Namespace
						From: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										akashNetworkNamespace: LidNS(b.lid),
									},
								},
							},
						},
					},
					{ // Allow Network Connections from NGINX ingress controller
						From: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										ingressLabelName: ingressLabelValue,
									},
								},
								PodSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										ingressLabelName: ingressLabelValue,
									},
								},
							},
						},
					},
				},
				Egress: []netv1.NetworkPolicyEgressRule{
					{ // Allow Network Connections to same Namespace
						To: []netv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										akashNetworkNamespace: LidNS(b.lid),
									},
								},
							},
						},
					},
					{ // Allow DNS to internal server
						Ports: []netv1.NetworkPolicyPort{
							{
								Protocol: &dnsProtocol,
								Port:     &dnsPort,
							},
						},
						To: []netv1.NetworkPolicyPeer{
							{
								PodSelector:       nil,
								NamespaceSelector: nil,
								IPBlock: &netv1.IPBlock{
									CIDR:   "169.254.0.0/16",
									Except: nil,
								},
							},
						},
					},
					{ // Allow access to IPV4 Public addresses only
						To: []netv1.NetworkPolicyPeer{
							{
								PodSelector:       nil,
								NamespaceSelector: nil,
								IPBlock: &netv1.IPBlock{
									CIDR: "0.0.0.0/0",
									Except: []string{
										"10.0.0.0/8",
										"192.168.0.0/16",
										"172.16.0.0/12",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, service := range b.group.Services {
		// find all the ports that are exposed directly
		ports := make([]netv1.NetworkPolicyPort, 0)
		for _, expose := range service.Expose {
			if !expose.Global || util.ShouldBeIngress(expose) {
				continue
			}

			portToOpen := util.ExposeExternalPort(expose)
			portAsIntStr := intstr.FromInt(int(portToOpen))

			var exposeProto corev1.Protocol
			switch expose.Proto {
			case manitypes.TCP:
				exposeProto = corev1.ProtocolTCP
			case manitypes.UDP:
				exposeProto = corev1.ProtocolUDP

			}
			entry := netv1.NetworkPolicyPort{
				Port:     &portAsIntStr,
				Protocol: &exposeProto,
			}
			ports = append(ports, entry)
		}

		// If no ports are found, skip this service
		if len(ports) == 0 {
			continue
		}

		// Make a network policy just to open these ports to incoming traffic
		serviceName := service.Name
		policyName := fmt.Sprintf("akash-%s-np", serviceName)
		policy := netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Labels:    b.labels(),
				Name:      policyName,
				Namespace: LidNS(b.lid),
			},
			Spec: netv1.NetworkPolicySpec{

				Ingress: []netv1.NetworkPolicyIngressRule{
					{ // Allow Network Connections to same Namespace
						Ports: ports,
					},
				},
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						AkashManifestServiceLabelName: serviceName,
					},
				},
				PolicyTypes: []netv1.PolicyType{
					netv1.PolicyTypeIngress,
				},
			},
		}
		result = append(result, &policy)
	}

	return result, nil
}

// Update a single NetworkPolicy with correct labels.
func (b *netPol) Update(obj *netv1.NetworkPolicy) (*netv1.NetworkPolicy, error) { // nolint:golint,unparam
	obj.Labels = b.labels()
	return obj, nil
}

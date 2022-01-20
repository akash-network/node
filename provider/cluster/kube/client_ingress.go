package kube

import (
	"context"
	"fmt"
	"github.com/ovrclk/akash/provider/cluster/kube/clientcommon"
	"math"
	"strconv"
	"strings"

	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	metricsutils "github.com/ovrclk/akash/util/metrics"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"

	netv1 "k8s.io/api/networking/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/pager"

	"github.com/ovrclk/akash/provider/cluster/kube/builder"
)

const (
	akashIngressClassName = "akash-ingress-class"
)

func kubeNginxIngressAnnotations(directive ctypes.ConnectHostnameToDeploymentDirective) map[string]string {
	// For kubernetes/ingress-nginx
	// https://github.com/kubernetes/ingress-nginx
	const root = "nginx.ingress.kubernetes.io"

	readTimeout := math.Ceil(float64(directive.ReadTimeout) / 1000.0)
	sendTimeout := math.Ceil(float64(directive.SendTimeout) / 1000.0)
	result := map[string]string{
		fmt.Sprintf("%s/proxy-read-timeout", root): fmt.Sprintf("%d", int(readTimeout)),
		fmt.Sprintf("%s/proxy-send-timeout", root): fmt.Sprintf("%d", int(sendTimeout)),

		fmt.Sprintf("%s/proxy-next-upstream-tries", root): strconv.Itoa(int(directive.NextTries)),
		fmt.Sprintf("%s/proxy-body-size", root):           strconv.Itoa(int(directive.MaxBodySize)),
	}

	nextTimeoutKey := fmt.Sprintf("%s/proxy-next-upstream-timeout", root)
	nextTimeout := 0 // default magic value for disable
	if directive.NextTimeout > 0 {
		nextTimeout = int(math.Ceil(float64(directive.NextTimeout) / 1000.0))
	}

	result[nextTimeoutKey] = fmt.Sprintf("%d", nextTimeout)

	strBuilder := strings.Builder{}

	for i, v := range directive.NextCases {
		first := string(v[0])
		isHTTPCode := strings.ContainsAny(first, "12345")

		if isHTTPCode {
			strBuilder.WriteString("http_")
		}
		strBuilder.WriteString(v)

		if i != len(directive.NextCases)-1 {
			// The actual separator is the space character for kubernetes/ingress-nginx
			strBuilder.WriteRune(' ')
		}
	}

	result[fmt.Sprintf("%s/proxy-next-upstream", root)] = strBuilder.String()
	return result
}

func (c *client) ConnectHostnameToDeployment(ctx context.Context, directive ctypes.ConnectHostnameToDeploymentDirective) error {
	ingressName := directive.Hostname
	ns := builder.LidNS(directive.LeaseID)
	rules := ingressRules(directive.Hostname, directive.ServiceName, directive.ServicePort)

	foundEntry, err := c.kc.NetworkingV1().Ingresses(ns).Get(ctx, ingressName, metav1.GetOptions{})
	metricsutils.IncCounterVecWithLabelValuesFiltered(kubeCallsCounter, "ingresses-get", err, kubeErrors.IsNotFound)

	labels := make(map[string]string)
	labels[builder.AkashManagedLabelName] = "true"
	builder.AppendLeaseLabels(directive.LeaseID, labels)

	ingressClassName := akashIngressClassName
	obj := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ingressName,
			Labels:      labels,
			Annotations: kubeNginxIngressAnnotations(directive),
		},
		Spec: netv1.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules:            rules,
		},
	}

	switch {
	case err == nil:
		obj.ResourceVersion = foundEntry.ResourceVersion
		_, err = c.kc.NetworkingV1().Ingresses(ns).Update(ctx, obj, metav1.UpdateOptions{})
		metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-ingresses-update", err)
	case kubeErrors.IsNotFound(err):
		_, err = c.kc.NetworkingV1().Ingresses(ns).Create(ctx, obj, metav1.CreateOptions{})
		metricsutils.IncCounterVecWithLabelValues(kubeCallsCounter, "networking-ingresses-create", err)
	}

	return err
}

func (c *client) RemoveHostnameFromDeployment(ctx context.Context, hostname string, leaseID mtypes.LeaseID, allowMissing bool) error {
	ns := builder.LidNS(leaseID)
	labelSelector := &strings.Builder{}
	kubeSelectorForLease(labelSelector, leaseID)

	fieldSelector := &strings.Builder{}
	_, _ = fmt.Fprintf(fieldSelector, "metadata.name=%s", hostname)

	// This delete only works if the ingress exists & the labels match the lease ID given
	err := c.kc.NetworkingV1().Ingresses(ns).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		TypeMeta:             metav1.TypeMeta{},
		LabelSelector:        labelSelector.String(),
		FieldSelector:        fieldSelector.String(),
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	})

	if err != nil && allowMissing && kubeErrors.IsNotFound(err) {
		return nil
	}

	return err
}

func ingressRules(hostname string, kubeServiceName string, kubeServicePort int32) []netv1.IngressRule {
	// for some reason we need to pass a pointer to this
	pathTypeForAll := netv1.PathTypePrefix
	ruleValue := netv1.HTTPIngressRuleValue{
		Paths: []netv1.HTTPIngressPath{{
			Path:     "/",
			PathType: &pathTypeForAll,
			Backend: netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: kubeServiceName,
					Port: netv1.ServiceBackendPort{
						Number: kubeServicePort,
					},
				},
			},
		}},
	}

	return []netv1.IngressRule{{
		Host:             hostname,
		IngressRuleValue: netv1.IngressRuleValue{HTTP: &ruleValue},
	}}
}

type leaseIDHostnameConnection struct {
	leaseID      mtypes.LeaseID
	hostname     string
	externalPort int32
	serviceName  string
}

func (lh leaseIDHostnameConnection) GetHostname() string {
	return lh.hostname
}

func (lh leaseIDHostnameConnection) GetLeaseID() mtypes.LeaseID {
	return lh.leaseID
}

func (lh leaseIDHostnameConnection) GetExternalPort() int32 {
	return lh.externalPort
}

func (lh leaseIDHostnameConnection) GetServiceName() string {
	return lh.serviceName
}

func (c *client) GetHostnameDeploymentConnections(ctx context.Context) ([]ctypes.LeaseIDHostnameConnection, error) {
	ingressPager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return c.kc.NetworkingV1().Ingresses(metav1.NamespaceAll).List(ctx, opts)
	})

	results := make([]ctypes.LeaseIDHostnameConnection, 0)
	err := ingressPager.EachListItem(ctx,
		metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", builder.AkashManagedLabelName)},
		func(obj runtime.Object) error {

			ingress := obj.(*netv1.Ingress)
			ingressLeaseID, err := clientcommon.RecoverLeaseIDFromLabels(ingress.Labels)
			if err != nil {
				return err
			}
			if len(ingress.Spec.Rules) != 1 {
				return fmt.Errorf("%w: invalid number of rules %d", ErrInvalidHostnameConnection, len(ingress.Spec.Rules))
			}
			rule := ingress.Spec.Rules[0]

			if len(rule.IngressRuleValue.HTTP.Paths) != 1 {
				return fmt.Errorf("%w: invalid number of paths %d", ErrInvalidHostnameConnection, len(rule.IngressRuleValue.HTTP.Paths))
			}
			rulePath := rule.IngressRuleValue.HTTP.Paths[0]
			results = append(results, leaseIDHostnameConnection{
				leaseID:      ingressLeaseID,
				hostname:     rule.Host,
				externalPort: rulePath.Backend.Service.Port.Number,
				serviceName:  rulePath.Backend.Service.Name,
			})

			return nil
		})

	if err != nil {
		return nil, err
	}

	return results, nil
}

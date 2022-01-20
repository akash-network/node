package kube

import (
	"context"
	"fmt"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	manifest "github.com/ovrclk/akash/manifest/v2beta1"
	akashtypes "github.com/ovrclk/akash/pkg/apis/akash.network/v2beta1"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	"github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/pager"
	"strings"
)

func ipResourceName(leaseID mtypes.LeaseID, serviceName string, externalPort uint32, proto manifest.ServiceProtocol) string {
	ns := builder.LidNS(leaseID)[0:20]
	resourceName := fmt.Sprintf("%s-%s-%d-%s", ns, serviceName, externalPort, proto)
	return strings.ToLower(resourceName)
}

func (c *client) PurgeDeclaredIP(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, externalPort uint32, proto manifest.ServiceProtocol) error {
	resourceName := ipResourceName(leaseID, serviceName, externalPort, proto)
	return c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", builder.AkashManagedLabelName),
		FieldSelector: fmt.Sprintf("metadata.name=%s", resourceName),
	})
}

func (c *client) DeclareIP(ctx context.Context, lID mtypes.LeaseID, serviceName string, port uint32, externalPort uint32, proto manifest.ServiceProtocol, sharingKey string) error {
	resourceName := ipResourceName(lID, serviceName, externalPort, proto)

	labels := map[string]string{
		builder.AkashManagedLabelName: "true",
	}
	builder.AppendLeaseLabels(lID, labels)
	foundEntry, err := c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).Get(ctx, resourceName, metav1.GetOptions{})

	exists := true
	if err != nil {
		if kubeErrors.IsNotFound(err) {
			exists = false
		} else {
			return err
		}
	}

	obj := akashtypes.ProviderLeasedIP{
		ObjectMeta: metav1.ObjectMeta{
			Name:   resourceName,
			Labels: labels,
		},
		Spec: akashtypes.ProviderLeasedIPSpec{
			LeaseID:      akashtypes.LeaseIDFromAkash(lID),
			ServiceName:  serviceName,
			ExternalPort: externalPort,
			SharingKey:   sharingKey,
			Protocol:     proto.ToString(),
			Port:         port,
		},
		Status: akashtypes.ProviderLeasedIPStatus{},
	}

	c.log.Info("declaring leased ip", "lease", lID,
		"service-name", serviceName,
		"port", port,
		"external-port", externalPort,
		"sharing-key", sharingKey,
		"exists", exists)
	// Create or update the entry
	if exists {
		obj.ObjectMeta.ResourceVersion = foundEntry.ResourceVersion
		_, err = c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).Update(ctx, &obj, metav1.UpdateOptions{})
	} else {
		_, err = c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).Create(ctx, &obj, metav1.CreateOptions{})
	}

	return err
}

func (c *client) PurgeDeclaredIPs(ctx context.Context, lID mtypes.LeaseID) error {
	labelSelector := &strings.Builder{}
	_, err := fmt.Fprintf(labelSelector, "%s=true,", builder.AkashManagedLabelName)
	if err != nil {
		return err
	}
	kubeSelectorForLease(labelSelector, lID)
	result := c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})

	return result
}

func (c *client) ObserveIPState(ctx context.Context) (<-chan v1beta2.IPResourceEvent, error) {
	var lastResourceVersion string
	phpager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		resources, err := c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).List(ctx, opts)

		if err == nil && len(resources.GetResourceVersion()) != 0 {
			lastResourceVersion = resources.GetResourceVersion()
		}
		return resources, err
	})

	data := make([]akashtypes.ProviderLeasedIP, 0, 128)
	err := phpager.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		plip := obj.(*akashtypes.ProviderLeasedIP)
		data = append(data, *plip)
		return nil
	})

	if err != nil {
		return nil, err
	}

	c.log.Info("starting ip passthrough watch", "resourceVersion", lastResourceVersion)
	watcher, err := c.ac.AkashV2beta1().ProviderLeasedIPs(c.ns).Watch(ctx, metav1.ListOptions{
		TypeMeta:             metav1.TypeMeta{},
		LabelSelector:        "",
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      lastResourceVersion,
		ResourceVersionMatch: "",
		TimeoutSeconds:       nil,
		Limit:                0,
		Continue:             "",
	})
	if err != nil {
		return nil, err
	}

	evData := make([]ipResourceEvent, len(data))
	for i, v := range data {
		ownerAddr, err := sdktypes.AccAddressFromBech32(v.Spec.LeaseID.Owner)
		if err != nil {
			return nil, err
		}
		providerAddr, err := sdktypes.AccAddressFromBech32(v.Spec.LeaseID.Provider)
		if err != nil {
			return nil, err
		}

		leaseID, err := v.Spec.LeaseID.ToAkash()
		if err != nil {
			return nil, err
		}

		proto, err := manifest.ParseServiceProtocol(v.Spec.Protocol)
		if err != nil {
			return nil, err
		}

		ev := ipResourceEvent{
			eventType:    ctypes.ProviderResourceAdd,
			lID:          leaseID,
			serviceName:  v.Spec.ServiceName,
			port:         v.Spec.Port,
			externalPort: v.Spec.ExternalPort,
			ownerAddr:    ownerAddr,
			providerAddr: providerAddr,
			sharingKey:   v.Spec.SharingKey,
			protocol:     proto,
		}
		evData[i] = ev
	}

	data = nil

	output := make(chan v1beta2.IPResourceEvent)

	go func() {
		defer close(output)
		for _, v := range evData {
			output <- v
		}
		evData = nil // do not hold the reference

		results := watcher.ResultChan()
		for {
			select {
			case result, ok := <-results:
				if !ok { // Channel closed when an error happens
					return
				}
				plip := result.Object.(*akashtypes.ProviderLeasedIP)
				ownerAddr, err := sdktypes.AccAddressFromBech32(plip.Spec.LeaseID.Owner)
				if err != nil {
					c.log.Error("invalid owner address in provider host", "addr", plip.Spec.LeaseID.Owner, "err", err)
					continue // Ignore event
				}
				providerAddr, err := sdktypes.AccAddressFromBech32(plip.Spec.LeaseID.Provider)
				if err != nil {
					c.log.Error("invalid provider address in provider host", "addr", plip.Spec.LeaseID.Provider, "err", err)
					continue // Ignore event
				}
				leaseID, err := plip.Spec.LeaseID.ToAkash()
				if err != nil {
					c.log.Error("invalid lease ID", "err", err)
					continue // Ignore event
				}
				proto, err := manifest.ParseServiceProtocol(plip.Spec.Protocol)
				if err != nil {
					c.log.Error("invalid protocol", "err", err)
					continue
				}

				ev := ipResourceEvent{
					lID:          leaseID,
					serviceName:  plip.Spec.ServiceName,
					port:         plip.Spec.Port,
					externalPort: plip.Spec.ExternalPort,
					sharingKey:   plip.Spec.SharingKey,
					providerAddr: providerAddr,
					ownerAddr:    ownerAddr,
					protocol:     proto,
				}
				switch result.Type {

				case watch.Added:
					ev.eventType = ctypes.ProviderResourceAdd
				case watch.Modified:
					ev.eventType = ctypes.ProviderResourceUpdate
				case watch.Deleted:
					ev.eventType = ctypes.ProviderResourceDelete

				case watch.Error:
					// Based on examination of the implementation code, this is basically never called anyways
					c.log.Error("watch error", "err", result.Object)

				default:
					continue
				}

				output <- ev

			case <-ctx.Done():
				return
			}
		}
	}()

	return output, nil
}

type ipResourceEvent struct {
	lID          mtypes.LeaseID
	eventType    ctypes.ProviderResourceEvent
	serviceName  string
	port         uint32
	externalPort uint32
	sharingKey   string
	providerAddr sdktypes.Address
	ownerAddr    sdktypes.Address
	protocol     manifest.ServiceProtocol
}

func (ev ipResourceEvent) GetLeaseID() mtypes.LeaseID {
	return ev.lID
}

func (ev ipResourceEvent) GetEventType() ctypes.ProviderResourceEvent {
	return ev.eventType
}

func (ev ipResourceEvent) GetServiceName() string {
	return ev.serviceName
}

func (ev ipResourceEvent) GetPort() uint32 {
	return ev.port
}

func (ev ipResourceEvent) GetExternalPort() uint32 {
	return ev.externalPort
}

func (ev ipResourceEvent) GetSharingKey() string {
	return ev.sharingKey
}

func (ev ipResourceEvent) GetProtocol() manifest.ServiceProtocol {
	return ev.protocol
}

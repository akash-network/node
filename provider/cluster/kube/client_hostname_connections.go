package kube

import (
	"context"
	"fmt"
	"strings"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/pager"

	akashtypes "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/provider/cluster/kube/builder"
	ctypes "github.com/ovrclk/akash/provider/cluster/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type hostnameResourceEvent struct {
	eventType ctypes.ProviderResourceEvent
	hostname  string

	owner        sdktypes.Address
	dseq         uint64
	oseq         uint32
	gseq         uint32
	provider     sdktypes.Address
	serviceName  string
	externalPort uint32
}

func (c *client) DeclareHostname(ctx context.Context, lID mtypes.LeaseID, host string, serviceName string, externalPort uint32) error {
	// Label each entry with the standard labels
	labels := map[string]string{
		builder.AkashManagedLabelName: "true",
	}
	builder.AppendLeaseLabels(lID, labels)

	foundEntry, err := c.ac.AkashV1().ProviderHosts(c.ns).Get(ctx, host, metav1.GetOptions{})
	exists := true
	var resourceVersion string

	if err != nil {
		if kubeErrors.IsNotFound(err) {
			exists = false
		} else {
			return err
		}
	} else {
		resourceVersion = foundEntry.ObjectMeta.ResourceVersion
	}

	obj := akashtypes.ProviderHost{
		ObjectMeta: metav1.ObjectMeta{
			Name:            host, // Name is always the hostname, to prevent duplicates
			Labels:          labels,
			ResourceVersion: resourceVersion,
		},
		Spec: akashtypes.ProviderHostSpec{
			Hostname:     host,
			Owner:        lID.GetOwner(),
			Dseq:         lID.GetDSeq(),
			Oseq:         lID.GetOSeq(),
			Gseq:         lID.GetGSeq(),
			Provider:     lID.GetProvider(),
			ServiceName:  serviceName,
			ExternalPort: externalPort,
		},
		Status: akashtypes.ProviderHostStatus{},
	}

	c.log.Info("declaring hostname", "lease", lID, "service-name", serviceName, "external-port", externalPort)
	// Create or update the entry
	if exists {
		_, err = c.ac.AkashV1().ProviderHosts(c.ns).Update(ctx, &obj, metav1.UpdateOptions{})
	} else {
		_, err = c.ac.AkashV1().ProviderHosts(c.ns).Create(ctx, &obj, metav1.CreateOptions{})
	}
	return err
}

func (c *client) PurgeDeclaredHostnames(ctx context.Context, lID mtypes.LeaseID) error {
	labelSelector := &strings.Builder{}
	kubeSelectorForLease(labelSelector, lID)
	result := c.ac.AkashV1().ProviderHosts(c.ns).DeleteCollection(ctx, metav1.DeleteOptions{}, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})

	return result
}

func (ev hostnameResourceEvent) GetLeaseID() mtypes.LeaseID {
	return mtypes.LeaseID{
		Owner:    ev.owner.String(),
		DSeq:     ev.dseq,
		GSeq:     ev.gseq,
		OSeq:     ev.oseq,
		Provider: ev.provider.String(),
	}
}

func (ev hostnameResourceEvent) GetHostname() string {
	return ev.hostname
}

func (ev hostnameResourceEvent) GetEventType() ctypes.ProviderResourceEvent {
	return ev.eventType
}

func (ev hostnameResourceEvent) GetServiceName() string {
	return ev.serviceName
}

func (ev hostnameResourceEvent) GetExternalPort() uint32 {
	return ev.externalPort
}

func (c *client) ObserveHostnameState(ctx context.Context) (<-chan ctypes.HostnameResourceEvent, error) {
	var lastResourceVersion string
	phpager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		resources, err := c.ac.AkashV1().ProviderHosts(c.ns).List(ctx, opts)

		if err == nil && len(resources.GetResourceVersion()) != 0 {
			lastResourceVersion = resources.GetResourceVersion()
		}
		return resources, err
	})

	data := make([]akashtypes.ProviderHost, 0, 128)
	err := phpager.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		ph := obj.(*akashtypes.ProviderHost)
		data = append(data, *ph)
		return nil
	})

	if err != nil {
		return nil, err
	}

	c.log.Info("starting hostname watch", "resourceVersion", lastResourceVersion)
	watcher, err := c.ac.AkashV1().ProviderHosts(c.ns).Watch(ctx, metav1.ListOptions{
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

	evData := make([]hostnameResourceEvent, len(data))
	for i, v := range data {
		ownerAddr, err := sdktypes.AccAddressFromBech32(v.Spec.Owner)
		if err != nil {
			return nil, err
		}
		providerAddr, err := sdktypes.AccAddressFromBech32(v.Spec.Provider)
		if err != nil {
			return nil, err
		}
		ev := hostnameResourceEvent{
			eventType:    ctypes.ProviderResourceAdd,
			hostname:     v.Spec.Hostname,
			oseq:         v.Spec.Oseq,
			gseq:         v.Spec.Gseq,
			dseq:         v.Spec.Dseq,
			owner:        ownerAddr,
			provider:     providerAddr,
			serviceName:  v.Spec.ServiceName,
			externalPort: v.Spec.ExternalPort,
		}
		evData[i] = ev
	}

	data = nil

	output := make(chan ctypes.HostnameResourceEvent)

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
				ph := result.Object.(*akashtypes.ProviderHost)
				ownerAddr, err := sdktypes.AccAddressFromBech32(ph.Spec.Owner)
				if err != nil {
					c.log.Error("invalid owner address in provider host", "addr", ph.Spec.Owner, "err", err)
					continue // Ignore event
				}
				providerAddr, err := sdktypes.AccAddressFromBech32(ph.Spec.Provider)
				if err != nil {
					c.log.Error("invalid provider address in provider host", "addr", ph.Spec.Provider, "err", err)
					continue // Ignore event
				}
				ev := hostnameResourceEvent{
					hostname:     ph.Spec.Hostname,
					dseq:         ph.Spec.Dseq,
					oseq:         ph.Spec.Oseq,
					gseq:         ph.Spec.Gseq,
					owner:        ownerAddr,
					provider:     providerAddr,
					serviceName:  ph.Spec.ServiceName,
					externalPort: ph.Spec.ExternalPort,
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

func (c *client) AllHostnames(ctx context.Context) ([]ctypes.ActiveHostname, error) {
	ingressPager := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return c.ac.AkashV1().ProviderHosts(c.ns).List(ctx, opts)
	})

	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", builder.AkashManagedLabelName),
	}

	result := make([]ctypes.ActiveHostname, 0)

	err := ingressPager.EachListItem(ctx, listOptions, func(obj runtime.Object) error {
		ph := obj.(*akashtypes.ProviderHost)
		hostname := ph.Spec.Hostname
		dseq := ph.Spec.Dseq
		gseq := ph.Spec.Gseq
		oseq := ph.Spec.Oseq

		owner, ok := ph.Labels[builder.AkashLeaseOwnerLabelName]
		if !ok || len(owner) == 0 {
			c.log.Error("providerhost missing owner label", "host", hostname)
			return nil
		}
		provider, ok := ph.Labels[builder.AkashLeaseProviderLabelName]
		if !ok || len(provider) == 0 {
			c.log.Error("providerhost missing provider label", "host", hostname)
			return nil
		}

		leaseID := mtypes.LeaseID{
			Owner:    owner,
			DSeq:     dseq,
			GSeq:     uint32(gseq),
			OSeq:     uint32(oseq),
			Provider: provider,
		}

		result = append(result, ctypes.ActiveHostname{
			ID:       leaseID,
			Hostname: hostname,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

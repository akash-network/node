package util

import (
	"context"
	"errors"
	"fmt"
	"github.com/boz/go-lifecycle"
	"github.com/desertbit/timer"
	"github.com/ovrclk/akash/util/runner"
	"github.com/tendermint/tendermint/libs/log"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"math/rand"
	"net"
	"time"
)

var (
	ErrShuttingDown     = errors.New("shutting down")
	errServiceDiscovery = errors.New("service discovery failure")
	errServiceClient    = errors.New("service client failure")
)

func NewServiceDiscoveryAgent(logger log.Logger, kubeConfig *rest.Config, portName, serviceName, namespace string, endpoint *net.SRV) (ServiceDiscoveryAgent, error) {
	// short circuit if a value is passed in, this is a convenience function for using manually specified values
	if endpoint != nil {
		return staticServiceDiscoveryAgent(*endpoint), nil
	}

	kc, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	sda := &serviceDiscoveryAgent{
		serviceName:     serviceName,
		namespace:       namespace,
		portName:        portName,
		lc:              lifecycle.New(),
		discoverch:      make(chan struct{}, 1),
		requests:        make(chan serviceDiscoveryRequest),
		pendingRequests: nil,
		result:          nil,
		log:             logger.With("cmp", "service-discovery-agent"),
		kube:            kc,
		kubeConfig:      kubeConfig,
	}
	go sda.run()

	return sda, nil
}

func (sda *serviceDiscoveryAgent) Stop() {
	sda.lc.Shutdown(nil)
}

func (sda *serviceDiscoveryAgent) DiscoverNow() {
	select {
	case sda.discoverch <- struct{}{}:
	default:
	}
}

func (sda *serviceDiscoveryAgent) run() {
	defer sda.lc.ShutdownCompleted()

	const retryInterval = time.Second * 2
	retryTimer := timer.NewTimer(retryInterval)
	retryTimer.Stop()
	defer retryTimer.Stop()
	var discoveryResult <-chan runner.Result

	discover := true
mainLoop:
	for {
		select {
		case <-sda.lc.ShutdownRequest():
			break mainLoop
		case <-sda.discoverch:
			discover = true // Could be ignored if discoveryResult is not nil
		case <-retryTimer.C:
			retryTimer.Stop()
			discover = true
		case result := <-discoveryResult:
			err := result.Error()
			if err != nil {
				sda.setResult(nil, err)
				retryTimer.Reset(retryInterval)
				break
			}

			factory := (result.Value()).(clientFactory)
			sda.setResult(factory, nil)
		case req := <-sda.requests:
			sda.handleRequest(req)
		}

		if discover && discoveryResult == nil {
			discoveryResult = runner.Do(func() runner.Result {
				return runner.NewResult(sda.discover())
			})
		}
	}
}

func (sda *serviceDiscoveryAgent) handleRequest(req serviceDiscoveryRequest) {
	if sda.result != nil {
		req.resultCh <- sda.result
		return
	}

	sda.pendingRequests = append(sda.pendingRequests, req)
}

func (sda *serviceDiscoveryAgent) setResult(factory clientFactory, err error) {
	sda.log.Debug("satisfying pending requests", "qty", len(sda.pendingRequests))

	for _, pendingRequest := range sda.pendingRequests {
		if err == nil {
			pendingRequest.resultCh <- factory
		} else {
			pendingRequest.errCh <- err
		}
	}

	sda.pendingRequests = nil // Clear pending requests
	if err == nil {
		sda.result = factory
	} else {
		sda.result = nil
	}
}

func (sda *serviceDiscoveryAgent) GetClient(ctx context.Context, isHTTPS, secure bool) (ServiceClient, error) {
	errCh := make(chan error, 1)
	resultCh := make(chan clientFactory, 1)
	req := serviceDiscoveryRequest{
		errCh:    errCh,
		resultCh: resultCh,
	}

	select {
	case sda.requests <- req:
	case <-sda.lc.ShutdownRequest():
		return nil, ErrShuttingDown
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case result := <-resultCh:
		return result(isHTTPS, secure), nil
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (sda *serviceDiscoveryAgent) discover() (clientFactory, error) {
	insideKubernetes, err := IsInsideKubernetes()
	if err != nil {
		return nil, err
	}

	if insideKubernetes {
		return sda.discoverDNS()
	}

	return sda.discoverKube()
}

func (sda *serviceDiscoveryAgent) discoverKube() (clientFactory, error) {
	// This code is retried forever, but don't wait on a result for a very long time. Just poll
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Ask Kubernetes to confirm that the requested resource exists
	service, err := sda.kube.CoreV1().Services(sda.namespace).Get(ctx, sda.serviceName, v1.GetOptions{})

	if err != nil {
		sda.log.Error("kube discovery failed")
		return nil, err
	}

	ports := service.Spec.Ports
	selectedPort := -1

	for i, port := range ports {
		if port.Name == sda.portName && corev1.ProtocolTCP == port.Protocol {
			selectedPort = i
			break
		}
	}

	if selectedPort == -1 {
		return nil, fmt.Errorf("%w: service %q exists but has no port %q (TCP)", errServiceDiscovery, sda.serviceName, sda.portName)
	}
	port := ports[selectedPort]

	// Get the host for the kube cluster
	kubeHost := sda.kubeConfig.Host
	// The kube config object has a builtin system for getting an HTTP transport that does all the auth
	// related things the cluster wants
	httpTransport, err := rest.TransportFor(sda.kubeConfig)
	if err != nil {
		return nil, err
	}

	return func(isHTTPS, _ bool) ServiceClient {
		serviceName := service.Name
		if isHTTPS {
			serviceName = fmt.Sprintf("https:%s", service.Name)
		}
		/**
		Documentation here: https://kubernetes.io/docs/tasks/administer-cluster/access-cluster-services/

		The structure is
		http://kubernetes_master_address/api/v1/namespaces/namespace_name/services/[https:]service_name[:port_name]/proxy
		*/
		proxyURL := fmt.Sprintf("%s/api/v1/namespaces/%s/services/%s:%s/proxy", kubeHost, service.Namespace, serviceName, port.Name)

		return newHTTPWrapperServiceClientWithTransport(httpTransport, proxyURL)
	}, nil
}

func (sda *serviceDiscoveryAgent) discoverDNS() (clientFactory, error) {
	// FUTURE - try and find a 3rd party API that allows timeouts to be put on this request
	_, addrs, err := net.LookupSRV(sda.portName, "TCP", fmt.Sprintf("%s.%s.svc.cluster.local", sda.serviceName, sda.namespace))
	if err != nil {
		sda.log.Error("dns discovery failed", "error", err, "portName", sda.portName, "service-name", sda.serviceName, "namespace", sda.namespace)
		return nil, err
	}

	// De-pointerize result
	result := make([]net.SRV, len(addrs))
	for i, addr := range addrs {
		result[i] = *addr
	}
	sda.log.Info("dns discovery success", "addrs", result, "portName", sda.portName, "service-name", sda.serviceName, "namespace", sda.namespace)
	// Ignore priority & weight, just make a random selection. This generally has a length of 1
	// nolint:gosec
	addrI := rand.Int31n(int32(len(addrs)))
	choice := result[addrI]
	return func(isHTTPS, secure bool) ServiceClient {
		proto := "http"
		if isHTTPS {
			proto = "https"
		}
		discoveredURL := fmt.Sprintf("%s://%v:%v", proto, choice.Target, choice.Port)

		return newHTTPWrapperServiceClient(isHTTPS, secure, discoveredURL)
	}, nil
}

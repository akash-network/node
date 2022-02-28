package loki

/**
Some code in this file is from the Loki project v1.6.1. It uses the Apache License version 2.0 which is the
same as this project.
 */

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/tendermint/tendermint/libs/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

const (
	lokiNamespace = "loki-stack"
	lokiServiceName = "loki-headless"
	lokiPortName = "http-metrics"

	queryPath       = "/loki/api/v1/query"
	queryRangePath  = "/loki/api/v1/query_range"
	labelsPath      = "/loki/api/v1/labels"
	labelValuesPath = "/loki/api/v1/label/%s/values"
	seriesPath      = "/loki/api/v1/series"
	tailPath        = "/loki/api/v1/tail"
	filenameLabel = "filename"
	podLabel = "pod"
	lokiOrgIdHeader = "X-Scope-OrgID"
)

var (
	ErrLoki = errors.New("error querying loki")
)

type Client interface {
	FindLogsByLease(ctx context.Context, leaseID mtypes.LeaseID) ([]LogStatus, error)
	Stop()
}

func NewClient(logger log.Logger, kubeConfig *rest.Config, port *net.SRV) (Client, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	sda, err := clusterutil.NewServiceDiscoveryAgent(logger, kubeConfig, lokiPortName, lokiServiceName, lokiNamespace, port)
	if err != nil {
		return nil, err
	}
	return &client{
		sda:sda,
		lock: &sync.Mutex{},
		kc: kubeClient,
	}, nil
}

type client struct {
	sda clusterutil.ServiceDiscoveryAgent
	kc kubernetes.Interface
	client clusterutil.ServiceClient
	lock sync.Locker
}

func (c *client) Stop() {
	c.sda.Stop()
}

func (c *client) GetLogByService(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, replicaIndex uint, runIndex uint, startTime, endTime time.Time) {

}

func (c *client) getLokiClient(ctx context.Context) (clusterutil.ServiceClient, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.client == nil {
		var err error
		c.client, err = c.sda.GetClient(ctx, false, false)
		if err != nil {
			return nil, err
		}
	}
	return c.client, nil
}

type lokiLabelValuesResponse struct {
	Status string `json:"status"`
	Data []string `json:"data"`
}

type LogStatus struct {
	ServiceName string
	ReplicaIndex int
	Present bool
}
func (c *client) FindLogsByLease(ctx context.Context, leaseID mtypes.LeaseID) ([]LogStatus, error) {
	lidNS := clusterutil.LeaseIDToNamespace(leaseID)
	lc, err := c.getLokiClient(ctx)
	if err != nil {
		return nil, err
	}

	// get a list of possible logs for this service
	possiblePods, err := c.detectRunsForLease(ctx, leaseID)

	// Query Loki for labels of the log filename. The label has a value like
	//   /var/log/pods/7ev5rjb3nfl9niulmnpaft53g27rck4fhm6v3n4nb5jvu_web-bb84fdfcf-brx2z_f192c48e-ccb3-441e-9778-58de019259d6/web/0.log
	// Where the last part of the filename is the associated restart number. The label value only appears if
	// the container actually logged something before it terminates. So, we can check the labels and parse out
	// that number to determine what containers logged anything before they died
	if err != nil {
		return nil, err
	}

	req, err := lc.CreateRequest(ctx,
		http.MethodGet,
		fmt.Sprintf(labelValuesPath, podLabel),
		nil)

	if err != nil {
		return nil, err
	}

	// TODO - on the query set end=1646072848813679310&start=1646065648813679310 or similar query args
	// Based on the retention set on the provider
	req.Header.Add(lokiOrgIdHeader, lidNS)
	resp, err := lc.DoRequest(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%w: fetching loki label values failed, got status code %d", ErrLoki, resp.StatusCode)
	}

	decoder := json.NewDecoder(resp.Body)
	result := lokiLabelValuesResponse{}
	err = decoder.Decode(&result)
	if err != nil  {
		return nil, fmt.Errorf("decoding loki label values failed: %w", err)
	}

	// Assign pods a replica index by their name, so it is consistent. This is done elsewhere as well
	podNames := make([]string, 0, len(possiblePods))
	for podName := range possiblePods {
		podNames = append(podNames, podName)
	}
	sort.Strings(podNames)
	podNameToReplicaIndex := make(map[string]int)
	for i, podName := range podNames {
		podNameToReplicaIndex[podName] = i
	}

	returnValue := make([]LogStatus, len(podNameToReplicaIndex))
	// By default nothing is found
	for possiblePodName, entry := range possiblePods {
		replicaIndex := podNameToReplicaIndex[possiblePodName]
		returnValue[replicaIndex] = LogStatus{
			ServiceName:  entry.serviceName,
			ReplicaIndex: replicaIndex,
		}
	}

	// Mark each pod that is found by name
	for _, podName := range result.Data {
		i, exists := podNameToReplicaIndex[podName]
		if !exists {
			continue
		}
		returnValue[i].Present = true
	}

	// TODO - query by pod name and then figure out how many restarts actually logged something?

	return returnValue, nil
}

type runEntry struct {
	restarts uint
	serviceName string
}
func (c *client) detectRunsForLease(ctx context.Context, leaseID mtypes.LeaseID) (map[string]runEntry, error) {
	// Containers can run more than once (i.e. a pod restarts containers when configured to do so)
	// so this code picks up on that by looking into the labels
	lidNS := clusterutil.LeaseIDToNamespace(leaseID)
	// TODO - paginate
	podsResult, err := c.kc.CoreV1().Pods(lidNS).List(ctx, metav1.ListOptions{
		// TODO - use a constant here
		LabelSelector:        "akash.network=true",
	})

	if err != nil {
		return nil, err
	}

	if len(podsResult.Items) == 0 {
		return nil, fmt.Errorf("%w: lease %q has no pods in kubernetes", ErrLoki, leaseID.String())
	}

	// Build up a mapping of pod names to the expected number of log files
	result := make(map[string]runEntry)
	for _, pod := range podsResult.Items {
		// We only define pods with a single container at this time
		if len(pod.Status.ContainerStatuses) != 1 {
			return nil, fmt.Errorf("%w: pod %q has %d containers, expected 1", ErrLoki, pod.Name, len(pod.Status.ContainerStatuses))
		}
		status := pod.Status.ContainerStatuses[0]

		// TODO - use a constant
		serviceName, ok := pod.ObjectMeta.Labels["akash.network/manifest-service"]
		if !ok {
			return nil, fmt.Errorf("%w: pod %q has no akash service name label", ErrLoki, pod.Name)
		}

		result[pod.Name] = runEntry{
			restarts:    uint(status.RestartCount),
			serviceName: serviceName,
		}
	}

	return result, nil
}

package loki

/**
Some code in this file is from the Loki project v1.6.1. It uses the Apache License version 2.0 which is the
same as this project.
 */

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
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

type lokiDirection string

const (
	FORWARD  lokiDirection = "FORWARD"
	BACKWARD lokiDirection = "BACKWARD"
)

var (
	ErrLoki = errors.New("error querying loki")
)

type Client interface {
	FindLogsByLease(ctx context.Context, leaseID mtypes.LeaseID) ([]LogStatus, error)
	GetLogByService(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, replicaIndex uint, runIndex int, startTime, endTime time.Time, forward bool) (LogResult, error)
	TailLogsByService(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, replicaIndex uint, runIndex int, startTime time.Time,
		eachLogLine func(at time.Time, line string)error,
		onFirstDroppedLog func() error) error

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
	websocketClient clusterutil.WebsocketServiceClient
	lock sync.Locker
}

func (c *client) Stop() {
	c.sda.Stop()
}

type lokiLogQueryResult struct {
	Stream map[string]string `json:"stream"`
	Values [][]string `json:"values"`
}

type lokiLogQueryData struct {
	ResultType string `json:"resultType"`
	Result []lokiLogQueryResult `json:"result"`
}

type lokiLogQueryResponse struct {
	Status string `json:"status"`
	Data lokiLogQueryData `json:"data"`
}

type LogResultLine struct {
	RunIndex uint
	At time.Time
	Line string
}

type LogResult struct {
	ServiceName string
	ReplicaIndex uint
	Entries []LogResultLine

}

func (c *client) discoverFilename(ctx context.Context, leaseID mtypes.LeaseID, startTime, endTime time.Time, podName string) (string,error) {
	// Query using the pod name to get the result
	lc, err := c.getLokiClient(ctx)
	if err != nil {
		return "", err
	}

	httpQueryString := url.Values{}
	httpQueryString.Set("start", fmt.Sprintf("%d", startTime.UnixNano()))
	httpQueryString.Set("end", fmt.Sprintf("%d", endTime.UnixNano()))
	httpQueryString.Set("limit", "1")
	// TODO - guard against injection
	lokiQuery := fmt.Sprintf("{%s=%q}", podLabel, podName) // Note this is not JSON
	httpQueryString.Set("query", lokiQuery)
	httpQueryString.Set("direction", string(BACKWARD))

	request, err := lc.CreateRequest(ctx, http.MethodGet, queryRangePath, nil )
	if err != nil {
		return "", err
	}
	request.URL.RawQuery = httpQueryString.Encode()

	request.Header.Add(lokiOrgIdHeader, clusterutil.LeaseIDToNamespace(leaseID))

	resp, err := lc.DoRequest(request)
	if err != nil {
		return "" , fmt.Errorf("loki filename discovery log query for pod %q failed: %w", podName, err)
	}

	if resp.StatusCode != 200 {
		buf := &bytes.Buffer{}
		_, _ = io.Copy(buf, resp.Body)
		msg := strings.Trim(buf.String(), "\n\t\r")
		return "", fmt.Errorf("%w: loki filename discovery log query failed for pod %q, got status code %d; %s", ErrLoki, podName, resp.StatusCode, msg)
	}

	decoder := json.NewDecoder(resp.Body)
	lokiResult := lokiLogQueryResponse{}
	err = decoder.Decode(&lokiResult)
	if err != nil {
		return "",fmt.Errorf("loki log query response for pod %q could not be decoded: %w", podName, err)
	}

	if len(lokiResult.Data.Result) == 0 {
		return "", fmt.Errorf("%w: loki filename discovery for pod %q returned no results", ErrLoki, podName)
	}

	filename, exists:= lokiResult.Data.Result[0].Stream[filenameLabel]
	if !exists {
		return "", fmt.Errorf("%w: loki filename discovery for pod %q had no label %q", ErrLoki, podName, filenameLabel)
	}

	return filename, nil
}

// DroppedStream represents a dropped stream in tail call
type droppedStream struct {
	Timestamp time.Time
	Labels    map[string]string
}

//Entry represents a log entry.  It includes a log message and the time it occurred at.
type entry struct {
	Timestamp time.Time
	Line      string
}

//Stream represents a log stream.  It includes a set of log entries and their labels.
type stream struct {
	Labels  map[string]string `json:"stream"`
	Entries [][]string  `json:"values"`
}

// tailResponse represents the http json response to a tail query from loki
type tailResponse struct {
	Streams        []stream        `json:"streams,omitempty"`
	DroppedStreams []droppedStream `json:"dropped_entries,omitempty"`
}

func (c *client) TailLogsByService(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, replicaIndex uint, runIndex int, startTime time.Time,
	eachLogLine func(at time.Time, line string)error,
	onFirstDroppedLog func() error) error {
	possiblePods, err := c.detectRunsForLease(ctx, leaseID)
	if err != nil {
		return err
	}

	datamap := serviceNameAndReplicaIndexToPodName(possiblePods)
	podName, exists := datamap[serviceNameAndReplicaIndex{
		replicaIndex: int(replicaIndex),
		serviceName:  serviceName,
	}]

	if !exists {
		return fmt.Errorf("%w: no entry for service %q and replica %d", ErrLoki, serviceName, replicaIndex)
	}

	httpQueryString := url.Values{}
	httpQueryString.Set("from", fmt.Sprintf("%d", startTime.UnixNano()))
	httpQueryString.Set("limit", "1000") // TODO - configurable or something? Maybe user requestable?
	httpQueryString.Set("delay_for", "3") // TODO - what does this do

	var filenameLabelFilter string
	specifyRunIndex := runIndex >= 0
	if specifyRunIndex {
		var err error
		filenameLabelFilter, err = c.getFilename(ctx, leaseID, startTime, time.Now(), runIndex, podName)
		if err != nil {
			return err
		}
	}

	lokiQueryBuf := &bytes.Buffer{}
	// Note this is not JSON
	// TODO - guard against injection here even though it is unlikely
	_, _ = fmt.Fprint(lokiQueryBuf, "{")
	_, _ = fmt.Fprintf(lokiQueryBuf, "%s=%q", podLabel, podName)
	// specify filename label here if runIndex >= 0
	if specifyRunIndex {
		_, _ = fmt.Fprintf(lokiQueryBuf,",%s=%q", filenameLabel, filenameLabelFilter)
	}
	_, _ = fmt.Fprint(lokiQueryBuf, "}")

	httpQueryString.Set("query", lokiQueryBuf.String())

	wslc, err := c.getLokiWebsocketClient(ctx)
	if err != nil {
		return err
	}

	headers := http.Header{
		lokiOrgIdHeader: []string{clusterutil.LeaseIDToNamespace(leaseID)},
	}
	conn, err := wslc.DialWebsocket(ctx, tailPath + "?" + httpQueryString.Encode(), headers)
	if err != nil {
		return err
	}

	dropCalled := false
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var logs tailResponse
		err := conn.ReadJSON(&logs)
		if err != nil {
			return fmt.Errorf("error parsing JSON from loki log tail: %w", err)
		}

		for _, stream := range logs.Streams {
			for _, entry := range stream.Entries {
				at := time.Time{} // TODO
				line := entry[1]
				err = eachLogLine(at, line)
				if err != nil {
					return err
				}
			}
		}

		if !dropCalled && len(logs.DroppedStreams) != 0{
			// caused by the client being too slow
			dropCalled = true
			err = onFirstDroppedLog()
			if err != nil {
				return err
			}
		}

	}

}

func (c *client) getFilename(ctx context.Context, leaseID mtypes.LeaseID, startTime, endTime time.Time, runIndex int, podName string) (string, error){
	filename, err := c.discoverFilename(ctx, leaseID, startTime, endTime, podName)
	if err != nil {
		return "", err
	}

	head, tail := path.Split(filename)
	parts := strings.SplitN(tail, ".", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("%w: while constructing fielname filter cannot make sense of filepath %q", ErrLoki, filename)
	}

	return fmt.Sprintf("%s/%d.%s", head, runIndex, parts[1]), nil
}


func (c *client) GetLogByService(ctx context.Context, leaseID mtypes.LeaseID, serviceName string, replicaIndex uint, runIndex int, startTime, endTime time.Time, forward bool) (LogResult, error) {
	lidNS := clusterutil.LeaseIDToNamespace(leaseID)
	// get a list of possible logs for this service
	possiblePods, err := c.detectRunsForLease(ctx, leaseID)
	if err != nil {
		return LogResult{}, err
	}

	datamap := serviceNameAndReplicaIndexToPodName(possiblePods)

	podName, exists := datamap[serviceNameAndReplicaIndex{
		replicaIndex: int(replicaIndex),
		serviceName:  serviceName,
	}]

	if !exists {
		return LogResult{},fmt.Errorf("%w: no entry for service %q and replica %d", ErrLoki, serviceName, replicaIndex)
	}

	// Query using the pod name to get the result
	lc, err := c.getLokiClient(ctx)
	if err != nil {
		return LogResult{},err
	}

	// if runIndex >= 0 then launch a query to get a single log line back from the backend
	// then use that log line to determine the correct label to query on. Will need to parse it out and then
	// modify it to be the expected value
	specifyRunIndex := runIndex >= 0
	filenameLabelFilter := ""
	if specifyRunIndex  {
		filenameLabelFilter, err = c.getFilename(ctx, leaseID, startTime, endTime, runIndex, podName)
		if err != nil {
			return LogResult{}, err
		}
	}

	httpQueryString := url.Values{}
	httpQueryString.Set("start", fmt.Sprintf("%d", startTime.UnixNano()))
	httpQueryString.Set("end", fmt.Sprintf("%d", endTime.UnixNano()))
	httpQueryString.Set("limit", "1000") // TODO - configurable or something? Maybe user requestable?

	lokiQueryBuf := &bytes.Buffer{}
	// Note this is not JSON
	// TODO - guard against injection here even though it is unlikely
	_, _ = fmt.Fprint(lokiQueryBuf, "{")
	_, _ = fmt.Fprintf(lokiQueryBuf, "%s=%q", podLabel, podName)
	// specify filename label here if runIndex >= 0
	if specifyRunIndex {
		_, _ = fmt.Fprintf(lokiQueryBuf,",%s=%q", filenameLabel, filenameLabelFilter)
	}
	_, _ = fmt.Fprint(lokiQueryBuf, "}")

	httpQueryString.Set("query", lokiQueryBuf.String())

	direction := FORWARD
	if !forward {
		direction = BACKWARD
	}
	httpQueryString.Set("direction", string(direction))

	request, err := lc.CreateRequest(ctx, http.MethodGet, queryRangePath, nil )
	if err != nil {
		return LogResult{},err
	}
	request.URL.RawQuery = httpQueryString.Encode()

	request.Header.Add(lokiOrgIdHeader, lidNS)

	resp, err := lc.DoRequest(request)
	if err != nil {
		return LogResult{},fmt.Errorf("loki log request failed: %w", err)
	}

	if resp.StatusCode != 200 {
		buf := &bytes.Buffer{}
		_, _ = io.Copy(buf, resp.Body)
		msg := strings.Trim(buf.String(), "\n\t\r")
		return LogResult{},fmt.Errorf("%w: fetching logs from loki failed, got status code %d; %s", ErrLoki, resp.StatusCode, msg)
	}

	// Parse the response & grab the values we care about
	decoder := json.NewDecoder(resp.Body)
	lokiResult := lokiLogQueryResponse{}
	err = decoder.Decode(&lokiResult)
	if err != nil {
		return LogResult{},fmt.Errorf("loki log query response could not be decoded: %w", err)
	}

	result := LogResult{
		ServiceName:  serviceName,
		ReplicaIndex: replicaIndex,
		Entries:      nil,
	}
	for _, resultSet := range lokiResult.Data.Result {
		filepath, exists := resultSet.Stream[filenameLabel]
		if !exists {
			return LogResult{},fmt.Errorf("%w: expected loki log result set to have label %q but it does not", ErrLoki, filenameLabel)
		}

		_, filename := path.Split(filepath)
		filenameParts := strings.SplitN(filename, ".", 2)

		runIndex, err := strconv.ParseUint(filenameParts[0], 0, 31)
		if err != nil {
			return LogResult{},fmt.Errorf("expected to parse filename %q as integer for kubernetes run index: %w", filename, err)

		}
		for _, logEntry := range resultSet.Values {
			if len(logEntry) != 2 {
				return LogResult{},fmt.Errorf("%w: expected log entry to have 2 values, not %d", ErrLoki, len(logEntry))
			}
			timeStampStr := logEntry[0]

			timestamp, err := strconv.ParseInt(timeStampStr,0, 64)
			if err != nil {
				return LogResult{},fmt.Errorf("could not parse log entry timestamp %q: %w", timeStampStr, err)
			}

			at := time.Unix(0, timestamp)
			logLine := logEntry[1]

			result.Entries = append(result.Entries, LogResultLine{
				RunIndex: uint(runIndex),
				At:       at,
				Line:     logLine,
			})

		}
	}

	return result, nil
}

func (c *client) getLokiWebsocketClient(ctx context.Context) (clusterutil.WebsocketServiceClient, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// TODO - reset this client on error after HTTP request
	if c.websocketClient == nil {
		var err error
		c.websocketClient, err = c.sda.GetWebsocketClient(ctx, false, false)
		if err != nil {
			return nil, err
		}
	}
	return c.websocketClient, nil
}

func (c *client) getLokiClient(ctx context.Context) (clusterutil.ServiceClient, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// TODO - reset this client on error after HTTP request
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

// TODO - should this be an interface ?
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
		return nil, fmt.Errorf("loki label values request failed: %w", err)
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

	podNameToPodData := podNamesToServiceNameAndReplicaIndex(possiblePods)
	returnValue := make([]LogStatus, 0, len(podNameToPodData))
	positionData := make(map[serviceNameAndReplicaIndex]int)

	// By default nothing is found
	for possiblePodName, entry := range possiblePods {
		podData := podNameToPodData[possiblePodName]
		positionData[serviceNameAndReplicaIndex{
			replicaIndex: podData.replicaIndex,
			serviceName:  podData.serviceName,
		}] = len(returnValue)
		returnValue = append(returnValue, LogStatus{
			ServiceName:  entry.serviceName,
			ReplicaIndex: podData.replicaIndex,
		})
	}

	// Mark each pod that is found with logs
	for _, podName := range result.Data {
		podData, exists := podNameToPodData[podName]
		if ! exists {
			continue
		}
		i := positionData[podData]
		returnValue[i].Present = true
	}

	// TODO - query by pod name and then figure out how many restarts actually logged something?

	return returnValue, nil
}

type serviceNameAndReplicaIndex struct{
	replicaIndex int
	serviceName string
}

func serviceNameAndReplicaIndexToPodName(input map[string]runEntry) map[serviceNameAndReplicaIndex]string {
	result := make(map[serviceNameAndReplicaIndex]string)

	withPartitionedPods(input, func(podName string, serviceName string, replicaIndex int){
		result[serviceNameAndReplicaIndex{
			replicaIndex: replicaIndex,
			serviceName:  serviceName,
		}] = podName
	})

	return result
}

func withPartitionedPods(input map[string]runEntry, fn func(podName string, serviceName string,  replicaIndex int)) {

	// Assign pods a replica index by their name, so it is consistent. This is done elsewhere as well
	partitionedPods := make(map[string][]string)
	for podName, entry := range input {
		listForService := partitionedPods[entry.serviceName]

		listForService = append(listForService, podName)
		partitionedPods[entry.serviceName] = listForService
	}

	for serviceName, podNames := range partitionedPods {
		sort.Strings(podNames)

		for i, podName := range podNames {
			fn(podName, serviceName, i)
		}
	}
}

func podNamesToServiceNameAndReplicaIndex(input map[string]runEntry) map[string]serviceNameAndReplicaIndex {
	podNameToReplicaIndex := make(map[string]serviceNameAndReplicaIndex)

	withPartitionedPods(input, func(podName string, serviceName string, replicaIndex int){
		podNameToReplicaIndex[podName] = serviceNameAndReplicaIndex{
			replicaIndex: replicaIndex,
			serviceName:  serviceName,
		}
	})

	return podNameToReplicaIndex
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

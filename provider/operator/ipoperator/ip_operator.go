package ipoperator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/mux"
	"github.com/ovrclk/akash/provider/cluster"
	clusterClient "github.com/ovrclk/akash/provider/cluster/kube"
	"github.com/ovrclk/akash/provider/cluster/kube/metallb"
	"github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	ctypes "github.com/ovrclk/akash/provider/cluster/types/v1beta2"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	provider_flags "github.com/ovrclk/akash/provider/cmd/flags"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/libs/log"
	"golang.org/x/sync/errgroup"
	"io"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	ipoptypes "github.com/ovrclk/akash/provider/operator/ipoperator/types"
	"github.com/ovrclk/akash/provider/operator/operatorcommon"
)

const (
	serviceMetalLb = "metal-lb"
)

type ipOperator struct {
	state             map[string]managedIP
	client            cluster.Client
	log               log.Logger
	server            operatorcommon.OperatorHTTP
	leasesIgnored     operatorcommon.IgnoreList
	flagState         operatorcommon.PrepareFlagFn
	flagIgnoredLeases operatorcommon.PrepareFlagFn
	flagUsage         operatorcommon.PrepareFlagFn
	cfg               operatorcommon.OperatorConfig

	available uint
	inUse     uint

	mllbc metallb.Client

	barrier *barrier

	dataLock sync.Locker
}

func (op *ipOperator) monitorUntilError(parentCtx context.Context) error {
	var err error

	op.log.Info("associated provider ", "addr", op.cfg.ProviderAddress)

	op.state = make(map[string]managedIP)
	op.log.Info("fetching existing IP passthroughs")
	entries, err := op.mllbc.GetIPPassthroughs(parentCtx)
	if err != nil {
		return err
	}
	startupTime := time.Now()
	for _, ipPassThrough := range entries {
		k := getStateKey(ipPassThrough.GetLeaseID(), ipPassThrough.GetSharingKey(), ipPassThrough.GetExternalPort())
		op.state[k] = managedIP{
			presentLease:        ipPassThrough.GetLeaseID(),
			presentServiceName:  ipPassThrough.GetServiceName(),
			lastEvent:           nil,
			presentSharingKey:   ipPassThrough.GetSharingKey(),
			presentExternalPort: ipPassThrough.GetExternalPort(),
			presentPort:         ipPassThrough.GetPort(),
			lastChangedAt:       startupTime,
		}
	}
	op.flagState()

	// Get the present counts before starting
	err = op.updateCounts(parentCtx)
	if err != nil {
		return err
	}

	op.log.Info("starting observation")

	events, err := op.client.ObserveIPState(parentCtx)
	if err != nil {
		return err
	}

	if err := op.server.PrepareAll(); err != nil {
		return err
	}

	var exitError error

	pruneTicker := time.NewTicker(op.cfg.PruneInterval)
	defer pruneTicker.Stop()
	prepareTicker := time.NewTicker(op.cfg.WebRefreshInterval)
	defer prepareTicker.Stop()

	op.log.Info("barrier can now be passed")
	op.barrier.enable()
loop:
	for {
		isUpdating := false
		prepareData := false
		select {
		case <-parentCtx.Done():
			exitError = parentCtx.Err()
			break loop

		case ev, ok := <-events:
			if !ok {
				exitError = operatorcommon.ErrObservationStopped
				break loop
			}
			err = op.applyEvent(parentCtx, ev)
			if err != nil {
				op.log.Error("failed applying event", "err", err)
				exitError = err
				break loop
			}

			isUpdating = true
		case <-pruneTicker.C:
			op.leasesIgnored.Prune()
			op.flagIgnoredLeases()
			prepareData = true
		case <-prepareTicker.C:
			prepareData = true
		}

		if isUpdating {
			err = op.updateCounts(parentCtx)
			if err != nil {
				exitError = err
				break loop
			}
			isUpdating = false
			prepareData = true
		}

		if prepareData {
			if err := op.server.PrepareAll(); err != nil {
				op.log.Error("preparing web data failed", "err", err)
			}
		}
	}
	op.barrier.disable()

	// Wait for up to 30 seconds
	ctxWithTimeout, timeoutCtxCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer timeoutCtxCancel()

	err = op.barrier.waitUntilClear(ctxWithTimeout)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			op.log.Error("did not clear barrier in given time")
		} else {
			op.log.Error("failed waiting on barrier to clear", "err", err)
		}
	}

	op.log.Info("ip operator done")

	return exitError
}

func (op *ipOperator) updateCounts(parentCtx context.Context) error {
	// This is tried in a loop, don't wait for a long period of time for a response
	ctx, cancel := context.WithTimeout(parentCtx, time.Minute)
	defer cancel()
	inUse, available, err := op.mllbc.GetIPAddressUsage(ctx)
	if err != nil {
		return err
	}

	op.dataLock.Lock()
	defer op.dataLock.Unlock()
	op.inUse = inUse
	op.available = available

	op.flagUsage()
	op.log.Info("ip address inventory", "in-use", op.inUse, "available", op.available)
	return nil
}

func (op *ipOperator) recordEventError(ev v1beta2.IPResourceEvent, failure error) {
	// ff no error, no action
	if failure == nil {
		return
	}

	mark := kubeErrors.IsNotFound(failure)

	if !mark {
		return
	}

	op.log.Info("recording error for", "lease", ev.GetLeaseID().String(), "err", failure)
	op.leasesIgnored.AddError(ev.GetLeaseID(), failure, ev.GetSharingKey())
	op.flagIgnoredLeases()
}

func (op *ipOperator) applyEvent(ctx context.Context, ev v1beta2.IPResourceEvent) error {
	op.log.Debug("apply event", "event-type", ev.GetEventType(), "lease", ev.GetLeaseID())
	switch ev.GetEventType() {
	case ctypes.ProviderResourceDelete:
		// note that on delete the resource might be gone anyways because the namespace is deleted
		return op.applyDeleteEvent(ctx, ev)
	case ctypes.ProviderResourceAdd, ctypes.ProviderResourceUpdate:
		if op.leasesIgnored.IsFlagged(ev.GetLeaseID()) {
			op.log.Info("ignoring event for", "lease", ev.GetLeaseID().String())
			return nil
		}
		err := op.applyAddOrUpdateEvent(ctx, ev)
		op.recordEventError(ev, err)
		return err
	default:
		return fmt.Errorf("%w: unknown event type %v", operatorcommon.ErrObservationStopped, ev.GetEventType())
	}
}

func (op *ipOperator) applyDeleteEvent(parentCtx context.Context, ev v1beta2.IPResourceEvent) error {
	directive := buildIPDirective(ev)

	// Delete events are a one-shot type thing. The operator always queries for existing CRDs but can't
	// query for the non-existence of something. The timeout used here is considerably higher as a result
	// In the future the operator can be improved by adding an optional purge routing which seeks out kube resources
	// for services that allocate an IP but that do not belong to at least 1 CRD
	ctx, cancel := context.WithTimeout(parentCtx, time.Minute*5)
	defer cancel()
	err := op.mllbc.PurgeIPPassthrough(ctx, directive)

	if err == nil {
		uid := getStateKey(ev.GetLeaseID(), ev.GetSharingKey(), ev.GetExternalPort())
		delete(op.state, uid)
		op.flagState()
	}

	return err
}

func buildIPDirective(ev v1beta2.IPResourceEvent) ctypes.ClusterIPPassthroughDirective {
	return ctypes.ClusterIPPassthroughDirective{
		LeaseID:      ev.GetLeaseID(),
		ServiceName:  ev.GetServiceName(),
		Port:         ev.GetPort(),
		ExternalPort: ev.GetExternalPort(),
		SharingKey:   ev.GetSharingKey(),
		Protocol:     ev.GetProtocol(),
	}
}

func getStateKey(leaseID mtypes.LeaseID, sharingKey string, externalPort uint32) string {
	// TODO - need to double check this makes sense
	return fmt.Sprintf("%v-%s-%d", leaseID.GetOwner(), sharingKey, externalPort)
}

func (op *ipOperator) applyAddOrUpdateEvent(ctx context.Context, ev v1beta2.IPResourceEvent) error {
	leaseID := ev.GetLeaseID()

	uid := getStateKey(ev.GetLeaseID(), ev.GetSharingKey(), ev.GetExternalPort())

	op.log.Info("connecting",
		"lease", leaseID,
		"service", ev.GetServiceName(),
		"externalPort", ev.GetExternalPort())
	entry, exists := op.state[uid]

	isSameLease := true
	if exists {
		isSameLease = entry.presentLease.Equals(leaseID)
	}

	directive := buildIPDirective(ev)

	var err error
	shouldConnect := false

	if isSameLease {
		if !exists {
			shouldConnect = true
			op.log.Debug("ip passthrough is new, applying", "lease", leaseID)
			// Check to see if port or service name is different
		} else {
			hasChanged := entry.presentServiceName != ev.GetServiceName() ||
				entry.presentPort != ev.GetPort() ||
				entry.presentSharingKey != ev.GetSharingKey() ||
				entry.presentExternalPort != ev.GetExternalPort() ||
				entry.presentProtocol != ev.GetProtocol()
			if hasChanged {
				shouldConnect = true
				op.log.Debug("ip passthrough has changed, applying", "lease", leaseID)
			}
		}

		if shouldConnect {
			op.log.Debug("Updating ip passthrough", "lease", leaseID)
			err = op.mllbc.CreateIPPassthrough(ctx, directive)
		}
	} else {

		/** TODO - the sharing key keeps the IP the same unless
		this directive purges all the services using that key. This creates
		a problem where the IP could change. This is not the desired behavior in the system
		We may need to add a bogus service here temporarily to prevent that from happening
		*/
		deleteDirective := ctypes.ClusterIPPassthroughDirective{
			LeaseID:      entry.presentLease,
			ServiceName:  entry.presentServiceName,
			Port:         entry.presentPort,
			ExternalPort: entry.presentExternalPort,
			SharingKey:   entry.presentSharingKey,
			Protocol:     entry.presentProtocol,
		}
		// Delete the entry & recreate it with the new lease associated  to it
		err = op.mllbc.PurgeIPPassthrough(ctx, deleteDirective)
		if err != nil {
			return err
		}
		// Remove the current value from the state
		delete(op.state, uid)
		err = op.mllbc.CreateIPPassthrough(ctx, directive)
	}

	if err != nil {
		return err
	}

	// Update stored entry
	entry.presentServiceName = ev.GetServiceName()
	entry.presentLease = leaseID
	entry.lastEvent = ev
	entry.presentExternalPort = ev.GetExternalPort()
	entry.presentSharingKey = ev.GetSharingKey()
	entry.presentPort = ev.GetPort()
	entry.presentProtocol = ev.GetProtocol()
	entry.lastChangedAt = time.Now()
	op.state[uid] = entry
	op.flagState()

	op.log.Info("update complete", "lease", leaseID)

	return nil
}

func (op *ipOperator) prepareUsage(pd operatorcommon.PreparedResult) error {
	op.dataLock.Lock()
	defer op.dataLock.Unlock()
	value := ipoptypes.IPAddressUsage{
		Available: op.available,
		InUse:     op.inUse,
	}

	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)

	err := encoder.Encode(value)
	if err != nil {
		return err
	}

	pd.Set(buf.Bytes())
	return nil
}

func (op *ipOperator) prepareState(pd operatorcommon.PreparedResult) error {
	results := make(map[string][]interface{})
	for _, mnagedIPEntry := range op.state {
		leaseID := mnagedIPEntry.presentLease

		result := struct {
			LastChangeTime string         `json:"last-event-time,omitempty"`
			LeaseID        mtypes.LeaseID `json:"lease-id"`
			Namespace      string         `json:"namespace"` // diagnostic only
			Port           uint32         `json:"port"`
			ExternalPort   uint32         `json:"external-port"`
			ServiceName    string         `json:"service-name"`
			SharingKey     string         `json:"sharing-key"`
		}{
			LeaseID:        leaseID,
			Namespace:      clusterutil.LeaseIDToNamespace(leaseID),
			Port:           mnagedIPEntry.presentPort,
			ExternalPort:   mnagedIPEntry.presentExternalPort,
			ServiceName:    mnagedIPEntry.presentServiceName,
			SharingKey:     mnagedIPEntry.presentSharingKey,
			LastChangeTime: mnagedIPEntry.lastChangedAt.UTC().String(),
		}

		entryList := results[leaseID.String()]
		entryList = append(entryList, result)
		results[leaseID.String()] = entryList
	}

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	err := enc.Encode(results)
	if err != nil {
		return err
	}

	pd.Set(buf.Bytes())
	return nil
}

func handleHTTPError(op *ipOperator, rw http.ResponseWriter, req *http.Request, err error, status int) {
	op.log.Error("http request processing failed", "method", req.Method, "path", req.URL.Path, "err", err)
	rw.WriteHeader(status)

	body := ipoptypes.IPOperatorErrorResponse{
		Error: err.Error(),
		Code:  -1,
	}

	if errors.Is(err, ipoptypes.ErrIPOperator) {
		code := err.(ipoptypes.IPOperatorError).GetCode()
		body.Code = code
	}

	encoder := json.NewEncoder(rw)
	err = encoder.Encode(body)
	if err != nil {
		op.log.Error("failed writing response body", "err", err)
	}
}

func newIPOperator(logger log.Logger, client cluster.Client, cfg operatorcommon.OperatorConfig, ilc operatorcommon.IgnoreListConfig, mllbc metallb.Client) (*ipOperator, error) {
	opHTTP, err := operatorcommon.NewOperatorHTTP()
	if err != nil {
		return nil, err
	}
	retval := &ipOperator{
		state:         make(map[string]managedIP),
		client:        client,
		log:           logger,
		server:        opHTTP,
		leasesIgnored: operatorcommon.NewIgnoreList(ilc),
		mllbc:         mllbc,
		dataLock:      &sync.Mutex{},
		barrier:       &barrier{},
		cfg:           cfg,
	}

	retval.server.GetRouter().Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if !retval.barrier.enter() {
				retval.log.Error("barrier is locked, can't service request", "path", req.URL.Path)
				rw.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(rw, req)
			retval.barrier.exit()
		})
	})

	retval.flagState = retval.server.AddPreparedEndpoint("/state", retval.prepareState)
	retval.flagIgnoredLeases = retval.server.AddPreparedEndpoint("/ignored-leases", retval.leasesIgnored.Prepare)
	retval.flagUsage = retval.server.AddPreparedEndpoint("/usage", retval.prepareUsage)

	retval.server.GetRouter().HandleFunc("/health", func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, "OK")
	})

	// TODO - add auth based off TokenReview via k8s interface to below methods OR just cache these so they can't abuse kube
	retval.server.GetRouter().HandleFunc("/ip-lease-status/{owner}/{dseq}/{gseq}/{oseq}", func(rw http.ResponseWriter, req *http.Request) {
		handleIPLeaseStatusGet(retval, rw, req)
	}).Methods(http.MethodGet)
	return retval, nil
}

func handleIPLeaseStatusGet(op *ipOperator, rw http.ResponseWriter, req *http.Request) {
	// Extract path variables, returning 404 if any are invalid
	vars := mux.Vars(req)
	dseqStr := vars["dseq"]
	dseq, err := strconv.ParseUint(dseqStr, 10, 64)
	if err != nil {
		op.log.Error("could not parse path component as uint64", "dseq", dseqStr, "error", err)
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	gseqStr := vars["gseq"]
	gseq, err := strconv.ParseUint(gseqStr, 10, 32)
	if err != nil {
		op.log.Error("could not parse path component as uint32", "gseq", gseqStr, "error", err)
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	oseqStr := vars["oseq"]
	oseq, err := strconv.ParseUint(oseqStr, 10, 32)
	if err != nil {
		op.log.Error("could not parse path component as uint32", "oseq", oseqStr, "error", err)
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	owner := vars["owner"]
	_, err = sdk.AccAddressFromBech32(owner) // Validate this is a bech32 address
	if err != nil {
		op.log.Error("could not parse owner address as bech32", "onwer", owner, "error", err)
		rw.WriteHeader(http.StatusNotFound)
		return
	}

	leaseID := mtypes.LeaseID{
		Owner:    owner,
		DSeq:     dseq,
		GSeq:     uint32(gseq),
		OSeq:     uint32(oseq),
		Provider: op.cfg.ProviderAddress,
	}

	ipStatus, err := op.mllbc.GetIPAddressStatusForLease(req.Context(), leaseID)
	if err != nil {
		op.log.Error("Could not get IP address status", "lease-id", leaseID, "error", err)
		handleHTTPError(op, rw, req, err, http.StatusInternalServerError)
		return
	}

	if len(ipStatus) == 0 {
		rw.WriteHeader(http.StatusNoContent)
		return
	}

	rw.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(rw)
	// ipStatus is a slice of interface types, so it can't be encoded directly
	responseData := make([]ipoptypes.LeaseIPStatus, len(ipStatus))
	for i, v := range ipStatus {
		responseData[i] = ipoptypes.LeaseIPStatus{
			Port:         v.GetPort(),
			ExternalPort: v.GetExternalPort(),
			ServiceName:  v.GetServiceName(),
			IP:           v.GetIP(),
			Protocol:     v.GetProtocol().ToString(),
		}
	}
	err = encoder.Encode(responseData)
	if err != nil {
		op.log.Error("failed writing JSON of ip status response", "error", err)
	}
}

func doIPOperator(cmd *cobra.Command) error {
	configPath := viper.GetString(provider_flags.FlagKubeConfig)
	ns := viper.GetString(provider_flags.FlagK8sManifestNS)
	listenAddr := viper.GetString(provider_flags.FlagListenAddress)
	poolName := viper.GetString(flagMetalLbPoolName)
	logger := operatorcommon.OpenLogger().With("operator", "ip")

	opcfg := operatorcommon.GetOperatorConfigFromViper()
	_, err := sdk.AccAddressFromBech32(opcfg.ProviderAddress)
	if err != nil {
		return fmt.Errorf("%w: provider address must valid bech32", err)
	}

	client, err := clusterClient.NewClient(logger, ns, configPath)
	if err != nil {
		return err
	}

	metalLbEndpoint, err := provider_flags.GetServiceEndpointFlagValue(logger, serviceMetalLb)
	if err != nil {
		return err
	}

	mllbc, err := metallb.NewClient(configPath, logger, poolName, metalLbEndpoint)
	if err != nil {
		return err
	}

	logger.Info("clients", "kube", client, "metallb", mllbc)
	logger.Info("HTTP listening", "address", listenAddr)

	op, err := newIPOperator(logger, client, opcfg, operatorcommon.IgnoreListConfigFromViper(), mllbc)
	if err != nil {
		return err
	}
	router := op.server.GetRouter()
	group, ctx := errgroup.WithContext(cmd.Context())

	group.Go(func() error {
		srv := http.Server{Addr: listenAddr, Handler: router}
		go func() {
			<-ctx.Done()
			_ = srv.Close()
		}()
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})

	group.Go(func() error {
		return op.run(ctx)
	})

	err = group.Wait()
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (op *ipOperator) run(parentCtx context.Context) error {
	op.log.Info("ip operator start")
	for {
		lastAttempt := time.Now()
		err := op.monitorUntilError(parentCtx)
		if errors.Is(err, context.Canceled) {
			op.log.Debug("ip operator terminate")
			break
		}

		op.log.Error("observation stopped", "err", err)

		// don't spin if there is a condition causing fast failure
		elapsed := time.Since(lastAttempt)
		if elapsed < op.cfg.RetryDelay {
			op.log.Info("delaying")
			select {
			case <-parentCtx.Done():
				return parentCtx.Err()
			case <-time.After(op.cfg.RetryDelay):
				// delay complete
			}
		}
	}

	op.mllbc.Stop()
	return parentCtx.Err()
}

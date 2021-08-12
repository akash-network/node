package bidengine

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/boz/go-lifecycle"

	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	"github.com/ovrclk/akash/types"
	atypes "github.com/ovrclk/akash/x/audit/types"
	ptypes "github.com/ovrclk/akash/x/provider/types"
)

const (
	attrFetchRetryPeriod = 5 * time.Second
	attrReqTimeout       = 5 * time.Second
)

var (
	errShuttingDown = errors.New("provider attribute signature service is shutting down")

	invalidProviderPattern = regexp.MustCompile("^.*invalid provider: address not found.*$")
)

type attrRequest struct {
	successCh chan<- types.Attributes
	errCh     chan<- error
}

type auditedAttrRequest struct {
	auditor   string
	successCh chan<- []atypes.Provider
	errCh     chan<- error
}

type providerAttrEntry struct {
	providerAttr []atypes.Provider
	at           time.Time
}

type auditedAttrResult struct {
	auditor      string
	providerAttr []atypes.Provider
	err          error
}

type ProviderAttrSignatureService interface {
	GetAuditorAttributeSignatures(auditor string) ([]atypes.Provider, error)
	GetAttributes() (types.Attributes, error)
}

type providerAttrSignatureService struct {
	providerAddr string
	lc           lifecycle.Lifecycle
	requests     chan auditedAttrRequest

	reqAttr         chan attrRequest
	currAttr        chan types.Attributes
	fetchAttr       chan struct{}
	pushAttr        chan struct{}
	fetchInProgress chan struct{}
	newAttr         chan types.Attributes
	errFetchAttr    chan error

	session session.Session
	fetchCh chan auditedAttrResult

	data       map[string]providerAttrEntry
	inProgress map[string]struct{}
	pending    map[string][]auditedAttrRequest

	wg sync.WaitGroup

	sub pubsub.Subscriber

	ttl time.Duration

	attr types.Attributes
}

func newProviderAttrSignatureService(s session.Session, bus pubsub.Bus) (*providerAttrSignatureService, error) {
	return newProviderAttrSignatureServiceInternal(s, bus, 18*time.Hour)
}

func newProviderAttrSignatureServiceInternal(s session.Session, bus pubsub.Bus, ttl time.Duration) (*providerAttrSignatureService, error) {
	subscriber, err := bus.Subscribe()
	if err != nil {
		return nil, err
	}
	retval := &providerAttrSignatureService{
		providerAddr: s.Provider().Owner,
		lc:           lifecycle.New(),
		session:      s,
		requests:     make(chan auditedAttrRequest),
		fetchCh:      make(chan auditedAttrResult),
		data:         make(map[string]providerAttrEntry),
		pending:      make(map[string][]auditedAttrRequest),
		inProgress:   make(map[string]struct{}),

		reqAttr:         make(chan attrRequest, 1),
		currAttr:        make(chan types.Attributes, 1),
		fetchAttr:       make(chan struct{}, 1),
		pushAttr:        make(chan struct{}, 1),
		fetchInProgress: make(chan struct{}, 1),
		newAttr:         make(chan types.Attributes),
		errFetchAttr:    make(chan error, 1),

		sub: subscriber,
		ttl: ttl,
	}

	go retval.run()

	return retval, nil
}

func (pass *providerAttrSignatureService) run() {
	defer pass.sub.Close()
	defer pass.lc.ShutdownCompleted()

	ctx, cancel := context.WithCancel(context.Background())

	pass.fetchAttributes()

loop:
	for {
		select {
		case ev := <-pass.sub.Events():
			pass.handleEvent(ev)
		case <-pass.lc.ShutdownRequest():
			pass.lc.ShutdownInitiated(nil)
			break loop
		case request := <-pass.requests:
			start := pass.addRequest(request)
			if start {
				pass.maybeStart(ctx, request.auditor)
			}
		case result := <-pass.fetchCh:
			if result.err != nil {
				pass.failAllPending(result.auditor, result.err)
			} else {
				pass.completeAllPending(result.auditor, result.providerAttr)
			}
			delete(pass.pending, result.auditor)
		case <-pass.fetchAttr:
			pass.tryFetchAttributes(ctx)
		case req := <-pass.reqAttr:
			pass.processAttrReq(ctx, req)
		case <-pass.pushAttr:
			select {
			case pass.currAttr <- pass.attr:
			default:
			}
		case attr := <-pass.newAttr:
			// todo fetch current cluster storage inventory
			pass.attr = attr
			pass.pushCurrAttributes()
		case <-pass.errFetchAttr:
			// if attributes fetch fails give it retry within reasonable timeout
			time.AfterFunc(attrFetchRetryPeriod, func() {
				pass.fetchAttributes()
			})
		}
	}

	cancel()
	pass.wg.Wait()
}

func (pass *providerAttrSignatureService) purgeAuditor(auditor string) {
	delete(pass.data, auditor)
}

func (pass *providerAttrSignatureService) fetchAttributes() {
	select {
	case pass.fetchAttr <- struct{}{}:
	default:
		return
	}
}

func (pass *providerAttrSignatureService) pushCurrAttributes() {
	select {
	case pass.pushAttr <- struct{}{}:
	default:
	}
}

func (pass *providerAttrSignatureService) handleEvent(ev pubsub.Event) {
	switch ev := ev.(type) {
	case atypes.EventTrustedAuditorCreated:
		if ev.Owner.String() == pass.providerAddr {
			pass.purgeAuditor(ev.Auditor.String())
		}
	case atypes.EventTrustedAuditorDeleted:
		if ev.Owner.String() == pass.providerAddr {
			pass.purgeAuditor(ev.Auditor.String())
		}
	case ptypes.EventProviderUpdated:
		if ev.Owner.String() == pass.providerAddr {
			pass.fetchAttributes()
		}
	default:
		// Ignore the event, we don't need it
	}
}

func (pass *providerAttrSignatureService) failAllPending(auditor string, err error) {
	pendingForAuditor := pass.pending[auditor]
	for _, req := range pendingForAuditor {
		req.errCh <- err
	}
	delete(pass.pending, auditor)
}

func (pass *providerAttrSignatureService) completeAllPending(auditor string, result []atypes.Provider) {
	pendingForAuditor := pass.pending[auditor]
	delete(pass.pending, auditor)

	// Store in cache for later usage
	pass.data[auditor] = providerAttrEntry{
		providerAttr: result,
		at:           time.Now(),
	}

	// Fill all requests
	for _, req := range pendingForAuditor {
		req.successCh <- result
	}

	pass.trimCache()
}

func providerAttrSize(entries []atypes.Provider) int {
	size := 0
	for _, x := range entries {
		size += len(x.Attributes)
	}
	return size
}

func (pass *providerAttrSignatureService) trimCache() {
	const maxEntries = 50000

	toDelete := make([]string, 0, 4)
	now := time.Now()
	size := 0
	for auditor, entry := range pass.data {
		elapsed := now.Sub(entry.at)
		expired := elapsed > pass.ttl
		if expired {
			toDelete = append(toDelete, auditor)
		} else {
			size += providerAttrSize(entry.providerAttr)
		}
	}

	// Remove expired entries
	for _, auditor := range toDelete {
		delete(pass.data, auditor)
	}
	toDelete = nil

	// Check if size is larger than what is wanted
	if size > maxEntries {
		pass.session.Log().Info("provider attr. cache size too large, pruning", "size", size)
	} else {
		return
	}

	// Remove approx. half of the stored values
	const target = maxEntries / 2
	size = 0
	// Map iteration order in golang is random
	for auditor, entry := range pass.data {
		size += providerAttrSize(entry.providerAttr)
		toDelete = append(toDelete, auditor)
		if size >= target {
			break
		}
	}

	// Delete entries to get the size back down
	for _, auditor := range toDelete {
		delete(pass.data, auditor)
	}
}

func (pass *providerAttrSignatureService) maybeStart(ctx context.Context, auditor string) {
	// Check that request exists
	if pendingForAuditor := pass.pending[auditor]; len(pendingForAuditor) == 0 {
		return
	}
	// Check that request is not in flight
	_, exists := pass.inProgress[auditor]
	if exists {
		return
	}

	pass.wg.Add(1)
	go func() {
		defer pass.wg.Done()
		pass.fetchCh <- pass.fetch(ctx, auditor)
	}()
}

func (pass *providerAttrSignatureService) fetch(ctx context.Context, auditor string) auditedAttrResult {
	req := &atypes.QueryProviderAuditorRequest{
		Owner:   pass.providerAddr,
		Auditor: auditor,
	}

	pass.session.Log().Info("fetching provider auditor attributes", "auditor", req.Auditor, "provider", req.Owner)
	result, err := pass.session.Client().Query().ProviderAuditorAttributes(ctx, req)
	if err != nil {
		// Error type is always "errors.fundamental" so use pattern matching here
		if invalidProviderPattern.MatchString(err.Error()) {
			return auditedAttrResult{auditor: auditor} // No data
		}
		return auditedAttrResult{auditor: auditor, err: err}
	}

	value := result.GetProviders()
	pass.session.Log().Info("got auditor attributes", "auditor", auditor, "size", providerAttrSize(value))

	return auditedAttrResult{
		auditor:      auditor,
		providerAttr: value,
	}
}

func (pass *providerAttrSignatureService) addRequest(request auditedAttrRequest) bool {
	entry, present := pass.data[request.auditor]

	if present { // Cached value is present
		elapsed := time.Since(entry.at) // Check if it is too old
		if elapsed < pass.ttl {
			request.successCh <- entry.providerAttr
			pass.session.Log().Debug("reused auditor attributes", "auditor", request.auditor, "elapsed", elapsed)
			return false
		}
	}

	pendingList := pass.pending[request.auditor]
	pendingList = append(pendingList, request)
	pass.pending[request.auditor] = pendingList

	return true
}

func (pass *providerAttrSignatureService) GetAuditorAttributeSignatures(auditor string) ([]atypes.Provider, error) {
	successCh := make(chan []atypes.Provider, 1)
	errCh := make(chan error, 1)
	req := auditedAttrRequest{
		auditor:   auditor,
		successCh: successCh,
		errCh:     errCh,
	}

	select {
	case pass.requests <- req:
	case <-pass.lc.ShuttingDown():
		return nil, errShuttingDown
	}

	select {
	case <-pass.lc.ShuttingDown():
		return nil, errShuttingDown
	case err := <-errCh:
		return nil, err
	case result := <-successCh:
		return result, nil
	}
}

func (pass *providerAttrSignatureService) GetAttributes() (types.Attributes, error) {
	successCh := make(chan types.Attributes, 1)
	errCh := make(chan error, 1)

	req := attrRequest{
		successCh: successCh,
		errCh:     errCh,
	}

	select {
	case pass.reqAttr <- req:
	case <-pass.lc.ShuttingDown():
		return nil, errShuttingDown
	}

	select {
	case <-pass.lc.ShuttingDown():
		return nil, errShuttingDown
	case err := <-errCh:
		return nil, err
	case result := <-successCh:
		return result, nil
	}
}

func (pass *providerAttrSignatureService) tryFetchAttributes(ctx context.Context) {
	select {
	case pass.fetchInProgress <- struct{}{}:
		go func() {
			var err error
			defer func() {
				<-pass.fetchInProgress
				if err != nil {
					pass.errFetchAttr <- err
				}
			}()

			var result *ptypes.QueryProviderResponse

			req := &ptypes.QueryProviderRequest{
				Owner: pass.providerAddr,
			}

			result, err = pass.session.Client().Query().Provider(ctx, req)
			if err != nil {
				pass.session.Log().Error("fetching provider attributes", "provider", req.Owner)
				return
			}
			pass.session.Log().Info("fetched provider attributes", "provider", req.Owner)

			pass.newAttr <- result.Provider.Attributes
		}()
	default:
		return
	}
}

func (pass *providerAttrSignatureService) processAttrReq(ctx context.Context, req attrRequest) {
	go func() {
		ctx, cancel := context.WithTimeout(ctx, attrReqTimeout)
		defer cancel()

		select {
		case <-ctx.Done():
			req.errCh <- ctx.Err()
		case attr := <-pass.currAttr:
			req.successCh <- attr
			pass.pushCurrAttributes()
		}
	}()
}

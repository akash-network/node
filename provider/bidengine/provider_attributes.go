package bidengine

import (
	"context"
	"errors"
	"github.com/boz/go-lifecycle"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/pubsub"
	atypes "github.com/ovrclk/akash/x/audit/types"
	"regexp"
	"sync"
	"time"
)

type ProviderAttrSignatureService interface {
	GetAuditorAttributeSignatures(auditor string) ([]atypes.Provider, error)
}

type providerAttrSignatureService struct {
	providerAddr string
	lc           lifecycle.Lifecycle
	requests     chan providerAttrRequest

	session session.Session
	fetchCh chan providerAttrResult

	data       map[string]providerAttrEntry
	inProgress map[string]struct{}
	pending    map[string][]providerAttrRequest

	wg sync.WaitGroup

	sub pubsub.Subscriber

	ttl time.Duration
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
		requests:     make(chan providerAttrRequest),
		fetchCh:      make(chan providerAttrResult),
		data:         make(map[string]providerAttrEntry),
		pending:      make(map[string][]providerAttrRequest),
		inProgress:   make(map[string]struct{}),
		sub:          subscriber,
		ttl:          ttl,
	}

	go retval.run()

	return retval, nil
}

func (pass *providerAttrSignatureService) run() {
	defer pass.sub.Close()
	defer pass.lc.ShutdownCompleted()

	ctx, cancel := context.WithCancel(context.Background())

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
		}
	}

	cancel()
	pass.wg.Wait()
}

func (pass *providerAttrSignatureService) purgeAuditor(auditor string) {
	delete(pass.data, auditor)
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

var invalidProviderPattern = regexp.MustCompile("^.*invalid provider: address not found.*$")

func (pass *providerAttrSignatureService) fetch(ctx context.Context, auditor string) providerAttrResult {
	req := &atypes.QueryProviderAuditorRequest{
		Owner:   pass.providerAddr,
		Auditor: auditor,
	}

	pass.session.Log().Info("fetching provider auditor attributes", "auditor", req.Auditor, "provider", req.Owner)
	result, err := pass.session.Client().Query().ProviderAuditorAttributes(ctx, req)
	if err != nil {
		// Error type is always "errors.fundamental" so use pattern matching here
		if invalidProviderPattern.MatchString(err.Error()) {
			return providerAttrResult{auditor: auditor} // No data
		}
		return providerAttrResult{auditor: auditor, err: err}
	}

	value := result.GetProviders()
	pass.session.Log().Info("got auditor attributes", "auditor", auditor, "size", providerAttrSize(value))

	return providerAttrResult{
		auditor:      auditor,
		providerAttr: value,
	}
}

func (pass *providerAttrSignatureService) addRequest(request providerAttrRequest) bool {
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

var errShuttingDown = errors.New("provider attribute signature service is shutting down")

func (pass *providerAttrSignatureService) GetAuditorAttributeSignatures(auditor string) ([]atypes.Provider, error) {
	successCh := make(chan []atypes.Provider, 1)
	errCh := make(chan error, 1)
	req := providerAttrRequest{
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

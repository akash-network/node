package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

const (
	expiryMaxRequests = 256
	expiryMaxResponse = 2 << 20
	expiryMaxTotal    = 64 << 20
	expiryTimeout     = 120 * time.Second
	expiryRequestTime = 8 * time.Second
	expiryRequestGap  = 260 * time.Millisecond
)

type expiryRPCError struct {
	method     string
	cause      error
	retryAfter time.Duration
}

func (e *expiryRPCError) Error() string {
	return "expiry RPC " + e.method + " failed; durable intent retained"
}

func (e *expiryRPCError) Unwrap() error { return e.cause }

type expiryRPC struct {
	service *service
	started time.Time
	last    time.Time
	calls   int
	bytes   int
}

func (r *expiryRPC) call(ctx context.Context, method string, params []any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := r.service.deps.now()
	if r.calls >= expiryMaxRequests || now.Sub(r.started) >= expiryTimeout {
		return nil, fmt.Errorf("expiry certification request or time bound exhausted")
	}
	if !r.last.IsZero() {
		if gap := expiryRequestGap - now.Sub(r.last); gap > 0 {
			if err := r.service.deps.sleep(ctx, gap); err != nil {
				return nil, err
			}
		}
	}
	r.last = r.service.deps.now()
	if r.last.Sub(r.started) >= expiryTimeout {
		return nil, fmt.Errorf("expiry certification time bound exhausted")
	}
	r.calls++
	requestCtx, cancel := context.WithTimeout(ctx, expiryRequestTime)
	defer cancel()
	var raw json.RawMessage
	var retryAfter time.Duration
	err := r.service.solana.RPCCallWithCallback(requestCtx, method, params, func(request *http.Request, response *http.Response) error {
		if seconds, err := strconv.ParseUint(response.Header.Get("Retry-After"), 10, 32); err == nil {
			retryAfter = time.Duration(seconds) * time.Second
		} else if at, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			retryAfter = max(0, at.Sub(r.service.deps.now()))
		}
		limit := min(expiryMaxResponse, expiryMaxTotal-r.bytes)
		data, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
		r.bytes += len(data)
		if err != nil || len(data) > limit {
			return fmt.Errorf("expiry response payload limit or read failure")
		}
		if err := expiryUniqueJSON(data); err != nil {
			return err
		}
		buffered := *response
		buffered.Body = io.NopCloser(bytes.NewReader(data))
		raw, err = historyResponse(request, &buffered)
		return err
	})
	if err != nil {
		if requestCtx.Err() != nil {
			return nil, requestCtx.Err()
		}
		return nil, &expiryRPCError{
			method:     method,
			cause:      err,
			retryAfter: retryAfter,
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.service.deps.now().Sub(r.started) >= expiryTimeout {
		return nil, fmt.Errorf("expiry certification time bound exhausted")
	}
	return raw, nil
}

// Ambiguous duplicate properties are not certificate evidence. Legacy journal
// decoding retains its existing JSON.parse semantics elsewhere.
func expiryUniqueJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("expiry JSON nesting limit")
		}
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid expiry JSON")
		}
		switch token {
		case json.Delim('{'):
			seen := make(map[string]bool)
			for decoder.More() {
				name, err := decoder.Token()
				key, ok := name.(string)
				if err != nil || !ok || seen[key] {
					return fmt.Errorf("duplicate or invalid expiry JSON property")
				}
				seen[key] = true
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return fmt.Errorf("invalid expiry JSON object")
			}
		case json.Delim('['):
			for decoder.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return fmt.Errorf("invalid expiry JSON array")
			}
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing expiry JSON")
	}
	return nil
}

func expiryHash(value string) bool {
	hash, err := solana.HashFromBase58(value)
	return err == nil && !hash.IsZero() && hash.String() == value
}

func (r *expiryRPC) genesis(ctx context.Context) error {
	raw, err := r.call(ctx, "getGenesisHash", nil)
	if err != nil {
		return err
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || !expiryHash(value) || value != r.service.genesis || value != r.service.store.identity.Genesis {
		return fmt.Errorf("expiry genesis mismatch")
	}
	return nil
}

func (r *expiryRPC) absentStatus(ctx context.Context, doc *intent, minimum uint64) (uint64, error) {
	raw, err := r.call(ctx, "getSignatureStatuses", []any{[]string{doc.Signature}, rpc.M{"searchTransactionHistory": true}})
	if err != nil {
		return 0, err
	}
	fields, err := jsonObject(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid expiry status response")
	}
	slot, err := expiryContext(fields)
	if err != nil || slot < minimum {
		return 0, fmt.Errorf("missing or stale expiry status context")
	}
	var values []json.RawMessage
	if historyField(fields, "value", &values) != nil || len(values) != 1 || !bytes.Equal(bytes.TrimSpace(values[0]), []byte("null")) {
		return 0, fmt.Errorf("original status is not explicitly absent; retained")
	}
	return slot, nil
}

func expiryContext(fields map[string]json.RawMessage) (uint64, error) {
	contextFields, err := jsonObject(fields["context"])
	if err != nil {
		return 0, err
	}
	return historyUint(contextFields["slot"])
}

func (p *payerSession) explicitExpiredStatus(ctx context.Context, doc *intent) error {
	r := expiryRPC{
		service: p.service,
		started: p.service.deps.now(),
	}
	_, err := r.absentStatus(ctx, doc, 0)
	return err
}

func (r *expiryRPC) absentReceipt(ctx context.Context, doc *intent) error {
	raw, err := r.call(ctx, "getTransaction", []any{doc.Signature, rpc.M{
		"commitment":                     "finalized",
		"encoding":                       "base64",
		"maxSupportedTransactionVersion": 0,
	}})
	if err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("original finalized receipt is not explicitly absent; retained")
	}
	return nil
}

type expiryBlock struct {
	Slot              uint64   `json:"slot"`
	Height            uint64   `json:"blockHeight"`
	ParentSlot        uint64   `json:"parentSlot"`
	Blockhash         string   `json:"blockhash"`
	PreviousBlockhash string   `json:"previousBlockhash"`
	Signatures        []string `json:"signatures"`
}

func (b expiryBlock) validate() error {
	if b.Slot > uint64(maxSafeInteger) || b.Height > b.Slot || b.ParentSlot >= b.Slot || !expiryHash(b.Blockhash) || !expiryHash(b.PreviousBlockhash) || len(b.Signatures) == 0 {
		return fmt.Errorf("invalid expiry block identity or empty signatures")
	}
	seen := make(map[string]bool, len(b.Signatures))
	for _, value := range b.Signatures {
		signature, err := solana.SignatureFromBase58(value)
		if err != nil || signature.IsZero() || signature.String() != value || seen[value] {
			return fmt.Errorf("invalid or duplicate expiry block signature")
		}
		seen[value] = true
	}
	return nil
}

func (r *expiryRPC) block(ctx context.Context, slot uint64) (expiryBlock, error) {
	raw, err := r.call(ctx, "getBlock", []any{slot, rpc.M{
		"commitment":                     "finalized",
		"transactionDetails":             "signatures",
		"rewards":                        false,
		"maxSupportedTransactionVersion": 0,
	}})
	if err != nil {
		return expiryBlock{}, err
	}
	fields, err := jsonObject(raw)
	if err != nil || requireFields(fields, "blockHeight", "parentSlot", "blockhash", "previousBlockhash", "signatures") != nil {
		return expiryBlock{}, fmt.Errorf("missing finalized expiry block or fields")
	}
	var b expiryBlock
	b.Slot = slot
	b.Height, err = historyUint(fields["blockHeight"])
	if err != nil {
		return expiryBlock{}, err
	}
	b.ParentSlot, err = historyUint(fields["parentSlot"])
	if err != nil {
		return expiryBlock{}, err
	}
	if historyField(fields, "blockhash", &b.Blockhash) != nil || historyField(fields, "previousBlockhash", &b.PreviousBlockhash) != nil || historyField(fields, "signatures", &b.Signatures) != nil {
		return expiryBlock{}, fmt.Errorf("invalid expiry block fields")
	}
	return b, b.validate()
}

func (r *expiryRPC) anchor(ctx context.Context) (expiryBlock, error) {
	raw, err := r.call(ctx, "getSlot", []any{rpc.M{"commitment": "finalized"}})
	if err != nil {
		return expiryBlock{}, err
	}
	slot, err := historyUint(raw)
	if err != nil {
		return expiryBlock{}, err
	}
	return r.block(ctx, slot)
}

type expiryLocator struct {
	Start uint64      `json:"start"`
	End   uint64      `json:"end"`
	Slots []uint64    `json:"slots"`
	Block expiryBlock `json:"block"`
}

func (r *expiryRPC) locate(ctx context.Context, anchor expiryBlock, target uint64) (expiryBlock, []expiryLocator, error) {
	if anchor.Height < target {
		return expiryBlock{}, nil, fmt.Errorf("finalized anchor has not reached expiry plus two")
	}
	if anchor.Height == target {
		return anchor, []expiryLocator{}, nil
	}
	estimate := anchor.Slot - (anchor.Height - target)
	low := uint64(0)
	if estimate > 4096 {
		low = estimate - 4096
	}
	high := min(anchor.Slot, estimate+4096)
	var evidence []expiryLocator
	for step := 0; step < 18 && low <= high; step++ {
		mid := low + (high-low)/2
		end := min(high, mid+4096)
		raw, err := r.call(ctx, "getBlocks", []any{mid, end, rpc.M{"commitment": "finalized"}})
		if err != nil {
			return expiryBlock{}, nil, err
		}
		var slots []uint64
		if json.Unmarshal(raw, &slots) != nil || slots == nil || len(slots) > 4097 {
			return expiryBlock{}, nil, fmt.Errorf("invalid expiry slot locator")
		}
		if len(slots) == 0 {
			// An empty upper search range is only a locator hint. The eventual
			// target still needs its exact height and complete parent chain.
			evidence = append(evidence, expiryLocator{
				Start: mid,
				End:   end,
				Slots: slots,
			})
			if mid == 0 {
				break
			}
			high = mid - 1
			continue
		}
		for i, slot := range slots {
			if slot < mid || slot > end || (i > 0 && slot <= slots[i-1]) {
				return expiryBlock{}, nil, fmt.Errorf("invalid expiry locator slot ordering")
			}
		}
		b, err := r.block(ctx, slots[0])
		if err != nil {
			return expiryBlock{}, nil, err
		}
		evidence = append(evidence, expiryLocator{
			Start: mid,
			End:   end,
			Slots: slots,
			Block: b,
		})
		if b.Height == target {
			return b, evidence, nil
		}
		if b.Height < target {
			low = b.Slot + 1
		} else {
			if mid == 0 {
				break
			}
			high = mid - 1
		}
	}
	return expiryBlock{}, nil, fmt.Errorf("expiry target outside bounded locator")
}

type expiryChecks struct {
	Genesis        string      `json:"genesis"`
	Anchor         expiryBlock `json:"anchor"`
	StatusContext  uint64      `json:"statusContext"`
	ReceiptAbsent  bool        `json:"receiptAbsent"`
	HashContext    uint64      `json:"hashContext"`
	BlockhashValid bool        `json:"blockhashValid"`
}

type nonInclusionCertificate struct {
	Version       int             `json:"version"`
	Policy        string          `json:"policy"`
	MaxAge        uint64          `json:"maxProcessingAge"`
	PendingDigest string          `json:"pendingDigest"`
	Endpoint      string          `json:"endpoint"`
	Started       int64           `json:"started"`
	Finished      int64           `json:"finished"`
	Requests      int             `json:"requests"`
	ResponseBytes int             `json:"responseBytes"`
	Initial       expiryChecks    `json:"initial"`
	Final         expiryChecks    `json:"final"`
	Locator       []expiryLocator `json:"locator"`
	Blocks        []expiryBlock   `json:"blocks"`
}

func (r *expiryRPC) finalChecks(ctx context.Context, saved pendingSnapshot, minimum expiryBlock, endSlot uint64) (expiryChecks, error) {
	if err := r.genesis(ctx); err != nil {
		return expiryChecks{}, err
	}
	anchor, err := r.anchor(ctx)
	if err != nil {
		return expiryChecks{}, err
	}
	if !expiryNonregressing(anchor, minimum) || (anchor.Slot == minimum.Slot && !reflect.DeepEqual(anchor, minimum)) {
		return expiryChecks{}, fmt.Errorf("expiry finalized anchor regressed or changed")
	}
	for _, signature := range anchor.Signatures {
		if signature == saved.doc.Signature {
			return expiryChecks{}, fmt.Errorf("original signature included in current finalized anchor")
		}
	}
	status, err := r.absentStatus(ctx, &saved.doc, endSlot)
	if err != nil {
		return expiryChecks{}, err
	}
	if err := r.absentReceipt(ctx, &saved.doc); err != nil {
		return expiryChecks{}, err
	}
	ordinary, err := validateOrdinaryIntent(&saved.doc)
	if err != nil {
		return expiryChecks{}, err
	}
	raw, err := r.call(ctx, "isBlockhashValid", []any{ordinary.blockhash, rpc.M{
		"commitment":     "finalized",
		"minContextSlot": endSlot,
	}})
	if err != nil {
		return expiryChecks{}, err
	}
	fields, err := jsonObject(raw)
	if err != nil || !bytes.Equal(bytes.TrimSpace(fields["value"]), []byte("false")) {
		return expiryChecks{}, fmt.Errorf("blockhash invalidity not explicit")
	}
	slot, err := expiryContext(fields)
	if err != nil || slot < endSlot {
		return expiryChecks{}, fmt.Errorf("blockhash invalidity context is stale or missing")
	}
	return expiryChecks{
		Genesis:       saved.doc.Genesis,
		Anchor:        anchor,
		StatusContext: status,
		ReceiptAbsent: true,
		HashContext:   slot,
	}, nil
}

func expiryNonregressing(next, prior expiryBlock) bool {
	if next.Slot < prior.Slot || next.Height < prior.Height {
		return false
	}
	if next.Slot == prior.Slot || next.Height == prior.Height {
		return next.Slot == prior.Slot && next.Height == prior.Height && next.Blockhash == prior.Blockhash
	}
	return next.Height-prior.Height <= next.Slot-prior.Slot
}

func (p *payerSession) certifyExpired(ctx context.Context, saved pendingSnapshot) (*nonInclusionCertificate, error) {
	ordinary, err := validateOrdinaryIntent(&saved.doc)
	if err != nil {
		return nil, err
	}
	if saved.doc.Payer != p.service.payer.PublicKey().String() {
		return nil, fmt.Errorf("expiry payer mismatch")
	}
	if err := p.service.store.expiryContradictions(ctx, saved); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, expiryTimeout)
	defer cancel()
	r := expiryRPC{
		service: p.service,
		started: p.service.deps.now(),
	}
	endpoint, err := expiryEndpoint(p.service.cfg.solanaRPC)
	if err != nil {
		return nil, err
	}
	cached, err := p.service.store.loadExpiryCertificateFor(saved, endpoint)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		if cached.Endpoint != endpoint {
			return nil, fmt.Errorf("expiry capsule endpoint identity changed")
		}
		if err := validateExpiryCertificate(saved, cached); err != nil {
			return nil, err
		}
		current, err := r.finalChecks(ctx, saved, cached.Final.Anchor, cached.Blocks[0].Slot)
		if err != nil {
			return nil, err
		}
		if err := validateExpiryBlockViews(append(cached.blockViews(), current.Anchor)); err != nil {
			return nil, err
		}
		return cached, nil
	}
	if err := r.genesis(ctx); err != nil {
		return nil, err
	}
	status, err := r.absentStatus(ctx, &saved.doc, 0)
	if err != nil {
		return nil, err
	}
	if err := r.absentReceipt(ctx, &saved.doc); err != nil {
		return nil, err
	}
	anchor, err := r.anchor(ctx)
	if err != nil {
		return nil, err
	}
	end, locator, err := r.locate(ctx, anchor, uint64(saved.doc.LastValidBlockHeight)+2)
	if err != nil {
		return nil, err
	}
	blocks := []expiryBlock{end}
	for len(blocks) < expiryWindowBlocks {
		previous := blocks[len(blocks)-1]
		parent, err := r.block(ctx, previous.ParentSlot)
		if err != nil {
			return nil, err
		}
		if parent.Height+1 != previous.Height || parent.Blockhash != previous.PreviousBlockhash {
			return nil, fmt.Errorf("expiry parent chain is discontinuous")
		}
		blocks = append(blocks, parent)
	}
	if err := validateExpiryWindow(saved.doc, ordinary.blockhash, blocks); err != nil {
		return nil, err
	}
	final, err := r.finalChecks(ctx, saved, anchor, end.Slot)
	if err != nil {
		return nil, err
	}
	cert := &nonInclusionCertificate{
		Version:       1,
		Policy:        expiryPolicy,
		MaxAge:        expiryMaxAge,
		PendingDigest: saved.Digest,
		Endpoint:      endpoint,
		Started:       r.started.UnixMilli(),
		Finished:      p.service.deps.now().UnixMilli(),
		Requests:      r.calls,
		ResponseBytes: r.bytes,
		Initial: expiryChecks{
			Genesis:       saved.doc.Genesis,
			Anchor:        anchor,
			StatusContext: status,
			ReceiptAbsent: true,
		},
		Final:   final,
		Locator: locator,
		Blocks:  blocks,
	}
	return cert, validateExpiryCertificate(saved, cert)
}

func expiryEndpoint(configured string) (string, error) {
	endpoint, err := url.Parse(configured)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return "", fmt.Errorf("expiry endpoint identity unavailable")
	}
	return digestHex([]byte(endpoint.Scheme + "://" + endpoint.Host)), nil
}

func validateExpiryWindow(doc intent, blockhash string, blocks []expiryBlock) error {
	if len(blocks) != expiryWindowBlocks || blocks[0].Height != uint64(doc.LastValidBlockHeight)+2 {
		return fmt.Errorf("incomplete expiry block window")
	}
	origin := 0
	hashes := make(map[string]bool)
	for i, block := range blocks {
		if err := block.validate(); err != nil {
			return err
		}
		if hashes[block.Blockhash] {
			return fmt.Errorf("duplicate expiry blockhash")
		}
		hashes[block.Blockhash] = true
		if i > 0 {
			child := blocks[i-1]
			if child.ParentSlot != block.Slot || child.Slot <= block.Slot || child.Height != block.Height+1 || child.PreviousBlockhash != block.Blockhash {
				return fmt.Errorf("expiry parent chain is discontinuous")
			}
		}
		if block.Blockhash == blockhash {
			origin++
			if block.Height+expiryMaxAge != uint64(doc.LastValidBlockHeight) {
				return fmt.Errorf("unsupported blockhash processing age; retained")
			}
		}
		for _, signature := range block.Signatures {
			if signature == doc.Signature {
				return fmt.Errorf("original signature included in finalized block; retained")
			}
		}
	}
	if origin != 1 {
		return fmt.Errorf("signed blockhash origin is not uniquely present")
	}
	return nil
}

func validateExpiryCertificate(saved pendingSnapshot, cert *nonInclusionCertificate) error {
	ordinary, err := validateOrdinaryIntent(&saved.doc)
	if err != nil {
		return err
	}
	if cert == nil || cert.Version != 1 || cert.Policy != expiryPolicy || cert.MaxAge != expiryMaxAge {
		return fmt.Errorf("invalid expiry certificate policy, binding or bounds")
	}
	if cert.PendingDigest != saved.Digest || !validHex(cert.Endpoint, 32, true) {
		return fmt.Errorf("invalid expiry certificate policy, binding or bounds")
	}
	if cert.Started <= 0 || cert.Finished < cert.Started || cert.Finished-cert.Started >= expiryTimeout.Milliseconds() {
		return fmt.Errorf("invalid expiry certificate policy, binding or bounds")
	}
	if cert.Requests < 170 || cert.Requests > expiryMaxRequests || cert.ResponseBytes <= 0 || cert.ResponseBytes > expiryMaxTotal {
		return fmt.Errorf("invalid expiry certificate policy, binding or bounds")
	}
	if len(cert.Locator) > 18 {
		return fmt.Errorf("invalid expiry certificate locator length")
	}
	if err := validateExpiryWindow(saved.doc, ordinary.blockhash, cert.Blocks); err != nil {
		return err
	}
	end := cert.Blocks[0]
	for _, check := range []expiryChecks{cert.Initial, cert.Final} {
		if err := check.Anchor.validate(); err != nil {
			return err
		}
		if check.Genesis != saved.doc.Genesis || !check.ReceiptAbsent || check.BlockhashValid || check.StatusContext > uint64(maxSafeInteger) || !expiryNonregressing(check.Anchor, end) {
			return fmt.Errorf("invalid expiry certificate checks")
		}
	}
	if !expiryNonregressing(cert.Final.Anchor, cert.Initial.Anchor) || cert.Final.StatusContext < end.Slot || cert.Final.HashContext < end.Slot || cert.Final.HashContext > uint64(maxSafeInteger) {
		return fmt.Errorf("invalid expiry certificate final checks")
	}
	all := cert.blockViews()
	if err := validateExpiryBlockViews(all); err != nil {
		return err
	}
	for _, block := range all {
		for _, signature := range block.Signatures {
			if signature == saved.doc.Signature {
				return fmt.Errorf("original signature included in expiry evidence")
			}
		}
	}
	return validateExpiryLocator(cert.Initial.Anchor, end, cert.Locator)
}

func (cert *nonInclusionCertificate) blockViews() []expiryBlock {
	all := append([]expiryBlock{cert.Initial.Anchor, cert.Final.Anchor}, cert.Blocks...)
	for _, step := range cert.Locator {
		if len(step.Slots) != 0 {
			all = append(all, step.Block)
		}
	}
	return all
}

func validateExpiryBlockViews(blocks []expiryBlock) error {
	views := make(map[uint64]expiryBlock)
	for _, block := range blocks {
		if prior, exists := views[block.Slot]; exists && !reflect.DeepEqual(prior, block) {
			return fmt.Errorf("conflicting expiry block views")
		}
		views[block.Slot] = block
	}
	for _, block := range blocks {
		if parent, exists := views[block.ParentSlot]; exists {
			if block.PreviousBlockhash != parent.Blockhash || block.Height != parent.Height+1 {
				return fmt.Errorf("conflicting expiry parent views")
			}
		}
	}
	// Implied parents supply identity facts, not full signature evidence.
	identities := make(map[string]expiryBlock)
	recordIdentity := func(block expiryBlock) error {
		if block.Slot > uint64(maxSafeInteger) || block.Height > block.Slot || !expiryHash(block.Blockhash) {
			return fmt.Errorf("invalid expiry block identity")
		}
		if prior, exists := identities[block.Blockhash]; exists && (prior.Slot != block.Slot || prior.Height != block.Height) {
			return fmt.Errorf("conflicting expiry blockhash identity")
		}
		identities[block.Blockhash] = block
		return nil
	}
	for _, block := range blocks {
		if err := recordIdentity(block); err != nil {
			return err
		}
		if block.Height == 0 || block.ParentSlot >= block.Slot || (block.Height == 1 && block.ParentSlot != 0) {
			return fmt.Errorf("conflicting expiry parent order")
		}
		parentView := expiryBlock{
			Slot:      block.ParentSlot,
			Height:    block.Height - 1,
			Blockhash: block.PreviousBlockhash,
		}
		for _, prior := range views {
			if prior.Slot < block.Slot && !expiryNonregressing(parentView, prior) {
				return fmt.Errorf("conflicting expiry parent order")
			}
		}
		if err := recordIdentity(parentView); err != nil {
			return err
		}
	}
	return nil
}

func validateExpiryLocator(anchor, end expiryBlock, locator []expiryLocator) error {
	if anchor.Height == end.Height {
		if len(locator) != 0 || !reflect.DeepEqual(anchor, end) {
			return fmt.Errorf("invalid expiry certificate anchor locator")
		}
		return nil
	}
	if len(locator) == 0 || len(locator) > 18 {
		return fmt.Errorf("invalid expiry certificate locator length")
	}
	estimate := anchor.Slot - (anchor.Height - end.Height)
	low := uint64(0)
	if estimate > 4096 {
		low = estimate - 4096
	}
	high := min(anchor.Slot, estimate+4096)
	for i, step := range locator {
		if low > high {
			return fmt.Errorf("invalid expiry certificate locator bounds")
		}
		mid := low + (high-low)/2
		if step.Start != mid || step.End != min(high, mid+4096) || step.Slots == nil || len(step.Slots) > 4097 {
			return fmt.Errorf("invalid expiry certificate locator bounds")
		}
		if len(step.Slots) == 0 {
			if mid == 0 || !reflect.DeepEqual(step.Block, expiryBlock{}) {
				return fmt.Errorf("invalid expiry empty locator range")
			}
			high = mid - 1
			continue
		}
		if step.Block.Slot != step.Slots[0] {
			return fmt.Errorf("invalid expiry certificate locator bounds")
		}
		if err := step.Block.validate(); err != nil {
			return err
		}
		for j, slot := range step.Slots {
			if slot < step.Start || slot > step.End || (j > 0 && slot <= step.Slots[j-1]) {
				return fmt.Errorf("invalid expiry certificate locator slots")
			}
		}
		if step.Block.Height == end.Height {
			if i != len(locator)-1 || !reflect.DeepEqual(step.Block, end) {
				return fmt.Errorf("expiry locator does not end at window")
			}
			return nil
		}
		if step.Block.Height < end.Height {
			low = step.Block.Slot + 1
		} else {
			if mid == 0 {
				return fmt.Errorf("expiry locator underflow")
			}
			high = mid - 1
		}
	}
	return fmt.Errorf("expiry locator did not find target")
}

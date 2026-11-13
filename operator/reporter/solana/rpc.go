package solana

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gagliardetto/solana-go"
	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/semaphore"

	"pkg.akt.dev/node/v2/operator/reporter/metrics"
)

const (
	metricsChain       = "solana"
	blockTimeCache     = "block_time"
	blockTimeCacheSize = 4096
	maxAccountsPerCall = 100
	commitment         = "finalized"
	encodingJSON       = "json"
	maxTxVersion       = 1
)

var (
	// ErrUnavailable reports any transport or RPC failure outside getBlock, as in TS.
	ErrUnavailable = errors.New("Solana node is temporarily unavailable") //nolint:staticcheck // verbatim TS text
	// ErrBlockUnavailable reports a getBlock error, a missing block or a malformed one.
	ErrBlockUnavailable = errors.New("resolution evidence unavailable: block missing from owned node history")
)

// causeError keeps a sentinel's text verbatim while retaining the underlying cause.
type causeError struct {
	sentinel error
	cause    error
}

func (e *causeError) Error() string   { return e.sentinel.Error() }
func (e *causeError) Unwrap() []error { return []error{e.sentinel, e.cause} }

func unavailable(err error) error {
	return &causeError{sentinel: ErrUnavailable, cause: err}
}

func blockUnavailable(err error) error {
	return &causeError{sentinel: ErrBlockUnavailable, cause: err}
}

// RPCOptions configures the Solana JSON-RPC client.
type RPCOptions struct {
	URL     string
	Timeout time.Duration
	// Concurrency limits concurrent requests other than getBlock.
	Concurrency int
	// BlockConcurrency limits concurrent getBlock requests.
	BlockConcurrency int
	Metrics          *metrics.Metrics
}

// RPC is a JSON-RPC client for one Solana node. Every read uses finalized commitment.
type RPC struct {
	url     string
	timeout time.Duration
	http    *http.Client
	reads   *semaphore.Weighted
	blocks  *semaphore.Weighted
	metrics *metrics.Metrics
	ids     atomic.Uint64
	times   *lru.Cache[uint64, int64]
}

// Account is an account as getMultipleAccounts returns it.
type Account struct {
	Owner      solana.PublicKey
	Executable bool
	Data       []byte
}

// NewRPC returns a client with one keep-alive connection pool sized to both concurrency limits.
func NewRPC(opts RPCOptions) *RPC {
	reads := max(opts.Concurrency, 1)
	blocks := max(opts.BlockConcurrency, 1)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = reads + blocks
	transport.MaxIdleConnsPerHost = reads + blocks
	transport.MaxConnsPerHost = reads + blocks

	times, err := lru.New[uint64, int64](blockTimeCacheSize)
	if err != nil {
		panic(err) // only for a non-positive size
	}

	return &RPC{
		url:     opts.URL,
		timeout: opts.Timeout,
		http:    &http.Client{Transport: transport},
		reads:   semaphore.NewWeighted(int64(reads)),
		blocks:  semaphore.NewWeighted(int64(blocks)),
		metrics: opts.Metrics,
		times:   times,
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

// rpcConfig is the configuration object of a request; nil fields are omitted.
type rpcConfig struct {
	Commitment                     string `json:"commitment"`
	Encoding                       string `json:"encoding"`
	TransactionDetails             string `json:"transactionDetails,omitempty"`
	Rewards                        *bool  `json:"rewards,omitempty"`
	MaxSupportedTransactionVersion *int   `json:"maxSupportedTransactionVersion,omitempty"`
}

func ptr[T any](v T) *T { return &v }

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// post sends one request under sem and returns the open response body. release must be called
// once the body is consumed; it closes the body, frees the slot and records the call.
func (r *RPC) post(
	ctx context.Context,
	sem *semaphore.Weighted,
	method string,
	params ...any,
) (body io.Reader, release func(error), err error) {
	if err := sem.Acquire(ctx, 1); err != nil {
		return nil, nil, err
	}

	start := time.Now()
	cancel := context.CancelFunc(func() {})
	if r.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
	}

	var resp *http.Response
	finish := func(err error) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		cancel()
		sem.Release(1)
		r.metrics.RecordRPC(metricsChain, method, time.Since(start), err)
	}

	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: r.ids.Add(1), Method: method, Params: params})
	if err != nil {
		finish(err)
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(payload))
	if err != nil {
		finish(err)
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = r.http.Do(req) //nolint:gosec // the URL is the operator's configured node
	if err != nil {
		finish(err)
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("%s: HTTP %s", method, resp.Status)
		finish(err)
		return nil, nil, err
	}

	return resp.Body, finish, nil
}

// call runs one non-block request and decodes its result into out. A null result leaves out untouched.
func (r *RPC) call(ctx context.Context, method string, out any, params ...any) (err error) {
	body, release, err := r.post(ctx, r.reads, method, params...)
	if err != nil {
		return err
	}
	defer func() { release(err) }()

	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return fmt.Errorf("%s: decode response: %w", method, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("%s: %w", method, resp.Error)
	}
	if len(resp.Result) == 0 {
		return fmt.Errorf("%s: response has no result", method)
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("%s: decode result: %w", method, err)
	}

	return nil
}

// read wraps call so that every failure reads as ErrUnavailable, unless ctx itself ended.
func (r *RPC) read(ctx context.Context, method string, out any, params ...any) error {
	err := r.call(ctx, method, out, params...)
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return unavailable(err)
}

// GenesisHash returns the node's genesis hash.
func (r *RPC) GenesisHash(ctx context.Context) (string, error) {
	var hash string
	if err := r.read(ctx, "getGenesisHash", &hash); err != nil {
		return "", err
	}

	return hash, nil
}

type accountJSON struct {
	Owner      string   `json:"owner"`
	Executable bool     `json:"executable"`
	Data       []string `json:"data"`
}

// MultipleAccounts reads up to 100 accounts at finalized commitment; a missing account is nil.
func (r *RPC) MultipleAccounts(ctx context.Context, keys []solana.PublicKey) (uint64, []*Account, error) {
	if len(keys) > maxAccountsPerCall {
		return 0, nil, fmt.Errorf("getMultipleAccounts takes at most %d keys, got %d", maxAccountsPerCall, len(keys))
	}

	encoded := make([]string, len(keys))
	for i, k := range keys {
		encoded[i] = k.String()
	}

	var res struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value []*accountJSON `json:"value"`
	}
	err := r.read(ctx, "getMultipleAccounts", &res, encoded, rpcConfig{Commitment: commitment, Encoding: "base64"})
	if err != nil {
		return 0, nil, err
	}

	if len(res.Value) != len(keys) {
		return 0, nil, unavailable(fmt.Errorf("getMultipleAccounts: %d accounts for %d keys", len(res.Value), len(keys)))
	}

	accts := make([]*Account, len(keys))
	for i, a := range res.Value {
		if a == nil {
			continue
		}
		acct, err := a.decode()
		if err != nil {
			return 0, nil, unavailable(fmt.Errorf("getMultipleAccounts: account %d: %w", i, err))
		}
		accts[i] = acct
	}

	return res.Context.Slot, accts, nil
}

func (a *accountJSON) decode() (*Account, error) {
	owner, err := solana.PublicKeyFromBase58(a.Owner)
	if err != nil {
		return nil, fmt.Errorf("owner: %w", err)
	}
	if len(a.Data) != 2 || a.Data[1] != "base64" {
		return nil, errors.New("data is not base64 encoded")
	}
	data, err := base64.StdEncoding.DecodeString(a.Data[0])
	if err != nil {
		return nil, fmt.Errorf("data: %w", err)
	}

	return &Account{Owner: owner, Executable: a.Executable, Data: data}, nil
}

// BlockTime returns the block time of slot, or nil when the node has none. Known times are cached per slot.
func (r *RPC) BlockTime(ctx context.Context, slot uint64) (*int64, error) {
	if t, ok := r.times.Get(slot); ok {
		r.metrics.RecordCache(blockTimeCache, true)
		return &t, nil
	}
	r.metrics.RecordCache(blockTimeCache, false)

	var t *int64
	if err := r.read(ctx, "getBlockTime", &t, slot); err != nil {
		return nil, err
	}
	if t != nil {
		r.times.Add(slot, *t)
	}

	return t, nil
}

// Transaction reads a finalized transaction by signature; it returns nil, nil when the node has none.
func (r *RPC) Transaction(ctx context.Context, sig string) (*Tx, error) {
	var res *struct {
		Slot uint64 `json:"slot"`
		txJSON
	}
	err := r.read(ctx, "getTransaction", &res, sig, rpcConfig{
		Commitment:                     commitment,
		Encoding:                       encodingJSON,
		MaxSupportedTransactionVersion: ptr(maxTxVersion),
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	tx, err := res.decode(res.Slot)
	if err != nil {
		return nil, unavailable(fmt.Errorf("getTransaction: %w", err))
	}

	return &tx, nil
}

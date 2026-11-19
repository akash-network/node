package simnode

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"

	"github.com/gagliardetto/solana-go"
)

// Agave's JSON-RPC limits, error codes and texts.
const (
	maxAccountsPerCall = 100
	agaveVersion       = "4.2.2"
	rentExemptEpoch    = uint64(math.MaxUint64)
	lamportsPerSig     = 5000

	codeParse              = -32700
	codeInternal           = -32603
	codeMethodNotFound     = -32601
	codeInvalidParams      = -32602
	codeBlockNotAvailable  = -32004
	codeSlotSkipped        = -32007
	codeUnsupportedVersion = -32015

	versionLegacy = "legacy"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func invalidParams(format string, args ...any) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: "Invalid params: " + fmt.Sprintf(format, args...)}
}

type txRef struct {
	slot  uint64
	index int
}

// indexSolana maps every signature to its transaction, the lowest slot first. The caller holds n.mu or owns n.
func (n *Node) indexSolana() {
	slots := make([]uint64, 0, len(n.sc.Solana.Blocks))
	for slot := range n.sc.Solana.Blocks {
		slots = append(slots, slot)
	}
	slices.Sort(slots)

	n.sigs = map[string]txRef{}
	for _, slot := range slots {
		for i, tx := range n.sc.Solana.Blocks[slot].Transactions {
			for _, sig := range tx.Signatures {
				if _, ok := n.sigs[sig]; !ok {
					n.sigs[sig] = txRef{slot: slot, index: i}
				}
			}
		}
	}
}

type solanaRequest struct {
	ID     json.RawMessage   `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

type solanaResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

type solanaMethod func(n *Node, params []json.RawMessage) (any, *rpcError)

var solanaMethods = map[string]solanaMethod{
	"getGenesisHash":      (*Node).getGenesisHash,
	"getMultipleAccounts": (*Node).getMultipleAccounts,
	"getBlockTime":        (*Node).getBlockTime,
	"getBlock":            (*Node).getBlock,
	"getTransaction":      (*Node).getTransaction,
}

// serveSolana answers one JSON-RPC request as an Agave node at finalized commitment does.
func (n *Node) serveSolana(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req solanaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSolana(w, nil, nil, &rpcError{Code: codeParse, Message: "Parse error"})
		return
	}
	n.count(req.Method)

	latency := n.opts.SolanaLatency
	if req.Method == "getBlock" {
		latency += n.opts.BlockFetchLatency
	}
	if delay(r.Context(), latency) != nil {
		return
	}

	if req.Method == "getMultipleAccounts" {
		n.applyChanges(n.Calls(req.Method))
	}

	if f, ok := n.takeFailure(req.Method); ok {
		if f.HTTPStatus != 0 {
			w.WriteHeader(f.HTTPStatus)
			return
		}
		writeSolana(w, req.ID, nil, &rpcError{Code: f.Code, Message: f.Message})

		return
	}

	method, ok := solanaMethods[req.Method]
	if !ok {
		writeSolana(w, req.ID, nil, &rpcError{Code: codeMethodNotFound, Message: "Method not found"})
		return
	}

	// Encode under the read lock, so a concurrent Mutate cannot change what is being encoded.
	n.mu.RLock()
	result, rerr := method(n, req.Params)
	var payload []byte
	if rerr == nil {
		var err error
		if payload, err = json.Marshal(result); err != nil {
			rerr = &rpcError{Code: codeInvalidParams, Message: err.Error()}
		}
	}
	n.mu.RUnlock()

	writeSolana(w, req.ID, payload, rerr)
}

func writeSolana(w http.ResponseWriter, id, result json.RawMessage, rerr *rpcError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}

	body, err := json.Marshal(solanaResponse{JSONRPC: "2.0", Result: result, Error: rerr, ID: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

// applyChanges applies, in order, the account changes due once getMultipleAccounts has been called calls times. It
// peeks under the read lock first, as takeFailure does.
func (n *Node) applyChanges(calls int) {
	n.mu.RLock()
	due := len(n.sc.Solana.Changes) > 0 && n.sc.Solana.Changes[0].AfterCalls < calls
	n.mu.RUnlock()
	if !due {
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	s := &n.sc.Solana
	for len(s.Changes) > 0 && s.Changes[0].AfterCalls < calls {
		c := s.Changes[0]
		s.Changes = s.Changes[1:]

		if c.Slot != 0 {
			s.Slot = c.Slot
		}
		if s.Accounts == nil {
			s.Accounts = map[solana.PublicKey]Account{}
		}
		for k, a := range c.Accounts {
			s.Accounts[k] = a
		}
	}
}

// takeFailure returns the failure injected for method, consuming one of its Times. It peeks under the read lock
// first, so a scenario without failures never takes the write lock and never waits behind block encodes.
func (n *Node) takeFailure(method string) (Failure, bool) {
	n.mu.RLock()
	_, ok := n.sc.Solana.Failures[method]
	n.mu.RUnlock()
	if !ok {
		return Failure{}, false
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	// Re-check: a concurrent request or Mutate may have removed the entry since the peek.
	f, ok := n.sc.Solana.Failures[method]
	if !ok {
		return Failure{}, false
	}

	switch {
	case f.Times == 1:
		delete(n.sc.Solana.Failures, method)
	case f.Times > 1:
		n.sc.Solana.Failures[method] = Failure{Code: f.Code, Message: f.Message, HTTPStatus: f.HTTPStatus, Times: f.Times - 1}
	}

	return f, true
}

func param(params []json.RawMessage, i int, out any) *rpcError {
	if i >= len(params) {
		return invalidParams("missing parameter %d", i)
	}
	if err := json.Unmarshal(params[i], out); err != nil {
		return invalidParams("parameter %d: %v", i, err)
	}

	return nil
}

type solanaConfig struct {
	Commitment                     string `json:"commitment"`
	Encoding                       string `json:"encoding"`
	TransactionDetails             string `json:"transactionDetails"`
	Rewards                        *bool  `json:"rewards"`
	MaxSupportedTransactionVersion *int   `json:"maxSupportedTransactionVersion"`
}

// config reads the optional configuration object at i. The simulator holds only finalized state, so it refuses any
// other commitment rather than answer it wrongly.
func config(params []json.RawMessage, i int) (solanaConfig, *rpcError) {
	var cfg solanaConfig
	if i >= len(params) {
		return cfg, nil
	}
	if err := param(params, i, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Commitment != "" && cfg.Commitment != "finalized" {
		return cfg, invalidParams("simnode serves finalized commitment only, not %q", cfg.Commitment)
	}

	return cfg, nil
}

// jsonEncoding refuses every transaction encoding but json, the only one the reporter may decode.
func (c solanaConfig) jsonEncoding() *rpcError {
	if c.Encoding != "" && c.Encoding != "json" {
		return invalidParams("simnode serves json transactions only, not %q", c.Encoding)
	}

	return nil
}

func (n *Node) getGenesisHash([]json.RawMessage) (any, *rpcError) {
	return n.sc.Solana.GenesisHash, nil
}

type accountJSON struct {
	Data       [2]string `json:"data"`
	Executable bool      `json:"executable"`
	Lamports   uint64    `json:"lamports"`
	Owner      string    `json:"owner"`
	RentEpoch  uint64    `json:"rentEpoch"`
	Space      int       `json:"space"`
}

type contextJSON struct {
	APIVersion string `json:"apiVersion"`
	Slot       uint64 `json:"slot"`
}

func (n *Node) getMultipleAccounts(params []json.RawMessage) (any, *rpcError) {
	var keys []string
	if err := param(params, 0, &keys); err != nil {
		return nil, err
	}
	if len(keys) > maxAccountsPerCall {
		return nil, &rpcError{Code: codeInvalidParams, Message: fmt.Sprintf("Too many inputs provided; max %d", maxAccountsPerCall)}
	}

	cfg, rerr := config(params, 1)
	if rerr != nil {
		return nil, rerr
	}
	if cfg.Encoding != "base64" {
		return nil, invalidParams("simnode serves base64 account data only, not %q", cfg.Encoding)
	}

	values := make([]*accountJSON, len(keys))
	for i, k := range keys {
		pk, err := solana.PublicKeyFromBase58(k)
		if err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "Invalid param: " + err.Error()}
		}
		a, ok := n.sc.Solana.Accounts[pk]
		if !ok {
			continue
		}
		values[i] = &accountJSON{
			Data:       [2]string{base64.StdEncoding.EncodeToString(a.Data), "base64"},
			Executable: a.Executable,
			Lamports:   a.Lamports,
			Owner:      a.Owner.String(),
			RentEpoch:  rentExemptEpoch,
			Space:      len(a.Data),
		}
	}

	return struct {
		Context contextJSON    `json:"context"`
		Value   []*accountJSON `json:"value"`
	}{contextJSON{APIVersion: agaveVersion, Slot: n.sc.Solana.Slot}, values}, nil
}

// blockTime returns a slot's block time in whole Unix seconds, rounded down as Agave's timestamps are.
func (n *Node) blockTime(slot uint64) *int64 {
	offset, ok := n.sc.Solana.BlockTimes[slot]
	if !ok {
		return nil
	}
	t := n.at(offset).Unix()

	return &t
}

func (n *Node) getBlockTime(params []json.RawMessage) (any, *rpcError) {
	var slot uint64
	if err := param(params, 0, &slot); err != nil {
		return nil, err
	}

	return n.blockTime(slot), nil
}

// blockhash derives a stable hash for slot.
func blockhash(slot uint64) string {
	sum := sha256.Sum256([]byte("simnode block " + strconv.FormatUint(slot, 10)))
	return solana.PublicKeyFromBytes(sum[:]).String()
}

func parent(slot uint64) uint64 {
	if slot == 0 {
		return 0
	}

	return slot - 1
}

type blockJSON struct {
	BlockHeight       uint64     `json:"blockHeight"`
	BlockTime         *int64     `json:"blockTime"`
	Blockhash         string     `json:"blockhash"`
	ParentSlot        uint64     `json:"parentSlot"`
	PreviousBlockhash string     `json:"previousBlockhash"`
	Rewards           *[]any     `json:"rewards,omitempty"`
	Transactions      []txResult `json:"transactions"`
}

func (n *Node) getBlock(params []json.RawMessage) (any, *rpcError) {
	var slot uint64
	if err := param(params, 0, &slot); err != nil {
		return nil, err
	}
	cfg, rerr := config(params, 1)
	if rerr != nil {
		return nil, rerr
	}
	if rerr := cfg.jsonEncoding(); rerr != nil {
		return nil, rerr
	}
	if cfg.TransactionDetails != "" && cfg.TransactionDetails != "full" {
		return nil, invalidParams("simnode serves full transaction details only, not %q", cfg.TransactionDetails)
	}

	if slot > n.sc.Solana.Slot {
		return nil, &rpcError{Code: codeBlockNotAvailable, Message: fmt.Sprintf("Block not available for slot %d", slot)}
	}
	block, ok := n.sc.Solana.Blocks[slot]
	if !ok {
		return nil, &rpcError{
			Code:    codeSlotSkipped,
			Message: fmt.Sprintf("Slot %d was skipped, or missing due to ledger jump to recent snapshot", slot),
		}
	}

	out := blockJSON{
		BlockHeight:       slot,
		BlockTime:         n.blockTime(slot),
		Blockhash:         blockhash(slot),
		ParentSlot:        parent(slot),
		PreviousBlockhash: blockhash(parent(slot)),
		Transactions:      make([]txResult, len(block.Transactions)),
	}
	if cfg.Rewards == nil || *cfg.Rewards {
		out.Rewards = &[]any{}
	}
	for i, tx := range block.Transactions {
		r, rerr := encodeTx(slot, tx, cfg.MaxSupportedTransactionVersion)
		if rerr != nil {
			return nil, rerr
		}
		out.Transactions[i] = r
	}

	return out, nil
}

func (n *Node) getTransaction(params []json.RawMessage) (any, *rpcError) {
	var sig string
	if err := param(params, 0, &sig); err != nil {
		return nil, err
	}
	cfg, rerr := config(params, 1)
	if rerr != nil {
		return nil, rerr
	}
	if rerr := cfg.jsonEncoding(); rerr != nil {
		return nil, rerr
	}

	ref, ok := n.sigs[sig]
	if !ok || ref.slot > n.sc.Solana.Slot {
		return nil, nil
	}

	r, rerr := encodeTx(ref.slot, n.sc.Solana.Blocks[ref.slot].Transactions[ref.index], cfg.MaxSupportedTransactionVersion)
	if rerr != nil {
		return nil, rerr
	}

	return struct {
		BlockTime *int64 `json:"blockTime"`
		Slot      uint64 `json:"slot"`
		txResult
	}{n.blockTime(ref.slot), ref.slot, r}, nil
}

type txResult struct {
	Meta        metaJSON        `json:"meta"`
	Transaction txJSON          `json:"transaction"`
	Version     json.RawMessage `json:"version,omitempty"`
}

type metaJSON struct {
	ComputeUnitsConsumed uint64                     `json:"computeUnitsConsumed"`
	CostUnits            uint64                     `json:"costUnits"`
	Err                  json.RawMessage            `json:"err"`
	Fee                  uint64                     `json:"fee"`
	InnerInstructions    []any                      `json:"innerInstructions"`
	LoadedAddresses      loadedJSON                 `json:"loadedAddresses"`
	LogMessages          []string                   `json:"logMessages"`
	PostBalances         []uint64                   `json:"postBalances"`
	PostTokenBalances    []any                      `json:"postTokenBalances"`
	PreBalances          []uint64                   `json:"preBalances"`
	PreTokenBalances     []any                      `json:"preTokenBalances"`
	Rewards              *[]any                     `json:"rewards"`
	Status               map[string]json.RawMessage `json:"status"`
}

type loadedJSON struct {
	Readonly []string `json:"readonly"`
	Writable []string `json:"writable"`
}

type txJSON struct {
	Message    messageJSON `json:"message"`
	Signatures []string    `json:"signatures"`
}

type messageJSON struct {
	AccountKeys         []string           `json:"accountKeys"`
	AddressTableLookups *[]lookupJSON      `json:"addressTableLookups,omitempty"`
	Header              Header             `json:"header"`
	Instructions        []any              `json:"instructions"`
	RecentBlockhash     string             `json:"recentBlockhash"`
	TransactionConfig   *transactionConfig `json:"transactionConfig,omitempty"`
}

type lookupJSON struct {
	AccountKey      string `json:"accountKey"`
	ReadonlyIndexes []int  `json:"readonlyIndexes"`
	WritableIndexes []int  `json:"writableIndexes"`
}

// transactionConfig is the v1 (SIMD-0385) message configuration; the simulator sets none of it.
type transactionConfig struct {
	ComputeUnitLimit            *uint32 `json:"computeUnitLimit"`
	HeapSize                    *uint32 `json:"heapSize"`
	LoadedAccountsDataSizeLimit *uint32 `json:"loadedAccountsDataSizeLimit"`
	PriorityFee                 *uint64 `json:"priorityFee"`
}

func keyStrings(keys []solana.PublicKey) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k.String()
	}

	return out
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}

	return s
}

// encodeTx renders tx in Agave's json encoding. As Agave does, it refuses a versioned transaction to a client whose
// maxSupportedTransactionVersion is absent or lower, and reports the version field only to a client that set it.
func encodeTx(slot uint64, tx Transaction, maxVersion *int) (txResult, *rpcError) {
	version := tx.Version
	if version == "" {
		version = versionLegacy
	}

	var out txResult
	if version == versionLegacy {
		out.Version = json.RawMessage(`"legacy"`)
	} else {
		v, err := strconv.Atoi(version)
		if err != nil || v < 0 {
			return txResult{}, &rpcError{Code: codeInternal, Message: fmt.Sprintf("simnode: invalid transaction version %q", version)}
		}
		if maxVersion == nil || v > *maxVersion {
			return txResult{}, &rpcError{Code: codeUnsupportedVersion, Message: fmt.Sprintf(
				"Transaction version (%d) is not supported by the requesting client. Please try the request again with "+
					`the following configuration parameter: "maxSupportedTransactionVersion": %d`, v, v)}
		}
		out.Version = json.RawMessage(strconv.Itoa(v))
	}
	if maxVersion == nil {
		out.Version = nil
	}

	numKeys := len(tx.AccountKeys) + len(tx.LoadedWritable) + len(tx.LoadedReadonly)
	status := map[string]json.RawMessage{"Ok": json.RawMessage("null")}
	txErr := json.RawMessage("null")
	if len(tx.Err) != 0 {
		txErr = tx.Err
		status = map[string]json.RawMessage{"Err": tx.Err}
	}

	out.Meta = metaJSON{
		Err:               txErr,
		Fee:               uint64(lamportsPerSig * max(tx.Header.NumRequiredSignatures, 1)), //nolint:gosec // small
		InnerInstructions: []any{},
		LoadedAddresses: loadedJSON{
			Readonly: keyStrings(tx.LoadedReadonly),
			Writable: keyStrings(tx.LoadedWritable),
		},
		LogMessages:       tx.Logs,
		PostBalances:      make([]uint64, numKeys),
		PostTokenBalances: []any{},
		PreBalances:       make([]uint64, numKeys),
		PreTokenBalances:  []any{},
		Status:            status,
	}

	msg := messageJSON{
		AccountKeys:     keyStrings(tx.AccountKeys),
		Header:          tx.Header,
		Instructions:    []any{},
		RecentBlockhash: blockhash(parent(slot)),
	}
	switch version {
	case "0":
		lookups := make([]lookupJSON, len(tx.AddressTableLookups))
		for i, l := range tx.AddressTableLookups {
			lookups[i] = lookupJSON{
				AccountKey:      l.AccountKey.String(),
				ReadonlyIndexes: orEmpty(l.ReadonlyIndexes),
				WritableIndexes: orEmpty(l.WritableIndexes),
			}
		}
		msg.AddressTableLookups = &lookups
	case "1":
		msg.TransactionConfig = &transactionConfig{}
	}
	out.Transaction = txJSON{Message: msg, Signatures: orEmpty(tx.Signatures)}

	return out, nil
}

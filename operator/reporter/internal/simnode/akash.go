package simnode

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"sync"
	"time"

	errorsmod "cosmossdk.io/errors"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// The chain's unordered-transaction limits (x/auth/ante/sigverify.go) and window alignment for AlignWindow.
const (
	maxUnorderedTTL = 10 * time.Minute
	alignWindow     = 2 * time.Minute
	alignOpen       = time.Minute
	alignMaxBehind  = 70 * time.Second
	// minBlockStep separates consecutive blocks when the clock has not moved past the previous block.
	minBlockStep = time.Millisecond

	gasPerTx  = 60_000
	gasPerMsg = 40_000
)

// Akash is the state of the Akash chain and of the node serving it. Every time in it is an offset from Start.
type Akash struct {
	ChainID string `json:"chainId"`
	// NodeID is the CometBFT node ID in hex, reported by status and GetNodeInfo.
	NodeID string `json:"nodeId"`
	// NoTxIndex reports tx_index "off", and tx then fails as on a node without an index.
	NoTxIndex  bool `json:"noTxIndex,omitempty"`
	CatchingUp bool `json:"catchingUp,omitempty"`

	// Height is the latest block. BlockTimes holds each block's time; its lowest key is the earliest retained height,
	// and state below it is pruned.
	Height     int64                   `json:"height"`
	BlockTimes map[int64]time.Duration `json:"blockTimes,omitempty"`
	// AppHeight is the application's last committed height when it trails Height; zero means Height. Committed blocks
	// keep the gap.
	AppHeight int64 `json:"appHeight,omitempty"`
	// EchoOffset is added to the height that query answers repeat, to serve a node that answers another height.
	EchoOffset int64 `json:"echoOffset,omitempty"`
	// PageOverlap serves an inconsistent pagination: a full page's next key resumes at its own last entry, so the next
	// page repeats it.
	PageOverlap bool `json:"pageOverlap,omitempty"`

	Params sinkv1.Params `json:"params"`
	// Policies holds every retained reporter policy; the highest version is current.
	Policies []sinkv1.ReporterPolicy `json:"policies,omitempty"`
	State    SinkState               `json:"state"`
	// Transfers holds every version of every transfer. A version applies from its Height until the next version of
	// the same ID; executed messages append versions.
	Transfers []Transfer    `json:"transfers,omitempty"`
	Accounts  []AuthAccount `json:"accounts,omitempty"`
	// Usage is each owner's sink account usage, served by the sink Account query.
	Usage map[string]sinkv1.AccountUsage `json:"usage,omitempty"`

	// SimFailures fail matching messages in Simulate, at their message index.
	SimFailures []SimFailure `json:"simFailures,omitempty"`
	// BroadcastFaults are consumed one per broadcast_tx_sync attempt, in order.
	BroadcastFaults []BroadcastFault `json:"broadcastFaults,omitempty"`
	// UnorderedDisabled refuses every transaction as a chain without unordered transactions does.
	UnorderedDisabled bool `json:"unorderedDisabled,omitempty"`
	// Deliveries are consumed one per executed transaction, in block order; a non-zero Code fails that transaction
	// after its replay key is stored, so it pays its fee and changes nothing.
	Deliveries []Delivery `json:"deliveries,omitempty"`
}

// SinkState is the module state the State query serves; the refund head is derived from the transfers.
type SinkState struct {
	Totals      sinkv1.Totals     `json:"totals"`
	Epoch       uint64            `json:"epoch"`
	SendUsage   sinkv1.EpochUsage `json:"sendUsage"`
	RefundUsage sinkv1.EpochUsage `json:"refundUsage"`
}

// Transfer is one version of a transfer, from Height on; zero means from the earliest height.
type Transfer struct {
	Height   int64                `json:"height,omitempty"`
	Transfer sinkv1.TransferState `json:"transfer"`
}

// AuthAccount is a base account; PubKey is a compressed secp256k1 key, absent before the account signs.
type AuthAccount struct {
	Address string `json:"address"`
	Number  uint64 `json:"number"`
	PubKey  []byte `json:"pubKey,omitempty"`
}

// SimFailure fails, in Simulate, every message about transfer ID (canonical text) of TypeURL, or of any sink type
// when TypeURL is empty.
type SimFailure struct {
	ID      string `json:"id"`
	TypeURL string `json:"typeUrl,omitempty"`
	Error   string `json:"error"`
}

// FaultKind is how a broadcast attempt goes wrong (spec §6.5).
type FaultKind string

// Broadcast faults.
const (
	// FaultNone admits the attempt normally.
	FaultNone FaultKind = ""
	// FaultTransport drops the connection before admission.
	FaultTransport FaultKind = "transport"
	// FaultLostResponse admits the transaction, then drops the connection.
	FaultLostResponse FaultKind = "lost-response"
	// FaultInCache answers the JSON-RPC error "tx already exists in cache".
	FaultInCache FaultKind = "in-cache"
	// FaultNonceTaken refuses the transaction with "failed to add unordered nonce".
	FaultNonceTaken FaultKind = "nonce-taken"
	// FaultReject refuses the transaction in CheckTx with Code, Codespace and Log.
	FaultReject FaultKind = "reject"
)

// BroadcastFault is the outcome injected into one broadcast attempt.
type BroadcastFault struct {
	Kind      FaultKind `json:"kind,omitempty"`
	Code      uint32    `json:"code,omitempty"`
	Codespace string    `json:"codespace,omitempty"`
	Log       string    `json:"log,omitempty"`
}

// Delivery is the result injected into one executed transaction; Code zero lets it execute.
type Delivery struct {
	Code      uint32 `json:"code,omitempty"`
	Codespace string `json:"codespace,omitempty"`
	Log       string `json:"log,omitempty"`
}

var encoding = sync.OnceValue(func() sdkutil.EncodingConfig {
	cfg := sdkutil.MakeEncodingConfig()
	authtypes.RegisterInterfaces(cfg.InterfaceRegistry)
	banktypes.RegisterInterfaces(cfg.InterfaceRegistry)
	sinkv1.RegisterInterfaces(cfg.InterfaceRegistry)

	return cfg
})

// TxConfig is the transaction configuration the node decodes broadcasts with: akash's encoding config with the
// sink, auth and bank types.
func TxConfig() client.TxConfig { return encoding().TxConfig }

// akashRuntime is the node's state beyond the scenario: its mempool, results and replay keys.
type akashRuntime struct {
	// shift rebases every Akash time for AlignWindow; lead is the latest block time minus the clock at Start, which
	// new blocks keep.
	shift, lead time.Duration
	lag         int64
	halted      bool
	earliest    int64
	index       transferIndex

	pending []*akashTx
	// known holds the hashes of pending and included transactions, which admission refuses as already in cache.
	known   map[string]struct{}
	results map[string]*txRecord
	blocks  map[int64][][]byte
	// nonces are the stored replay keys.
	nonces     map[replayKey]struct{}
	executions map[string]int
	broadcasts [][]byte
}

type replayKey struct {
	timeout int64
	signer  string
}

type txRecord struct {
	height int64
	index  uint32
	raw    []byte
	result execResult
}

type execResult struct {
	code               uint32
	codespace, log     string
	gasWanted, gasUsed int64
}

// akashTx is a broadcast or gossiped transaction, decoded with TxConfig.
type akashTx struct {
	raw       []byte
	hash      string
	decodeErr error
	msgs      []sdk.Msg
	unordered bool
	timeout   time.Time
	signers   [][]byte
	gas       uint64
	// local marks a transaction this node admitted; only those hold replay keys at admission.
	local bool
}

func decodeTx(raw []byte) *akashTx {
	tx := &akashTx{raw: bytes.Clone(raw), hash: string(cmttypes.Tx(raw).Hash())}

	decoded, err := TxConfig().TxDecoder()(raw)
	if err != nil {
		tx.decodeErr = err
		return tx
	}

	tx.msgs = decoded.GetMsgs()
	if u, ok := decoded.(sdk.TxWithUnordered); ok {
		tx.unordered, tx.timeout = u.GetUnordered(), u.GetTimeoutTimeStamp()
	}
	if f, ok := decoded.(sdk.FeeTx); ok {
		tx.gas = f.GetGas()
	}
	if s, ok := decoded.(authsigning.SigVerifiableTx); ok {
		if tx.signers, err = s.GetSigners(); err != nil {
			tx.decodeErr = err
		}
	}

	return tx
}

func (tx *akashTx) keys() []replayKey {
	keys := make([]replayKey, len(tx.signers))
	for i, s := range tx.signers {
		keys[i] = replayKey{timeout: tx.timeout.UnixNano(), signer: string(s)}
	}

	return keys
}

func gasFor(msgs int) int64 { return gasPerTx + gasPerMsg*int64(msgs) }

// initAkash prepares the Akash runtime at Start; the caller owns n.
func (n *Node) initAkash() {
	if n.sc.Akash == nil {
		n.sc.Akash = &Akash{}
	}

	a := n.sc.Akash
	n.ak = akashRuntime{
		known:      map[string]struct{}{},
		results:    map[string]*txRecord{},
		blocks:     map[int64][][]byte{},
		nonces:     map[replayKey]struct{}{},
		executions: map[string]int{},
	}

	if off, ok := a.BlockTimes[a.Height]; ok && a.Height > 0 {
		if n.opts.AlignWindow {
			n.ak.shift = aligned(n.base).Sub(n.base.Add(off))
		}
		n.ak.lead = off + n.ak.shift
	}

	n.indexAkash()
}

// aligned returns the time in the first minute of a Unix-aligned two-minute window that is at most 70 s before now.
func aligned(now time.Time) time.Time {
	into := time.Duration(now.UnixNano() % int64(alignWindow))
	if into < 0 {
		into += alignWindow
	}

	return now.Add(-min(into, alignMaxBehind))
}

// indexAkash rebuilds the transfer index and the earliest height; the caller holds n.mu or owns n.
func (n *Node) indexAkash() {
	if n.sc.Akash == nil {
		n.sc.Akash = &Akash{}
	}

	a := n.sc.Akash

	n.ak.earliest = 0
	for h := range a.BlockTimes {
		if n.ak.earliest == 0 || h < n.ak.earliest {
			n.ak.earliest = h
		}
	}

	n.ak.index = buildIndex(a.Transfers)
}

func (n *Node) akashTime(off time.Duration) time.Time { return n.base.Add(n.ak.shift + off) }

func (n *Node) akashBlockTime(h int64) (time.Time, bool) {
	off, ok := n.sc.Akash.BlockTimes[h]
	if !ok {
		return time.Time{}, false
	}

	return n.akashTime(off), true
}

// latestTime is the latest block's time, which admission checks timeouts against.
func (n *Node) latestTime() time.Time {
	t, _ := n.akashBlockTime(n.sc.Akash.Height)
	return t
}

// servedHeight is the latest height the node reports, SetLag blocks behind the chain.
func (n *Node) servedHeight() int64 {
	h := n.sc.Akash.Height - n.ak.lag
	if h < n.ak.earliest {
		return min(n.ak.earliest, n.sc.Akash.Height)
	}

	return h
}

// appHeight is the application's last committed height as the node reports it.
func (n *Node) appHeight() int64 {
	app := n.sc.Akash.Height
	if n.sc.Akash.AppHeight != 0 {
		app = n.sc.Akash.AppHeight
	}

	return min(app, n.servedHeight())
}

// queryHeight resolves a query height as BaseApp's query context does: zero is the latest committed height.
func (n *Node) queryHeight(h int64) (int64, error) {
	app := n.appHeight()

	switch {
	case h < 0:
		return 0, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "cannot query with height < 0; please provide a valid height")
	case app <= 0:
		return 0, errorsmod.Wrap(sdkerrors.ErrInvalidHeight, "akash is not ready; please wait for first block")
	case h == 0:
		return app, nil
	case h > app:
		return 0, errorsmod.Wrap(sdkerrors.ErrInvalidHeight, "cannot query with height in the future; please provide a valid height")
	case h < n.ak.earliest:
		return 0, errorsmod.Wrapf(sdkerrors.ErrNotFound,
			"failed to load state at height %d; version does not exist (latest height: %d)", h, app)
	}

	return h, nil
}

// policy returns the retained policy of version.
func (n *Node) policy(version uint64) (sinkv1.ReporterPolicy, bool) {
	for _, p := range n.sc.Akash.Policies {
		if p.Version == version {
			return p, true
		}
	}

	return sinkv1.ReporterPolicy{}, false
}

func (n *Node) currentPolicy() uint64 {
	var v uint64
	for _, p := range n.sc.Akash.Policies {
		v = max(v, p.Version)
	}

	return v
}

// SetLag makes status, abci_info, block, tx and unpinned queries report the height blocks below the latest, like a
// lagging node behind a load balancer.
func (n *Node) SetLag(blocks int64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ak.lag = max(blocks, 0)
}

// Halt stops block production: CommitBlock and the BlockInterval ticker do nothing afterwards. The node still
// admits transactions.
func (n *Node) Halt() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ak.halted = true
}

// Broadcasts returns the bytes of every broadcast_tx_sync attempt, in order.
func (n *Node) Broadcasts() [][]byte {
	n.mu.RLock()
	defer n.mu.RUnlock()

	out := make([][]byte, len(n.ak.broadcasts))
	for i, b := range n.ak.broadcasts {
		out[i] = bytes.Clone(b)
	}

	return out
}

// Executions counts the code-0 executions of each message key (spec §6.1), duplicate no-ops included:
// "<type URL>|<id>|<outcome>|<round>|<policy version>" for a vote, "<type URL>|<id>|<round at execution>" for a veto
// and "<type URL>|<id>" for a refund.
func (n *Node) Executions() map[string]int {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return maps.Clone(n.ak.executions)
}

// Gossip adds a transaction to the pending pool without admission, as if another node had accepted it. It holds no
// replay key at this node's admission, so a copy can be admitted here and lose in the block. Bytes already pending
// or included are ignored.
func (n *Node) Gossip(txBytes []byte) {
	n.mu.Lock()
	defer n.mu.Unlock()

	tx := decodeTx(txBytes)
	if _, ok := n.ak.known[tx.hash]; ok {
		return
	}

	n.ak.known[tx.hash] = struct{}{}
	n.ak.pending = append(n.ak.pending, tx)
}

// CommitBlock produces the next block from every pending transaction and advances the height and time (spec §2.2).
func (n *Node) CommitBlock() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.ak.halted {
		return
	}

	n.commit(true, nil)
}

// Resolve applies a governance resolution in a block of its own, as x/sink/keeper/settlement.go:296-362 does: a new
// round under the current policy, without votes, hold or veto, with status, and a new challenge window from the
// block's time when status is STATUS_REFUND_PENDING. A confirmation (STATUS_MINTED, or STATUS_REFUND_PENDING for a
// confirmed void) records the governance observation, with the slot and signature of evidence[0] when given; the
// restart statuses record none. Pending transactions stay pending. The block is produced even on a halted node. It
// panics on an unknown transfer, a test error.
func (n *Node) Resolve(id sinkid.ID, status sinkv1.Status, evidence ...sinkv1.Observation) {
	n.mu.Lock()
	defer n.mu.Unlock()

	current, _ := n.policy(n.currentPolicy())
	n.commit(false, func(s *stage) {
		t, key, err := s.get(id.Proto())
		if err != nil {
			panic(fmt.Sprintf("simnode: resolve %s: %v", id, err))
		}

		next := *t
		st := &next.Settlement
		next.Status = status
		st.Round++
		st.Policy = current
		st.Votes, st.Hold = nil, sinkv1.HoldNone
		st.VetoBy, st.VetoReason, st.VetoPolicyVersion = "", "", 0
		st.ChallengeEnd, st.GovernanceObservation = 0, nil
		if outcome, ok := confirmedOutcome[status]; ok {
			observed := sinkv1.Observation{Outcome: outcome}
			if len(evidence) > 0 {
				observed.Slot, observed.TransactionSignature = evidence[0].Slot, evidence[0].TransactionSignature
			}
			st.GovernanceObservation = &observed
		}
		if status == sinkv1.StatusRefundPending {
			st.ChallengeEnd = deadline(s.time, n.sc.Akash.Params.ReporterChallengePeriod)
		}
		s.put(key, &next)
	})
}

// confirmedOutcome is the outcome a governance confirmation of each status records: ConfirmMint produces
// STATUS_MINTED and ConfirmVoid STATUS_REFUND_PENDING (x/sink/keeper/settlement.go:310-357).
var confirmedOutcome = map[sinkv1.Status]sinkv1.Outcome{
	sinkv1.StatusMinted:        sinkv1.OutcomeMinted,
	sinkv1.StatusRefundPending: sinkv1.OutcomeVoided,
}

// deadline is the challenge end in whole Unix seconds, rounded up as x/sink/keeper/settlement.go challengeDeadline does.
func deadline(now time.Time, period time.Duration) int64 {
	end := now.Add(period)
	if end.Nanosecond() > 0 {
		return end.Unix() + 1
	}

	return end.Unix()
}

// nextBlockTime follows the clock at the Start offset and strictly increases. It never equals the timeout of a held
// or pending replay key, staying clear of the SDK boundary that TestUnorderedReplayKeyBoundary covers.
func (n *Node) nextBlockTime() time.Time {
	t := n.opts.Clock.Now().Add(n.ak.lead)
	if n.sc.Akash.Height > 0 {
		if prev := n.latestTime(); !t.After(prev) {
			t = prev.Add(minBlockStep)
		}
	}

	for n.isReplayTimeout(t) {
		t = t.Add(time.Nanosecond)
	}

	return t
}

func (n *Node) isReplayTimeout(t time.Time) bool {
	ns := t.UnixNano()
	for k := range n.ak.nonces {
		if k.timeout == ns {
			return true
		}
	}

	return slices.ContainsFunc(n.ak.pending, func(tx *akashTx) bool { return tx.unordered && tx.timeout.UnixNano() == ns })
}

// commit produces one block, executing the pending pool when withTxs and then effect; the caller holds n.mu.
func (n *Node) commit(withTxs bool, effect func(*stage)) {
	a := n.sc.Akash
	h := a.Height + 1
	t := n.nextBlockTime()

	// 1. A replay key is kept until the first block whose time reaches its timeout (x/auth PreBlock).
	for k := range n.ak.nonces {
		if k.timeout <= t.UnixNano() {
			delete(n.ak.nonces, k)
		}
	}

	s := n.newStage(a.Height, t)
	var txs [][]byte
	if withTxs {
		pending := n.ak.pending
		n.ak.pending = nil

		for _, tx := range pending {
			// 2. Expired transactions leave the mempool before the block is built.
			if tx.decodeErr == nil && tx.unordered && tx.timeout.Before(t) {
				delete(n.ak.known, tx.hash)
				continue
			}

			// 3. The rest execute in arrival order.
			r := &txRecord{height: h, index: uint32(len(txs)), raw: tx.raw, result: n.deliver(s, tx, t)} //nolint:gosec // block sizes are small
			n.ak.results[tx.hash] = r
			txs = append(txs, tx.raw)

			// CometBFT keeps only successful transactions in its cache (mempool/clist_mempool.go Update, with
			// KeepInvalidTxsInCache false), so a failed one can be sent again.
			if r.result.code != 0 {
				delete(n.ak.known, tx.hash)
			}
		}
	}

	if effect != nil {
		effect(s)
	}

	n.writeStage(s, h)
	a.Height = h
	if a.AppHeight != 0 {
		a.AppHeight++
	}
	if a.BlockTimes == nil {
		a.BlockTimes = map[int64]time.Duration{}
	}
	a.BlockTimes[h] = t.Sub(n.base) - n.ak.shift
	if n.ak.earliest == 0 {
		n.ak.earliest = h
	}
	n.ak.blocks[h] = txs
}

// deliver executes one transaction in a block at time t.
func (n *Node) deliver(s *stage, tx *akashTx, t time.Time) execResult {
	gasWanted := int64(tx.gas) //nolint:gosec // gas limits fit int64
	fail := func(err error) execResult {
		space, code, log := errorsmod.ABCIInfo(err, false)
		return execResult{code: code, codespace: space, log: log, gasWanted: gasWanted, gasUsed: gasPerTx}
	}

	if err := n.anteError(tx, t); err != nil {
		return fail(err)
	}

	keys := tx.keys()
	for _, k := range keys {
		if _, held := n.ak.nonces[k]; held {
			return fail(nonceError(k))
		}
	}

	// The key is stored and the fee charged; balances are not modelled.
	for _, k := range keys {
		n.ak.nonces[k] = struct{}{}
	}

	used := gasFor(len(tx.msgs))
	if len(n.sc.Akash.Deliveries) > 0 {
		d := n.sc.Akash.Deliveries[0]
		n.sc.Akash.Deliveries = n.sc.Akash.Deliveries[1:]
		if d.Code != 0 {
			return execResult{code: d.Code, codespace: d.Codespace, log: d.Log, gasWanted: gasWanted, gasUsed: used}
		}
	}

	execs, err := s.run(tx.msgs, nil)
	if err != nil {
		r := fail(err)
		r.gasUsed = used

		return r
	}

	for _, e := range execs {
		n.ak.executions[e]++
	}

	return execResult{gasWanted: gasWanted, gasUsed: used}
}

// anteError applies the unordered-transaction checks of x/auth/ante/sigverify.go against a block time.
func (n *Node) anteError(tx *akashTx, blockTime time.Time) error {
	switch {
	case tx.decodeErr != nil:
		return errorsmod.Wrap(sdkerrors.ErrTxDecode, tx.decodeErr.Error())
	case n.sc.Akash.UnorderedDisabled:
		return errorsmod.Wrap(sdkerrors.ErrNotSupported, "unordered transactions are not enabled")
	case !tx.unordered:
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "simnode accepts unordered transactions only")
	case tx.timeout.IsZero() || tx.timeout.Unix() == 0:
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "unordered transaction must have timeout_timestamp set")
	case tx.timeout.Before(blockTime):
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "unordered transaction has a timeout_timestamp that has already passed")
	case tx.timeout.After(blockTime.Add(maxUnorderedTTL)):
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unordered tx ttl exceeds %s", maxUnorderedTTL.String())
	}

	return nil
}

func nonceError(k replayKey) error {
	sender, err := bech32.ConvertAndEncode("akash", []byte(k.signer))
	if err != nil {
		sender = fmt.Sprintf("%X", k.signer)
	}

	return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "failed to add unordered nonce: %s",
		fmt.Errorf("sender %s has already used timeout %d", sender, k.timeout))
}

// errInCache is CometBFT's mempool error, which broadcast_tx_sync returns as a JSON-RPC error.
var errInCache = errors.New("tx already exists in cache")

// checkTx admits tx to the pending pool as CometBFT and the ante handler do; the caller holds n.mu. A non-nil error
// is a JSON-RPC error, not a CheckTx result.
func (n *Node) checkTx(tx *akashTx) (execResult, error) {
	if _, ok := n.ak.known[tx.hash]; ok {
		return execResult{}, errInCache
	}

	reject := func(err error) (execResult, error) {
		space, code, log := errorsmod.ABCIInfo(err, false)
		return execResult{code: code, codespace: space, log: log}, nil
	}

	if err := n.anteError(tx, n.latestTime()); err != nil {
		return reject(err)
	}

	for _, k := range tx.keys() {
		if n.replayKeyHeld(k) {
			return reject(nonceError(k))
		}
	}

	tx.local = true
	n.ak.known[tx.hash] = struct{}{}
	n.ak.pending = append(n.ak.pending, tx)

	return execResult{}, nil
}

// replayKeyHeld reports a key stored by a block or held by a transaction this node admitted.
func (n *Node) replayKeyHeld(k replayKey) bool {
	if _, ok := n.ak.nonces[k]; ok {
		return true
	}

	return slices.ContainsFunc(n.ak.pending, func(tx *akashTx) bool {
		return tx.local && slices.Contains(tx.keys(), k)
	})
}

// stage is the sink state as a block or a simulation changes it: the state at height plus changes.
type stage struct {
	n       *Node
	height  int64
	time    time.Time
	changes map[string]*sinkv1.TransferState
}

func (n *Node) newStage(height int64, t time.Time) *stage {
	return &stage{n: n, height: height, time: t, changes: map[string]*sinkv1.TransferState{}}
}

func (s *stage) get(id sinkv1.ID) (*sinkv1.TransferState, string, error) {
	key := string(keyOf(id))
	if t, ok := s.changes[key]; ok {
		return t, key, nil
	}

	if e := s.n.ak.index.byKey[key]; e != nil {
		if t := e.at(s.height); t != nil {
			return t, key, nil
		}
	}

	return nil, key, sinkv1.ErrTransferNotFound
}

func (s *stage) put(key string, t *sinkv1.TransferState) { s.changes[key] = t }

// run applies msgs atomically, returning their execution keys. fail injects a failure for message i.
func (s *stage) run(msgs []sdk.Msg, fail func(i int, msg sdk.Msg) error) ([]string, error) {
	fork := &stage{n: s.n, height: s.height, time: s.time, changes: maps.Clone(s.changes)}

	execs := make([]string, 0, len(msgs))
	for i, msg := range msgs {
		var err error
		if fail != nil {
			err = fail(i, msg)
		}

		var exec string
		if err == nil {
			exec, err = fork.apply(msg)
		}
		if err != nil {
			return nil, errorsmod.Wrapf(err, "failed to execute message; message index: %d", i)
		}

		execs = append(execs, exec)
	}

	s.changes = fork.changes

	return execs, nil
}

func (s *stage) apply(msg sdk.Msg) (string, error) {
	switch m := msg.(type) {
	case *sinkv1.MsgReportOutcome:
		return s.vote(m)
	case *sinkv1.MsgVetoVoid:
		return s.veto(m)
	case *sinkv1.MsgFinalizeRefund:
		return s.refund(m)
	default:
		return "", errorsmod.Wrapf(sdkerrors.ErrUnknownRequest, "simnode does not execute %s", sdk.MsgTypeURL(msg))
	}
}

// vote follows ReportOutcome (x/sink/keeper/settlement.go:88-150): a stale policy or round fails; a conflict hold, a
// terminal, refund-pending or held transfer, or a repeated reporter makes it a no-op; a stale settlement policy is
// refreshed first; a contradicting vote sets HOLD_CONFLICT. Quorum and the status changes it causes are not modelled.
func (s *stage) vote(m *sinkv1.MsgReportOutcome) (string, error) {
	t, key, err := s.get(m.ID)
	if err != nil {
		return "", err
	}

	current, ok := s.n.policy(s.n.currentPolicy())
	if !ok || m.PolicyVersion != current.Version || m.Round != t.Settlement.Round {
		return "", sinkv1.ErrStaleReport
	}

	exec := fmt.Sprintf("%s|%s|%d|%d|%d", sdk.MsgTypeURL(m), idText(m.ID), m.Observation.Outcome, m.Round, m.PolicyVersion)
	if t.Settlement.Hold == sinkv1.HoldConflict {
		return exec, nil
	}

	// Changes stay on next and are written only when the vote is recorded, as the keeper writes its cache context.
	next := *t
	st := &next.Settlement
	if st.Policy.Version != current.Version && st.Hold == sinkv1.HoldNone && !next.Status.Terminal() && st.GovernanceObservation == nil {
		st.Policy, st.Votes, st.ChallengeEnd = current, nil, 0
		next.Status = sinkv1.StatusTransferPending
		if st.VoidAuthorization != nil {
			next.Status = sinkv1.StatusVoided
		}
	}

	primary := sinkv1.OutcomeUnspecified
	switch {
	case st.GovernanceObservation != nil:
		primary = st.GovernanceObservation.Outcome
	case len(st.Votes) > 0:
		primary = st.Votes[0].Observation.Outcome
	}

	conflict := primary != sinkv1.OutcomeUnspecified && primary != m.Observation.Outcome
	if !conflict {
		if next.Status.Terminal() || next.Status == sinkv1.StatusRefundPending || st.Hold != sinkv1.HoldNone {
			return exec, nil
		}
		if slices.ContainsFunc(st.Votes, func(v sinkv1.Vote) bool { return v.Reporter == m.Reporter }) {
			return exec, nil
		}
	}

	st.Votes = append(slices.Clone(st.Votes), sinkv1.Vote{
		Reporter:      m.Reporter,
		Observation:   m.Observation,
		PolicyVersion: m.PolicyVersion,
	})
	if conflict {
		st.Hold = sinkv1.HoldConflict
	}
	s.put(key, &next)

	return exec, nil
}

// veto holds a pending refund (x/sink/keeper/settlement.go:204-245); a repeated veto executes again.
func (s *stage) veto(m *sinkv1.MsgVetoVoid) (string, error) {
	t, key, err := s.get(m.ID)
	if err != nil {
		return "", err
	}

	if t.Status != sinkv1.StatusRefundPending {
		return "", sinkv1.ErrNoPendingVoid
	}

	if s.time.Unix() >= t.Settlement.ChallengeEnd {
		return "", sinkv1.ErrChallengeWindowEnded
	}

	next := *t
	st := &next.Settlement
	if st.Hold != sinkv1.HoldConflict {
		st.Hold = sinkv1.HoldVeto
	}
	st.VetoBy, st.VetoReason, st.VetoPolicyVersion = m.Signer, m.Reason, s.n.currentPolicy()
	s.put(key, &next)

	return fmt.Sprintf("%s|%s|%d", sdk.MsgTypeURL(m), idText(m.ID), t.Settlement.Round), nil
}

// refund pays the refund head, or is a no-op for a paid refund (x/sink/keeper/settlement.go:385-480). Caps, payability
// and the challenge window are not modelled.
func (s *stage) refund(m *sinkv1.MsgFinalizeRefund) (string, error) {
	t, key, err := s.get(m.ID)
	if err != nil {
		return "", err
	}

	exec := fmt.Sprintf("%s|%s", sdk.MsgTypeURL(m), idText(m.ID))
	if t.Status == sinkv1.StatusRefunded {
		return exec, nil
	}

	if head := s.refundHead(); head == nil || string(keyOf(*head)) != key {
		return "", sinkv1.ErrRefundOutOfOrder
	}

	next := *t
	next.Status = sinkv1.StatusRefunded
	next.Settlement.ChallengeEnd = 0
	s.put(key, &next)

	return exec, nil
}

// refundHead is the queue head of x/sink/keeper/refundq.go: the unheld refund-pending transfer under governance or
// the current policy with the earliest challenge end, then the lowest record key.
func (s *stage) refundHead() *sinkv1.ID {
	current := s.n.currentPolicy()

	var head *sinkv1.TransferState
	for _, e := range s.n.ak.index.record {
		t, ok := s.changes[string(e.key)]
		if !ok {
			t = e.at(s.height)
		}

		if t == nil || t.Status != sinkv1.StatusRefundPending || t.Settlement.Hold != sinkv1.HoldNone || t.Settlement.ChallengeEnd <= 0 {
			continue
		}
		if t.Settlement.GovernanceObservation == nil && t.Settlement.Policy.Version != current {
			continue
		}
		if head == nil || t.Settlement.ChallengeEnd < head.Settlement.ChallengeEnd {
			head = t
		}
	}

	if head == nil {
		return nil
	}

	id := head.ID

	return &id
}

// writeStage records the stage's changes as versions at height h, in the scenario and the index.
func (n *Node) writeStage(s *stage, h int64) {
	keys := slices.Sorted(maps.Keys(s.changes))
	for _, key := range keys {
		t := s.changes[key]
		n.sc.Akash.Transfers = append(n.sc.Akash.Transfers, Transfer{Height: h, Transfer: *t})
		n.ak.index.add(key, h, t)
	}
}

func idText(id sinkv1.ID) string { return fmt.Sprintf("%s/%d/%d", id.Owner, id.Height, id.Sequence) }

// keyOf is the transfer's record key, len(owner) || owner || BE64(height) || BE64(sequence), which orders the
// keeper's record and settlement stores. A non-canonical ID keeps a distinct key after every canonical one.
func keyOf(id sinkv1.ID) []byte {
	if sid, err := sinkid.FromProto(id); err == nil {
		return sid.KeySuffix()
	}

	out := append([]byte{0xff}, id.Owner...)
	out = binary.BigEndian.AppendUint64(out, uint64(id.Height)) //nolint:gosec // any bits will do

	return binary.BigEndian.AppendUint64(out, id.Sequence)
}

// compareChrono orders record keys chronologically, as the keeper's history: height, sequence, then owner.
func compareChrono(a, b []byte) int {
	tail := func(k []byte) (uint64, uint64) {
		if len(k) < 16 {
			return 0, 0
		}

		return binary.BigEndian.Uint64(k[len(k)-16:]), binary.BigEndian.Uint64(k[len(k)-8:])
	}

	ah, as := tail(a)
	bh, bs := tail(b)

	return cmp.Or(cmp.Compare(ah, bh), cmp.Compare(as, bs), bytes.Compare(a, b))
}

type transferVersion struct {
	height int64
	state  *sinkv1.TransferState
}

type transferEntry struct {
	key      []byte
	versions []transferVersion
}

// at returns the version in effect at height h, or nil before the transfer exists.
func (e *transferEntry) at(h int64) *sinkv1.TransferState {
	i := sort.Search(len(e.versions), func(i int) bool { return e.versions[i].height > h })
	if i == 0 {
		return nil
	}

	return e.versions[i-1].state
}

// transferIndex holds every transfer's versions, in record-key and in chronological order.
type transferIndex struct {
	byKey  map[string]*transferEntry
	record []*transferEntry
	chrono []*transferEntry
}

func buildIndex(transfers []Transfer) transferIndex {
	idx := transferIndex{byKey: map[string]*transferEntry{}}
	for i := range transfers {
		t := &transfers[i]
		key := keyOf(t.Transfer.ID)

		e := idx.byKey[string(key)]
		if e == nil {
			e = &transferEntry{key: key}
			idx.byKey[string(key)] = e
			idx.record = append(idx.record, e)
		}
		e.versions = append(e.versions, transferVersion{height: t.Height, state: &t.Transfer})
	}

	for _, e := range idx.record {
		// A later version at the same height replaces an earlier one.
		slices.SortStableFunc(e.versions, func(a, b transferVersion) int { return cmp.Compare(a.height, b.height) })

		kept := e.versions[:0]
		for _, v := range e.versions {
			if last := len(kept) - 1; last >= 0 && kept[last].height == v.height {
				kept[last] = v
				continue
			}
			kept = append(kept, v)
		}
		e.versions = kept
	}

	idx.chrono = slices.Clone(idx.record)
	slices.SortFunc(idx.record, func(a, b *transferEntry) int { return bytes.Compare(a.key, b.key) })
	slices.SortFunc(idx.chrono, func(a, b *transferEntry) int { return compareChrono(a.key, b.key) })

	return idx
}

// add records a new version of an existing transfer at height h, the newest height.
func (idx *transferIndex) add(key string, h int64, t *sinkv1.TransferState) {
	e := idx.byKey[key]
	if last := len(e.versions) - 1; last >= 0 && e.versions[last].height == h {
		e.versions[last].state = t
		return
	}

	e.versions = append(e.versions, transferVersion{height: h, state: t})
}

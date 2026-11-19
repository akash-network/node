package observe_test

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/observe"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	contextSlot  = uint64(110)
	resolvedSlot = uint64(100)
	chainID      = "akash-1"
	maxAge       = 60 * time.Second
	// slotLead is defaultSlotLead(60) of the TS reporter: (60 + 60) × 5.
	slotLead = uint64(600)
	vetoMsg  = "/akash.sink.v1.MsgVetoVoid"
	reporter = "reporter"

	errStatusID      = "wrong status transfer ID"
	errConfigOwner   = "wrong configuration owner"
	errMintConfig    = "wrong AKT mint configuration"
	errMintAuthority = "wrong mint authority"
)

var start = time.Unix(1_800_000_000, 0)

func seedKey(label string) solana.PublicKey {
	return solana.PublicKeyFromBytes(bytes.Repeat([]byte(label), 32)[:32])
}

func filled(fill byte, n int) []byte { return bytes.Repeat([]byte{fill}, n) }

func sigString(fill byte) string { return solana.SignatureFromBytes(filled(fill, 64)).String() }

// fixture mirrors the TS fixture of solana.test.ts on simnode: a deployed bridge at finalized slot 110 and a
// minted status resolved at slot 100, whose block holds a failed replay and then the resolution.
type fixture struct {
	program     solana.PublicKey
	mint        solana.PublicKey
	config      solana.PublicKey
	bridge      solana.PublicKey
	address     solana.PublicKey
	lightClient solana.PublicKey
	record      sinkid.Record
	status      simnode.TransferStatus
	clock       *clock.Fake
	node        *simnode.Node
	rpc         *aktsolana.RPC
}

type option func(*fixture)

func withProgram(program solana.PublicKey) option { return func(f *fixture) { f.program = program } }

func withRecord(change func(*sinkid.Record)) option { return func(f *fixture) { change(&f.record) } }

func mustBech(t *testing.T, raw []byte) string {
	t.Helper()

	s, err := bech32.ConvertAndEncode("akash", raw)
	require.NoError(t, err)

	return s
}

func newFixture(t *testing.T, opts ...option) *fixture {
	t.Helper()

	owner := filled(42, 20)
	f := &fixture{
		program:     simnode.ProgramID,
		lightClient: simnode.LightClient,
		clock:       clock.NewFake(start),
		record: sinkid.Record{
			ID:        sinkid.ID{Owner: mustBech(t, owner), OwnerBytes: owner, Height: 2, Sequence: 1},
			Recipient: seedKey("r"),
			Amount:    60,
		},
	}
	for _, o := range opts {
		o(f)
	}

	f.mint = aktsolana.PDA(f.program, []byte("mint"))
	f.config = aktsolana.PDA(f.program, []byte("config"))
	f.bridge = aktsolana.PDA(f.program, []byte("bridge"))
	f.address = aktsolana.StatusAddress(f.program, f.record.ID)
	f.status = simnode.TransferStatus{
		ID:             f.record.ID,
		ProofHeight:    uint64(f.record.ID.Height) + 1, //nolint:gosec // positive test height
		ResolvedAtSlot: resolvedSlot,
		Outcome:        simnode.Outcome{Minted: &simnode.Minted{Amount: f.record.Amount, Recipient: f.record.Recipient}},
	}

	authority := aktsolana.PDA(f.program, []byte("mint_authority"))
	header := simnode.Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1}
	keys := []solana.PublicKey{f.record.Recipient, f.address, f.program}
	sc := simnode.Scenario{Solana: simnode.Solana{
		GenesisHash: simnode.GenesisHash,
		Slot:        contextSlot,
		BlockTimes:  map[uint64]time.Duration{contextSlot: 0},
		Accounts: map[solana.PublicKey]simnode.Account{
			f.program: {Owner: solana.BPFLoaderUpgradeableProgramID, Executable: true, Lamports: 1},
			f.config: {Owner: f.program, Lamports: 1, Data: simnode.EncodeLaunchConfig(simnode.LaunchConfig{
				Version: 2, Initializer: f.record.Recipient, Mint: f.mint, BridgeEnabled: true,
				InitializedAtSlot: 1, Bump: 1, MintAuthorityBump: 1,
			})},
			f.bridge: {Owner: f.program, Lamports: 1, Data: bridgeData(f.lightClient)},
			f.mint: {Owner: solana.TokenProgramID, Lamports: 1, Data: simnode.EncodeMint(simnode.Mint{
				MintAuthority: &authority, Supply: 60, Decimals: 6, Initialized: true,
			})},
			f.address: {Owner: f.program, Lamports: 1, Data: simnode.EncodeTransferStatus(f.status)},
		},
		Blocks: map[uint64]simnode.Block{resolvedSlot: {Transactions: []simnode.Transaction{
			// A failed replay in the same block emits the event bytes before failing; it must not supply the signature.
			{
				Signatures: []string{sigString(9)}, Header: header, AccountKeys: keys,
				Err: json.RawMessage(`{"InstructionError":[0,"Custom"]}`),
				Logs: []string{
					"Program " + f.program.String() + " invoke [1]",
					simnode.TransferResolvedLog(f.status),
					"Program " + f.program.String() + " failed: custom program error: 0x0",
				},
			},
			{Signatures: []string{sigString(7)}, Header: header, AccountKeys: keys, Logs: f.logs(f.status)},
		}}},
	}}

	n, err := simnode.Start(t.Context(), sc, simnode.Options{Clock: f.clock})
	require.NoError(t, err)
	t.Cleanup(n.Close)
	f.node = n
	f.rpc = aktsolana.NewRPC(aktsolana.RPCOptions{URL: n.SolanaURL, Timeout: 5 * time.Second, Concurrency: 4, BlockConcurrency: 2})

	return f
}

func bridgeData(lightClient solana.PublicKey) []byte {
	return simnode.EncodeBridgeConfig(simnode.BridgeConfig{
		LightClientProgram: lightClient, ChainID: chainID,
		PerTransferMax: 100, DailyMintCap: 1000, MintDay: 1, MintedToday: 60,
	})
}

func (f *fixture) logs(st simnode.TransferStatus) []string {
	return []string{
		"Program " + f.program.String() + " invoke [1]",
		simnode.TransferResolvedLog(st),
		"Program " + f.program.String() + " success",
	}
}

func (f *fixture) options() observe.Options {
	pin := f.lightClient

	return observe.Options{
		GenesisHash:    simnode.GenesisHash,
		ProgramID:      f.program,
		Mint:           f.mint,
		LightClientPin: &pin,
		ChainID:        chainID,
		MaxAge:         maxAge,
		SlotLead:       slotLead,
		Clock:          f.clock,
	}
}

// observer returns an observer with a fresh evidence index.
func (f *fixture) observer(change func(*observe.Options), onChange func(observe.Change)) *observe.Observer {
	o := f.options()
	if change != nil {
		change(&o)
	}

	return observe.New(f.rpc, aktsolana.NewEvidenceIndex(f.rpc, f.program, 256, 64<<20), o, onChange)
}

func (f *fixture) solana(change func(*simnode.Solana)) {
	f.node.Mutate(func(sc *simnode.Scenario) { change(&sc.Solana) })
}

func (f *fixture) setAccount(address solana.PublicKey, change func(*simnode.Account)) {
	f.solana(func(s *simnode.Solana) {
		a := s.Accounts[address]
		change(&a)
		s.Accounts[address] = a
	})
}

func (f *fixture) removeAccount(address solana.PublicKey) {
	f.solana(func(s *simnode.Solana) { delete(s.Accounts, address) })
}

func (f *fixture) setResolution(change func(*simnode.Transaction)) {
	f.solana(func(s *simnode.Solana) { change(&s.Blocks[resolvedSlot].Transactions[1]) })
}

func observeOne(
	t *testing.T,
	o *observe.Observer,
	rec sinkid.Record,
	reported []sinkv1.Observation,
	own *sinkv1.Observation,
) (policy.Observed, error) {
	t.Helper()

	chunk, err := o.Chunk(t.Context(), []sinkid.ID{rec.ID})
	if err != nil {
		return policy.Observed{}, err
	}

	return o.Observe(t.Context(), chunk, rec, reported, own)
}

func finalizedOne(t *testing.T, o *observe.Observer, rec sinkid.Record) (observe.Finalized, error) {
	t.Helper()

	chunk, err := o.Chunk(t.Context(), []sinkid.ID{rec.ID})
	if err != nil {
		return observe.Finalized{}, err
	}

	return o.Finalized(chunk, rec)
}

func report(slot uint64, outcome sinkv1.Outcome, fill byte) sinkv1.Observation {
	return sinkv1.Observation{Outcome: outcome, Slot: slot, TransactionSignature: filled(fill, 64)}
}

func voidReport(slot uint64) sinkv1.Observation { return report(slot, sinkv1.OutcomeVoided, 5) }

func voidReports(slots ...uint64) []sinkv1.Observation {
	out := make([]sinkv1.Observation, 0, len(slots))
	for _, s := range slots {
		out = append(out, voidReport(s))
	}

	return out
}

func ptr(v uint64) *uint64 { return &v }

func requireMinted(t *testing.T, observed policy.Observed, signature []byte) {
	t.Helper()

	require.Equal(t, policy.KindResolved, observed.Kind)
	require.Equal(t, contextSlot, observed.ContextSlot)
	require.Equal(t, sinkv1.OutcomeMinted, observed.Observation.Outcome)
	require.Equal(t, signature, observed.Observation.TransactionSignature)
}

// voided is a void that reached threshold and is inside its challenge window, with the given votes.
func voided(rec sinkid.Record, observations []sinkv1.Observation) policy.Snapshot {
	threshold := sdkmath.LegacyMustNewDecFromStr("0.75")
	members := []string{reporter, "b", "c", "d"}
	rp := sinkv1.ReporterPolicy{Version: 1, Members: members, Threshold: threshold}

	votes := make([]sinkv1.Vote, 0, len(observations))
	for i, o := range observations {
		votes = append(votes, sinkv1.Vote{Reporter: members[i+1], PolicyVersion: 1, Observation: o})
	}

	return policy.Snapshot{
		Params: sinkv1.Params{Reporters: members, ReportersThreshold: threshold, RefundsEnabled: true},
		Policy: rp,
		Time:   86409,
		Transfer: sinkv1.TransferState{
			ID:         rec.ID.Proto(),
			Data:       sinkv1.RecordData{Dest: rec.Recipient.String(), Amount: sdk.NewInt64Coin("uakt", 60)},
			Status:     sinkv1.StatusRefundPending,
			Settlement: sinkv1.SettlementData{Round: 1, ChallengeEnd: 86410, Policy: rp, Votes: votes},
		},
	}
}

func planURL(t *testing.T, s policy.Snapshot, o policy.Observed) (string, []string) {
	t.Helper()

	p, err := policy.PlanOutcome(reporter, s, o)
	require.NoError(t, err)
	require.NotEmpty(t, p.Messages)

	return sdk.MsgTypeURL(p.Messages[0]), p.Alerts
}

func TestObserverFromTS(t *testing.T) {
	sig7 := filled(7, 64)

	t.Run("observer uses the configured deployment without rebuilding its IDL", func(t *testing.T) {
		program := solana.PublicKeyFromBytes(filled(42, 32))
		require.NotEqual(t, simnode.ProgramID, program)

		f := newFixture(t, withProgram(program))
		observed, err := observeOne(t, f.observer(nil, nil), f.record, nil, nil)
		require.NoError(t, err)
		requireMinted(t, observed, sig7)
	})

	t.Run("configured deployment retains mint and account ownership checks", func(t *testing.T) {
		program := solana.PublicKeyFromBytes(filled(42, 32))

		t.Run("mint", func(t *testing.T) {
			f := newFixture(t, withProgram(program))
			oldMint := aktsolana.PDA(simnode.ProgramID, []byte("mint"))
			require.PanicsWithValue(t, "observe: noncanonical mint", func() {
				f.observer(func(o *observe.Options) { o.Mint = oldMint }, nil)
			})
		})

		t.Run("owner", func(t *testing.T) {
			f := newFixture(t, withProgram(program))
			f.setAccount(f.address, func(a *simnode.Account) { a.Owner = simnode.ProgramID })
			_, err := observeOne(t, f.observer(nil, nil), f.record, nil, nil)
			require.EqualError(t, err, "wrong transfer status owner")
		})
	})

	t.Run("observer rejects wrong cluster, status owner, amount, recipient and stale state", func(t *testing.T) {
		for _, tc := range []struct {
			failure string
			apply   func(f *fixture, rec *sinkid.Record)
			want    string
		}{
			{"cluster", func(f *fixture, _ *sinkid.Record) {
				f.solana(func(s *simnode.Solana) { s.GenesisHash = "different" })
			}, "wrong Solana cluster"},
			{"owner", func(f *fixture, _ *sinkid.Record) {
				f.setAccount(f.address, func(a *simnode.Account) { a.Owner = solana.SystemProgramID })
			}, "wrong transfer status owner"},
			{"amount", func(_ *fixture, rec *sinkid.Record) { rec.Amount++ }, "mint amount differs from Akash deposit"},
			{"recipient", func(_ *fixture, rec *sinkid.Record) { rec.Recipient = seedKey("x") }, "mint recipient differs from Akash deposit"},
			{"stale", func(f *fixture, _ *sinkid.Record) {
				f.solana(func(s *simnode.Solana) { s.BlockTimes[contextSlot] = -time.Hour })
			}, "stale Solana finalized state"},
			{"event", func(f *fixture, _ *sinkid.Record) {
				f.setResolution(func(tx *simnode.Transaction) { tx.Logs = []string{} })
			}, aktsolana.ErrNotInBlock.Error()},
			{"failed", func(f *fixture, _ *sinkid.Record) {
				f.setResolution(func(tx *simnode.Transaction) { tx.Err = json.RawMessage(`{"InstructionError":[0,"InvalidArgument"]}`) })
			}, aktsolana.ErrNotInBlock.Error()},
		} {
			t.Run(tc.failure, func(t *testing.T) {
				f := newFixture(t)
				rec := f.record
				tc.apply(f, &rec)
				_, err := observeOne(t, f.observer(nil, nil), rec, nil, nil)
				require.EqualError(t, err, tc.want)
			})
		}
	})

	t.Run("missing status stays absent; pruned transaction evidence never produces a report", func(t *testing.T) {
		f := newFixture(t)
		o := f.observer(nil, nil)

		f.solana(func(s *simnode.Solana) {
			s.Failures = map[string]simnode.Failure{"getBlock": {Code: -32001, Message: "Block 100 cleaned up, does not exist on node", Times: 1}}
		})
		_, err := observeOne(t, o, f.record, nil, nil)
		require.ErrorIs(t, err, aktsolana.ErrBlockMissing)
		require.ErrorContains(t, err, "evidence unavailable: block missing")

		f.solana(func(s *simnode.Solana) { s.Blocks = map[uint64]simnode.Block{} })
		_, err = observeOne(t, o, f.record, nil, nil)
		require.ErrorContains(t, err, "evidence unavailable: block missing")

		f.solana(func(s *simnode.Solana) { s.Blocks[resolvedSlot] = simnode.Block{Transactions: []simnode.Transaction{}} })
		_, err = observeOne(t, o, f.record, nil, nil)
		require.EqualError(t, err, "resolution evidence unavailable in its block")

		f.removeAccount(f.address)
		_, err = observeOne(t, o, f.record, voidReports(111), nil)
		require.ErrorContains(t, err, "behind reported evidence")

		observed, err := observeOne(t, o, f.record, nil, nil)
		require.NoError(t, err)
		require.Equal(t, policy.Observed{Kind: policy.KindAbsent, ContextSlot: contextSlot, SlotLimit: ptr(contextSlot + slotLead)}, observed)
	})

	t.Run("a fabricated slot beyond any plausible finalized slot cannot block a veto", func(t *testing.T) {
		f := newFixture(t)
		f.removeAccount(f.address)

		snapshot := voided(f.record, voidReports(1<<63, 1<<63, 1<<63))
		observed, err := observeOne(t, f.observer(nil, nil), f.record, policy.ReportedObservations(snapshot.Transfer), nil)
		require.NoError(t, err)
		require.Equal(t, policy.Observed{Kind: policy.KindAbsent, ContextSlot: contextSlot, SlotLimit: ptr(710)}, observed)
		require.Zero(t, f.node.Calls("getTransaction"))

		url, alerts := planURL(t, snapshot, observed)
		require.Equal(t, vetoMsg, url)
		require.Contains(t, alerts, policy.AlertFabricatedSlot)

		// A configured lead replaces the default derived from maxAgeSeconds.
		strict := f.observer(func(o *observe.Options) { o.SlotLead = 50 }, nil)
		near := voided(f.record, voidReports(161, 161, 161))
		observed, err = observeOne(t, strict, f.record, policy.ReportedObservations(near.Transfer), nil)
		require.NoError(t, err)
		url, _ = planURL(t, near, observed)
		require.Equal(t, vetoMsg, url)

		require.PanicsWithValue(t, "observe: invalid slot lead", func() {
			f.observer(func(o *observe.Options) { o.SlotLead = 0 }, nil)
		})
	})

	t.Run("a lagging but fresh node never vetoes a void it has not reached", func(t *testing.T) {
		f := newFixture(t)
		f.removeAccount(f.address)
		o := f.observer(nil, nil)

		for _, slots := range [][]uint64{{111, 111, 111}, {710, 710, 710}, {1 << 63, 400, 400}} {
			snapshot := voided(f.record, voidReports(slots...))
			_, err := observeOne(t, o, f.record, policy.ReportedObservations(snapshot.Transfer), nil)
			require.EqualError(t, err, "Solana node is behind reported evidence; absence cannot authorize a veto")
		}
		require.Zero(t, f.node.Calls("getTransaction"))
	})

	t.Run("a reported void signature missing at a passed slot is vetoed", func(t *testing.T) {
		f := newFixture(t)
		f.removeAccount(f.address)
		// Signature 5 names a transaction at slot 105 whose first signature is the resolution's, so a node returns a
		// transaction for it that is not the reported one.
		f.solana(func(s *simnode.Solana) {
			res := s.Blocks[resolvedSlot].Transactions[1]
			res.Signatures = []string{sigString(7), sigString(5)}
			s.Blocks[105] = simnode.Block{Transactions: []simnode.Transaction{res}}
		})
		o := f.observer(nil, nil)

		snapshot := voided(f.record, []sinkv1.Observation{
			report(100, sinkv1.OutcomeVoided, 7), voidReport(105), voidReport(110),
		})
		// Neither a transaction under another first signature nor the mint at slot 100 shows a void.
		observed, err := observeOne(t, o, f.record, policy.ReportedObservations(snapshot.Transfer), nil)
		require.NoError(t, err)
		require.Equal(t, policy.KindAbsent, observed.Kind)
		url, _ := planURL(t, snapshot, observed)
		require.Equal(t, vetoMsg, url)

		// A missing transaction shows nothing either.
		f.solana(func(s *simnode.Solana) { delete(s.Blocks, 105) })
		observed, err = observeOne(t, o, f.record, policy.ReportedObservations(snapshot.Transfer), nil)
		require.NoError(t, err)
		url, _ = planURL(t, snapshot, observed)
		require.Equal(t, vetoMsg, url)

		// A finalized transaction that does show the reported outcome means this node's missing status is
		// inconsistent; it must not support a veto.
		before := f.node.Calls("getTransaction")
		_, err = observeOne(t, o, f.record, []sinkv1.Observation{voidReport(100), report(100, sinkv1.OutcomeMinted, 7)}, nil)
		require.EqualError(t, err, "reported Solana transaction contradicts missing status")
		require.Equal(t, before+2, f.node.Calls("getTransaction"))

		// The same transaction reported at another slot is false evidence.
		observed, err = observeOne(t, o, f.record, []sinkv1.Observation{report(101, sinkv1.OutcomeMinted, 7)}, nil)
		require.NoError(t, err)
		require.Equal(t, policy.KindAbsent, observed.Kind)

		f.solana(func(s *simnode.Solana) {
			s.Failures = map[string]simnode.Failure{"getTransaction": {Code: -32000, Message: "connection reset"}}
		})
		_, err = observeOne(t, o, f.record, voidReports(100), nil)
		require.ErrorIs(t, err, aktsolana.ErrUnavailable)
		require.ErrorContains(t, err, "temporarily unavailable")
	})

	t.Run("a stale node never vetoes, even against a fabricated slot", func(t *testing.T) {
		f := newFixture(t)
		f.removeAccount(f.address)
		f.solana(func(s *simnode.Solana) { s.BlockTimes[contextSlot] = -time.Hour })

		snapshot := voided(f.record, voidReports(1<<63, 1<<63, 1<<63))
		_, err := observeOne(t, f.observer(nil, nil), f.record, policy.ReportedObservations(snapshot.Transfer), nil)
		require.EqualError(t, err, "stale Solana finalized state")
	})

	t.Run("a fabricated future report slot cannot hide an existing finalized mint", func(t *testing.T) {
		f := newFixture(t)
		observed, err := observeOne(t, f.observer(nil, nil), f.record, voidReports(math.MaxUint64), nil)
		require.NoError(t, err)
		requireMinted(t, observed, sig7)
	})

	t.Run("own stored evidence survives pruned history while finalized status is rechecked", func(t *testing.T) {
		f := newFixture(t)
		initial, err := observeOne(t, f.observer(nil, nil), f.record, nil, nil)
		require.NoError(t, err)
		requireMinted(t, initial, sig7)

		f.solana(func(s *simnode.Solana) { delete(s.Blocks, resolvedSlot) })
		blocks := f.node.Calls("getBlock")
		o := f.observer(nil, nil)
		again, err := observeOne(t, o, f.record, nil, &initial.Observation)
		require.NoError(t, err)
		require.Equal(t, initial, again)
		require.Equal(t, blocks, f.node.Calls("getBlock"))

		rec := f.record
		rec.Amount++
		_, err = observeOne(t, o, rec, nil, &initial.Observation)
		require.EqualError(t, err, "mint amount differs from Akash deposit")

		f.clock.Advance(maxAge + time.Second)
		_, err = observeOne(t, o, f.record, nil, &initial.Observation)
		require.EqualError(t, err, "stale Solana finalized state")
	})

	t.Run("different or malformed stored evidence cannot replace transaction discovery", func(t *testing.T) {
		f := newFixture(t)
		initial, err := observeOne(t, f.observer(nil, nil), f.record, nil, nil)
		require.NoError(t, err)

		f.solana(func(s *simnode.Solana) { delete(s.Blocks, resolvedSlot) })
		o := f.observer(nil, nil)
		base := initial.Observation
		for name, own := range map[string]sinkv1.Observation{
			"slot":          {Outcome: base.Outcome, Slot: 101, TransactionSignature: base.TransactionSignature},
			"outcome":       {Outcome: sinkv1.OutcomeVoided, Slot: base.Slot, TransactionSignature: base.TransactionSignature},
			"short":         {Outcome: base.Outcome, Slot: base.Slot, TransactionSignature: filled(7, 63)},
			"all zero":      {Outcome: base.Outcome, Slot: base.Slot, TransactionSignature: make([]byte, 64)},
			"empty":         {Outcome: base.Outcome, Slot: base.Slot},
			"over 64 bytes": {Outcome: base.Outcome, Slot: base.Slot, TransactionSignature: filled(7, 65)},
		} {
			_, err := observeOne(t, o, f.record, nil, &own)
			require.ErrorIs(t, err, aktsolana.ErrBlockMissing, name)
			require.ErrorContains(t, err, "evidence unavailable", name)
		}
	})

	t.Run("uint64 deposit amounts and identity components retain precision", func(t *testing.T) {
		f := newFixture(t, withRecord(func(r *sinkid.Record) {
			r.ID.Height = 9007199254740993
			r.ID.Sequence = math.MaxUint64
			r.Amount = 9007199254740993
		}))

		fin, err := finalizedOne(t, f.observer(nil, nil), f.record)
		require.NoError(t, err)
		require.Equal(t, policy.KindResolved, fin.Kind)
		require.Equal(t, uint64(9007199254740993), fin.Status.ID.Height)
		require.Equal(t, uint64(math.MaxUint64), fin.Status.ID.Sequence)
		require.Equal(t, uint64(9007199254740993), fin.Status.Outcome.Minted.Amount)

		observed, err := observeOne(t, f.observer(nil, nil), f.record, nil, nil)
		require.NoError(t, err)
		requireMinted(t, observed, sig7)

		rec := f.record
		rec.Amount = 9007199254740992
		_, err = observeOne(t, f.observer(nil, nil), rec, nil, nil)
		require.EqualError(t, err, "mint amount differs from Akash deposit")
	})

	t.Run("a pinned light client fails closed when governance re-pins", func(t *testing.T) {
		f := newFixture(t)
		changes := 0
		o := f.observer(nil, func(observe.Change) { changes++ })

		d, err := o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, f.lightClient, d.LightClient)

		repinned := seedKey("p")
		f.setAccount(f.bridge, func(a *simnode.Account) { a.Data = bridgeData(repinned) })

		reads := map[string]func() error{
			"deployment": func() error { _, err := o.Deployment(t.Context()); return err },
			"observe":    func() error { _, err := observeOne(t, o, f.record, nil, nil); return err },
			"finalized":  func() error { _, err := finalizedOne(t, o, f.record); return err },
		}
		for name, read := range reads {
			err := read()
			var mismatch *observe.MismatchError
			require.ErrorAs(t, err, &mismatch, name)
			require.Equal(t, repinned, mismatch.Current)
			require.Equal(t, f.lightClient, mismatch.Pinned)
			require.EqualError(t, err, "bridge light client "+repinned.String()+" differs from pinned "+f.lightClient.String())
		}
		require.Zero(t, f.node.Calls("getBlock"))
		require.Zero(t, changes)
	})

	t.Run("an unpinned observer follows the bridge light client and records changes", func(t *testing.T) {
		f := newFixture(t)
		first := f.lightClient
		var changes []observe.Change
		o := f.observer(func(o *observe.Options) { o.LightClientPin = nil }, func(c observe.Change) { changes = append(changes, c) })

		d, err := o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, first, d.LightClient)
		observed, err := observeOne(t, o, f.record, nil, nil)
		require.NoError(t, err)
		require.Equal(t, policy.KindResolved, observed.Kind)
		require.Empty(t, changes)

		second := seedKey("s")
		f.setAccount(f.bridge, func(a *simnode.Account) { a.Data = bridgeData(second) })
		observed, err = observeOne(t, o, f.record, nil, nil)
		require.NoError(t, err)
		requireMinted(t, observed, sig7)
		d, err = o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, second, d.LightClient)
		require.Equal(t, []observe.Change{{Previous: first, Current: second, ContextSlot: contextSlot}}, changes)
		changes = nil

		// An older finalized snapshot cannot move the followed program back.
		at := func(slot uint64) {
			f.solana(func(s *simnode.Solana) {
				s.Slot = slot
				s.BlockTimes[slot] = 0
				a := s.Accounts[f.bridge]
				a.Data = bridgeData(first)
				s.Accounts[f.bridge] = a
			})
		}
		at(105)
		d, err = o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, first, d.LightClient)
		require.Empty(t, changes)

		at(120)
		_, err = o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, []observe.Change{{Previous: second, Current: first, ContextSlot: 120}}, changes)
	})

	t.Run("observer binds every composite component and proof height in finalized accounts", func(t *testing.T) {
		for _, tc := range []struct {
			component string
			change    func(st *simnode.TransferStatus)
			want      string
		}{
			{"owner", func(st *simnode.TransferStatus) { st.ID.OwnerBytes = filled(9, 20) }, errStatusID},
			{"height", func(st *simnode.TransferStatus) { st.ID.Height++ }, errStatusID},
			{"sequence", func(st *simnode.TransferStatus) { st.ID.Sequence++ }, errStatusID},
			{"proof", func(st *simnode.TransferStatus) { st.ProofHeight = uint64(st.ID.Height) }, "proof predates deposit commitment"}, //nolint:gosec // positive
		} {
			t.Run(tc.component, func(t *testing.T) {
				f := newFixture(t)
				st := f.status
				tc.change(&st)
				f.setAccount(f.address, func(a *simnode.Account) { a.Data = simnode.EncodeTransferStatus(st) })
				_, err := finalizedOne(t, f.observer(nil, nil), f.record)
				require.EqualError(t, err, tc.want)
			})
		}
	})
}

// TestDeploymentChecks covers each check of the deployment snapshot.
func TestDeploymentChecks(t *testing.T) {
	t.Run("deployment reports the finalized bridge", func(t *testing.T) {
		f := newFixture(t)
		d, err := f.observer(nil, nil).Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, observe.Deployment{ContextSlot: contextSlot, BridgeEnabled: true, LightClient: f.lightClient}, d)

		f.setAccount(f.config, func(a *simnode.Account) {
			a.Data = simnode.EncodeLaunchConfig(simnode.LaunchConfig{Version: 2, Mint: f.mint})
		})
		d, err = f.observer(nil, nil).Deployment(t.Context())
		require.NoError(t, err)
		require.False(t, d.BridgeEnabled)
	})

	authority := func(f *fixture) *solana.PublicKey {
		k := aktsolana.PDA(f.program, []byte("mint_authority"))
		return &k
	}
	setMint := func(f *fixture, m simnode.Mint) {
		f.setAccount(f.mint, func(a *simnode.Account) { a.Data = simnode.EncodeMint(m) })
	}
	goodMint := func(f *fixture) simnode.Mint {
		return simnode.Mint{MintAuthority: authority(f), Supply: 60, Decimals: 6, Initialized: true}
	}

	for _, tc := range []struct {
		name  string
		apply func(f *fixture)
		want  string
	}{
		{"zero slot", func(f *fixture) { f.solana(func(s *simnode.Solana) { s.Slot = 0 }) }, "Solana node has no finalized positive slot"},
		{"future block time", func(f *fixture) {
			f.solana(func(s *simnode.Solana) { s.BlockTimes[contextSlot] = 11 * time.Second })
		}, "stale Solana finalized state"},
		{"program not executable", func(f *fixture) {
			f.setAccount(f.program, func(a *simnode.Account) { a.Executable = false })
		}, "mint program is not executable"},
		{"program missing", func(f *fixture) { f.removeAccount(f.program) }, "mint program is not executable"},
		{"launch owner", func(f *fixture) {
			f.setAccount(f.config, func(a *simnode.Account) { a.Owner = solana.SystemProgramID })
		}, errConfigOwner},
		{"launch executable", func(f *fixture) {
			f.setAccount(f.config, func(a *simnode.Account) { a.Executable = true })
		}, errConfigOwner},
		{"bridge missing", func(f *fixture) { f.removeAccount(f.bridge) }, errConfigOwner},
		{"launch version", func(f *fixture) {
			f.setAccount(f.config, func(a *simnode.Account) {
				a.Data = simnode.EncodeLaunchConfig(simnode.LaunchConfig{Version: 1, Mint: f.mint, BridgeEnabled: true})
			})
		}, "unsupported launch configuration"},
		{"launch mint", func(f *fixture) {
			f.setAccount(f.config, func(a *simnode.Account) {
				a.Data = simnode.EncodeLaunchConfig(simnode.LaunchConfig{Version: 2, Mint: seedKey("m"), BridgeEnabled: true})
			})
		}, "wrong configured mint"},
		{"bridge chain", func(f *fixture) {
			f.setAccount(f.bridge, func(a *simnode.Account) {
				a.Data = simnode.EncodeBridgeConfig(simnode.BridgeConfig{LightClientProgram: f.lightClient, ChainID: "akash-2"})
			})
		}, "wrong bridge origin chain"},
		{"mint missing", func(f *fixture) { f.removeAccount(f.mint) }, "AKT mint account is missing"},
		{"mint owner", func(f *fixture) {
			f.setAccount(f.mint, func(a *simnode.Account) { a.Owner = solana.Token2022ProgramID })
		}, "AKT mint is not owned by the SPL Token program"},
		{"mint size", func(f *fixture) {
			f.setAccount(f.mint, func(a *simnode.Account) { a.Data = append(a.Data, 0) })
		}, "wrong AKT mint configuration: decode Mint: 83 bytes, want 82"},
		{"mint decimals", func(f *fixture) { m := goodMint(f); m.Decimals = 9; setMint(f, m) }, errMintConfig},
		{"mint uninitialized", func(f *fixture) { m := goodMint(f); m.Initialized = false; setMint(f, m) }, errMintConfig},
		{"mint freeze authority", func(f *fixture) {
			m := goodMint(f)
			m.FreezeAuthority = authority(f)
			setMint(f, m)
		}, errMintConfig},
		{"mint authority", func(f *fixture) {
			m := goodMint(f)
			k := seedKey("a")
			m.MintAuthority = &k
			setMint(f, m)
		}, errMintAuthority},
		{"no mint authority", func(f *fixture) { m := goodMint(f); m.MintAuthority = nil; setMint(f, m) }, errMintAuthority},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.apply(f)
			o := f.observer(nil, nil)

			_, err := o.Deployment(t.Context())
			require.EqualError(t, err, tc.want)
			_, err = o.Chunk(t.Context(), []sinkid.ID{f.record.ID})
			require.EqualError(t, err, tc.want)
		})
	}
}

// keyCounter forwards to the Solana node and records the key count of every getMultipleAccounts call.
type keyCounter struct {
	mu    sync.Mutex
	calls []int
}

func (k *keyCounter) proxy(t *testing.T, target string) string {
	t.Helper()

	u, err := url.Parse(target)
	require.NoError(t, err)
	forward := httputil.NewSingleHostReverseProxy(u)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		var keys []string
		if json.Unmarshal(body, &req) == nil && req.Method == "getMultipleAccounts" && json.Unmarshal(req.Params[0], &keys) == nil {
			k.mu.Lock()
			k.calls = append(k.calls, len(keys))
			k.mu.Unlock()
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		forward.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

func TestChunkUsesOneAccountsCall(t *testing.T) {
	f := newFixture(t)
	var counter keyCounter
	f.rpc = aktsolana.NewRPC(aktsolana.RPCOptions{URL: counter.proxy(t, f.node.SolanaURL), Timeout: 5 * time.Second})
	o := f.observer(nil, nil)

	ids := make([]sinkid.ID, 0, 96)
	ids = append(ids, f.record.ID)
	for i := range 95 {
		id := f.record.ID
		id.Height = int64(100 + i)
		ids = append(ids, id)
	}

	chunk, err := o.Chunk(t.Context(), ids)
	require.NoError(t, err)
	require.Equal(t, []int{100}, counter.calls)
	require.Equal(t, 1, f.node.Calls("getMultipleAccounts"))
	require.Equal(t, 1, f.node.Calls("getGenesisHash"))
	require.Equal(t, 1, f.node.Calls("getBlockTime"))

	for i, id := range ids {
		rec := f.record
		rec.ID = id
		fin, err := o.Finalized(chunk, rec)
		require.NoError(t, err)
		require.Equal(t, contextSlot, fin.ContextSlot)
		if i == 0 {
			require.Equal(t, policy.KindResolved, fin.Kind)
			require.Equal(t, f.address, fin.Address)
			require.Equal(t, sinkv1.OutcomeMinted, fin.Outcome)
			require.Equal(t, resolvedSlot, fin.Slot)
			continue
		}
		require.Equal(t, policy.KindAbsent, fin.Kind)
	}

	// Observations reuse the snapshot: no further account read.
	observed, err := o.Observe(t.Context(), chunk, f.record, nil, nil)
	require.NoError(t, err)
	require.Equal(t, policy.KindResolved, observed.Kind)
	require.Equal(t, 1, f.node.Calls("getMultipleAccounts"))

	// A transfer outside the chunk has no status in the snapshot.
	rec := f.record
	rec.ID.Height = 5
	_, err = o.Finalized(chunk, rec)
	require.EqualError(t, err, "transfer status is not in the chunk snapshot")
}

func TestNullBlockTimeIsStale(t *testing.T) {
	f := newFixture(t)
	f.solana(func(s *simnode.Solana) { delete(s.BlockTimes, contextSlot) })
	o := f.observer(nil, nil)

	_, err := o.Chunk(t.Context(), []sinkid.ID{f.record.ID})
	require.EqualError(t, err, "stale Solana finalized state")
	_, err = o.Deployment(t.Context())
	require.EqualError(t, err, "stale Solana finalized state")
}

func TestFollowRuleOutOfOrder(t *testing.T) {
	f := newFixture(t)
	a, b := seedKey("A"), seedKey("B")
	var changes []observe.Change
	o := f.observer(func(o *observe.Options) { o.LightClientPin = nil }, func(c observe.Change) { changes = append(changes, c) })

	for _, step := range []struct {
		slot    uint64
		program solana.PublicKey
	}{{110, a}, {105, b}, {108, a}, {120, b}, {130, b}, {125, a}, {140, a}} {
		f.solana(func(s *simnode.Solana) {
			s.Slot = step.slot
			s.BlockTimes[step.slot] = 0
			acct := s.Accounts[f.bridge]
			acct.Data = bridgeData(step.program)
			s.Accounts[f.bridge] = acct
		})
		d, err := o.Deployment(t.Context())
		require.NoError(t, err)
		require.Equal(t, step.program, d.LightClient)
	}

	require.Equal(t, []observe.Change{
		{Previous: a, Current: b, ContextSlot: 120},
		{Previous: b, Current: a, ContextSlot: 140},
	}, changes)
}

// TestFollowStateIsSerialized runs onChange from concurrent snapshots: the follow lock lets only one run at a time.
func TestFollowStateIsSerialized(t *testing.T) {
	f := newFixture(t)
	other := seedKey("o")

	var (
		mu      sync.Mutex
		inside  int
		maxSeen int
	)
	o := f.observer(func(o *observe.Options) { o.LightClientPin = nil }, func(observe.Change) {
		mu.Lock()
		inside++
		maxSeen = max(maxSeen, inside)
		mu.Unlock()
		time.Sleep(time.Millisecond)
		mu.Lock()
		inside--
		mu.Unlock()
	})

	_, err := o.Deployment(t.Context())
	require.NoError(t, err)

	// Every snapshot now alternates the bridge at one slot; equal slots are not older, so each one may change it.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			program := f.lightClient
			if i%2 == 0 {
				program = other
			}
			f.setAccount(f.bridge, func(a *simnode.Account) { a.Data = bridgeData(program) })
			_, _ = o.Deployment(t.Context())
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.LessOrEqual(t, maxSeen, 1)
}

func TestOwnSignatureWithZeroBytes(t *testing.T) {
	f := newFixture(t)
	f.solana(func(s *simnode.Solana) { delete(s.Blocks, resolvedSlot) })
	o := f.observer(nil, nil)

	sig := filled(0, 64)
	sig[17] = 3
	sig[63] = 0xff
	own := sinkv1.Observation{Outcome: sinkv1.OutcomeMinted, Slot: resolvedSlot, TransactionSignature: sig}

	observed, err := observeOne(t, o, f.record, nil, &own)
	require.NoError(t, err)
	requireMinted(t, observed, sig)
	require.Equal(t, resolvedSlot, observed.Observation.Slot)
	require.Zero(t, f.node.Calls("getBlock"))
}

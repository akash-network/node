package simnode

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strconv"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// The identities every named scenario shares, for configuring a reporter against it. None is a mainnet identity.
const (
	ChainID = "simnode-1"
	// GenesisHash is Solana devnet's.
	GenesisHash = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	// ReporterMnemonic derives Reporter on akash's HD path m/44'/118'/0'/0/0.
	ReporterMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon " +
		"abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon art"
	// ReporterMaxAge is the max-age the goldens configure their reporters with, the reporter's default.
	ReporterMaxAge = 60 * time.Second
	nodeID         = "5ca1ab1e0000000000000000000000000000c0de"
)

var (
	ProgramID   = seedKey("program")
	LightClient = seedKey("light-client")
	MintAddress = aktsolana.PDA(ProgramID, []byte("mint"))
	// Reporter is the policy member whose key ReporterMnemonic derives.
	Reporter = reporterAddress()

	// memberA and memberB are the other members of the first policy; memberC replaces memberB in a rotation.
	memberA = mustBech(bytes.Repeat([]byte{0xa1}, 20))
	memberB = mustBech(bytes.Repeat([]byte{0xa2}, 20))
	memberC = mustBech(bytes.Repeat([]byte{0xa3}, 20))
)

// Shape of the named scenarios.
const (
	scenarioHeight   = int64(100)
	blockSpacing     = 6 * time.Second
	finalizedSlot    = uint64(1_000)
	resolvedSlotBase = uint64(900)
	transferAmount   = 1_000_000
	staleAge         = 15 * time.Minute
	votesCount       = 30
	cleanCount       = 5
	// pagingCount fills more than one listing page of the reporter's default page size, 500.
	pagingCount = 501
	// auditCount spans three audit pages of the default audit-per-scan, 100.
	auditCount = 250
	// auditResumeCount spans two audit pages.
	auditResumeCount = 150
	spamCount        = 500
	// challengeEnded and challengeOpen are challenge ends long past and far ahead of any run, in Unix seconds.
	challengeEnded = int64(1_000_000_000)
	challengeOpen  = int64(4_000_000_000)
	// fabricatedSlot lies beyond any slot a fresh node could have finalized.
	fabricatedSlot = uint64(1) << 63
)

// Scenarios returns the named scenarios that goldens and benchmarks serve, each built fresh by its function. The
// comment of each names the parity cases of spec §11.2 it covers.
func Scenarios() map[string]func() Scenario {
	return map[string]func() Scenario{
		// clean: open transfers without a Solana status or reported observation, which plan nothing.
		"clean": func() Scenario { return openScenario(time.Second, cleanCount) },
		// stale-akash: the latest Akash block is older than any max-age.
		"stale-akash": func() Scenario { return openScenario(staleAge, cleanCount) },
		// votes: void-requested transfers voided on Solana without this reporter's vote; each plans one vote.
		"votes": votesScenario,
		// stale-solana: the finalized Solana block is older than any max-age.
		"stale-solana": staleSolanaScenario,
		// lagging-akash: the application trails the latest block; decisions use its last committed height.
		"lagging-akash": laggingAkashScenario,
		// lagging-solana: a fresh Solana node behind every reported slot never vetoes.
		"lagging-solana": laggingSolanaScenario,
		// evidence: replayed evidence, CPI and truncated logs, and block spam.
		"evidence": evidenceScenario,
		// wrong-accounts: wrong status owners, amounts and recipients, and the other finalized status checks.
		"wrong-accounts": wrongAccountsScenario,
		// reported-slots: fabricated future slots.
		"reported-slots": reportedSlotsScenario,
		// missing-evidence: missing reported signatures and missing history.
		"missing-evidence": missingEvidenceScenario,
		// own-votes: own-vote reuse, duplicate votes and policy rotation.
		"own-votes": ownVotesScenario,
		// settlement: contradictions, vetoes and refund eligibility.
		"settlement": settlementScenario,
		// priority: dispute and refund priority; a held transfer keeps its dispute position.
		"priority": priorityScenario,
		// light-client-follow: the bridge names a new light client after the pass-start read.
		"light-client-follow": lightClientFollowScenario,
		// light-client-pin: governance re-pins the bridge after the pass-start read of a pinned reporter.
		"light-client-pin": lightClientPinScenario,
		// paging: open transfers spanning two listing pages.
		"paging": func() Scenario { return openScenario(time.Second, pagingCount) },
		// paging-overlap: a node whose second page repeats the first page's last entry.
		"paging-overlap": pagingOverlapScenario,
		// audit-unavailable: an invalid terminal record fails the audit page but not the open listing.
		"audit-unavailable": auditUnavailableScenario,
		// audit-round-robin: three passes audit consecutive pages of settled transfers; the fourth starts over.
		"audit-round-robin": auditRoundRobinScenario,
		// audit-resume: a failed audit page keeps the cursor, so the next pass resumes where the failed one began.
		"audit-resume": auditResumeScenario,
	}
}

func seedKey(label string) solana.PublicKey {
	return solana.PublicKeyFromBytes(digestBytes("simnode solana " + label))
}

func digestBytes(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}

func reporterAddress() string {
	raw, err := hd.Secp256k1.Derive()(ReporterMnemonic, "", hd.CreateHDPath(sdk.CoinType, 0, 0).String())
	if err != nil {
		panic(err)
	}

	return mustBech((&secp256k1.PrivKey{Key: raw}).PubKey().Address())
}

func mustBech(raw []byte) string {
	s, err := bech32.ConvertAndEncode("akash", raw)
	if err != nil {
		panic(err)
	}

	return s
}

// scenarioID is the i-th transfer of a named scenario: a distinct owner, in chronological order.
func scenarioID(i int) sinkid.ID {
	owner := digestBytes("simnode owner " + strconv.Itoa(i))[:20]

	return sinkid.ID{Owner: mustBech(owner), OwnerBytes: owner, Height: int64(10 + i), Sequence: 1}
}

func recipient(i int) solana.PublicKey {
	return seedKey("recipient " + strconv.Itoa(i))
}

// signature is the transaction signature a scenario derives from label.
func signature(label string) solana.Signature {
	return solana.SignatureFromBytes(append(digestBytes("sig a "+label), digestBytes("sig b "+label)...))
}

func sigBytes(label string) []byte {
	sig := signature(label)
	return sig[:]
}

func bridgeAccount(lightClient solana.PublicKey) Account {
	return Account{Owner: ProgramID, Lamports: 1, Data: EncodeBridgeConfig(BridgeConfig{
		LightClientProgram: lightClient, ChainID: ChainID,
		PerTransferMax: 1_000_000_000_000, DailyMintCap: 1_000_000_000_000,
	})}
}

func policyOf(version uint64, members ...string) sinkv1.ReporterPolicy {
	return sinkv1.ReporterPolicy{Version: version, Members: members, Threshold: sdkmath.LegacyMustNewDecFromStr("0.67")}
}

// baseScenario is a deployed bridge with a fresh Solana node and an Akash chain whose latest block is age old.
func baseScenario(age time.Duration) Scenario {
	policy := policyOf(1, Reporter, memberA, memberB)
	zero := sdk.NewInt64Coin("uakt", 0)

	times := map[int64]time.Duration{}
	for h := scenarioHeight - 9; h <= scenarioHeight; h++ {
		times[h] = -age - time.Duration(scenarioHeight-h)*blockSpacing
	}

	config := aktsolana.PDA(ProgramID, []byte("config"))
	bridge := aktsolana.PDA(ProgramID, []byte("bridge"))
	authority := aktsolana.PDA(ProgramID, []byte("mint_authority"))

	return Scenario{
		Akash: &Akash{
			ChainID:    ChainID,
			NodeID:     nodeID,
			Height:     scenarioHeight,
			BlockTimes: times,
			Params: sinkv1.Params{
				Enabled:                   true,
				MinAmountPerTransfer:      sdk.NewInt64Coin("uakt", 1),
				MaxAmountPerTransfer:      sdk.NewInt64Coin("uakt", 1_000_000_000_000),
				MaxTransferAmountPerEpoch: sdk.NewInt64Coin("uakt", 1_000_000_000_000),
				MaxEpochAmountPerAccount:  sdk.NewInt64Coin("uakt", 1_000_000_000_000),
				MaxEpochRefund:            sdk.NewInt64Coin("uakt", 1_000_000_000_000),
				Reporters:                 policy.Members,
				ReportersThreshold:        policy.Threshold,
				ReporterChallengePeriod:   72 * time.Hour,
				RefundsEnabled:            true,
			},
			Policies: []sinkv1.ReporterPolicy{policy},
			State: SinkState{
				Totals: sinkv1.Totals{
					PendingAmount: zero, MintedAmount: zero, VoidedAmount: zero, RefundPendingAmount: zero, RefundedAmount: zero,
				},
				Epoch:       20_000,
				SendUsage:   sinkv1.EpochUsage{Epoch: 20_000, Amount: zero},
				RefundUsage: sinkv1.EpochUsage{Epoch: 20_000, Amount: zero},
			},
			Accounts: []AuthAccount{{Address: Reporter, Number: 7}},
		},
		Solana: Solana{
			GenesisHash: GenesisHash,
			Slot:        finalizedSlot,
			BlockTimes:  map[uint64]time.Duration{finalizedSlot: -time.Second},
			Accounts: map[solana.PublicKey]Account{
				ProgramID: {Owner: solana.BPFLoaderUpgradeableProgramID, Executable: true, Lamports: 1, Data: []byte{2, 0, 0, 0}},
				config: {Owner: ProgramID, Lamports: 1, Data: EncodeLaunchConfig(LaunchConfig{
					Version: 2, Initializer: seedKey("initializer"), Mint: MintAddress, BridgeEnabled: true,
					InitializedAtSlot: 1, Bump: 255, MintAuthorityBump: 255,
				})},
				bridge: bridgeAccount(LightClient),
				MintAddress: {Owner: solana.TokenProgramID, Lamports: 1, Data: EncodeMint(Mint{
					MintAuthority: &authority, Supply: 1 << 40, Decimals: 6, Initialized: true,
				})},
			},
			Blocks: map[uint64]Block{finalizedSlot: {Transactions: []Transaction{}}},
		},
	}
}

func (a *Akash) policy() sinkv1.ReporterPolicy { return a.Policies[len(a.Policies)-1] }

// rotate makes a new policy of members current, as a governance rotation does.
func (a *Akash) rotate(members ...string) {
	p := policyOf(a.policy().Version+1, members...)
	a.Policies = append(a.Policies, p)
	a.Params.Reporters = p.Members
}

func (a *Akash) add(t sinkv1.TransferState) { a.Transfers = append(a.Transfers, Transfer{Transfer: t}) }

func pendingTransfer(a *Akash, i int, st sinkv1.Status) sinkv1.TransferState {
	return sinkv1.TransferState{
		ID:         scenarioID(i).Proto(),
		Data:       sinkv1.RecordData{Dest: recipient(i).String(), Amount: sdk.NewInt64Coin("uakt", transferAmount)},
		Status:     st,
		Settlement: sinkv1.SettlementData{Round: 1, Policy: a.policy()},
	}
}

// voidRequested is a void-requested transfer, authorized by a governance void order.
func voidRequested(a *Akash, i int) sinkv1.TransferState {
	t := pendingTransfer(a, i, sinkv1.StatusVoided)
	t.Settlement.VoidAuthorization = &sinkv1.VoidAuthorization{RequestedHeight: scenarioHeight - 20, RequestedRound: 1}

	return t
}

// refundPending is a confirmed void whose challenge window ends at end.
func refundPending(a *Akash, i int, end int64, votes ...sinkv1.Vote) sinkv1.TransferState {
	t := voidRequested(a, i)
	t.Status = sinkv1.StatusRefundPending
	t.Settlement.ChallengeEnd = end
	t.Settlement.Votes = votes

	return t
}

// vetoed holds t with a veto by memberA.
func vetoed(t sinkv1.TransferState) sinkv1.TransferState {
	t.Settlement.Hold = sinkv1.HoldVeto
	t.Settlement.VetoBy, t.Settlement.VetoReason, t.Settlement.VetoPolicyVersion = memberA, "No Solana status", 1

	return t
}

func vote(reporter string, version uint64, outcome sinkv1.Outcome, slot uint64, sig []byte) sinkv1.Vote {
	return sinkv1.Vote{
		Reporter:      reporter,
		PolicyVersion: version,
		Observation:   sinkv1.Observation{Outcome: outcome, Slot: slot, TransactionSignature: sig},
	}
}

func mintedTo(i int) Outcome {
	return Outcome{Minted: &Minted{Amount: transferAmount, Recipient: recipient(i)}}
}

func voidedFor(reason VoidReason) Outcome { return Outcome{Voided: &reason} }

// statusOf is the status of the i-th transfer resolved at slot.
func statusOf(i int, slot uint64, o Outcome) TransferStatus {
	id := scenarioID(i)

	return TransferStatus{ID: id, ProofHeight: uint64(id.Height) + 1, ResolvedAtSlot: slot, Outcome: o} //nolint:gosec // positive
}

func (s *Solana) putStatus(st TransferStatus) {
	s.Accounts[aktsolana.StatusAddress(ProgramID, st.ID)] = Account{Owner: ProgramID, Lamports: 1, Data: EncodeTransferStatus(st)}
}

func (s *Solana) putBlock(slot uint64, txs ...Transaction) {
	s.Blocks[slot] = Block{Transactions: txs}
	s.BlockTimes[slot] = -time.Minute
}

// resolve stores st with a block holding its one resolution transaction, signed as st's ID.
func (s *Solana) resolve(st TransferStatus) {
	s.putStatus(st)
	s.putBlock(st.ResolvedAtSlot, resolutionTx(st, st.ID.String(), aktFrame(TransferResolvedLog(st))))
}

// resolutionTx is a successful transaction signed as label that writes st's status account and logs logs.
func resolutionTx(st TransferStatus, label string, logs []string) Transaction {
	return Transaction{
		Signatures:  []string{signature(label).String()},
		Header:      Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1},
		AccountKeys: []solana.PublicKey{seedKey("payer"), aktsolana.StatusAddress(ProgramID, st.ID), ProgramID},
		Logs:        logs,
	}
}

func invoke(program solana.PublicKey, depth int) string {
	return "Program " + program.String() + " invoke [" + strconv.Itoa(depth) + "]"
}

func success(program solana.PublicKey) string { return "Program " + program.String() + " success" }

// aktFrame is the log of one top-level AKT instruction that logs lines.
func aktFrame(lines ...string) []string {
	return append(append([]string{invoke(ProgramID, 1)}, lines...), success(ProgramID))
}

// openScenario has n transfer-pending transfers without votes or a Solana status, so a review plans nothing.
func openScenario(age time.Duration, n int) Scenario {
	sc := baseScenario(age)
	for i := range n {
		sc.Akash.add(pendingTransfer(sc.Akash, i, sinkv1.StatusTransferPending))
	}

	return sc
}

// votesScenario has void-requested transfers that Solana voided by governance void order, with the resolution
// transaction in its block, and no vote from Reporter: each plans one MsgReportOutcome.
func votesScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	for i := range votesCount {
		a.add(voidRequested(a, i))

		st := statusOf(i, resolvedSlotBase+uint64(i), voidedFor(VoidGovernance)) //nolint:gosec // small
		s.putStatus(st)
		s.putBlock(st.ResolvedAtSlot, resolutionTx(st, st.ID.String(),
			aktFrame("Program log: Instruction: VoidTransfer", TransferResolvedLog(st))))
	}

	return sc
}

func staleSolanaScenario() Scenario {
	sc := openScenario(time.Second, cleanCount)
	sc.Solana.BlockTimes[finalizedSlot] = -staleAge

	return sc
}

// laggingAkashScenario serves the latest block 100 while the application has committed only 97. Transfer 5 is minted
// at 99 and transfer 6 deposited at 99, so a reporter at 97 still sees 5 pending and does not see 6.
func laggingAkashScenario() Scenario {
	sc := openScenario(time.Second, cleanCount)
	a := sc.Akash
	a.AppHeight = scenarioHeight - 3

	a.add(pendingTransfer(a, 5, sinkv1.StatusTransferPending))
	minted := pendingTransfer(a, 5, sinkv1.StatusMinted)
	a.Transfers = append(a.Transfers,
		Transfer{Height: scenarioHeight - 1, Transfer: minted},
		Transfer{Height: scenarioHeight - 1, Transfer: pendingTransfer(a, 6, sinkv1.StatusTransferPending)},
	)

	return sc
}

// laggingSolanaScenario has refund-pending transfers that memberA reports voided at a slot this fresh node has not
// finalized yet; each review fails instead of vetoing.
func laggingSolanaScenario() Scenario {
	sc := baseScenario(time.Second)
	a := sc.Akash

	for i := range cleanCount {
		id := scenarioID(i).String()
		a.add(refundPending(a, i, challengeOpen,
			vote(memberA, 1, sinkv1.OutcomeVoided, finalizedSlot+100, sigBytes("void "+id))))
	}

	return sc
}

// evidenceScenario has minted transfer-pending transfers, each resolved at its own slot by a block of one evidence
// case. Valid evidence plans a vote; invalid evidence fails the transfer.
func evidenceScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	wrapper, other, budget := seedKey("wrapper program"), seedKey("other program"), seedKey("compute budget")
	token := solana.TokenProgramID
	consumed := "Program " + ProgramID.String() + " consumed 1 of 2 compute units"
	failed := json.RawMessage(`{"InstructionError":[0,"InvalidArgument"]}`)

	cases := []func(st TransferStatus, event string) []Transaction{
		// A failed replay of the resolution does not hide it.
		func(st TransferStatus, event string) []Transaction {
			replay := resolutionTx(st, "replay "+st.ID.String(), aktFrame(event))
			replay.Err = failed

			return []Transaction{replay, resolutionTx(st, st.ID.String(), aktFrame(event))}
		},
		// Two resolutions of one status fail closed.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{
				resolutionTx(st, st.ID.String(), aktFrame(event)),
				resolutionTx(st, "again "+st.ID.String(), aktFrame(event)),
			}
		},
		// A version 1 transaction is evidence.
		func(st TransferStatus, event string) []Transaction {
			tx := resolutionTx(st, st.ID.String(), aktFrame(event))
			tx.Version = "1"

			return []Transaction{tx}
		},
		// A version 0 transaction writes the status through a lookup table.
		func(st TransferStatus, event string) []Transaction {
			tx := resolutionTx(st, st.ID.String(), aktFrame(event))
			tx.Version = "0"
			tx.AccountKeys = []solana.PublicKey{seedKey("payer"), ProgramID}
			tx.AddressTableLookups = []AddressTableLookup{{AccountKey: seedKey("lookup table"), WritableIndexes: []int{0}}}
			tx.LoadedWritable = []solana.PublicKey{aktsolana.StatusAddress(ProgramID, st.ID)}

			return []Transaction{tx}
		},
		// An AKT frame inside a wrapper's CPI counts.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{
				invoke(wrapper, 1), invoke(ProgramID, 2), invoke(token, 3), success(token),
				event, consumed, success(ProgramID), success(wrapper),
			})}
		},
		// A wrapper frame replaying AKT event bytes does not count.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{
				invoke(wrapper, 1), invoke(ProgramID, 2), success(ProgramID), event, success(wrapper),
			})}
		},
		// An event inside a nested program's frame does not count.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{
				invoke(ProgramID, 1), invoke(token, 2), event, success(token), success(ProgramID),
			})}
		},
		// Truncation after a completed top-level AKT frame keeps its event.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{
				invoke(budget, 1), success(budget),
				invoke(ProgramID, 1), invoke(other, 2), success(other), event, consumed, success(ProgramID),
				invoke(other, 1), "Program log: padding", "Log truncated",
			})}
		},
		// Truncation inside the AKT frame supplies nothing.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{
				invoke(ProgramID, 1), event, invoke(other, 2), "Log truncated",
			})}
		},
		// Unbalanced logs supply nothing.
		func(st TransferStatus, event string) []Transaction {
			return []Transaction{resolutionTx(st, st.ID.String(), []string{invoke(ProgramID, 1), event})}
		},
		// Spam referencing the status account cannot hide the resolution.
		func(st TransferStatus, event string) []Transaction {
			txs := make([]Transaction, 0, 2*spamCount+1)
			for k := range 2 * spamCount {
				if k == spamCount {
					txs = append(txs, resolutionTx(st, st.ID.String(), aktFrame(event)))
				}
				txs = append(txs, resolutionTx(st, "spam "+strconv.Itoa(k)+" "+st.ID.String(), aktFrame()))
			}

			return txs
		},
	}

	for i, build := range cases {
		a.add(pendingTransfer(a, i, sinkv1.StatusTransferPending))

		st := statusOf(i, resolvedSlotBase+uint64(i), mintedTo(i)) //nolint:gosec // small
		s.putStatus(st)
		s.putBlock(st.ResolvedAtSlot, build(st, TransferResolvedLog(st))...)
	}

	return sc
}

// wrongAccountsScenario has transfer-pending transfers whose finalized status fails one check each.
func wrongAccountsScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	cases := []func(i int, st *TransferStatus, account *Account){
		// The status account is not the program's.
		func(_ int, _ *TransferStatus, account *Account) { account.Owner = solana.SystemProgramID },
		// The mint amount differs from the deposit.
		func(i int, st *TransferStatus, _ *Account) {
			st.Outcome = Outcome{Minted: &Minted{Amount: transferAmount + 1, Recipient: recipient(i)}}
		},
		// The mint recipient differs from the deposit.
		func(_ int, st *TransferStatus, _ *Account) {
			st.Outcome = Outcome{Minted: &Minted{Amount: transferAmount, Recipient: seedKey("someone else")}}
		},
		// The status at this transfer's address records another transfer.
		func(_ int, st *TransferStatus, _ *Account) { st.ID = scenarioID(100) },
		// The proof does not postdate the deposit.
		func(_ int, st *TransferStatus, _ *Account) { st.ProofHeight = uint64(st.ID.Height) }, //nolint:gosec // positive
		// The status is resolved after the finalized slot.
		func(_ int, st *TransferStatus, _ *Account) { st.ResolvedAtSlot = finalizedSlot + 1 },
	}

	for i, change := range cases {
		a.add(pendingTransfer(a, i, sinkv1.StatusTransferPending))

		st := statusOf(i, resolvedSlotBase+uint64(i), mintedTo(i)) //nolint:gosec // small
		account := Account{Owner: ProgramID, Lamports: 1}
		change(i, &st, &account)
		account.Data = EncodeTransferStatus(st)
		s.Accounts[aktsolana.StatusAddress(ProgramID, scenarioID(i))] = account
	}

	return sc
}

// reportedSlotsScenario has reports at fabricated future slots.
func reportedSlotsScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	void := func(reporter string, slot uint64, i int) sinkv1.Vote {
		return vote(reporter, 1, sinkv1.OutcomeVoided, slot, sigBytes(reporter+" "+scenarioID(i).String()))
	}

	// 0: voids reported beyond any plausible slot cannot block a veto of a missing status.
	a.add(refundPending(a, 0, challengeOpen, void(memberA, fabricatedSlot, 0), void(memberB, fabricatedSlot, 0)))

	// 1: a void reported at a slot this node has not finalized yet stops the review.
	a.add(refundPending(a, 1, challengeOpen, void(memberA, finalizedSlot+1, 1), void(memberB, finalizedSlot+1, 1)))

	// 2: fabricated void reports cannot hide a finalized mint.
	a.add(refundPending(a, 2, challengeOpen, void(memberA, fabricatedSlot, 2), void(memberB, fabricatedSlot, 2)))
	s.resolve(statusOf(2, resolvedSlotBase+2, mintedTo(2)))

	// 3: a mint reported at the largest slot does not hide the finalized mint either.
	t := pendingTransfer(a, 3, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{vote(memberA, 1, sinkv1.OutcomeMinted, ^uint64(0), sigBytes("future"))}
	a.add(t)
	s.resolve(statusOf(3, resolvedSlotBase+3, mintedTo(3)))

	return sc
}

// missingEvidenceScenario has reports whose transactions are missing or do not match, and statuses whose block
// history is missing.
func missingEvidenceScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	void := func(slot uint64, sig []byte) sinkv1.Vote { return vote(memberA, 1, sinkv1.OutcomeVoided, slot, sig) }

	// 0: the reported void signature is missing at a passed slot: veto.
	a.add(refundPending(a, 0, challengeOpen, void(950, sigBytes("missing"))))

	// 1: the reported transaction shows the void although the status is missing: the node is inconsistent.
	shown := statusOf(1, 951, voidedFor(VoidGovernance))
	s.putBlock(951, resolutionTx(shown, shown.ID.String(), aktFrame(TransferResolvedLog(shown))))
	a.add(refundPending(a, 1, challengeOpen, void(951, sigBytes(shown.ID.String()))))

	// 2: a reported signature that is not 64 bytes shows nothing: veto.
	a.add(refundPending(a, 2, challengeOpen, void(952, sigBytes("short")[:32])))

	// 3: the reported transaction exists at another slot than reported: veto.
	moved := statusOf(3, 953, voidedFor(VoidGovernance))
	s.putBlock(953, resolutionTx(moved, moved.ID.String(), aktFrame(TransferResolvedLog(moved))))
	a.add(refundPending(a, 3, challengeOpen, void(954, sigBytes(moved.ID.String()))))

	// 4: the resolution's block is missing from the node's history.
	a.add(pendingTransfer(a, 4, sinkv1.StatusTransferPending))
	s.putStatus(statusOf(4, 955, mintedTo(4)))

	// 5: the resolution's block does not hold it.
	a.add(pendingTransfer(a, 5, sinkv1.StatusTransferPending))
	s.putStatus(statusOf(5, 956, mintedTo(5)))
	s.putBlock(956)

	return sc
}

// ownVotesScenario rotates the policy from {Reporter, memberA, memberB} to {Reporter, memberA, memberC} and serves
// transfers with this reporter's stored votes, duplicate votes and settlements under the old policy.
func ownVotesScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana
	old := a.policy()
	a.rotate(Reporter, memberA, memberC)

	minted := func(reporter string, version uint64, slot uint64, i int) sinkv1.Vote {
		return vote(reporter, version, sinkv1.OutcomeMinted, slot, sigBytes(scenarioID(i).String()))
	}

	// 0: this reporter's stored vote is reused although the block is pruned; it plans nothing more.
	t := pendingTransfer(a, 0, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{minted(Reporter, 2, 900, 0)}
	a.add(t)
	s.putStatus(statusOf(0, 900, mintedTo(0)))

	// 1: a stored vote at another slot cannot replace discovery, and the block is pruned.
	t = pendingTransfer(a, 1, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{minted(Reporter, 2, 900, 1)}
	a.add(t)
	s.putStatus(statusOf(1, 901, mintedTo(1)))

	// 2: another reporter's duplicate vote does not stop this reporter's vote.
	t = pendingTransfer(a, 2, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{minted(memberA, 2, 902, 2)}
	a.add(t)
	s.resolve(statusOf(2, 902, mintedTo(2)))

	// 3: this reporter's vote under the old policy is not a vote under the current one; its evidence is reused.
	t = pendingTransfer(a, 3, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{minted(Reporter, 1, 903, 3)}
	a.add(t)
	s.putStatus(statusOf(3, 903, mintedTo(3)))

	// 4: a settlement under the old policy is refreshed by a vote under the current one.
	t = pendingTransfer(a, 4, sinkv1.StatusTransferPending)
	t.Settlement.Policy = old
	t.Settlement.Votes = []sinkv1.Vote{minted(Reporter, 1, 904, 4), minted(memberB, 1, 904, 4)}
	a.add(t)
	s.resolve(statusOf(4, 904, mintedTo(4)))

	// 5: a refund-pending settlement under the old policy is refreshed too.
	t = refundPending(a, 5, challengeOpen)
	t.Settlement.Policy = old
	a.add(t)
	s.resolve(statusOf(5, 905, voidedFor(VoidInvalidRecipient)))

	return sc
}

// settlementScenario covers contradictions, vetoes and refund eligibility.
func settlementScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	own := func(outcome sinkv1.Outcome, slot uint64, i int) sinkv1.Vote {
		return vote(Reporter, 1, outcome, slot, sigBytes(scenarioID(i).String()))
	}

	// 0: Akash minted what Solana voided; only the audit finds it.
	a.add(pendingTransfer(a, 0, sinkv1.StatusMinted))
	s.resolve(statusOf(0, 900, voidedFor(VoidGovernance)))

	// 1: memberA reported a mint that Solana voided.
	t := pendingTransfer(a, 1, sinkv1.StatusTransferPending)
	t.Settlement.Votes = []sinkv1.Vote{vote(memberA, 1, sinkv1.OutcomeMinted, 901, sigBytes("other mint"))}
	a.add(t)
	s.resolve(statusOf(1, 901, voidedFor(VoidGovernance)))

	// 2: a refund pending without a Solana status is vetoed.
	a.add(refundPending(a, 2, challengeOpen,
		vote(memberA, 1, sinkv1.OutcomeVoided, 902, sigBytes("void a")),
		vote(memberB, 1, sinkv1.OutcomeVoided, 902, sigBytes("void b"))))

	// 3: a held refund without a Solana status is not vetoed again.
	a.add(vetoed(refundPending(a, 3, challengeOpen,
		vote(memberA, 1, sinkv1.OutcomeVoided, 903, sigBytes("void c")))))

	// 4: the refund head past its challenge window is finalized.
	a.add(refundPending(a, 4, challengeEnded, own(sinkv1.OutcomeVoided, 904, 4)))
	s.resolve(statusOf(4, 904, voidedFor(VoidGovernance)))

	// 5: a refund past its window behind the head waits.
	a.add(refundPending(a, 5, challengeEnded+1, own(sinkv1.OutcomeVoided, 905, 5)))
	s.resolve(statusOf(5, 905, voidedFor(VoidGovernance)))

	// 6: a refund inside its challenge window waits.
	a.add(refundPending(a, 6, challengeOpen+1, own(sinkv1.OutcomeVoided, 906, 6)))
	s.resolve(statusOf(6, 906, voidedFor(VoidGovernance)))

	// 7: a conflict hold needs governance; nothing is planned.
	t = pendingTransfer(a, 7, sinkv1.StatusTransferPending)
	t.Settlement.Hold = sinkv1.HoldConflict
	t.Settlement.Votes = []sinkv1.Vote{
		vote(memberA, 1, sinkv1.OutcomeMinted, 907, sigBytes(scenarioID(7).String())),
		vote(memberB, 1, sinkv1.OutcomeVoided, 907, sigBytes("conflict")),
	}
	a.add(t)
	s.resolve(statusOf(7, 907, mintedTo(7)))

	return sc
}

// priorityScenario has one transfer of each open kind without a Solana status, in reverse priority by height; the
// held refund is listed both as a dispute and as a refund.
func priorityScenario() Scenario {
	sc := baseScenario(time.Second)
	a := sc.Akash

	void := vote(memberA, 1, sinkv1.OutcomeVoided, 950, sigBytes("void"))
	minted := pendingTransfer(a, 4, sinkv1.StatusMinted)
	minted.Settlement.Hold = sinkv1.HoldConflict
	minted.Settlement.Votes = []sinkv1.Vote{
		vote(memberA, 1, sinkv1.OutcomeMinted, 951, sigBytes("mint")),
		vote(memberB, 1, sinkv1.OutcomeVoided, 951, sigBytes("void")),
	}

	a.add(pendingTransfer(a, 0, sinkv1.StatusTransferPending))
	a.add(voidRequested(a, 1))
	a.add(refundPending(a, 2, challengeOpen, void))
	a.add(vetoed(refundPending(a, 3, challengeOpen, void)))
	a.add(minted)

	return sc
}

func lightClientFollowScenario() Scenario {
	sc := openScenario(time.Second, cleanCount)
	moveBridge(&sc.Solana, seedKey("light-client 2"))

	return sc
}

func lightClientPinScenario() Scenario {
	sc := openScenario(time.Second, cleanCount)
	pin := LightClient
	sc.LightClientPin = &pin
	moveBridge(&sc.Solana, seedKey("light-client 2"))

	return sc
}

// moveBridge names lightClient in the bridge from the next finalized slot, served after the first account read: the
// read a pass starts with.
func moveBridge(s *Solana, lightClient solana.PublicKey) {
	s.BlockTimes[finalizedSlot+1] = -time.Second
	s.Changes = append(s.Changes, AccountChange{
		AfterCalls: 1,
		Slot:       finalizedSlot + 1,
		Accounts:   map[solana.PublicKey]Account{aktsolana.PDA(ProgramID, []byte("bridge")): bridgeAccount(lightClient)},
	})
}

func pagingOverlapScenario() Scenario {
	sc := openScenario(time.Second, pagingCount)
	sc.Akash.PageOverlap = true

	return sc
}

// auditUnavailableScenario adds a minted record with a zero amount to the clean transfers: it is not open, so only
// the audit page reads it, and its decoding fails.
func auditUnavailableScenario() Scenario {
	sc := openScenario(time.Second, cleanCount)
	a := sc.Akash

	t := pendingTransfer(a, cleanCount, sinkv1.StatusMinted)
	t.Data.Amount = sdk.NewInt64Coin("uakt", 0)
	a.add(t)

	return sc
}

// auditRoundRobinScenario has only minted transfers, settled consistently with Solana. Passes 1 to 3 audit the three
// pages of them; the third is the last, so pass 4 audits the first page again.
func auditRoundRobinScenario() Scenario {
	sc := settledScenario(auditCount)
	sc.Passes = 4

	return sc
}

// auditResumeScenario has two pages of settled transfers. Pass 1 audits the first page; pass 2's audit page fails;
// pass 3 resumes at the second page, because a failed audit keeps the cursor.
func auditResumeScenario() Scenario {
	sc := settledScenario(auditResumeCount)
	sc.Passes = 3
	sc.Akash.FailHistory = []int{2}

	return sc
}

// settledScenario has n minted transfers, settled consistently with Solana, so each audit takes the early exit.
func settledScenario(n int) Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana

	for i := range n {
		a.add(pendingTransfer(a, i, sinkv1.StatusMinted))
		s.putStatus(statusOf(i, resolvedSlotBase, mintedTo(i)))
	}

	return sc
}

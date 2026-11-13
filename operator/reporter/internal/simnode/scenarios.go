package simnode

import (
	"bytes"
	"crypto/sha256"
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
	nodeID = "5ca1ab1e0000000000000000000000000000c0de"
)

var (
	ProgramID   = seedKey("program")
	LightClient = seedKey("light-client")
	MintAddress = aktsolana.PDA(ProgramID, []byte("mint"))
	// Reporter is the policy member whose key ReporterMnemonic derives.
	Reporter = reporterAddress()
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
)

// Scenarios returns the named scenarios that goldens and benchmarks serve, each built fresh by its function.
func Scenarios() map[string]func() Scenario {
	return map[string]func() Scenario{
		// clean: open transfers without a Solana status or reported observation, which plan nothing.
		"clean": func() Scenario { return openScenario(time.Second) },
		// stale-akash: the latest Akash block is older than any max-age.
		"stale-akash": func() Scenario { return openScenario(staleAge) },
		// votes: void-requested transfers voided on Solana without this reporter's vote; each plans one vote.
		"votes": votesScenario,
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

// baseScenario is a deployed bridge with a fresh Solana node and an Akash chain whose latest block is age old.
func baseScenario(age time.Duration) Scenario {
	threshold := sdkmath.LegacyMustNewDecFromStr("0.67")
	policy := sinkv1.ReporterPolicy{
		Version:   1,
		Members:   []string{Reporter, mustBech(bytes.Repeat([]byte{0xa1}, 20)), mustBech(bytes.Repeat([]byte{0xa2}, 20))},
		Threshold: threshold,
	}
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
				ReportersThreshold:        threshold,
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
				bridge: {Owner: ProgramID, Lamports: 1, Data: EncodeBridgeConfig(BridgeConfig{
					LightClientProgram: LightClient, ChainID: ChainID,
					PerTransferMax: 1_000_000_000_000, DailyMintCap: 1_000_000_000_000,
				})},
				MintAddress: {Owner: solana.TokenProgramID, Lamports: 1, Data: EncodeMint(Mint{
					MintAuthority: &authority, Supply: 1 << 40, Decimals: 6, Initialized: true,
				})},
			},
			Blocks: map[uint64]Block{finalizedSlot: {Transactions: []Transaction{}}},
		},
	}
}

func (a *Akash) policy() sinkv1.ReporterPolicy { return a.Policies[len(a.Policies)-1] }

func pendingTransfer(a *Akash, i int, st sinkv1.Status) sinkv1.TransferState {
	return sinkv1.TransferState{
		ID:         scenarioID(i).Proto(),
		Data:       sinkv1.RecordData{Dest: recipient(i).String(), Amount: sdk.NewInt64Coin("uakt", transferAmount)},
		Status:     st,
		Settlement: sinkv1.SettlementData{Round: 1, Policy: a.policy()},
	}
}

// openScenario has transfer-pending transfers without votes or a Solana status, so a review plans nothing.
func openScenario(age time.Duration) Scenario {
	sc := baseScenario(age)
	a := sc.Akash
	for i := range cleanCount {
		a.Transfers = append(a.Transfers, Transfer{Transfer: pendingTransfer(a, i, sinkv1.StatusTransferPending)})
	}

	return sc
}

// votesScenario has void-requested transfers that Solana voided by governance void order, with the resolution
// transaction in its block, and no vote from Reporter: each plans one MsgReportOutcome.
func votesScenario() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana
	reason := VoidGovernance

	for i := range votesCount {
		t := pendingTransfer(a, i, sinkv1.StatusVoided)
		t.Settlement.VoidAuthorization = &sinkv1.VoidAuthorization{RequestedHeight: scenarioHeight - 20, RequestedRound: 1}
		a.Transfers = append(a.Transfers, Transfer{Transfer: t})

		id := scenarioID(i)
		slot := resolvedSlotBase + uint64(i)                                                                                          //nolint:gosec // small
		status := TransferStatus{ID: id, ProofHeight: uint64(id.Height) + 1, ResolvedAtSlot: slot, Outcome: Outcome{Voided: &reason}} //nolint:gosec // positive
		address := aktsolana.StatusAddress(ProgramID, id)
		s.Accounts[address] = Account{Owner: ProgramID, Lamports: 1, Data: EncodeTransferStatus(status)}
		s.BlockTimes[slot] = -time.Minute
		s.Blocks[slot] = Block{Transactions: []Transaction{{
			Signatures:  []string{solana.SignatureFromBytes(append(digestBytes("sig a "+id.String()), digestBytes("sig b "+id.String())...)).String()},
			Header:      Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1},
			AccountKeys: []solana.PublicKey{seedKey("payer"), address, ProgramID},
			Logs: []string{
				"Program " + ProgramID.String() + " invoke [1]",
				"Program log: Instruction: VoidTransfer",
				TransferResolvedLog(status),
				"Program " + ProgramID.String() + " success",
			},
		}}}
	}

	return sc
}

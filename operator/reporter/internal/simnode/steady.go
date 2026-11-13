package simnode

import (
	"strconv"
	"time"

	"github.com/gagliardetto/solana-go"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
)

// The workload of spec §11.4. Open transfers repeat a cycle of steadyCycle: steadyQuiet quiet transfers, then
// steadyVoted, steadyRefunds, steadyDisputes and steadyVoids of the other categories.
const (
	steadyAudited  = 100
	steadyOpen     = 100_000
	steadyCycle    = 100
	steadyQuiet    = 70
	steadyVoted    = 20
	steadyRefunds  = 4
	steadyDisputes = 1
	steadyVoids    = 5
	// steadyBlocks blocks of steadyBlockTxs transactions each hold the resolutions of the void-requested transfers.
	steadyBlocks   = 100
	steadyBlockTxs = 1_500
)

// Steady100k is the steady-100k benchmark workload of spec §11.4: 100,000 open transfers under a 3-member policy,
// after 100 settled transfers that make up the first audit page. Of the open transfers:
//   - 70,000 are transfer-pending without votes or a Solana status; they plan nothing and need no evidence;
//   - 20,000 transfer-pending, 4,000 refund-pending inside their challenge window and 1,000 held disputes are resolved
//     on Solana with this reporter's matching vote stored, so its evidence is reused and no block is read; each
//     dispute alerts for governance review;
//   - 5,000 are void-requested and voided on Solana without this reporter's vote, so each plans one MsgReportOutcome.
//     Their resolutions are spread over 100 blocks of 1,500 transactions each.
func Steady100k() Scenario {
	sc := baseScenario(time.Second)
	a, s := sc.Akash, &sc.Solana
	a.Transfers = make([]Transfer, 0, steadyAudited+steadyOpen)

	for i := range steadyAudited {
		a.add(pendingTransfer(a, i, sinkv1.StatusMinted))
		s.putStatus(statusOf(i, resolvedSlotBase, mintedTo(i)))
	}

	own := func(outcome sinkv1.Outcome, i int) sinkv1.Vote {
		return vote(Reporter, 1, outcome, resolvedSlotBase, sigBytes(scenarioID(i).String()))
	}

	perBlock := steadyOpen / steadyCycle * steadyVoids / steadyBlocks
	blocks := make([][]Transaction, steadyBlocks)
	voids := 0

	for j := range steadyOpen {
		i := steadyAudited + j

		switch k := j % steadyCycle; {
		case k < steadyQuiet:
			a.add(pendingTransfer(a, i, sinkv1.StatusTransferPending))
		case k < steadyQuiet+steadyVoted:
			t := pendingTransfer(a, i, sinkv1.StatusTransferPending)
			t.Settlement.Votes = []sinkv1.Vote{own(sinkv1.OutcomeMinted, i)}
			a.add(t)
			s.putStatus(statusOf(i, resolvedSlotBase, mintedTo(i)))
		case k < steadyQuiet+steadyVoted+steadyRefunds:
			a.add(refundPending(a, i, challengeOpen, own(sinkv1.OutcomeVoided, i)))
			s.putStatus(statusOf(i, resolvedSlotBase, voidedFor(VoidGovernance)))
		case k < steadyQuiet+steadyVoted+steadyRefunds+steadyDisputes:
			a.add(vetoed(refundPending(a, i, challengeOpen, own(sinkv1.OutcomeVoided, i))))
			s.putStatus(statusOf(i, resolvedSlotBase, voidedFor(VoidGovernance)))
		default:
			b := voids / perBlock
			voids++

			a.add(voidRequested(a, i))
			st := statusOf(i, resolvedSlotBase+uint64(b), voidedFor(VoidGovernance)) //nolint:gosec // small
			s.putStatus(st)
			blocks[b] = append(blocks[b], resolutionTx(st, st.ID.String(),
				aktFrame("Program log: Instruction: VoidTransfer", TransferResolvedLog(st))))
		}
	}

	for b, txs := range blocks {
		slot := resolvedSlotBase + uint64(b) //nolint:gosec // small
		s.putBlock(slot, append(txs, fillers(slot, steadyBlockTxs-len(txs))...)...)
	}

	return sc
}

// fillers are n token transfers of slot, unrelated to the bridge.
func fillers(slot uint64, n int) []Transaction {
	token := solana.TokenProgramID
	keys := []solana.PublicKey{seedKey("payer"), seedKey("token account"), token}
	logs := []string{invoke(token, 1), "Program log: Instruction: Transfer", success(token)}
	prefix := "filler " + strconv.FormatUint(slot, 10) + " "

	txs := make([]Transaction, n)
	for k := range txs {
		txs[k] = Transaction{
			Signatures:  []string{signature(prefix + strconv.Itoa(k)).String()},
			Header:      Header{NumRequiredSignatures: 1, NumReadonlyUnsignedAccounts: 1},
			AccountKeys: keys,
			Logs:        logs,
		}
	}

	return txs
}

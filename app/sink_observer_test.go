package app_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"pkg.akt.dev/go/sdkutil"

	"github.com/stretchr/testify/require"

	types "pkg.akt.dev/go/node/sink/v1"
	sinkkeeper "pkg.akt.dev/node/v2/x/sink/keeper"
)

// Optional cross-runtime test. SINK_REPORTER_FIXTURE names a JSON file with
// three signed MsgReportOutcome transactions, produced outside Go by reporters
// that observed the mint of the transfer created here on a local Solana validator.
func TestSinkAcceptsSolanaObserverReports(t *testing.T) {
	path := os.Getenv("SINK_REPORTER_FIXTURE")
	if path == "" {
		t.Skip("SINK_REPORTER_FIXTURE is not set")
	}

	data, err := os.ReadFile(path) //nolint:gosec // test fixture named by the caller
	require.NoError(t, err)
	var fixture struct {
		Transactions []string `json:"transactions"`
		Slot         uint64   `json:"slot"`
		Signature    []byte   `json:"signature"`
	}

	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Len(t, fixture.Transactions, 3)
	require.Positive(t, fixture.Slot)
	require.Len(t, fixture.Signature, 64)
	h := newSinkHarness(t, nil)
	res := h.deliver(t, "", h.deposit(123456789))
	require.Zero(t, res.Code, res.Log)
	k := h.a.Keepers.Akash.Sink
	locked := h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), sinkkeeper.Address(), sdkutil.DenomUakt)

	for _, raw := range fixture.Transactions {
		bz, err := base64.StdEncoding.DecodeString(raw)
		require.NoError(t, err)
		tx, err := h.a.TxConfig().TxDecoder()(bz)
		require.NoError(t, err)
		require.Len(t, tx.GetMsgs(), 1)
		msg, ok := tx.GetMsgs()[0].(*types.MsgReportOutcome)
		require.True(t, ok)
		require.Equal(t, types.OutcomeMinted, msg.Observation.Outcome)
		require.Equal(t, fixture.Slot, msg.Observation.Evidence.Slot)
		require.Equal(t, fixture.Signature, msg.Observation.Evidence.TransactionSignature)
		res = h.deliverBytes(t, bz)
		require.Zero(t, res.Code, res.Log)
	}

	entry, err := k.GetTransfer(h.ctx(), h.id(1))
	require.NoError(t, err)
	require.Equal(t, types.StatusMinted, entry.Settlement.Status)
	require.Len(t, entry.Settlement.Votes, 3)
	require.Equal(t, coin(123456789), h.totals().MintedAmount)
	require.Zero(t, h.totals().PendingCount)
	require.Equal(t, locked, h.a.Keepers.Cosmos.Bank.GetBalance(h.ctx(), sinkkeeper.Address(), sdkutil.DenomUakt))
	require.NoError(t, k.CheckBalance(h.ctx()))
}

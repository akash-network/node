package clitest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cosmoskeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	decredecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/v2/operator/reporter/internal/simnode"
	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/signer/kmspb"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	reportertx "pkg.akt.dev/node/v2/operator/reporter/tx"
)

const (
	reporterKey = "reporter"
	kmsKeyID    = "reporter"
	// blockInterval keeps blocks coming while a broadcasting reporter waits for its transactions.
	blockInterval = 200 * time.Millisecond
	voteType      = "/akash.sink.v1.MsgReportOutcome"
)

// startVotes starts the votes scenario on a node whose Akash time is window-aligned and that commits a block every
// blockInterval, or, when opts.BlockInterval is negative, only when the test commits one. change edits the scenario
// first.
func startVotes(t *testing.T, opts simnode.Options, change func(*simnode.Scenario)) *simnode.Node {
	t.Helper()

	s := simnode.Scenarios()["votes"]()
	if change != nil {
		change(&s)
	}

	opts.AlignWindow = true
	switch {
	case opts.BlockInterval == 0:
		opts.BlockInterval = blockInterval
	case opts.BlockInterval < 0:
		opts.BlockInterval = 0
	}

	n, err := simnode.Start(t.Context(), s, opts)
	require.NoError(t, err)
	t.Cleanup(n.Close)

	return n
}

// reporterArgs configures a reporter against n with home and max-age 80s; extra comes last and overrides.
func reporterArgs(n *simnode.Node, home, name string, extra ...string) []string {
	return append([]string{name,
		"--home", home,
		"--network", "testnet",
		"--chain-id", simnode.ChainID,
		"--reporter", simnode.Reporter,
		"--node", n.CometURL,
		"--grpc-addr", n.GRPCAddr,
		"--solana.rpc", n.SolanaURL,
		"--solana.genesis-hash", simnode.GenesisHash,
		"--solana.program-id", simnode.ProgramID.String(),
		"--solana.mint", simnode.MintAddress.String(),
		"--request-timeout", "5s",
		"--max-age", "80s",
	}, extra...)
}

var keyringBroadcast = []string{"--broadcast", "--from", reporterKey, "--keyring-backend", cosmoskeyring.BackendTest}

// addReporterKey stores the reporter's key in the test keyring of home.
func addReporterKey(t *testing.T, home string) {
	t.Helper()

	kr, err := cosmoskeyring.New(sdk.KeyringServiceName(), cosmoskeyring.BackendTest, home, nil, sdkutil.MakeEncodingConfig().Codec)
	require.NoError(t, err)

	rec, err := kr.NewAccount(reporterKey, simnode.ReporterMnemonic, "", hd.CreateHDPath(sdk.CoinType, 0, 0).String(), hd.Secp256k1)
	require.NoError(t, err)
	addr, err := rec.GetAddress()
	require.NoError(t, err)
	require.Equal(t, simnode.Reporter, addr.String())
}

func reporterPriv(t *testing.T) *secp256k1.PrivateKey {
	t.Helper()

	raw, err := hd.Secp256k1.Derive()(simnode.ReporterMnemonic, "", hd.CreateHDPath(sdk.CoinType, 0, 0).String())
	require.NoError(t, err)

	return secp256k1.PrivKeyFromBytes(raw)
}

// fakeKMS is a SignerService for one ECDSA_SECP256K1ETH key; it signs the digest it receives.
type fakeKMS struct {
	kmspb.UnimplementedSignerServiceServer

	priv  *secp256k1.PrivateKey
	signs atomic.Int64
}

func (f *fakeKMS) GetKey(context.Context, *kmspb.GetKeyRequest) (*kmspb.GetKeyResponse, error) {
	return &kmspb.GetKeyResponse{Key: &kmspb.Key{
		Id:     kmsKeyID,
		Pubkey: f.priv.PubKey().SerializeCompressed(),
		Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1ETH,
	}}, nil
}

func (f *fakeKMS) Sign(_ context.Context, req *kmspb.SignRequest) (*kmspb.SignResponse, error) {
	f.signs.Add(1)
	compact := decredecdsa.SignCompact(f.priv, req.GetPayload().GetGeneric(), true)

	return &kmspb.SignResponse{Signature: compact[1:]}, nil
}

// serveKMS serves f over TLS with a certificate for 127.0.0.1 from a CA generated here, and returns the address and
// the CA file. No key material is committed.
func serveKMS(t *testing.T, f *fakeKMS) (addr, caFile string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	srvKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	srvDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "kms.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}, caCert, &srvKey.PublicKey, caKey)
	require.NoError(t, err)

	caFile = filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{srvDER}, PrivateKey: srvKey}},
		MinVersion:   tls.VersionTLS12,
	})
	srv := grpc.NewServer(grpc.Creds(creds))
	kmspb.RegisterSignerServiceServer(srv, f)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String(), caFile
}

// remoteBroadcast configures the remote signer at addr; max-age plus the 2s timeout stays within 80s.
func remoteBroadcast(addr, caFile string) []string {
	return []string{
		"--broadcast",
		"--signer.remote.address", addr,
		"--signer.remote.key-id", kmsKeyID,
		"--signer.remote.ca-file", caFile,
		"--signer.remote.timeout", "2s",
		"--max-age", "78s",
	}
}

// countingListener accepts and drops connections, counting them.
func countingListener(t *testing.T) (string, *atomic.Int64) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	var accepted atomic.Int64
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()

	return lis.Addr().String(), &accepted
}

func withKey(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	addReporterKey(t, home)

	return home
}

func TestStartupRefusals(t *testing.T) {
	refused := func(t *testing.T, r result, want string) {
		t.Helper()

		require.Equal(t, 1, r.code, r.stderr)
		require.Contains(t, r.stderr, want)
		require.Empty(t, r.stdout, "no event before the startup checks pass")
	}

	t.Run("node-ID mismatch", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, nil)
		other := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) {
			sc.Akash.NodeID = "0123456789abcdef0123456789abcdef01234567"
		})

		args := append(reporterArgs(n, withKey(t), "scan", keyringBroadcast...), "--grpc-addr", other.GRPCAddr)
		refused(t, runToEnd(t, args...), "Akash RPC and gRPC endpoints are different nodes")
	})

	t.Run("height-echo mismatch on the first snapshot, before the keyring is read", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) { sc.Akash.EchoOffset = 1 })

		// The keyring holds no key: the snapshot check comes first.
		refused(t, runToEnd(t, reporterArgs(n, t.TempDir(), "scan", keyringBroadcast...)...), "Akash query height mismatch")
	})

	t.Run("a missing keyring key", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, nil)

		refused(t, runToEnd(t, reporterArgs(n, t.TempDir(), "scan", keyringBroadcast...)...), `find key "reporter"`)
	})

	t.Run("a KMS key that does not derive reporter, before the account is read", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) { sc.Akash.Accounts = nil })

		other, err := secp256k1.GeneratePrivateKey()
		require.NoError(t, err)
		kms := &fakeKMS{priv: other}
		addr, ca := serveKMS(t, kms)

		refused(t, runToEnd(t, reporterArgs(n, t.TempDir(), "scan", remoteBroadcast(addr, ca)...)...), "signer does not match reporter")
		require.Zero(t, kms.signs.Load())
	})

	t.Run("a missing account", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) { sc.Akash.Accounts = nil })

		refused(t, runToEnd(t, reporterArgs(n, withKey(t), "scan", keyringBroadcast...)...), "reporter account not found")
		require.Empty(t, n.Broadcasts())
	})

	t.Run("preview with both signers configured opens neither", func(t *testing.T) {
		n := startVotes(t, simnode.Options{}, nil)
		kmsAddr, dials := countingListener(t)
		keyringDir := filepath.Join(t.TempDir(), "keyring")

		r := runToEnd(t, reporterArgs(n, t.TempDir(), "scan",
			"--from", "absent-key",
			"--keyring-backend", cosmoskeyring.BackendTest,
			"--keyring-dir", keyringDir,
			"--signer.remote.address", kmsAddr,
			"--signer.remote.key-id", kmsKeyID,
		)...)
		require.Equal(t, 0, r.code, r.stderr)

		events := lines(t, r.stdout)
		require.Equal(t, 1, summaries(events))
		require.Equal(t, "preview", events[0]["mode"])
		require.Zero(t, dials.Load(), "preview dialed the remote signer")
		require.NoDirExists(t, keyringDir)
		require.Empty(t, n.Broadcasts())
	})
}

// txHashes returns the transfer IDs of the transactionHash events.
func txHashes(events []map[string]any) []string {
	var ids []string
	for _, ev := range events {
		if _, ok := ev["transactionHash"]; ok {
			ids = append(ids, ev["id"].(string))
		}
	}

	return ids
}

// summaryIndexes returns the positions of the summaries in events.
func summaryIndexes(events []map[string]any) []int {
	var out []int
	for i, ev := range events {
		if _, ok := ev["scanned"]; ok {
			out = append(out, i)
		}
	}

	return out
}

// requireSlotTimeouts checks that every broadcast is unordered with the slot timeout of each of its messages.
func requireSlotTimeouts(t *testing.T, n *simnode.Node) {
	t.Helper()

	broadcasts := n.Broadcasts()
	require.NotEmpty(t, broadcasts)

	for _, raw := range broadcasts {
		decoded, err := simnode.TxConfig().TxDecoder()(raw)
		require.NoError(t, err)

		u, ok := decoded.(sdk.TxWithUnordered)
		require.True(t, ok)
		require.True(t, u.GetUnordered())

		timeout := u.GetTimeoutTimeStamp()
		for _, msg := range decoded.GetMsgs() {
			vote, ok := msg.(*sinkv1.MsgReportOutcome)
			require.True(t, ok, "%T", msg)

			id, err := sinkid.FromProto(vote.ID)
			require.NoError(t, err)
			slot := reportertx.SlotOf(id, policy.KeyOf(id, vote))
			require.True(t, timeout.Equal(reportertx.SlotTimeout(reportertx.WindowOf(timeout), slot)),
				"timeout %s is not the slot timeout of slot %d", timeout, slot)
		}
	}
}

// requireVotedOnce checks that each of want transfers' votes executed exactly once.
func requireVotedOnce(t *testing.T, n *simnode.Node, want int) {
	t.Helper()

	execs := n.Executions()
	require.Len(t, execs, want)
	for key, count := range execs {
		require.True(t, strings.HasPrefix(key, voteType+"|"), key)
		require.Equal(t, 1, count, key)
	}
}

func TestScanBroadcastEndToEnd(t *testing.T) {
	const votes = 30

	signers := map[string]func(t *testing.T) (home string, args []string){
		"keyring": func(t *testing.T) (string, []string) {
			return withKey(t), keyringBroadcast
		},
		"remote": func(t *testing.T) (string, []string) {
			addr, ca := serveKMS(t, &fakeKMS{priv: reporterPriv(t)})
			return t.TempDir(), remoteBroadcast(addr, ca)
		},
	}

	for name, configure := range signers {
		t.Run(name, func(t *testing.T) {
			n := startVotes(t, simnode.Options{}, nil)
			home, args := configure(t)

			r := runToEnd(t, reporterArgs(n, home, "scan", args...)...)
			require.Equal(t, 0, r.code, r.stderr)
			require.Contains(t, r.stderr, `mode=broadcast`)

			events := lines(t, r.stdout)
			require.Equal(t, []int{len(events) - 1}, summaryIndexes(events), "the summary is the last event")
			require.EqualValues(t, votes, events[len(events)-1]["scanned"])
			require.EqualValues(t, 0, events[len(events)-1]["failures"])

			hashes := txHashes(events)
			require.Len(t, hashes, votes)
			seen := map[string]bool{}
			for _, id := range hashes {
				require.False(t, seen[id], "two transactionHash events for %s", id)
				seen[id] = true
			}

			for _, ev := range events {
				if _, ok := ev["observed"]; ok {
					require.Equal(t, "broadcast", ev["mode"])
				}
			}

			requireSlotTimeouts(t, n)
			requireVotedOnce(t, n, votes)
		})
	}
}

func TestScanTwiceNoRepeat(t *testing.T) {
	const votes = 30

	n := startVotes(t, simnode.Options{}, nil)
	home := withKey(t)

	first := runToEnd(t, reporterArgs(n, home, "scan", keyringBroadcast...)...)
	require.Equal(t, 0, first.code, first.stderr)
	require.Len(t, txHashes(lines(t, first.stdout)), votes)

	broadcasts := len(n.Broadcasts())

	second := runToEnd(t, reporterArgs(n, home, "scan", keyringBroadcast...)...)
	require.Equal(t, 0, second.code, second.stderr)

	events := lines(t, second.stdout)
	require.Empty(t, txHashes(events))
	reviews := 0
	for _, ev := range events {
		if _, ok := ev["observed"]; ok {
			reviews++
			require.Empty(t, ev["messages"], "the second run planned %v", ev["messages"])
		}
	}
	require.Equal(t, votes, reviews)
	require.EqualValues(t, 0, events[len(events)-1]["failures"])

	require.Len(t, n.Broadcasts(), broadcasts, "the second run sent a transaction")
	requireVotedOnce(t, n, votes)
}

func TestScanUnderBlockMaxGas(t *testing.T) {
	t.Run("simulation and sending under a block max gas below tx.max-gas", func(t *testing.T) {
		const votes = 30
		n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) { sc.Akash.BlockMaxGas = 10_000_000 })

		r := runToEnd(t, reporterArgs(n, withKey(t), "scan", keyringBroadcast...)...)
		require.Equal(t, 0, r.code, r.stderr)
		require.Contains(t, r.stderr, "gas-cap=10000000")

		events := lines(t, r.stdout)
		require.EqualValues(t, 0, events[len(events)-1]["failures"])
		require.Len(t, txHashes(events), votes)
		requireVotedOnce(t, n, votes)
	})

	t.Run("a vote over the block max gas is not sent", func(t *testing.T) {
		// One vote needs 100,000 gas in simnode, a limit of 200,000 at gas-adjustment 2.0.
		n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) {
			sc.Akash.Transfers = sc.Akash.Transfers[:1]
			sc.Akash.BlockMaxGas = 150_000
		})

		r := runToEnd(t, reporterArgs(n, withKey(t), "scan", keyringBroadcast...)...)
		require.Equal(t, 1, r.code, r.stderr)
		require.Contains(t, r.stderr, "gas-cap=150000")

		var errs []string
		for _, ev := range lines(t, r.stdout) {
			if e, ok := ev["error"].(string); ok {
				errs = append(errs, e)
			}
		}
		require.Equal(t, []string{"gas limit 200000 exceeds the block max gas 150000"}, errs)
		require.Empty(t, n.Broadcasts())
	})
}

// oneVote keeps only the first transfer of the votes scenario and fails the first executed transaction.
func oneVote(sc *simnode.Scenario) {
	sc.Akash.Transfers = sc.Akash.Transfers[:1]
	sc.Akash.Deliveries = []simnode.Delivery{{Code: 5, Codespace: "sdk", Log: "insufficient funds"}}
}

func TestScanCountsTransactionFailures(t *testing.T) {
	n := startVotes(t, simnode.Options{}, oneVote)

	r := runToEnd(t, reporterArgs(n, withKey(t), "scan", keyringBroadcast...)...)
	require.Equal(t, 1, r.code, r.stderr)

	events := lines(t, r.stdout)
	require.Equal(t, []int{len(events) - 1}, summaryIndexes(events))
	require.EqualValues(t, 1, events[len(events)-1]["failures"])

	var errs []string
	for _, ev := range events {
		if e, ok := ev["error"].(string); ok {
			errs = append(errs, e)
		}
	}
	require.Len(t, errs, 1)
	require.Contains(t, errs[0], "sink transaction failed")
	require.Empty(t, n.Executions())
}

func TestStartAttributesLateOutcomes(t *testing.T) {
	n := startVotes(t, simnode.Options{}, oneVote)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(ctx, &stdout, &stderr, reporterArgs(n, withKey(t), "start",
			append([]string{"--scan.interval", "5s"}, keyringBroadcast...)...)...)
	}()

	require.Eventually(t, func() bool {
		return strings.Count(stdout.String(), `"scanned"`) >= 2
	}, 60*time.Second, 20*time.Millisecond, "no second summary: %s", stderr.String())

	cancel()
	select {
	case code := <-done:
		require.Equal(t, 0, code, stderr.String())
	case <-time.After(60 * time.Second):
		t.Fatal("start did not stop after cancellation")
	}

	events := lines(t, stdout.String())
	at := summaryIndexes(events)
	require.GreaterOrEqual(t, len(at), 2)
	require.EqualValues(t, 0, events[at[0]]["failures"], "the first pass's transaction had not resolved")
	require.EqualValues(t, 1, events[at[1]]["failures"], "the second pass counts the late outcome")

	failed := -1
	for i, ev := range events {
		if e, ok := ev["error"].(string); ok && strings.Contains(e, "sink transaction failed") {
			failed = i
		}
	}
	require.Greater(t, failed, at[0], "the outcome resolved after the first summary")
	require.Less(t, failed, at[1])
}

func TestSigtermOrdering(t *testing.T) {
	const votes = 30

	// One vote per transaction, and slow evidence reads, so that transactions go out while the pass still reviews.
	n := startVotes(t, simnode.Options{BlockFetchLatency: 150 * time.Millisecond}, nil)

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(t.Context(), &stdout, &stderr, reporterArgs(n, withKey(t), "scan",
			append([]string{"--tx.max-msgs", "1", "--scan.workers", "1"}, keyringBroadcast...)...)...)
	}()

	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), `"transactionHash"`)
	}, 60*time.Second, 5*time.Millisecond, "no transaction confirmed: %s", stderr.String())

	signalled := len(n.Broadcasts())
	sigterm(t)
	require.Equal(t, 0, exitCodeOf(t, done), stderr.String())

	events := lines(t, stdout.String())
	require.Equal(t, []int{len(events) - 1}, summaryIndexes(events), "the summary is the last line")
	require.EqualValues(t, votes, events[len(events)-1]["scanned"])
	require.EqualValues(t, 0, events[len(events)-1]["failures"])
	require.Less(t, reviewCount(events), votes, "the pass stopped early")

	// Every transaction sent confirmed within the grace, and at most the one being built at the signal was signed
	// after it.
	broadcasts := n.Broadcasts()
	require.LessOrEqual(t, len(broadcasts)-signalled, 1, "transactions were signed after the signal")
	require.Len(t, txHashes(events), len(broadcasts))
	requireVotedOnce(t, n, len(broadcasts))
}

// TestStartSigtermOrdering signals start mid-pass while transactions are in flight: they resolve within the grace,
// before the pass's summary, which counts the failure among them and is the last line.
func TestStartSigtermOrdering(t *testing.T) {
	const votes = 30

	// Blocks come only when the test commits them, so that transactions are in flight when the signal arrives.
	n := startVotes(t, simnode.Options{BlockInterval: -1, BlockFetchLatency: 300 * time.Millisecond}, nil)

	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(t.Context(), &stdout, &stderr, reporterArgs(n, withKey(t), "start",
			append([]string{"--scan.interval", "60s", "--tx.max-msgs", "1", "--scan.workers", "1"}, keyringBroadcast...)...)...)
	}()

	require.Eventually(t, func() bool {
		n.CommitBlock()
		return strings.Contains(stdout.String(), `"transactionHash"`)
	}, 60*time.Second, 200*time.Millisecond, "no transaction confirmed: %s", stderr.String())

	// Without blocks, the next votes stay pending; the first of them to execute fails.
	require.Eventually(t, func() bool {
		return len(n.Broadcasts()) > len(n.Executions())
	}, 30*time.Second, 5*time.Millisecond, "no transaction in flight")
	n.Mutate(func(sc *simnode.Scenario) {
		sc.Akash.Deliveries = []simnode.Delivery{{Code: 5, Codespace: "sdk", Log: "insufficient funds"}}
	})
	require.NotContains(t, stdout.String(), `"scanned"`, "the pass ended before the signal")

	signalled := len(n.Broadcasts())
	sigterm(t)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
				n.CommitBlock()
			}
		}
	}()

	require.Equal(t, 0, exitCodeOf(t, done), stderr.String())

	events := lines(t, stdout.String())
	require.Equal(t, []int{len(events) - 1}, summaryIndexes(events), "the summary is the last line")
	require.EqualValues(t, votes, events[len(events)-1]["scanned"])
	require.EqualValues(t, 1, events[len(events)-1]["failures"], "the summary counts the failure resolved in the grace")
	require.Less(t, reviewCount(events), votes, "the pass stopped early")

	var errs []string
	for _, ev := range events {
		if e, ok := ev["error"].(string); ok {
			errs = append(errs, e)
		}
	}
	require.Len(t, errs, 1)
	require.Contains(t, errs[0], "sink transaction failed")

	broadcasts := n.Broadcasts()
	require.LessOrEqual(t, len(broadcasts)-signalled, 1, "transactions were signed after the signal")
	require.Len(t, txHashes(events), len(broadcasts)-1, "every other transaction confirmed within the grace")
	requireVotedOnce(t, n, len(broadcasts)-1)
}

// TestScanSigtermJoinsFollowUp signals scan while a follow-up review waits on evidence. In refund-queue, R1's inclusion
// starts a refund-head review of R2. The pass reviews R2 using its stored vote, so it calls no getBlock; after the test
// drops that vote, the follow-up's evidence read is slow and fails. That failure precedes the summary and counts in it.
func TestScanSigtermJoinsFollowUp(t *testing.T) {
	sc := simnode.Scenarios()["refund-queue"]()
	sc.Solana.Failures = map[string]simnode.Failure{"getBlock": {Code: -32004, Message: "block not available"}}
	rec, err := sinkid.DecodeRecord(sc.Akash.Transfers[1].Transfer)
	require.NoError(t, err)

	// Blocks come only when the test commits one, so R1's refund is included after the pass.
	n, err := simnode.Start(t.Context(), sc, simnode.Options{AlignWindow: true, BlockFetchLatency: 3 * time.Second})
	require.NoError(t, err)
	t.Cleanup(n.Close)

	home := withKey(t)
	var stdout, stderr syncBuffer
	done := make(chan int, 1)
	go func() {
		done <- run(t.Context(), &stdout, &stderr, reporterArgs(n, home, "scan", keyringBroadcast...)...)
	}()

	require.Eventually(t, func() bool {
		return strings.Count(stdout.String(), `"observed"`) >= 3
	}, 60*time.Second, 5*time.Millisecond, stderr.String())
	n.Mutate(func(sc *simnode.Scenario) { sc.Akash.Transfers[1].Transfer.Settlement.Votes = nil })

	require.Eventually(t, func() bool {
		n.CommitBlock()
		return n.Calls("getBlock") > 0
	}, 60*time.Second, 200*time.Millisecond, stderr.String())
	sigterm(t) // the follow-up now waits on getBlock, and no transaction is in flight

	require.Equal(t, 1, exitCodeOf(t, done), stderr.String())

	events := lines(t, stdout.String())
	last := len(events) - 1
	require.Equal(t, []int{last}, summaryIndexes(events), "the summary is the last line")
	require.EqualValues(t, 1, events[last]["failures"], "the follow-up's failure counts")

	failed := -1
	for i, ev := range events {
		if ev["id"] == rec.ID.String() && ev["error"] != nil {
			failed = i
		}
	}
	require.True(t, failed >= 0 && failed < last, "R2's follow-up error precedes the summary")
}

// sigterm sends SIGTERM to the test process, which the running CLI handles.
func sigterm(t *testing.T) {
	t.Helper()

	self, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, self.Signal(syscall.SIGTERM))
}

func exitCodeOf(t *testing.T, done <-chan int) int {
	t.Helper()

	select {
	case code := <-done:
		return code
	case <-time.After(60 * time.Second):
		t.Fatal("the CLI did not stop after SIGTERM")
		return -1
	}
}

func reviewCount(events []map[string]any) int {
	n := 0
	for _, ev := range events {
		if _, ok := ev["observed"]; ok {
			n++
		}
	}

	return n
}

func TestReadOnlyHome(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}

	n := startVotes(t, simnode.Options{}, func(sc *simnode.Scenario) { sc.Akash.Transfers = sc.Akash.Transfers[:1] })
	home := withKey(t)

	var dirs []string
	require.NoError(t, filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	}))
	for _, d := range dirs {
		require.NoError(t, os.Chmod(d, 0o500)) //nolint:gosec // a directory needs its search bit
	}
	t.Cleanup(func() {
		for _, d := range dirs {
			_ = os.Chmod(d, 0o700) //nolint:gosec // restored so that the test can remove it
		}
	})

	r := runToEnd(t, reporterArgs(n, home, "scan", keyringBroadcast...)...)
	require.Equal(t, 0, r.code, r.stderr)
	require.Len(t, txHashes(lines(t, r.stdout)), 1)
	requireVotedOnce(t, n, 1)

	add := runToEnd(t, "keys", "add", "other", "--home", home, "--keyring-backend", cosmoskeyring.BackendTest)
	require.Equal(t, 1, add.code)
	require.Contains(t, add.stderr, "permission denied")
}

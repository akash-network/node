package sink_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	sinktypes "pkg.akt.dev/go/node/sink/v1"
)

const (
	loadTransfers   = 100_000
	loadBlockSize   = 1_000
	loadClients     = 32
	loadDuration    = 30 * time.Second
	loadMinQPS      = 2_000
	loadCallTimeout = 10 * time.Second
)

// depositTx signs a deposit of the harness sender with an explicit sequence, so that one block can carry many of them.
func depositTx(t *testing.T, h *testSuite, seq uint64) []byte {
	t.Helper()

	return h.signedTx(t, h.key, seq, "", []sdk.Msg{h.deposit(10)}, nil)
}

type loadResult struct {
	latencies []time.Duration
	errs      int
	firstErr  error
}

func cpuModel() string {
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}

	return runtime.GOARCH
}

// TestReporterNodeLoad checks the spec §11.4 node-load floor: a reporter pass reads every open transfer through
// height-pinned Transfer queries, so the node must serve at least loadMinQPS of them from loadClients concurrent
// clients over loadTransfers transfers. Only the query phase is measured.
func TestReporterNodeLoad(t *testing.T) {
	if os.Getenv("REPORTER_PERF") != "1" {
		t.Skip("set REPORTER_PERF=1 to run the node-load benchmark")
	}

	setupStart := time.Now()
	h := setupReportingTestSuite(t)

	p := h.params()
	p.MaxAmountPerTransfer = coin(100_000_000)
	p.MaxEpochRefund = coin(100_000_000) // validation requires the refund cap to cover the transfer cap
	p.MaxEpochAmountPerAccount = coin(100_000_000)
	p.MaxTransferAmountPerEpoch = coin(100_000_000)
	require.NoError(t, h.update(p))

	seq := h.a.Keepers.Cosmos.Acct.GetAccount(h.ctx(), h.sender).GetSequence()
	for sent := 0; sent < loadTransfers; sent += loadBlockSize {
		txs := make([][]byte, loadBlockSize)
		for i := range txs {
			txs[i] = depositTx(t, h, seq)
			seq++
		}

		for _, res := range h.commitBlock(t, h.now.Add(time.Second), txs...) {
			require.Zero(t, res.Code, res.Log)
		}
	}

	require.Len(t, h.ids, loadTransfers)
	require.Equal(t, uint64(loadTransfers), h.totals().PendingCount)
	setup := time.Since(setupStart)
	t.Logf("setup: %d deposits in %d blocks, height %d, %s", loadTransfers, loadTransfers/loadBlockSize, h.height, setup.Round(time.Millisecond))

	cctx := client.Context{}.
		WithInterfaceRegistry(h.a.InterfaceRegistry()).
		WithTxConfig(h.a.TxConfig()).
		WithChainID(h.a.ChainID())
	srv, err := servergrpc.NewGRPCServer(cctx, h.a, config.GRPCConfig{})
	require.NoError(t, err)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(codec.NewProtoCodec(h.a.InterfaceRegistry()).GRPCCodec())))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	query := sinktypes.NewQueryClient(conn)
	height := strconv.FormatInt(h.height, 10)
	results := make([]loadResult, loadClients)

	var wg sync.WaitGroup
	start := time.Now()
	deadline := start.Add(loadDuration)

	for w := range uint64(loadClients) {
		wg.Add(1)

		go func() {
			defer wg.Done()
			res := &results[w]
			rng := rand.New(rand.NewPCG(w, loadTransfers)) //nolint:gosec // load spread, not security
			for time.Now().Before(deadline) {
				id := h.ids[rng.IntN(len(h.ids))]

				ctx, cancel := context.WithTimeout(metadata.AppendToOutgoingContext(context.Background(), grpctypes.GRPCBlockHeightHeader, height), loadCallTimeout)
				var header metadata.MD
				began := time.Now()
				out, err := query.Transfer(ctx, &sinktypes.QueryTransferRequest{ID: id}, grpc.Header(&header))
				took := time.Since(began)
				cancel()

				switch {
				case err != nil:
				case out.Transfer.ID != id:
					err = fmt.Errorf("asked for transfer %s, got %s", id.String(), out.Transfer.ID.String())
				case len(header.Get(grpctypes.GRPCBlockHeightHeader)) != 1 || header.Get(grpctypes.GRPCBlockHeightHeader)[0] != height:
					err = fmt.Errorf("height header %v, want %s", header.Get(grpctypes.GRPCBlockHeightHeader), height)
				}

				if err != nil {
					res.errs++
					if res.firstErr == nil {
						res.firstErr = err
					}

					continue
				}

				res.latencies = append(res.latencies, took)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	var latencies []time.Duration
	var errs int
	var firstErr error
	for _, r := range results {
		latencies = append(latencies, r.latencies...)
		errs += r.errs
		if firstErr == nil {
			firstErr = r.firstErr
		}
	}

	require.NotEmpty(t, latencies, "no query succeeded: %v", firstErr)
	slices.Sort(latencies)
	qps := float64(len(latencies)) / elapsed.Seconds()
	p50 := latencies[len(latencies)/2]
	p99 := latencies[len(latencies)*99/100]

	report := fmt.Sprintf("qps=%.0f p50=%s p99=%s queries=%d errors=%d duration=%s clients=%d transfers=%d height=%d setup=%s cpu=%q GOMAXPROCS=%d\n",
		qps, p50, p99, len(latencies), errs, elapsed.Round(time.Millisecond), loadClients, loadTransfers, h.height, setup.Round(time.Millisecond), cpuModel(), runtime.GOMAXPROCS(0))
	t.Log(report)

	if dir := os.Getenv("REPORTER_PERF_OUT"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0o750))                                                  //nolint:gosec // operator-chosen output directory
		require.NoError(t, os.WriteFile(filepath.Join(dir, "node-load.txt"), []byte(report), 0o600)) //nolint:gosec // operator-chosen output directory
	}

	require.Zero(t, errs, "first error: %v", firstErr)
	require.GreaterOrEqual(t, qps, float64(loadMinQPS), "spec §14: the node serves too few height-pinned Transfer queries per second")
}

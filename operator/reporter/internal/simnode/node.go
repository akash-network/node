package simnode

import (
	"context"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	"pkg.akt.dev/node/v2/operator/reporter/internal/clock"
)

const readHeaderTimeout = 10 * time.Second

// Options configures a Node.
type Options struct {
	// Clock is the time the scenario's offsets are rebased against at Start; it defaults to the system clock.
	Clock clock.Clock
	// AkashLatency delays every CometBFT and gRPC request.
	AkashLatency time.Duration
	// SolanaLatency delays every Solana request.
	SolanaLatency time.Duration
	// BlockFetchLatency further delays every getBlock request.
	BlockFetchLatency time.Duration
	// BlockInterval, when positive, commits an Akash block each time that much Clock time passes.
	BlockInterval time.Duration
	// AlignWindow rebases Akash time at Start so the latest block time lies in the first 60 s of a Unix-aligned
	// 2-minute window and 0 to 70 s behind Clock.Now(). Block times then advance with the clock at that offset.
	AlignWindow bool
}

// Node serves a Scenario on loopback listeners until Close.
type Node struct {
	CometURL  string
	GRPCAddr  string
	SolanaURL string

	opts Options
	// base is Options.Clock.Now() at Start; every scenario offset is relative to it.
	base time.Time

	mu sync.RWMutex
	sc Scenario
	// sigs locates each signature's first transaction, rebuilt whenever the scenario changes.
	sigs map[string]txRef
	ak   akashRuntime
	// queries serves abci_query paths; it is read-only after Start.
	queries queryRouter

	callsMu sync.Mutex
	calls   map[string]int
	// historyWalks counts the chronological history walks served, for Akash.FailHistory.
	historyWalks atomic.Int64

	servers    []*http.Server
	grpcServer *grpc.Server
	done       chan struct{}
	closeOnce  sync.Once
}

// Start serves sc and returns once every listener is bound; ctx bounds only the start-up. Start takes ownership of
// sc: change it afterwards only through Mutate. The CometBFT and gRPC endpoints are served only when sc has an Akash
// part.
func Start(ctx context.Context, sc Scenario, opts Options) (*Node, error) {
	if opts.Clock == nil {
		opts.Clock = clock.Real()
	}

	n := &Node{opts: opts, base: opts.Clock.Now(), sc: sc, calls: map[string]int{}, queries: queryRouter{}, done: make(chan struct{})}
	serveAkash := sc.Akash != nil
	n.indexSolana()
	n.initAkash()
	n.registerQueries(n.queries)

	var err error
	n.SolanaURL, err = n.serveHTTP(ctx, http.HandlerFunc(n.serveSolana))
	if err == nil && serveAkash {
		if n.CometURL, err = n.serveHTTP(ctx, n.cometHandler()); err == nil {
			n.GRPCAddr, err = n.serveGRPC(ctx)
		}
	}
	if err != nil {
		n.Close()
		return nil, err
	}

	if serveAkash && opts.BlockInterval > 0 {
		go n.produceBlocks()
	}

	return n, nil
}

// produceBlocks commits a block each BlockInterval of Clock time until Close.
func (n *Node) produceBlocks() {
	for {
		select {
		case <-n.done:
			return
		case <-n.opts.Clock.After(n.opts.BlockInterval):
			n.CommitBlock()
		}
	}
}

// serveHTTP serves h on a new loopback listener and returns its base URL.
func (n *Node) serveHTTP(ctx context.Context, h http.Handler) (string, error) {
	var lc net.ListenConfig
	l, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	srv := &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout}
	n.servers = append(n.servers, srv)
	go func() { _ = srv.Serve(l) }() // returns http.ErrServerClosed once Close runs

	return "http://" + l.Addr().String(), nil
}

// Close stops every server and drops open connections. It is safe to call more than once.
func (n *Node) Close() {
	n.closeOnce.Do(func() {
		close(n.done)
		for _, srv := range n.servers {
			_ = srv.Close()
		}
		if n.grpcServer != nil {
			n.grpcServer.Stop()
		}
	})
}

// Calls returns how many calls of method the node has received: a Solana or CometBFT JSON-RPC method name, or a gRPC
// full method name such as "/akash.sink.v1.Query/Transfer".
func (n *Node) Calls(method string) int {
	n.callsMu.Lock()
	defer n.callsMu.Unlock()

	return n.calls[method]
}

func (n *Node) count(method string) {
	n.callsMu.Lock()
	defer n.callsMu.Unlock()

	n.calls[method]++
}

// Mutate changes the served scenario under the node's lock; requests see either the old or the new state.
func (n *Node) Mutate(f func(*Scenario)) {
	n.mu.Lock()
	defer n.mu.Unlock()

	f(&n.sc)
	n.indexSolana()
	n.indexAkash()
}

// at returns the time of a scenario offset.
func (n *Node) at(offset time.Duration) time.Time {
	return n.base.Add(offset)
}

// delay waits d, or until ctx ends.
func delay(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

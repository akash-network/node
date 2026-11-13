package akash

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"time"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	cmttypes "github.com/cometbft/cometbft/types"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// maxClockSkew is how far ahead of the local clock a block time may be.
const maxClockSkew = 10 * time.Second

// The tracker's texts, verbatim from contracts/solana/reporter/akash.ts.
var (
	errWrongChain       = errors.New("wrong Akash chain")                           //nolint:staticcheck // verbatim TS text
	errSyncing          = errors.New("Akash node is syncing")                       //nolint:staticcheck // verbatim TS text
	errStaleNode        = errors.New("stale Akash node")                            //nolint:staticcheck // verbatim TS text
	errNoCommittedState = errors.New("Akash node has no queryable committed state") //nolint:staticcheck // verbatim TS text
	errBlockHeight      = errors.New("Akash block height mismatch")                 //nolint:staticcheck // verbatim TS text
	errWrongBlockChain  = errors.New("wrong Akash block chain")                     //nolint:staticcheck // verbatim TS text
	errStaleCommitted   = errors.New("stale Akash committed state")                 //nolint:staticcheck // verbatim TS text
	errQueryHeight      = errors.New("Akash query height mismatch")                 //nolint:staticcheck // verbatim TS text
	errNoPolicy         = errors.New("missing current reporter policy")
	errMissingState     = errors.New("missing sink state")
	errEpoch            = errors.New("invalid quota epoch")
	errUsageEpoch       = errors.New("quota usage epoch mismatch")
	errEarlyUsage       = errors.New("usage before quota epoch starts")
)

// State is an immutable snapshot of the sink module at one committed height. Callers share it and must not change it.
type State struct {
	Height      int64
	Time        time.Time
	Params      sinkv1.Params
	Policy      sinkv1.ReporterPolicy
	RefundUsage sinkv1.EpochUsage
	SendUsage   sinkv1.EpochUsage
	Totals      sinkv1.Totals
	Epoch       uint64
	RefundHead  *sinkid.ID
}

// tracker holds the latest refresh result, a snapshot or an error, for MinRefresh.
type tracker struct {
	group singleflight.Group

	mu   sync.Mutex
	last *refreshResult
}

type refreshResult struct {
	// at is when the refresh started.
	at    time.Time
	state *State
	err   error
}

// cached returns the last refresh result while it is younger than MinRefresh, or nil.
func (c *Client) cached() *refreshResult {
	c.track.mu.Lock()
	defer c.track.mu.Unlock()

	if r := c.track.last; r != nil && c.clock.Now().Sub(r.at) < c.minRefresh {
		return r
	}

	return nil
}

// Latest returns the latest snapshot. At most one refresh runs per MinRefresh: concurrent callers share an in-flight
// refresh, and its snapshot or error is reused until MinRefresh has passed since it started. A snapshot may be lower
// than an earlier one, for example from a load-balanced node; safety does not depend on height order.
func (c *Client) Latest(ctx context.Context) (*State, error) {
	if r := c.cached(); r != nil {
		return r.state, r.err
	}

	// The refresh outlives a caller that gives up, so the others sharing it still get its result.
	refreshCtx := context.WithoutCancel(ctx)
	ch := c.track.group.DoChan("latest", func() (any, error) {
		// A refresh that finished after this caller missed the cache still counts.
		if r := c.cached(); r != nil {
			return r.state, r.err
		}

		r := &refreshResult{at: c.clock.Now()}
		r.state, r.err = c.refresh(refreshCtx)

		c.track.mu.Lock()
		c.track.last = r
		c.track.mu.Unlock()

		return r.state, r.err
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		s, _ := r.Val.(*State)
		return s, r.Err
	}
}

func (c *Client) status(ctx context.Context) (st *ctypes.ResultStatus, err error) {
	err = c.comet(ctx, "status", func(ctx context.Context) error {
		st, err = c.rpc.Status(ctx)
		return err
	})

	return st, err
}

// refresh takes a snapshot as AkashClient.state() does (spec §5.3).
func (c *Client) refresh(ctx context.Context) (*State, error) {
	st, err := c.status(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.checkStatus(st); err != nil {
		return nil, err
	}

	// CometBFT may advertise H while BaseApp is still committing it: read only a height the application has
	// committed, and use that block's clock.
	var info *ctypes.ResultABCIInfo
	if err := c.comet(ctx, "abci_info", func(ctx context.Context) (err error) {
		info, err = c.rpc.ABCIInfo(ctx)
		return err
	}); err != nil {
		return nil, err
	}

	height := min(st.SyncInfo.LatestBlockHeight, info.Response.LastBlockHeight)
	if height <= 1 {
		return nil, errNoCommittedState
	}

	var block *ctypes.ResultBlock
	if err := c.comet(ctx, "block", func(ctx context.Context) (err error) {
		block, err = c.rpc.Block(ctx, &height)
		return err
	}); err != nil {
		return nil, err
	}

	if block.Block == nil {
		return nil, errBlockHeight
	}

	if err := c.checkBlock(height, block.Block.Header); err != nil {
		return nil, err
	}

	var (
		state     *sinkv1.QueryStateResponse
		params    *sinkv1.QueryParamsResponse
		reporters *sinkv1.QueryReportersResponse
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return c.pinned(gctx, height, errQueryHeight, func(ctx context.Context, opts ...grpc.CallOption) (err error) {
			state, err = c.sink.State(ctx, &sinkv1.QueryStateRequest{}, opts...)
			return err
		})
	})
	g.Go(func() error {
		return c.pinned(gctx, height, errQueryHeight, func(ctx context.Context, opts ...grpc.CallOption) (err error) {
			params, err = c.sink.Params(ctx, &sinkv1.QueryParamsRequest{}, opts...)
			return err
		})
	})
	g.Go(func() error {
		return c.pinned(gctx, height, errQueryHeight, func(ctx context.Context, opts ...grpc.CallOption) (err error) {
			reporters, err = c.sink.Reporters(ctx, &sinkv1.QueryReportersRequest{Version: 0}, opts...)
			return err
		})
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return newState(height, block.Block.Time, state, params.Params, reporters.Reporters)
}

func (c *Client) checkStatus(st *ctypes.ResultStatus) error {
	switch {
	case st.NodeInfo.Network != c.chainID:
		return errWrongChain
	case st.SyncInfo.CatchingUp:
		return errSyncing
	case !c.fresh(st.SyncInfo.LatestBlockTime):
		return errStaleNode
	}

	return nil
}

func (c *Client) checkBlock(height int64, header cmttypes.Header) error {
	switch {
	case header.Height != height:
		return errBlockHeight
	case header.ChainID != c.chainID:
		return errWrongBlockChain
	case !c.fresh(header.Time):
		return errStaleCommitted
	}

	return nil
}

// fresh reports whether a block's age lies in [-10 s, MaxAge], with the block time floored to a second as in TS.
func (c *Client) fresh(t time.Time) bool {
	age := c.clock.Now().Sub(time.Unix(t.Unix(), 0))
	return age >= -maxClockSkew && age <= c.maxAge
}

// newState applies the TS checks to the sink reads at height.
func newState(height int64, t time.Time, st *sinkv1.QueryStateResponse, params sinkv1.Params, reporters []sinkv1.ReporterPolicy) (*State, error) {
	if len(reporters) != 1 {
		return nil, errNoPolicy
	}

	policy := reporters[0]
	if isZero(params) || isZero(st.Totals) || policy.Version == 0 || isZero(st.SendUsage) || isZero(st.RefundUsage) {
		return nil, errMissingState
	}

	if st.Epoch > math.MaxInt64 {
		return nil, errEpoch
	}

	for _, usage := range []sinkv1.EpochUsage{st.SendUsage, st.RefundUsage} {
		if usage.Epoch != st.Epoch {
			return nil, errUsageEpoch
		}

		amount, err := sinkid.CoinAmount(usage.Amount, false)
		if err != nil {
			return nil, err
		}

		if st.Epoch == 0 && !amount.IsZero() {
			return nil, errEarlyUsage
		}
	}

	var head *sinkid.ID
	if st.RefundHead != nil {
		id, err := sinkid.FromProto(*st.RefundHead)
		if err != nil {
			return nil, err
		}
		head = &id
	}

	return &State{
		Height:      height,
		Time:        t,
		Params:      params,
		Policy:      policy,
		RefundUsage: st.RefundUsage,
		SendUsage:   st.SendUsage,
		Totals:      st.Totals,
		Epoch:       st.Epoch,
		RefundHead:  head,
	}, nil
}

// isZero reports a message field that was absent on the wire: gogoproto leaves a non-nullable field at its zero value.
func isZero(v any) bool { return reflect.ValueOf(v).IsZero() }

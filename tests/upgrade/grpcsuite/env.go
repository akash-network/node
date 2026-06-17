package grpcsuite

import (
	"context"
	"sync"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	cmtservice "github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"pkg.akt.dev/go/sdkutil"
)

// Env is the harness-agnostic environment the suite runs against. Both the upgrade
// post-upgrade worker and the in-process integration driver populate one of these.
type Env struct {
	// GRPCEndpoint is the host:port of the node's gRPC server (e.g. 127.0.0.1:9090).
	GRPCEndpoint string
	// ChainID of the running chain (e.g. localakash).
	ChainID string
	// RepoRoot is the path to the node repository root, used to locate testdata
	// (SDL/provider yaml). For the upgrade worker this is TestParams.SourceDir.
	RepoRoot string

	Cdc          codec.Codec
	InterfaceReg codectypes.InterfaceRegistry
	TxConfig     sdkclient.TxConfig
	Amino        *codec.LegacyAmino

	// Keyring contains Funder (and is where the suite creates funded sub-accounts).
	Keyring    keyring.Keyring
	Funder     string         // key name of a well-funded account (e.g. validator0)
	FunderAddr sdk.AccAddress // address of Funder

	BondDenom string // staking/fee denom, e.g. uakt
	GasPrices string // e.g. "0.025uakt"

	// RequireFullCoverage makes the coverage gate fail the test if any in-scope
	// Msg or query RPC was never exercised. Drivers set this true; it can be
	// relaxed while authoring new packs.
	RequireFullCoverage bool
}

// World threads state created by earlier packs to later ones (a deployment's dseq,
// the provider address, an order/bid/lease id, etc.). Packs read and write it.
type World struct {
	mu sync.Mutex

	// Accounts created/funded by the suite, keyed by logical role.
	Accounts map[string]sdk.AccAddress

	// Cross-pack handles populated as scenarios run. Concrete types are filled in
	// by the packs that own them; consumers type-assert.
	Handles map[string]interface{}
}

func newWorld() *World {
	return &World{Accounts: map[string]sdk.AccAddress{}, Handles: map[string]interface{}{}}
}

// Set stores a cross-pack handle.
func (w *World) Set(key string, v interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.Handles[key] = v
}

// Get retrieves a cross-pack handle (nil if absent).
func (w *World) Get(key string) interface{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Handles[key]
}

// Suite is the running context shared by every pack. It owns the gRPC connection,
// the gRPC-backed client.Context, the tx broadcaster, the coverage tracker and the
// World.
type Suite struct {
	T   *testing.T
	Ctx context.Context

	Env  Env
	Conn *grpc.ClientConn
	Cctx sdkclient.Context

	TX   *Broadcaster
	Cov  *Coverage
	Disc *Discovery

	World *World

	dseqMu   sync.Mutex
	dseqSeed uint64
}

// Run is the single entrypoint. It connects to the gRPC endpoint, runs every
// registered pack in dependency order, then enforces the coverage gate.
func Run(ctx context.Context, t *testing.T, env Env) {
	t.Helper()

	require.NotEmpty(t, env.GRPCEndpoint, "grpcsuite: GRPCEndpoint required")
	require.NotNil(t, env.InterfaceReg, "grpcsuite: InterfaceReg required")

	cov := newCoverage()

	conn, err := dialGRPC(env.GRPCEndpoint, env.InterfaceReg, cov)
	require.NoError(t, err, "grpcsuite: dial gRPC")
	defer func() { _ = conn.Close() }()

	cctx := newGRPCClientContext(env, conn)

	s := &Suite{
		T:     t,
		Ctx:   ctx,
		Env:   env,
		Conn:  conn,
		Cctx:  cctx,
		Cov:   cov,
		World: newWorld(),
	}
	s.TX = newBroadcaster(s)

	// Discover what the running binary actually serves so the coverage gate and
	// pack availability adapt to the branch under test (e.g. verification appears
	// on AEP-86 but not on main).
	disc, err := discover(ctx, conn, env.InterfaceReg)
	require.NoError(t, err, "grpcsuite: discover served services")
	cov.setExpected(disc)
	cov.setStrict(env.RequireFullCoverage)
	s.Disc = disc
	t.Logf("grpcsuite: discovered %d in-scope tx methods, %d in-scope query methods",
		len(disc.InScopeMsgs()), len(disc.InScopeQueries()))

	// Packs run sequentially (later packs depend on state earlier ones create via
	// the shared World), so it is safe to retarget s.T at the active subtest rather
	// than copying the Suite (which holds a mutex).
	for _, p := range packs {
		if !p.Available(disc) {
			t.Logf("grpcsuite: pack %q not available on this binary; skipping", p.Name())
			continue
		}
		t.Run(p.Name(), func(subT *testing.T) {
			prev := s.T
			s.T = subT
			defer func() { s.T = prev }()
			p.Run(s)
		})
	}

	// Dynamic query smoke sweep: reach every advertised query handler over gRPC.
	// This auto-covers the query surface and catches advertised-but-unimplemented
	// methods; authored query cases (in packs) verify correctness with real inputs.
	t.Run("query-smoke-sweep", func(subT *testing.T) {
		prev := s.T
		s.T = subT
		defer func() { s.T = prev }()
		s.querySmokeSweep(disc)
	})

	// Coverage gate: fail if any in-scope tx or query RPC was never exercised.
	cov.assert(t)
}

// FundAccount creates a fresh key named role (if absent) and funds it from the
// suite's Funder with coins. Returns the account address and records it in the
// World.
func (s *Suite) FundAccount(role string, coins sdk.Coins) sdk.AccAddress {
	s.T.Helper()

	addr := s.ensureKey(role)
	msg := banktypes.NewMsgSend(s.Env.FunderAddr, addr, coins)
	s.BroadcastOK(s.Env.Funder, msg)

	s.World.mu.Lock()
	s.World.Accounts[role] = addr
	s.World.mu.Unlock()
	return addr
}

// FundAccountDefault funds role with a generous bundle of fee (uakt) and deposit
// (uact) tokens, sufficient for any scenario in the suite.
func (s *Suite) FundAccountDefault(role string) sdk.AccAddress {
	coins := sdk.NewCoins(
		sdk.NewCoin(s.Env.BondDenom, sdkmath.NewInt(1_000_000_000)),   // ~1000 AKT for fees
		sdk.NewCoin(sdkutil.DenomUact, sdkmath.NewInt(1_000_000_000)), // for deposits
	)
	return s.FundAccount(role, coins)
}

// NextDSeq returns a process-unique, monotonically increasing deployment sequence
// number seeded from the chain's current height (the conventional DSeq source).
func (s *Suite) NextDSeq() uint64 {
	s.dseqMu.Lock()
	defer s.dseqMu.Unlock()
	if s.dseqSeed == 0 {
		s.dseqSeed = uint64(s.LatestHeight())
	}
	s.dseqSeed++
	return s.dseqSeed
}

// WaitBlocks blocks until the chain advances by at least n blocks.
func (s *Suite) WaitBlocks(n int64) {
	s.T.Helper()
	start := s.LatestHeight()
	ctx, cancel := context.WithTimeout(s.Ctx, 60*time.Second)
	defer cancel()
	for {
		if s.LatestHeight() >= start+n {
			return
		}
		select {
		case <-ctx.Done():
			s.T.Fatalf("grpcsuite: WaitBlocks(%d) timed out", n)
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// LatestHeight returns the chain's latest block height over gRPC.
func (s *Suite) LatestHeight() int64 {
	s.T.Helper()
	resp, err := cmtservice.NewServiceClient(s.Conn).GetLatestBlock(s.Ctx, &cmtservice.GetLatestBlockRequest{})
	require.NoError(s.T, err, "grpcsuite: GetLatestBlock")
	if resp.SdkBlock != nil {
		return resp.SdkBlock.Header.Height
	}
	return resp.Block.Header.Height
}

// LatestBlockTime returns the chain's latest block time over gRPC (use this rather
// than wall-clock time for on-chain timestamp fields).
func (s *Suite) LatestBlockTime() time.Time {
	s.T.Helper()
	resp, err := cmtservice.NewServiceClient(s.Conn).GetLatestBlock(s.Ctx, &cmtservice.GetLatestBlockRequest{})
	require.NoError(s.T, err, "grpcsuite: GetLatestBlock")
	if resp.SdkBlock != nil {
		return resp.SdkBlock.Header.Time
	}
	return resp.Block.Header.Time
}

// ensureKey returns the address of a keyring key named role, creating it if needed.
func (s *Suite) ensureKey(role string) sdk.AccAddress {
	s.T.Helper()
	if rec, err := s.Env.Keyring.Key(role); err == nil {
		addr, err := rec.GetAddress()
		require.NoError(s.T, err)
		return addr
	}
	rec, _, err := s.Env.Keyring.NewMnemonic(role, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	require.NoError(s.T, err, "grpcsuite: create key %q", role)
	addr, err := rec.GetAddress()
	require.NoError(s.T, err)
	return addr
}

// Addr returns the address of a previously funded role, failing if unknown.
func (s *Suite) Addr(role string) sdk.AccAddress {
	s.World.mu.Lock()
	defer s.World.mu.Unlock()
	addr, ok := s.World.Accounts[role]
	require.Truef(s.T, ok, "grpcsuite: account role %q not set up", role)
	return addr
}

// logf is a small helper to keep pack output consistent.
func (s *Suite) logf(format string, args ...interface{}) {
	s.T.Logf("grpcsuite: "+format, args...)
}

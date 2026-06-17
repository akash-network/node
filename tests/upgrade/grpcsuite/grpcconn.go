// Package grpcsuite implements a harness-agnostic, exhaustive post-upgrade
// verification suite that exercises every Akash (and targeted Cosmos) transaction
// and query strictly over the chain's gRPC API.
//
// The suite is driven by two thin callers:
//   - the upgrade post-upgrade worker (tests/upgrade, build tag e2e.upgrade), which
//     points it at a testnetify-forked, freshly-upgraded validator; and
//   - an in-process integration test (tests/e2e, build tag e2e.integration), which
//     points it at a single-validator testutil/network for fast iteration.
//
// Both supply an Env; the suite owns everything else. All checks run against the
// gRPC endpoint: queries route through a gRPC-backed client.Context (WithGRPCClient),
// and transactions are signed locally and broadcast via the cosmos tx ServiceClient
// over the same connection.
package grpcsuite

import (
	"context"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	cflags "pkg.akt.dev/go/cli/flags"
)

// dialGRPC opens a gRPC connection to endpoint (host:port) configured with the
// cosmos gogoproto codec so that interface (Any) fields decode correctly. A unary
// interceptor records every invoked method path into cov, so that every query made
// through any generated QueryClient is automatically counted for coverage.
func dialGRPC(endpoint string, ireg codectypes.InterfaceRegistry, cov *Coverage) (*grpc.ClientConn, error) {
	pc := codec.NewProtoCodec(ireg)
	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(pc.GRPCCodec())),
		grpc.WithChainUnaryInterceptor(coverageInterceptor(cov)),
	)
	if err != nil {
		return nil, fmt.Errorf("dial gRPC %q: %w", endpoint, err)
	}
	return conn, nil
}

// coverageInterceptor records every gRPC method invoked through the connection.
func coverageInterceptor(cov *Coverage) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		cov.recordMethod(method)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// newGRPCClientContext builds a client.Context whose query path is the gRPC
// connection. Generated module QueryClients constructed from this context (and the
// auth AccountRetriever) invoke over gRPC rather than the Comet RPC/ABCI path.
func newGRPCClientContext(env Env, conn *grpc.ClientConn) sdkclient.Context {
	return sdkclient.Context{}.
		WithCodec(env.Cdc).
		WithInterfaceRegistry(env.InterfaceReg).
		WithTxConfig(env.TxConfig).
		WithLegacyAmino(env.Amino).
		WithChainID(env.ChainID).
		WithKeyring(env.Keyring).
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithBroadcastMode(cflags.BroadcastSync).
		WithSkipConfirmation(true).
		WithSignModeStr("direct").
		WithGRPCClient(conn)
}

//go:build e2e.integration

// Package fullsurface hosts the in-process driver for the exhaustive gRPC
// transaction/query suite (grpcsuite). It spins up a single-validator
// testutil/network and runs the same suite that the post-upgrade worker runs
// against a testnetify-forked node, giving a fast (minutes) iteration and per-PR
// CI signal without the full upgrade cycle.
//
// It lives in its own package (not tests/e2e) so it compiles standalone against
// the pinned SDK; the existing tests/e2e integration tests currently only build
// against the workspace-local chain-sdk.
package fullsurface

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"pkg.akt.dev/node/v2/tests/upgrade/grpcsuite"
	"pkg.akt.dev/node/v2/testutil"
	"pkg.akt.dev/node/v2/testutil/network"
)

func TestFullSurfaceGRPC(t *testing.T) {
	// Short gov voting period + low min deposit so the suite's gov fast-path (used
	// for every MsgUpdateParams and other gov-gated messages) completes quickly.
	// The post-upgrade harness gets the equivalent via tests/upgrade/testnet.json.
	cfg := network.DefaultConfig(testutil.NewTestNetworkFixture,
		network.WithInterceptState(func(cdc codec.Codec, moduleName string, state json.RawMessage) json.RawMessage {
			if moduleName != govtypes.ModuleName {
				return nil
			}
			var gs govv1.GenesisState
			cdc.MustUnmarshalJSON(state, &gs)
			vp := 8 * time.Second
			ep := 6 * time.Second
			gs.Params.VotingPeriod = &vp
			gs.Params.ExpeditedVotingPeriod = &ep
			gs.Params.MinDeposit = sdk.NewCoins(sdk.NewInt64Coin("uakt", 10_000_000))
			return cdc.MustMarshalJSON(&gs)
		}),
	)
	cfg.NumValidators = 1

	net := network.New(t, cfg)
	defer net.Cleanup()

	_, err := net.WaitForHeightWithTimeout(2, 30*time.Second)
	require.NoError(t, err)

	val := net.Validators[0]
	require.NotEmpty(t, val.AppConfig.GRPC.Address, "gRPC server must be enabled")

	root, err := filepath.Abs("../..")
	require.NoError(t, err)

	env := grpcsuite.Env{
		GRPCEndpoint: val.AppConfig.GRPC.Address,
		ChainID:      cfg.ChainID,
		RepoRoot:     root,
		Cdc:          cfg.Codec,
		InterfaceReg: cfg.InterfaceRegistry,
		TxConfig:     cfg.TxConfig,
		Amino:        cfg.LegacyAmino,
		Keyring:      val.ClientCtx.Keyring,
		Funder:       "node0", // validator key name in the keyring
		FunderAddr:   val.Address,
		BondDenom:    cfg.BondDenom,
		GasPrices:    "0.025uakt",
		// Enforce full coverage: fail if any in-scope Akash tx or query is not
		// exercised (a new RPC added by a future upgrade turns this red).
		RequireFullCoverage: true,
	}

	grpcsuite.Run(context.Background(), t, env)
}

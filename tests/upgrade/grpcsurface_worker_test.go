//go:build e2e.upgrade

package upgrade

import (
	"context"
	"testing"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/go/sdkutil"

	akash "pkg.akt.dev/node/v2/app"
	"pkg.akt.dev/node/v2/tests/upgrade/grpcsuite"
	uttypes "pkg.akt.dev/node/v2/tests/upgrade/types"
)

// The exhaustive gRPC tx/query suite runs after EVERY upgrade as the universal
// post-upgrade worker, against the testnetify-forked, freshly-upgraded validator.
func init() {
	uttypes.RegisterUniversalPostUpgradeWorker(&grpcSurfaceWorker{})
}

type grpcSurfaceWorker struct{}

var _ uttypes.TestWorker = (*grpcSurfaceWorker)(nil)

func (w *grpcSurfaceWorker) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {
	encCfg := sdkutil.MakeEncodingConfig()
	akash.ModuleBasics().RegisterInterfaces(encCfg.InterfaceRegistry)

	// Load the keyring shipped with the testnetify state (contains the funded,
	// voting-power-holding `params.From` account, e.g. validator0).
	cctx := sdkclient.Context{}.
		WithCodec(encCfg.Codec).
		WithKeyringDir(params.Home)
	kr, err := sdkclient.NewKeyringFromBackend(cctx, params.KeyringBackend)
	require.NoError(t, err)

	env := grpcsuite.Env{
		GRPCEndpoint: params.GRPC,
		ChainID:      params.ChainID,
		RepoRoot:     params.SourceDir,
		Cdc:          encCfg.Codec,
		InterfaceReg: encCfg.InterfaceRegistry,
		TxConfig:     encCfg.TxConfig,
		Amino:        encCfg.Amino,
		Keyring:      kr,
		Funder:       params.From,
		FunderAddr:   params.FromAddress,
		BondDenom:    sdkutil.DenomUakt,
		GasPrices:    "0.025uakt",
		// Partial pack coverage today; flip to true once every Akash tx has an
		// authored pack so the gate enforces full coverage post-upgrade.
		RequireFullCoverage: false,
	}

	grpcsuite.Run(ctx, t, env)
}

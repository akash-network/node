package testutil

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	pruningtypes "cosmossdk.io/store/pruning/types"
	dbm "github.com/cosmos/cosmos-db"
	bam "github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"

	cflags "pkg.akt.dev/go/cli/flags"
	"pkg.akt.dev/go/sdkutil"

	"pkg.akt.dev/node/app"
	"pkg.akt.dev/node/testutil/network"
)

// NewTestNetworkFixture returns a new simapp AppConstructor for network simulation tests
func NewTestNetworkFixture() network.TestFixture {
	dir, err := os.MkdirTemp("", "simapp")
	if err != nil {
		panic(fmt.Sprintf("failed creating temporary directory: %v", err))
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	encodingConfig := sdkutil.MakeEncodingConfig()

	tapp := app.NewApp(
		log.NewNopLogger(),
		dbm.NewMemDB(),
		nil,
		true,
		0,
		make(map[int64]bool),
		encodingConfig,
		simtestutil.NewAppOptionsWithFlagHome(dir),
	)

	appCtr := func(val network.ValidatorI) servertypes.Application {
		return app.NewApp(
			val.GetCtx().Logger,
			dbm.NewMemDB(),
			nil,
			true,
			0,
			make(map[int64]bool),
			encodingConfig,
			simtestutil.NewAppOptionsWithFlagHome(val.GetCtx().Config.RootDir),
			bam.SetPruning(pruningtypes.NewPruningOptionsFromString(val.GetAppConfig().Pruning)),
			bam.SetMinGasPrices(val.GetAppConfig().MinGasPrices),
			bam.SetChainID(val.GetCtx().Viper.GetString(cflags.FlagChainID)),
		)
	}

	return network.TestFixture{
		AppConstructor: appCtr,
		GenesisState:   app.NewDefaultGenesisState(tapp.AppCodec()),
		EncodingConfig: testutil.TestEncodingConfig{
			InterfaceRegistry: tapp.InterfaceRegistry(),
			Codec:             tapp.AppCodec(),
			TxConfig:          tapp.TxConfig(),
			Amino:             tapp.LegacyAmino(),
		},
	}
}

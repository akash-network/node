package testutil

import (
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"

	dbm "github.com/cometbft/cometbft-db"
	bam "github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	pruningtypes "github.com/cosmos/cosmos-sdk/store/pruning/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"

	cflags "pkg.akt.dev/go/cli/flags"

	rtypes "pkg.akt.dev/go/node/types/resources/v1beta4"

	"pkg.akt.dev/node/app"
	"pkg.akt.dev/node/testutil/network"
)

func RandRangeInt(min, max int) int {
	return rand.Intn(max-min) + min // nolint: gosec
}

func RandRangeUint(min, max uint) uint {
	val := rand.Uint64() // nolint: gosec
	val %= uint64(max - min)
	val += uint64(min)
	return uint(val)
}

func RandRangeUint64(min, max uint64) uint64 {
	val := rand.Uint64() // nolint: gosec
	val %= max - min
	val += min
	return val
}

func ResourceUnits(_ testing.TB) rtypes.Resources {
	return rtypes.Resources{
		ID: 1,
		CPU: &rtypes.CPU{
			Units: rtypes.NewResourceValue(uint64(RandCPUUnits())),
		},
		Memory: &rtypes.Memory{
			Quantity: rtypes.NewResourceValue(RandMemoryQuantity()),
		},
		GPU: &rtypes.GPU{
			Units: rtypes.NewResourceValue(uint64(RandGPUUnits())),
		},
		Storage: rtypes.Volumes{
			rtypes.Storage{
				Quantity: rtypes.NewResourceValue(RandStorageQuantity()),
			},
		},
	}
}

// NewTestNetworkFixture returns a new simapp AppConstructor for network simulation tests
func NewTestNetworkFixture() network.TestFixture {
	dir, err := os.MkdirTemp("", "simapp")
	if err != nil {
		panic(fmt.Sprintf("failed creating temporary directory: %v", err))
	}
	defer func() {
		_ = os.RemoveAll(dir)
	}()

	encodingConfig := app.MakeEncodingConfig()

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

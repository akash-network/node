package testutil

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	tmrand "github.com/tendermint/tendermint/libs/rand"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/app"
	"github.com/ovrclk/akash/types"
)

const (
	defaultMinCPUUnit     = 10
	defaultMaxCPUUnit     = 500
	defaultMinMemorySize  = 1024
	defaultMaxMemorySize  = 1073741824
	defaultMinStorageSize = 1024
	defaultMaxStorageSize = 1073741824
)

func RandRangeInt(min, max int) int {
	return rand.Intn(max-min) + min
}

func ResourceUnits(_ testing.TB) types.ResourceUnits {
	return types.ResourceUnits{
		CPU: &types.CPU{
			Units: types.NewResourceValue(uint64(RandRangeInt(defaultMinCPUUnit, defaultMaxCPUUnit))),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(uint64(RandRangeInt(defaultMinMemorySize, defaultMaxMemorySize))),
		},
		Storage: &types.Storage{
			Quantity: types.NewResourceValue(uint64(RandRangeInt(defaultMinStorageSize, defaultMaxStorageSize))),
		},
	}
}

func NewApp(val network.Validator) servertypes.Application {
	return app.NewApp(
		val.Ctx.Logger, dbm.NewMemDB(), nil, 0, make(map[int64]bool), val.Ctx.Config.RootDir,
		baseapp.SetPruning(storetypes.NewPruningOptionsFromString(val.AppConfig.Pruning)),
		baseapp.SetMinGasPrices(val.AppConfig.MinGasPrices),
	)
}

// DefaultConfig returns a default configuration suitable for nearly all
// testing requirements.
func DefaultConfig() network.Config {
	encCfg := app.MakeEncodingConfig()

	return network.Config{
		Codec:             encCfg.Marshaler,
		TxConfig:          encCfg.TxConfig,
		LegacyAmino:       encCfg.Amino,
		InterfaceRegistry: encCfg.InterfaceRegistry,
		AccountRetriever:  authtypes.AccountRetriever{},
		AppConstructor:    NewApp,
		GenesisState:      app.ModuleBasics().DefaultGenesis(encCfg.Marshaler),
		TimeoutCommit:     2 * time.Second,
		ChainID:           "chain-" + tmrand.NewRand().Str(6),
		NumValidators:     4,
		BondDenom:         sdk.DefaultBondDenom,
		MinGasPrices:      fmt.Sprintf("0.000006%s", sdk.DefaultBondDenom),
		AccountTokens:     sdk.TokensFromConsensusPower(10000),
		StakingTokens:     sdk.TokensFromConsensusPower(500),
		BondedTokens:      sdk.TokensFromConsensusPower(100),
		PruningStrategy:   storetypes.PruningOptionNothing,
		CleanupDir:        true,
		SigningAlgo:       string(hd.Secp256k1Type),
		KeyringOptions:    []keyring.Option{},
	}
}

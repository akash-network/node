package testutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
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

	"github.com/cosmos/cosmos-sdk/simapp"
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
	return rand.Intn(max-min) + min // nolint: gosec
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
		val.Ctx.Logger, dbm.NewMemDB(), nil, true, 0, make(map[int64]bool), val.Ctx.Config.RootDir,
		simapp.EmptyAppOptions{},
		baseapp.SetPruning(storetypes.NewPruningOptionsFromString(val.AppConfig.Pruning)),
		baseapp.SetMinGasPrices(val.AppConfig.MinGasPrices),
	)
}

// DefaultConfig returns a default configuration suitable for nearly all
// testing requirements.
func DefaultConfig() network.Config {
	encCfg := app.MakeEncodingConfig()
	origGenesisState := app.ModuleBasics().DefaultGenesis(encCfg.Marshaler)

	genesisState := make(map[string]json.RawMessage)
	for k, v := range origGenesisState {
		data, err := v.MarshalJSON()
		if err != nil {
			panic(err)
		}

		buf := &bytes.Buffer{}
		_, err = buf.Write(data)
		if err != nil {
			panic(err)
		}

		stringData := buf.String()
		stringDataAfter := strings.ReplaceAll(stringData, `"stake"`, `"uakt"`)
		if stringData == stringDataAfter {
			genesisState[k] = v
			continue
		}

		var val map[string]interface{}
		err = json.Unmarshal(buf.Bytes(), &val)
		if err != nil {
			panic(err)
		}

		replacementV := json.RawMessage(stringDataAfter)
		genesisState[k] = replacementV

	}

	return network.Config{
		Codec:             encCfg.Marshaler,
		TxConfig:          encCfg.TxConfig,
		LegacyAmino:       encCfg.Amino,
		InterfaceRegistry: encCfg.InterfaceRegistry,
		AccountRetriever:  authtypes.AccountRetriever{},
		AppConstructor:    NewApp,

		GenesisState:    genesisState,
		TimeoutCommit:   2 * time.Second,
		ChainID:         "chain-" + tmrand.NewRand().Str(6),
		NumValidators:   4,
		BondDenom:       CoinDenom,
		MinGasPrices:    fmt.Sprintf("0.000006%s", CoinDenom),
		AccountTokens:   sdk.TokensFromConsensusPower(10000),
		StakingTokens:   sdk.TokensFromConsensusPower(500),
		BondedTokens:    sdk.TokensFromConsensusPower(100),
		PruningStrategy: storetypes.PruningOptionNothing,
		CleanupDir:      true,
		SigningAlgo:     string(hd.Secp256k1Type),
		KeyringOptions:  []keyring.Option{},
	}
}

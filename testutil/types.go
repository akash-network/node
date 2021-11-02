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
	"github.com/cosmos/cosmos-sdk/simapp"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	"github.com/cosmos/cosmos-sdk/testutil/network"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	tmrand "github.com/tendermint/tendermint/libs/rand"
	dbm "github.com/tendermint/tm-db"

	"github.com/ovrclk/akash/app"
	types "github.com/ovrclk/akash/types/v1beta2"
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

func ResourceUnits(_ testing.TB) types.ResourceUnits {
	return types.ResourceUnits{
		CPU: &types.CPU{
			Units: types.NewResourceValue(uint64(RandCPUUnits())),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(RandMemoryQuantity()),
		},
		Storage: types.Volumes{
			types.Storage{
				Quantity: types.NewResourceValue(RandStorageQuantity()),
			},
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
		GenesisState:      genesisState,
		TimeoutCommit:     2 * time.Second,
		ChainID:           "chain-" + tmrand.NewRand().Str(6),
		NumValidators:     4,
		BondDenom:         CoinDenom,
		MinGasPrices:      fmt.Sprintf("0.000006%s", CoinDenom),
		AccountTokens:     sdk.TokensFromConsensusPower(1000000000000, sdk.DefaultPowerReduction),
		StakingTokens:     sdk.TokensFromConsensusPower(100000, sdk.DefaultPowerReduction),
		BondedTokens:      sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction),
		PruningStrategy:   storetypes.PruningOptionNothing,
		CleanupDir:        true,
		SigningAlgo:       string(hd.Secp256k1Type),
		KeyringOptions:    []keyring.Option{},
	}
}

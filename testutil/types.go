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
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	tmrand "github.com/tendermint/tendermint/libs/rand"
	dbm "github.com/tendermint/tm-db"

	types "github.com/akash-network/akash-api/go/node/types/v1beta3"

	"github.com/akash-network/node/app"
	"github.com/akash-network/node/testutil/network"
)

type InterceptState func(codec.Codec, string, json.RawMessage) json.RawMessage

type networkConfigOptions struct {
	interceptState InterceptState
}

type ConfigOption func(*networkConfigOptions)

// WithInterceptState set custom name of the log object
func WithInterceptState(val InterceptState) ConfigOption {
	return func(t *networkConfigOptions) {
		t.interceptState = val
	}
}

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

func ResourceUnits(_ testing.TB) types.Resources {
	return types.Resources{
		ID: 1,
		CPU: &types.CPU{
			Units: types.NewResourceValue(uint64(RandCPUUnits())),
		},
		Memory: &types.Memory{
			Quantity: types.NewResourceValue(RandMemoryQuantity()),
		},
		GPU: &types.GPU{
			Units: types.NewResourceValue(uint64(RandGPUUnits())),
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
func DefaultConfig(opts ...ConfigOption) network.Config {
	cfg := &networkConfigOptions{}
	for _, opt := range opts {
		opt(cfg)
	}

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

	if cfg.interceptState != nil {
		for k, v := range genesisState {
			res := cfg.interceptState(encCfg.Marshaler, k, v)
			if res != nil {
				genesisState[k] = res
			}
		}
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
		Denoms: []string{
			"ibc/12C6A0C374171B595A0A9E18B83FA09D295FB1F2D8C6DAA3AC28683471752D84",
		},
		MinGasPrices:    fmt.Sprintf("0.000006%s", CoinDenom),
		AccountTokens:   sdk.TokensFromConsensusPower(1000000000000, sdk.DefaultPowerReduction),
		StakingTokens:   sdk.TokensFromConsensusPower(100000, sdk.DefaultPowerReduction),
		BondedTokens:    sdk.TokensFromConsensusPower(100, sdk.DefaultPowerReduction),
		PruningStrategy: storetypes.PruningOptionNothing,
		CleanupDir:      true,
		SigningAlgo:     string(hd.Secp256k1Type),
		KeyringOptions:  []keyring.Option{},
	}
}

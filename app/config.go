package app

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cast"
	"github.com/spf13/cobra"

	"cosmossdk.io/x/evidence"
	feegrantmodule "cosmossdk.io/x/feegrant/module"
	"cosmossdk.io/x/upgrade"
	"github.com/CosmWasm/wasmd/x/wasm"
	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/auth/vesting"
	authzmodule "github.com/cosmos/cosmos-sdk/x/authz/module"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/cosmos-sdk/x/consensus"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/cosmos/cosmos-sdk/x/gov"
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/cosmos/cosmos-sdk/x/params"
	paramsclient "github.com/cosmos/cosmos-sdk/x/params/client"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	"github.com/cosmos/cosmos-sdk/x/staking"
	"github.com/cosmos/ibc-go/v10/modules/apps/transfer"
	ibc "github.com/cosmos/ibc-go/v10/modules/core"
	ibclightclient "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
)

var mbasics = module.NewBasicManager(
	append([]module.AppModuleBasic{
		// accounts, fees.
		auth.AppModuleBasic{},
		// authorizations
		authzmodule.AppModuleBasic{},
		// genesis utilities
		genutil.NewAppModuleBasic(genutiltypes.DefaultMessageValidator),
		// tokens, token balance.
		bank.AppModuleBasic{},
		// validator staking
		staking.AppModuleBasic{},
		// inflation
		mint.AppModuleBasic{},
		// distribution of fees and inflation
		distr.AppModuleBasic{},
		// governance functionality (voting)
		gov.NewAppModuleBasic(
			[]govclient.ProposalHandler{
				paramsclient.ProposalHandler,
			},
		),
		// chain parameters
		params.AppModuleBasic{},
		consensus.AppModuleBasic{},
		crisis.AppModuleBasic{},
		slashing.AppModuleBasic{},
		ibclightclient.AppModuleBasic{},
		ibc.AppModuleBasic{},
		upgrade.AppModuleBasic{},
		evidence.AppModuleBasic{},
		transfer.AppModuleBasic{},
		vesting.AppModuleBasic{},
		feegrantmodule.AppModuleBasic{},
		wasm.AppModuleBasic{},
	},
		// akash
		akashModuleBasics()...,
	)...,
)

// ModuleBasics returns all app modules basics
func ModuleBasics() module.BasicManager {
	return mbasics
}

// WasmLimitsKeyPrefix is the app.toml table and flag prefix for Wasm static validation limits.
const WasmLimitsKeyPrefix = "wasm.limits"

// WasmLimits mirrors wasmvmtypes.WasmLimits with concrete, chain-canonical values.
// The mapstructure tags are the single schema for app.toml keys, flag names and env vars.
//
// CONSENSUS-CRITICAL: limits are applied when Wasm code is stored during block execution;
// all validators must run identical values.
type WasmLimits struct {
	InitialMemoryLimitPages uint32 `mapstructure:"initial_memory_limit_pages"`
	TableSizeLimitElements  uint32 `mapstructure:"table_size_limit_elements"`
	MaxImports              uint32 `mapstructure:"max_imports"`
	MaxFunctions            uint32 `mapstructure:"max_functions"`
	MaxFunctionParams       uint32 `mapstructure:"max_function_params"`
	MaxTotalFunctionParams  uint32 `mapstructure:"max_total_function_params"`
	MaxFunctionResults      uint32 `mapstructure:"max_function_results"`
	MaxFunctionLocals       uint32 `mapstructure:"max_function_locals"`
	MaxTotalFunctionLocals  uint32 `mapstructure:"max_total_function_locals"`
}

// WasmLimitOverride describes a limit whose effective value differs from the chain default.
type WasmLimitOverride struct {
	Key     string
	Default uint32
	Value   uint32
}

type wasmLimitField struct {
	key string
	ptr *uint32
}

// DefaultWasmLimits returns the chain-canonical limits. Values equal cosmwasm-vm 3.0.10
// defaults, except MaxFunctionLocals which is raised from 100 to 140 so that the pyth_vaa
// contract (137 locals after wasm-opt) stored by the v2.2.0 upgrade passes validation.
func DefaultWasmLimits() WasmLimits {
	return WasmLimits{
		InitialMemoryLimitPages: 512,
		TableSizeLimitElements:  2500,
		MaxImports:              100,
		MaxFunctions:            20_000,
		MaxFunctionParams:       100,
		MaxTotalFunctionParams:  10_000,
		MaxFunctionResults:      1,
		MaxFunctionLocals:       140,
		MaxTotalFunctionLocals:  10_000,
	}
}

// ToVM converts the limits into the wasmvm representation with every field explicitly set,
// so libwasmvm defaults never apply implicitly.
func (l WasmLimits) ToVM() wasmvmtypes.WasmLimits {
	ptr := func(v uint32) *uint32 { return &v }

	return wasmvmtypes.WasmLimits{
		InitialMemoryLimitPages: ptr(l.InitialMemoryLimitPages),
		TableSizeLimitElements:  ptr(l.TableSizeLimitElements),
		MaxImports:              ptr(l.MaxImports),
		MaxFunctions:            ptr(l.MaxFunctions),
		MaxFunctionParams:       ptr(l.MaxFunctionParams),
		MaxTotalFunctionParams:  ptr(l.MaxTotalFunctionParams),
		MaxFunctionResults:      ptr(l.MaxFunctionResults),
		MaxFunctionLocals:       ptr(l.MaxFunctionLocals),
		MaxTotalFunctionLocals:  ptr(l.MaxTotalFunctionLocals),
	}
}

// Validate rejects zero limits; a zero limit rejects virtually every contract and
// almost always indicates a typo or an empty environment variable.
func (l WasmLimits) Validate() error {
	for _, f := range l.fields() {
		if *f.ptr == 0 {
			return fmt.Errorf("invalid wasm limits: %s must be > 0", f.key)
		}
	}

	return nil
}

// Overrides returns every limit whose value differs from DefaultWasmLimits.
func (l WasmLimits) Overrides() []WasmLimitOverride {
	defaults := DefaultWasmLimits()
	defaultFields := defaults.fields()

	var res []WasmLimitOverride
	for i, f := range l.fields() {
		if *f.ptr != *defaultFields[i].ptr {
			res = append(res, WasmLimitOverride{Key: f.key, Default: *defaultFields[i].ptr, Value: *f.ptr})
		}
	}

	return res
}

// ReadWasmLimits reads limits from app options (flags > env > app.toml), falling back to
// DefaultWasmLimits for every key that is not set.
func ReadWasmLimits(opts servertypes.AppOptions) (WasmLimits, error) {
	limits := DefaultWasmLimits()

	for _, f := range limits.fields() {
		key := WasmLimitsKeyPrefix + "." + f.key

		raw := opts.Get(key)
		if raw == nil {
			continue
		}

		val, err := cast.ToUint32E(raw)
		if err != nil {
			return limits, fmt.Errorf("%s: %w", key, err)
		}

		*f.ptr = val
	}

	return limits, nil
}

// AddWasmLimitsFlags registers one start flag per limit. Environment variables
// (AKASH_WASM_LIMITS_<KEY>) are bound automatically by the CLI config handler.
func AddWasmLimitsFlags(cmd *cobra.Command) {
	defaults := DefaultWasmLimits()

	for _, f := range defaults.fields() {
		cmd.Flags().Uint32(
			WasmLimitsKeyPrefix+"."+f.key,
			*f.ptr,
			fmt.Sprintf("Wasm static validation limit %q. CONSENSUS-CRITICAL: must be identical on all validators", f.key),
		)
	}
}

// WasmLimitsConfigTemplate returns the [wasm.limits] app.toml section. Keys are commented out
// so nodes always follow the defaults built into the binary unless explicitly overridden.
func WasmLimitsConfigTemplate() string {
	var sb strings.Builder

	sb.WriteString(`
[wasm.limits]
# Static validation limits applied when Wasm code is stored on chain.
# CONSENSUS-CRITICAL: every validator must use identical values.
# Leave keys commented out to use the chain defaults built into the binary.
`)

	defaults := DefaultWasmLimits()
	for _, f := range defaults.fields() {
		fmt.Fprintf(&sb, "# %s = %d\n", f.key, *f.ptr)
	}

	return sb.String()
}

// fields returns the limits in declaration order, keyed by their mapstructure tag.
func (l *WasmLimits) fields() []wasmLimitField {
	rv := reflect.ValueOf(l).Elem()
	rt := rv.Type()

	res := make([]wasmLimitField, rt.NumField())
	for i := range res {
		res[i] = wasmLimitField{
			key: rt.Field(i).Tag.Get("mapstructure"),
			ptr: rv.Field(i).Addr().Interface().(*uint32),
		}
	}

	return res
}

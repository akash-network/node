package cmd

import (
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
)

type AppConfig struct {
	serverconfig.Config

	WasmConfig wasmtypes.NodeConfig `mapstructure:"wasm"`
}

var AppTemplate = serverconfig.DefaultConfigTemplate + `
###############################################################################
###                            Wasm Configuration                           ###
###############################################################################
` + wasmtypes.DefaultConfigTemplate()

func InitAppConfig() (string, interface{}) {
	appCfg := AppConfig{
		Config:     *serverconfig.DefaultConfig(),
		WasmConfig: wasmtypes.DefaultNodeConfig(),
	}

	appCfg.MinGasPrices = "0.0025uakt"
	appCfg.API.Enable = true
	appCfg.API.Address = "tcp://localhost:1317"

	return AppTemplate, appCfg
}

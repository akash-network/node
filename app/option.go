package app

import (
	"github.com/cosmos/cosmos-sdk/codec"

	"pkg.akt.dev/go/sdkutil"
)

type SetupGenesisFn func(cdc codec.Codec) GenesisState

type setupAppOptions struct {
	encCfg    sdkutil.EncodingConfig
	home      string
	chainID   string
	checkTx   bool
	genesisFn SetupGenesisFn
}

type SetupAppOption func(*setupAppOptions)

// WithHome sets home dir for app
func WithHome(val string) SetupAppOption {
	return func(t *setupAppOptions) {
		t.home = val
	}
}

// WithChainID sets home dir for app
func WithChainID(val string) SetupAppOption {
	return func(t *setupAppOptions) {
		t.chainID = val
	}
}

// WithCheckTx sets home dir for app
func WithCheckTx(val bool) SetupAppOption {
	return func(t *setupAppOptions) {
		t.checkTx = val
	}
}

func WithGenesis(val SetupGenesisFn) SetupAppOption {
	return func(t *setupAppOptions) {
		t.genesisFn = val
	}
}

func WithEncConfig(val sdkutil.EncodingConfig) SetupAppOption {
	return func(t *setupAppOptions) {
		t.encCfg = val
	}
}

package network

import (
	"encoding/json"

	"github.com/cosmos/cosmos-sdk/codec"
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

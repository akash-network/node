package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
)

var cdc = codec.New()

func init() {
	RegisterCodec(cdc)
}

func RegisterCodec(cdc *codec.Codec) {
	cdc.RegisterConcrete(MsgCreate{}, "deployment/msg-create", nil)
	cdc.RegisterConcrete(MsgClose{}, "deployment/msg-close", nil)
	cdc.RegisterConcrete(MsgUpdate{}, "deployment/msg-update", nil)
}

func MustMarshalJSON(o interface{}) []byte {
	return cdc.MustMarshalJSON(o)
}

func UnmarshalJSON(bz []byte, ptr interface{}) error {
	return cdc.UnmarshalJSON(bz, ptr)
}

func MustUnmarshalJSON(bz []byte, ptr interface{}) {
	cdc.MustUnmarshalJSON(bz, ptr)
}

package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
)

var cdc = codec.New()

func init() {
	RegisterCodec(cdc)
}

func RegisterCodec(cdc *codec.Codec) {
	cdc.RegisterConcrete(MsgCloseOrder{}, ModuleName+"/msg-close-order", nil)
	cdc.RegisterConcrete(MsgCreateBid{}, ModuleName+"/msg-create-bid", nil)
	cdc.RegisterConcrete(MsgCloseBid{}, ModuleName+"/msg-close-bid", nil)
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

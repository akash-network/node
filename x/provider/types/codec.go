package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	amino = codec.New()

	// ModuleCdc references the global x/provider module codec. Note, the codec should
	// ONLY be used in certain instances of tests and for JSON encoding as Amino is
	// still used for that purpose.
	//
	// The actual codec used for serialization should be provided to x/provider and
	// defined at the application level.
	ModuleCdc = codec.NewHybridCodec(amino, cdctypes.NewInterfaceRegistry())
)

func init() {
	RegisterCodec(amino)
	cryptocodec.RegisterCrypto(amino)
	amino.Seal()
}

// RegisterCodec register concrete types on codec
func RegisterCodec(cdc *codec.Codec) {
	cdc.RegisterConcrete(&MsgCreateProvider{}, ModuleName+"/"+MsgTypeCreateProvider, nil)
	cdc.RegisterConcrete(&MsgUpdateProvider{}, ModuleName+"/"+MsgTypeUpdateProvider, nil)
	cdc.RegisterConcrete(&MsgDeleteProvider{}, ModuleName+"/"+MsgTypeDeleteProvider, nil)
}

// RegisterInterfaces registers the x/provider interfaces types with the interface registry
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateProvider{},
		&MsgUpdateProvider{},
		&MsgDeleteProvider{},
	)
}

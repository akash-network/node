package v1beta2

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

var (
	amino = codec.NewLegacyAmino()

	// ModuleCdc references the global x/deployment module codec. Note, the codec should
	// ONLY be used in certain instances of tests and for JSON encoding as Amino is
	// still used for that purpose.
	//
	// The actual codec used for serialization should be provided to x/deployment and
	// defined at the application level.
	ModuleCdc = codec.NewProtoCodec(cdctypes.NewInterfaceRegistry())
)

func init() {
	RegisterLegacyAminoCodec(amino)
	cryptocodec.RegisterCrypto(amino)
	amino.Seal()
}

// RegisterLegacyAminoCodec register concrete types on codec
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateDeployment{}, ModuleName+"/"+MsgTypeCreateDeployment, nil)
	cdc.RegisterConcrete(&MsgUpdateDeployment{}, ModuleName+"/"+MsgTypeUpdateDeployment, nil)
	cdc.RegisterConcrete(&MsgDepositDeployment{}, ModuleName+"/"+MsgTypeDepositDeployment, nil)
	cdc.RegisterConcrete(&MsgCloseDeployment{}, ModuleName+"/"+MsgTypeCloseDeployment, nil)
	cdc.RegisterConcrete(&MsgCloseGroup{}, ModuleName+"/"+MsgTypeCloseGroup, nil)
	cdc.RegisterConcrete(&MsgPauseGroup{}, ModuleName+"/"+MsgTypePauseGroup, nil)
	cdc.RegisterConcrete(&MsgStartGroup{}, ModuleName+"/"+MsgTypeStartGroup, nil)
}

// RegisterInterfaces registers the x/deployment interfaces types with the interface registry
func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateDeployment{},
		&MsgUpdateDeployment{},
		&MsgDepositDeployment{},
		&MsgCloseDeployment{},
		&MsgCloseGroup{},
		&MsgPauseGroup{},
		&MsgStartGroup{},
	)
	registry.RegisterImplementations(
		(*authz.Authorization)(nil),
		&DepositDeploymentAuthorization{},
	)

	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

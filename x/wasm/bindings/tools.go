package bindings

import (
	wasmvmtypes "github.com/CosmWasm/wasmvm/v3/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/gogoproto/proto"
)

// ConvertProtoToJSONMarshal unmarshals the given bytes into a proto message and then marshals it to json.
// This is done so that clients calling stargate queries do not need to define their own proto unmarshalers,
// being able to use response directly by json marshalling, which is supported in cosmwasm.
func ConvertProtoToJSONMarshal(protoResponseType proto.Message, bz []byte, cdc codec.Codec) ([]byte, error) {
	// unmarshal binary into stargate response data structure
	err := cdc.Unmarshal(bz, protoResponseType)
	if err != nil {
		return nil, wasmvmtypes.Unknown{}
	}

	bz, err = cdc.MarshalJSON(protoResponseType)
	if err != nil {
		return nil, wasmvmtypes.Unknown{}
	}

	protoResponseType.Reset()

	return bz, nil
}

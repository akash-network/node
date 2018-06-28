package grpc

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
	crypto "github.com/tendermint/go-crypto"
)

func Marshal(obj proto.Message) ([]byte, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	if err := marshaler.Marshal(&buf, obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func VerifySignature(request *types.GRPCRequest) (crypto.Address, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	// switch v := request.Payload.(type) {
	// case *types.GRPCRequest_ManifestRequest:
	// 	if err := marshaler.Marshal(&buf, v.ManifestRequest); err != nil {
	// 		return nil, err
	// 	}
	// default:
	// 	return nil, types.ErrInvalidPayload{Message: "invalid payload"}
	// }

	if err := marshaler.Marshal(&buf, request.ManifestRequest); err != nil {
		return nil, err
	}

	key, err := crypto.PubKeyFromBytes(request.Key)
	if err != nil {
		return nil, err
	}

	sig, err := crypto.SignatureFromBytes(request.Signature)
	if err != nil {
		return nil, err
	}

	if !key.VerifyBytes(buf.Bytes(), sig) {
		return nil, types.ErrInvalidSignature{"invalud signature"}
	}
	return key.Address(), err
}

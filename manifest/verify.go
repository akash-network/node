package manifest

import (
	"bytes"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/types"
	crypto "github.com/tendermint/go-crypto"
)

var ErrInvalidSignature = errors.New("Invalid signature")

func VerifyRequestSig(mr *types.ManifestRequest) error {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	baseReq := &types.ManifestRequest{
		Deployment: mr.Deployment,
		Manifest:   mr.Manifest,
	}
	if err := marshaler.Marshal(&buf, baseReq); err != nil {
		return err
	}

	key, err := crypto.PubKeyFromBytes(mr.Key)
	if err != nil {
		return err
	}

	sig, err := crypto.SignatureFromBytes(mr.Signature)
	if err != nil {
		return err
	}

	if !key.VerifyBytes(buf.Bytes(), sig) {
		return ErrInvalidSignature
	}

	return nil
}

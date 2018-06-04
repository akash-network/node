package manifest

import (
	"bytes"
	"context"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/types"
	crypto "github.com/tendermint/go-crypto"
)

var ErrInvalidSignature = errors.New("Invalid signature")
var ErrInvalidKey = errors.New("Key is not deployment owner")

func VerifyRequest(mr *types.ManifestRequest, session session.Session) error {
	address, err := verifySignature(mr)
	if err != nil {
		return err
	}
	if err := verifyDeploymentTennant(mr, session, address); err != nil {
		return err
	}
	return nil
}

func verifySignature(mr *types.ManifestRequest) (crypto.Address, error) {
	buf := bytes.Buffer{}
	marshaler := jsonpb.Marshaler{}
	baseReq := &types.ManifestRequest{
		Deployment: mr.Deployment,
		Manifest:   mr.Manifest,
	}
	if err := marshaler.Marshal(&buf, baseReq); err != nil {
		return nil, err
	}

	key, err := crypto.PubKeyFromBytes(mr.Key)
	if err != nil {
		return nil, err
	}

	sig, err := crypto.SignatureFromBytes(mr.Signature)
	if err != nil {
		return nil, err
	}

	if !key.VerifyBytes(buf.Bytes(), sig) {
		return nil, ErrInvalidSignature
	}
	return key.Address(), err
}

func verifyDeploymentTennant(mr *types.ManifestRequest, session session.Session, address crypto.Address) error {
	dep, err := session.Query().Deployment(context.TODO(), mr.Deployment)
	if err != nil {
		return err
	}
	if eq := bytes.Compare(dep.Tenant, address); eq != 0 {
		return ErrInvalidKey
	}
	return nil
}

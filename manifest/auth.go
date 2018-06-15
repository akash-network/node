package manifest

import (
	"bytes"
	"context"
	"errors"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/ovrclk/akash/provider/session"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
	crypto "github.com/tendermint/go-crypto"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidKey       = errors.New("key is not deployment owner")
)

func SignManifest(manifest *types.Manifest, signer txutil.Signer, deployment []byte) (*types.ManifestRequest, []byte, error) {
	mr := &types.ManifestRequest{
		Deployment: deployment,
		Manifest:   manifest,
	}
	buf, err := marshalRequest(mr)
	if err != nil {
		return nil, nil, err
	}
	sig, key, err := signer.SignBytes(buf)
	if err != nil {
		return nil, nil, err
	}
	mr.Signature = sig.Bytes()
	mr.Key = key.Bytes()
	buf, err = marshalRequest(mr)
	if err != nil {
		return nil, nil, err
	}
	return mr, buf, nil
}

func VerifyRequest(mr *types.ManifestRequest, session session.Session) error {
	address, err := verifySignature(mr)
	if err != nil {
		return err
	}
	if err := verifyDeploymentTennant(mr, session, address); err != nil {
		return err
	}
	if err := verifyManifestVersion(mr, session); err != nil {
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

func verifyDeploymentTennant(mr *types.ManifestRequest, session session.Session, signerAddress crypto.Address) error {
	dep, err := session.Query().Deployment(context.TODO(), mr.Deployment)
	if err != nil {
		return err
	}
	if !bytes.Equal(dep.Tenant, signerAddress) {
		return ErrInvalidKey
	}
	return nil
}

func verifyManifestVersion(mr *types.ManifestRequest, session session.Session) error {
	dep, err := session.Query().Deployment(context.TODO(), mr.Deployment)
	if err != nil {
		return err
	}
	err = verifyHash(mr.Manifest, dep.Version)
	if err != nil {
		return err
	}
	return nil
}

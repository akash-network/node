package manifest

import (
	"errors"
)

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidKey       = errors.New("key is not deployment owner")
)

// func SignManifest(manifest *types.Manifest, signer txutil.Signer, deployment []byte) (*types.ManifestRequest, []byte, error) {
// 	mr := &types.ManifestRequest{
// 		Deployment: deployment,
// 		Manifest:   manifest,
// 	}
// 	buf, err := marshalRequest(mr)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	sig, key, err := signer.SignBytes(buf)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	mr.Signature = sig
// 	mr.Key = key.Bytes()
// 	buf, err = marshalRequest(mr)
// 	if err != nil {
// 		return nil, nil, err
// 	}
// 	return mr, buf, nil
// }

// func VerifyRequest(mr *types.ManifestRequest, deployment *types.Deployment) error {
// 	address, err := verifySignature(mr)
// 	if err != nil {
// 		return err
// 	}
// 	if err := verifyDeploymentTenant(deployment, address); err != nil {
// 		return err
// 	}
// 	if err := verifyManifestVersion(mr, deployment); err != nil {
// 		return err
// 	}
// 	return nil
// }

// func verifySignature(mr *types.ManifestRequest) (crypto.Address, error) {
// 	buf := bytes.Buffer{}
// 	marshaler := jsonpb.Marshaler{}
// 	baseReq := &types.ManifestRequest{
// 		Deployment: mr.Deployment,
// 		Manifest:   mr.Manifest,
// 	}
// 	if err := marshaler.Marshal(&buf, baseReq); err != nil {
// 		return nil, err
// 	}

// 	key, err := camino.PubKeyFromBytes(mr.Key)
// 	if err != nil {
// 		return nil, err
// 	}

// 	sig := mr.Signature

// 	if !key.VerifyBytes(buf.Bytes(), sig) {
// 		return nil, ErrInvalidSignature
// 	}

// 	return key.Address(), nil
// }

// func verifyDeploymentTenant(
// 	deployment *types.Deployment,
// 	signerAddress crypto.Address) error {

// 	if !bytes.Equal(deployment.Tenant, signerAddress) {
// 		return ErrInvalidKey
// 	}

// 	return nil
// }

// func verifyManifestVersion(mr *types.ManifestRequest, deployment *types.Deployment) error {
// 	return verifyHash(mr.Manifest, deployment.Version)
// }

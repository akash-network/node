package manifest

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"github.com/gogo/protobuf/proto"
	"github.com/ovrclk/akash/types"
)

var ErrDifferentHashes = errors.New("manifest hash does not match the expected hash")

func Hash(m proto.Message) ([]byte, error) {
	bytes, err := marshal(m)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	_, err = h.Write(bytes)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), err
}

func verifyHash(m *types.Manifest, expectedHash []byte) error {
	hash, err := Hash(m)
	if err != nil {
		return err
	}
	if !bytes.Equal(hash, expectedHash) {
		return ErrDifferentHashes
	}
	return nil
}

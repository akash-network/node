package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

func DeploymentAddress(account []byte, nonce uint64) []byte {
	return NonceAddress(account, nonce)
}

func ProviderAddress(account []byte, nonce uint64) []byte {
	return NonceAddress(account, nonce)
}

func NonceAddress(account []byte, nonce uint64) []byte {
	buf := new(bytes.Buffer)
	buf.Write(account)
	binary.Write(buf, binary.BigEndian, nonce)
	address := sha256.Sum256(buf.Bytes())
	return address[:]
}

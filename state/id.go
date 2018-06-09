package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

const (
	AccountPath = "/accounts/"

	DeploymentPath         = "/deployments/"
	DeploymentSequencePath = "/deployments-seq/"

	DeploymentGroupPath = "/deployment-groups/"
	ProviderPath        = "/providers/"
	OrderPath           = "/orders/"
	FulfillmentPath     = "/fulfillment-orders/"
	LeasePath           = "/lease/"

	MaxRangeLimit = math.MaxInt64

	AddressSize = 32 // XXX: check
)

func MaxAddress() []byte {
	return bytes.Repeat([]byte{0xff}, AddressSize)
}

func MinAddress() []byte {
	return make([]byte, AddressSize)
}

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

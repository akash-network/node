package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/ovrclk/akash/types"
)

// {deployment-address}{group-sequence-id}
func DeploymentGroupID(daddr []byte, gseq uint64) []byte {
	buf := new(bytes.Buffer)
	buf.Write(daddr)
	binary.Write(buf, binary.BigEndian, gseq)
	return buf.Bytes()
}

// {deployment-address}{group-sequence-id}{order-sequence-id}
//
//
// AKA: DeploymentGroupID(daddr,gseq) + oseq
func OrderID(daddr []byte, gseq uint64, oseq uint64) []byte {
	buf := new(bytes.Buffer)
	buf.Write(daddr)
	binary.Write(buf, binary.BigEndian, gseq)
	binary.Write(buf, binary.BigEndian, oseq)
	return buf.Bytes()
}

// {deployment-address}{group-sequence-id}{order-sequence-id}{provider-address}
//
//
// AKA: OrderID(daddr,gseq,oseq) + provider-address
func FulfillmentID(daddr []byte, gseq uint64, oseq uint64, paddr []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Write(daddr)
	binary.Write(buf, binary.BigEndian, gseq)
	binary.Write(buf, binary.BigEndian, oseq)
	buf.Write(paddr)
	return buf.Bytes()
}

// {deployment-address}{group-sequence-id}{order-sequence-id}{provider-address}
//
//
// AKA: FulfillmentID(daddr,gseq,oseq)
func LeaseID(daddr []byte, gseq uint64, oseq uint64, paddr []byte) []byte {
	return FulfillmentID(daddr, gseq, oseq, paddr)
}

func IDForLease(obj *types.Lease) []byte {
	return LeaseID(obj.Deployment, obj.Group, obj.Order, obj.Provider)
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

package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"

	"github.com/ovrclk/akash/types"
)

// {deployment-address}{group-sequence-id}
func DeploymentGroupID(daddr []byte, gseq uint64) []byte {
	buf := make([]byte, len(daddr)+8)
	copy(buf, daddr)
	binary.BigEndian.PutUint64(buf[len(daddr):], gseq)
	return buf
}

// {deployment-address}{group-sequence-id}{order-sequence-id}
//
//
// AKA: DeploymentGroupID(daddr,gseq) + oseq
func OrderID(daddr []byte, gseq uint64, oseq uint64) []byte {
	buf := make([]byte, len(daddr)+8+8)
	copy(buf, daddr)
	binary.BigEndian.PutUint64(buf[len(daddr):], gseq)
	binary.BigEndian.PutUint64(buf[len(daddr)+8:], oseq)
	return buf
}

// {deployment-address}{group-sequence-id}{order-sequence-id}{provider-address}
//
//
// AKA: OrderID(daddr,gseq,oseq) + provider-address
func FulfillmentID(daddr []byte, gseq uint64, oseq uint64, paddr []byte) []byte {
	buf := make([]byte, len(daddr)+8+8+len(paddr))
	copy(buf, daddr)
	binary.BigEndian.PutUint64(buf[len(daddr):], gseq)
	binary.BigEndian.PutUint64(buf[len(daddr)+8:], oseq)
	copy(buf[len(daddr)+8+8:], paddr)
	return buf
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

// TODO: these addresses are susceptible to DoS attacks because they
//       are guessable and generated client side.
func NonceAddress(account []byte, nonce uint64) []byte {
	buf := new(bytes.Buffer)
	buf.Write(account)
	binary.Write(buf, binary.BigEndian, nonce)
	address := sha256.Sum256(buf.Bytes())
	return address[:]
}

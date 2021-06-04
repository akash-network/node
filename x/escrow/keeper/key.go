package keeper

import (
	"bytes"
	"reflect"
	"unsafe"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	"github.com/ovrclk/akash/x/escrow/types"
)

var (
	accountKeyPrefix = []byte{0x01}
	paymentKeyPrefix = []byte{0x02}
)

func accountKey(id types.AccountID) []byte {
	// TODO: validate scope, xid
	buf := bytes.Buffer{}
	buf.Write(accountKeyPrefix)
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	return buf.Bytes()
}

func accountPaymentsKey(id types.AccountID) []byte {
	// TODO: validate scope, xid, pid
	buf := bytes.Buffer{}
	buf.Write(paymentKeyPrefix)
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
	return buf.Bytes()
}

func paymentKey(id types.AccountID, pid string) []byte {
	// TODO: validate scope, xid, pid
	buf := bytes.Buffer{}
	buf.Write(paymentKeyPrefix)
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
	buf.WriteString(pid)
	return buf.Bytes()
}

// grantStoreKey - return authorization store key
// Items are stored with the following key: values
//
// - 0x01<granterAddressLen (1 Byte)><granterAddress_Bytes><granteeAddressLen (1 Byte)><granteeAddress_Bytes><msgType_Bytes>: Grant
func grantStoreKey(grantee sdk.AccAddress, granter sdk.AccAddress, msgType string) []byte {
	m := UnsafeStrToBytes(msgType)
	granter = address.MustLengthPrefix(granter)
	grantee = address.MustLengthPrefix(grantee)

	l := 1 + len(grantee) + len(granter) + len(m)
	var key = make([]byte, l)
	copy(key, authzkeeper.GrantKey)
	copy(key[1:], granter)
	copy(key[1+len(granter):], grantee)
	copy(key[l-len(m):], m)
	return key
}

// UnsafeStrToBytes uses unsafe to convert string into byte array. Returned bytes
// must not be altered after this function is called as it will cause a segmentation fault.
func UnsafeStrToBytes(s string) []byte {
	var buf []byte
	sHdr := (*reflect.StringHeader)(unsafe.Pointer(&s))
	bufHdr := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	bufHdr.Data = sHdr.Data
	bufHdr.Cap = sHdr.Len
	bufHdr.Len = sHdr.Len
	return buf
}

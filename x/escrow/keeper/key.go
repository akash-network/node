package keeper

import (
	"bytes"

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

package keeper

import (
	"bytes"

	"github.com/ovrclk/akash/x/escrow/types"
)

func accountKey(id types.AccountID) []byte {
	// TODO: validate scope, xid
	buf := bytes.Buffer{}
	buf.Write(types.AccountKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	return buf.Bytes()
}

func accountPaymentsKey(id types.AccountID) []byte {
	// TODO: validate scope, xid, pid
	buf := bytes.Buffer{}
	buf.Write(types.PaymentKeyPrefix())
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
	buf.Write(types.PaymentKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
	buf.WriteString(pid)
	return buf.Bytes()
}

package keeper

import (
	"bytes"

	"pkg.akt.dev/go/node/escrow/v1"
)

func accountKey(id v1.AccountID) []byte {
	// TODO: validate scope, xid
	buf := bytes.Buffer{}
	buf.Write(v1.AccountKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	return buf.Bytes()
}

func accountPaymentsKey(id v1.AccountID) []byte {
	// TODO: validate scope, xid, pid
	buf := bytes.Buffer{}
	buf.Write(v1.PaymentKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
	return buf.Bytes()
}

func paymentKey(id v1.AccountID, pid string) []byte {
	// TODO: validate scope, xid, pid
	buf := bytes.Buffer{}
	buf.Write(v1.PaymentKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
	buf.WriteString(pid)
	return buf.Bytes()
}

package keeper

import (
	"bytes"

	"pkg.akt.dev/go/node/escrow/v1"
)

func AccountKey(id v1.AccountID) []byte {
	// TODO: validate scope, xid
	buf := &bytes.Buffer{}

	buf.Write(v1.AccountKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)

	return buf.Bytes()
}

func keyWritePaymentPrefix(buf *bytes.Buffer, id v1.AccountID) {
	buf.Write(v1.PaymentKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
}

func AccountPaymentsKey(id v1.AccountID) []byte {
	// TODO: validate scope, xid, pid
	buf := &bytes.Buffer{}
	keyWritePaymentPrefix(buf, id)

	return buf.Bytes()
}

func PaymentKey(id v1.AccountID, pid string) []byte {
	// TODO: validate scope, xid, pid
	buf := &bytes.Buffer{}

	keyWritePaymentPrefix(buf, id)
	buf.WriteString(pid)

	return buf.Bytes()
}

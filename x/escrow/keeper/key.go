package keeper

import (
	"bytes"
	"fmt"

	"pkg.akt.dev/go/node/escrow/v1"
)

const (
	StateOpenPrefixID      = byte(0x01)
	StateClosedPrefixID    = byte(0x02)
	StateOverdrawnPrefixID = byte(0x03)
)

var (
	AccountPrefix        = []byte{0x11, 0x00}
	PaymentPrefix        = []byte{0x12, 0x00}
	StateOpenPrefix      = []byte{StateOpenPrefixID}
	StateClosedPrefix    = []byte{StateClosedPrefixID}
	StateOverdrawnPrefix = []byte{StateOverdrawnPrefixID}
)

func BuildAccountsKey(state v1.State, id *v1.AccountID) []byte {
	buf := &bytes.Buffer{}
	buf.Write(AccountPrefix)
	if state != v1.StateInvalid {
		buf.Write(stateToPrefix(state))

		if id != nil {
			writeAccountPrefix(buf, id)
		}
	}

	return buf.Bytes()
}

func BuildPaymentsKey(state v1.State, id *v1.AccountID, pid string) []byte {
	buf := &bytes.Buffer{}
	buf.Write(PaymentPrefix)
	if state != v1.StateInvalid {
		buf.Write(stateToPrefix(state))

		if id != nil {
			writeAccountPrefix(buf, id)

			if pid != "" {
				buf.WriteRune('/')
				buf.WriteString(pid)
			}
		}
	}

	return buf.Bytes()
}

func writeAccountPrefix(buf *bytes.Buffer, id *v1.AccountID) {
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
}

func stateToPrefix(state v1.State) []byte {
	switch state {
	case v1.StateOpen:
		return StateOpenPrefix
	case v1.StateClosed:
		return StateClosedPrefix
	case v1.StateOverdrawn:
		return StateOverdrawnPrefix
	default:
		panic(fmt.Sprintf("invalid state %d", state))
	}
}

func LegacyAccountKey(id v1.AccountID) []byte {
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

func LegacyPaymentKey(id v1.AccountID, pid string) []byte {
	// TODO: validate scope, xid, pid
	buf := &bytes.Buffer{}

	keyWritePaymentPrefix(buf, id)
	buf.WriteString(pid)

	return buf.Bytes()
}

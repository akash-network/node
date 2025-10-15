package keeper

import (
	"bytes"
	"fmt"
	"strings"

	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	etypes "pkg.akt.dev/go/node/escrow/types/v1"
	"pkg.akt.dev/go/node/escrow/v1beta3"
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

func BuildAccountsKey(state etypes.State, id escrowid.ID) []byte {
	buf := &bytes.Buffer{}
	buf.Write(AccountPrefix)
	writeKey(buf, state, id)

	return buf.Bytes()
}

func writeKey(buf *bytes.Buffer, state etypes.State, id escrowid.ID) {
	if state != etypes.StateInvalid {
		buf.Write(stateToPrefix(state))

		if id != nil {
			writeId(buf, id)
		}
	}
}

func writeId(buf *bytes.Buffer, id escrowid.ID) {
	buf.WriteRune('/')
	buf.WriteString(id.Key())
}

func BuildPaymentsKey(state etypes.State, id escrowid.ID) []byte {
	buf := &bytes.Buffer{}
	buf.Write(PaymentPrefix)
	writeKey(buf, state, id)

	return buf.Bytes()
}

func stateToPrefix(state etypes.State) []byte {
	switch state {
	case etypes.StateOpen:
		return StateOpenPrefix
	case etypes.StateClosed:
		return StateClosedPrefix
	case etypes.StateOverdrawn:
		return StateOverdrawnPrefix
	default:
		panic(fmt.Sprintf("invalid state %d", state))
	}
}

func ParseAccountKey(key []byte) (escrowid.Account, etypes.State) {
	if len(key) < len(AccountPrefix)+2 {
		panic("malformed account key")
	}

	if !bytes.HasPrefix(key, AccountPrefix) {
		panic("malformed account prefix")
	}

	key = key[len(AccountPrefix):]
	state := etypes.State(key[0])

	key = key[1:]

	if key[0] != '/' {
		panic("malformed account separator")
	}

	key = key[1:]

	parts := strings.Split(string(key), "/")
	if len(parts) < 3 {
		panic(fmt.Sprintf("malformed account key \"%s\"", string(key)))
	}

	scopeVal, valid := escrowid.Scope_value[parts[0]]
	if !valid {
		panic(fmt.Sprintf("invalid account scope \"%s\"", parts[0]))
	}

	parts = parts[1:]

	scope := escrowid.Scope(scopeVal)

	switch scope {
	case escrowid.ScopeDeployment:
		if len(parts) != 2 {
			panic(fmt.Sprintf("malformed account key \"%s\"", string(key)))
		}
	case escrowid.ScopeBid:
		if len(parts) != 5 {
			panic(fmt.Sprintf("malformed account key \"%s\"", string(key)))
		}
	}

	return escrowid.Account{
		Scope: scope,
		XID:   strings.Join(parts, "/"),
	}, state
}

func ParsePaymentKey(key []byte) (escrowid.Payment, etypes.State) {
	if len(key) < len(PaymentPrefix)+1 {
		panic("malformed payment key")
	}

	if !bytes.HasPrefix(key, PaymentPrefix) {
		panic("malformed payment prefix")
	}

	key = key[len(PaymentPrefix):]
	state := etypes.State(key[0])

	key = key[1:]

	if key[0] != '/' {
		panic("malformed payment separator")
	}

	key = key[1:]

	parts := strings.Split(string(key), "/")

	if len(parts) != 6 {
		panic(fmt.Sprintf("malformed payment key \"%s\"", string(key)))
	}

	scope, valid := escrowid.Scope_value[parts[0]]
	if !valid {
		panic(fmt.Sprintf("invalid payment scope \"%s\"", parts[0]))
	}

	return escrowid.Payment{
		AID: escrowid.Account{
			Scope: escrowid.Scope(scope),
			XID:   strings.Join(parts[1:3], "/"),
		},
		XID: strings.Join(parts[3:], "/"),
	}, state
}

func LegacyAccountKey(id v1beta3.AccountID) []byte {
	// TODO: validate scope, xid
	buf := &bytes.Buffer{}

	buf.Write(v1beta3.AccountKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)

	return buf.Bytes()
}

func keyWritePaymentPrefix(buf *bytes.Buffer, id v1beta3.AccountID) {
	buf.Write(v1beta3.PaymentKeyPrefix())
	buf.WriteRune('/')
	buf.WriteString(id.Scope)
	buf.WriteRune('/')
	buf.WriteString(id.XID)
	buf.WriteRune('/')
}

func LegacyPaymentKey(id v1beta3.AccountID, pid string) []byte {
	// TODO: validate scope, xid, pid
	buf := &bytes.Buffer{}

	keyWritePaymentPrefix(buf, id)
	buf.WriteString(pid)

	return buf.Bytes()
}

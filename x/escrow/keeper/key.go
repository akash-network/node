package keeper

import (
	"bytes"
	"fmt"

	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	emodule "pkg.akt.dev/go/node/escrow/module"
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
	BmeAccountsPrefix    = []byte{0x14, 0x01}
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

func ParseAccountKey(key []byte) (escrowid.Account, etypes.State, error) {
	if len(key) < len(AccountPrefix)+2 {
		return escrowid.Account{}, etypes.StateInvalid, emodule.ErrMalformedKey
	}

	if !bytes.HasPrefix(key, AccountPrefix) {
		return escrowid.Account{}, etypes.StateInvalid, emodule.ErrMalformedKey.Wrap("malformed prefix")
	}

	key = key[len(AccountPrefix):]
	state := etypes.State(key[0])

	key = key[1:]

	if key[0] != '/' {
		return escrowid.Account{}, etypes.StateInvalid, emodule.ErrMalformedKey.Wrap("malformed separator")
	}

	key = key[1:]

	acc, err := escrowid.ParseAccount(string(key))
	if err != nil {
		return escrowid.Account{}, etypes.StateInvalid, err
	}

	return acc, state, nil
}

func ParsePaymentKey(key []byte) (escrowid.Payment, etypes.State, error) {
	if len(key) < len(PaymentPrefix)+1 {
		return escrowid.Payment{}, etypes.StateInvalid, emodule.ErrMalformedKey
	}

	if !bytes.HasPrefix(key, PaymentPrefix) {
		return escrowid.Payment{}, etypes.StateInvalid, emodule.ErrMalformedKey.Wrap("malformed prefix")
	}

	key = key[len(PaymentPrefix):]
	state := etypes.State(key[0])

	key = key[1:]

	if key[0] != '/' {
		return escrowid.Payment{}, etypes.StateInvalid, emodule.ErrMalformedKey.Wrap("malformed separator")
	}

	key = key[1:]

	pmnt, err := escrowid.ParsePayment(string(key))
	if err != nil {
		return escrowid.Payment{}, etypes.StateInvalid, err
	}

	return pmnt, state, nil
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

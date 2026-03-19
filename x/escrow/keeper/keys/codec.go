package keys

import (
	"encoding/json"
	"strings"

	"cosmossdk.io/collections/codec"
	escrowid "pkg.akt.dev/go/node/escrow/id/v1"
	"pkg.akt.dev/go/node/escrow/module"
)

type EscrowAccountIDCodec struct{}
type EscrowPaymentIDCodec struct{}

var (
	EscrowAccountIDKey codec.KeyCodec[escrowid.Account] = EscrowAccountIDCodec{}
	EscrowPaymentIDKey codec.KeyCodec[escrowid.Payment] = EscrowPaymentIDCodec{}
)

func (d EscrowAccountIDCodec) Encode(buffer []byte, key escrowid.Account) (int, error) {
	res := copy(buffer, key.Key())

	return res, nil
}

func (d EscrowAccountIDCodec) Decode(buffer []byte) (int, escrowid.Account, error) {
	parts := strings.SplitN(string(buffer), "/", 1)

	if len(parts) < 2 {
		return 0, escrowid.Account{}, module.ErrMalformedKey.Wrap("malformed account key")
	}

	scope := parts[0]
	scopeVal, valid := escrowid.Scope_value[scope]
	if !valid {
		return 0, escrowid.Account{}, module.ErrMalformedKey.Wrapf("invalid account scope \"%s\"", scope)
	}

	var expectedParts int
	switch escrowid.Scope(scopeVal) {
	case escrowid.ScopeDeployment:
		expectedParts = 3
	case escrowid.ScopeBid:
		expectedParts = 6
	}

	parts = strings.SplitN(string(buffer), "/", expectedParts-1)

	if len(parts) != expectedParts {
		return 0, escrowid.Account{}, module.ErrMalformedKey.Wrapf("malformed account key for %s scope", scope)
	}

	decodedLen := len(parts) - 1
	for _, part := range parts {
		decodedLen += len(part)
	}

	return decodedLen, escrowid.Account{
		Scope: escrowid.Scope(scopeVal),
		XID:   strings.Join(parts, "/"),
	}, nil
}

func (d EscrowAccountIDCodec) Size(key escrowid.Account) int {
	return len(key.Key())
}

func (d EscrowAccountIDCodec) EncodeJSON(key escrowid.Account) ([]byte, error) {
	return json.Marshal(key.Key())
}

func (d EscrowAccountIDCodec) DecodeJSON(b []byte) (escrowid.Account, error) {
	var acc escrowid.Account
	err := json.Unmarshal(b, &acc)

	return acc, err
}

func (d EscrowAccountIDCodec) Stringify(key escrowid.Account) string {
	return key.Key()
}

func (d EscrowAccountIDCodec) KeyType() string {
	return "EscrowAccountID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d EscrowAccountIDCodec) EncodeNonTerminal(buffer []byte, key escrowid.Account) (int, error) {
	return d.Encode(buffer, key)
}

func (d EscrowAccountIDCodec) DecodeNonTerminal(buffer []byte) (int, escrowid.Account, error) {
	return d.Decode(buffer)
}

func (d EscrowAccountIDCodec) SizeNonTerminal(key escrowid.Account) int {
	return d.Size(key)
}

func (d EscrowPaymentIDCodec) Encode(buffer []byte, key escrowid.Payment) (int, error) {
	res := copy(buffer, key.Key())

	return res, nil
}

func (d EscrowPaymentIDCodec) Decode(buffer []byte) (int, escrowid.Payment, error) {
	parts := strings.SplitN(string(buffer), "/", 1)

	if len(parts) < 2 {
		return 0, escrowid.Payment{}, module.ErrMalformedKey.Wrap("malformed account key")
	}

	scopeStr := parts[0]

	scope, valid := escrowid.Scope_value[parts[0]]
	if !valid {
		return 0, escrowid.Payment{}, module.ErrMalformedKey.Wrapf("invalid payment scope \"%s\"", parts[0])
	}

	parts = strings.SplitN(string(buffer), "/", 5)

	if len(parts) != 6 {
		return 0, escrowid.Payment{}, module.ErrMalformedKey.Wrapf("malformed payment key for %s scope", scopeStr)
	}

	decodedLen := len(parts) - 1
	for _, part := range parts {
		decodedLen += len(part)
	}

	return decodedLen, escrowid.Payment{
		AID: escrowid.Account{
			Scope: escrowid.Scope(scope),
			XID:   strings.Join(parts[1:3], "/"),
		},
		XID: strings.Join(parts[3:], "/"),
	}, nil
}

func (d EscrowPaymentIDCodec) Size(key escrowid.Payment) int {
	return len(key.Key())
}

func (d EscrowPaymentIDCodec) EncodeJSON(key escrowid.Payment) ([]byte, error) {
	return json.Marshal(key.Key())
}

func (d EscrowPaymentIDCodec) DecodeJSON(b []byte) (escrowid.Payment, error) {
	var acc escrowid.Payment
	err := json.Unmarshal(b, &acc)

	return acc, err
}

func (d EscrowPaymentIDCodec) Stringify(key escrowid.Payment) string {
	return key.Key()
}

func (d EscrowPaymentIDCodec) KeyType() string {
	return "EscrowAccountID"
}

// NonTerminal variants - for use in composite keys
// Must use length-prefixing or fixed-size encoding

func (d EscrowPaymentIDCodec) EncodeNonTerminal(buffer []byte, key escrowid.Payment) (int, error) {
	return d.Encode(buffer, key)
}

func (d EscrowPaymentIDCodec) DecodeNonTerminal(buffer []byte) (int, escrowid.Payment, error) {
	return d.Decode(buffer)
}

func (d EscrowPaymentIDCodec) SizeNonTerminal(key escrowid.Payment) int {
	return d.Size(key)
}

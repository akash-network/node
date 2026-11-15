package native

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/mr-tron/base58"
	"google.golang.org/protobuf/encoding/protowire"

	sink "pkg.akt.dev/go/node/sink/v1"
	_ "pkg.akt.dev/go/sdkutil"
)

func ParseID(value string) (sink.ID, error) {
	id, err := sink.ParseID(value)
	if err != nil {
		return sink.ID{}, err
	}

	owner, err := sdk.AccAddressFromBech32(id.Owner)
	if err != nil {
		return sink.ID{}, err
	}
	if len(owner) == 0 || len(owner) > 255 || owner.String() != id.Owner || id.String() != value {
		return sink.ID{}, errors.New("noncanonical Sink ID")
	}

	return id, nil
}

func validCoin(coin sdk.Coin, positive bool) error {
	if coin.Denom != "uakt" || coin.Amount.IsNil() || coin.Amount.IsNegative() || (positive && (!coin.Amount.IsPositive() || !coin.Amount.IsUint64())) {
		return errors.New("invalid uakt Coin")
	}
	return nil
}

func validTransfer(value sink.TransferState) error {
	if _, err := ParseID(value.ID.String()); err != nil {
		return err
	}

	recipient, err := base58.Decode(value.Data.Dest)
	if err != nil || len(recipient) != 32 || base58.Encode(recipient) != value.Data.Dest {
		return errors.New("noncanonical recipient")
	}

	return validCoin(value.Data.Amount, true)
}

// Protobuf value fields erase presence. Inspect only those external boundaries
// whose presence or oneof shape mattered to the original generated TS decoder.
func fields(raw []byte) (map[protowire.Number][][]byte, error) {
	result := make(map[protowire.Number][][]byte)
	for len(raw) > 0 {
		number, kind, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		raw = raw[n:]

		size := protowire.ConsumeFieldValue(number, kind, raw)
		if size < 0 {
			return nil, protowire.ParseError(size)
		}

		if kind == protowire.BytesType {
			value, n := protowire.ConsumeBytes(raw)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			result[number] = append(result[number], value)
		} else {
			result[number] = append(result[number], nil)
		}
		raw = raw[size:]
	}

	return result, nil
}

func requiredBytes(values map[protowire.Number][][]byte, number protowire.Number) ([]byte, error) {
	v := values[number]
	if len(v) == 0 || v[len(v)-1] == nil {
		return nil, fmt.Errorf("missing protobuf field %d", number)
	}
	return v[len(v)-1], nil
}

func coinShape(raw []byte) error {
	value, err := fields(raw)
	if err != nil {
		return err
	}

	amount, err := requiredBytes(value, 2)
	if err != nil {
		return err
	}
	text := string(amount)
	if len(text) == 0 || (len(text) > 1 && text[0] == '0') || strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return errors.New("noncanonical coin amount")
	}
	return nil
}

func recordShape(raw []byte) error {
	value, err := fields(raw)
	if err != nil {
		return err
	}

	coin, err := requiredBytes(value, 2)
	if err != nil {
		return err
	}
	return coinShape(coin)
}

func transferShape(raw []byte) error {
	value, err := fields(raw)
	if err != nil {
		return err
	}

	for _, number := range []protowire.Number{1, 2, 3} {
		if _, err := requiredBytes(value, number); err != nil {
			return err
		}
	}
	record, _ := requiredBytes(value, 2)
	return recordShape(record)
}

func responseShape(method string, raw []byte) error {
	value, err := fields(raw)
	if err != nil {
		return err
	}

	switch method {
	case "/akash.sink.v1.Query/Transfer":
		transfer, err := requiredBytes(value, 1)
		if err != nil {
			return err
		}
		return transferShape(transfer)

	case "/akash.sink.v1.Query/Transfers", "/akash.sink.v1.Query/Disputes":
		if _, err := requiredBytes(value, 2); err != nil {
			return err
		}
		for _, transfer := range value[1] {
			if err := transferShape(transfer); err != nil {
				return err
			}
		}

	case "/akash.sink.v1.Query/Params":
		_, err := requiredBytes(value, 1)
		return err

	case "/akash.sink.v1.Query/State":
		for _, number := range []protowire.Number{1, 3, 4} {
			raw, err := requiredBytes(value, number)
			if err != nil {
				return err
			}
			if number == 1 {
				continue
			}

			usage, err := fields(raw)
			if err != nil {
				return err
			}
			coin, err := requiredBytes(usage, 2)
			if err != nil {
				return err
			}
			if err := coinShape(coin); err != nil {
				return err
			}
		}
	}

	return nil
}

func compareIDs(a, b sink.ID) int {
	if a.Height < b.Height {
		return -1
	}
	if a.Height > b.Height {
		return 1
	}

	if a.Sequence < b.Sequence {
		return -1
	}
	if a.Sequence > b.Sequence {
		return 1
	}

	x, _ := sdk.AccAddressFromBech32(a.Owner)
	y, _ := sdk.AccAddressFromBech32(b.Owner)
	if len(x) < len(y) {
		return -1
	}
	if len(x) > len(y) {
		return 1
	}
	return bytes.Compare(x, y)
}

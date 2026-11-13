// Package sinkid ports the transfer ID and record decoding rules of contracts/solana/shared.
package sinkid

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/types/bech32"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
)

const (
	ownerPrefix   = "akash"
	maxOwnerBytes = 255
)

var (
	errID     = errors.New("ID must be canonical owner/positive-int64-height/positive-uint64-sequence")
	component = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// ID identifies a sink transfer: the owner account, the creating block height and the sequence within it.
type ID struct {
	Owner      string
	OwnerBytes []byte
	Height     int64
	Sequence   uint64
}

// Parse decodes the canonical "owner/height/sequence" text form.
func Parse(s string) (ID, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 || !component.MatchString(parts[1]) || !component.MatchString(parts[2]) {
		return ID{}, errID
	}

	height, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ID{}, errID
	}

	sequence, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return ID{}, errID
	}

	raw, ok := ownerBytes(parts[0])
	if !ok {
		return ID{}, errID
	}

	return ID{Owner: parts[0], OwnerBytes: raw, Height: height, Sequence: sequence}, nil
}

// FromProto validates a chain ID.
func FromProto(id sinkv1.ID) (ID, error) {
	return Parse(fmt.Sprintf("%s/%d/%d", id.Owner, id.Height, id.Sequence))
}

// String returns the canonical "owner/height/sequence" text form.
func (id ID) String() string {
	return fmt.Sprintf("%s/%d/%d", id.Owner, id.Height, id.Sequence)
}

// Proto returns the chain representation of the ID.
func (id ID) Proto() sinkv1.ID {
	return sinkv1.ID{Owner: id.Owner, Height: id.Height, Sequence: id.Sequence}
}

// Compare orders IDs by height, then sequence, then owner length, then owner bytes.
func Compare(a, b ID) int {
	return cmp.Or(
		cmp.Compare(a.Height, b.Height),
		cmp.Compare(a.Sequence, b.Sequence),
		cmp.Compare(len(a.OwnerBytes), len(b.OwnerBytes)),
		bytes.Compare(a.OwnerBytes, b.OwnerBytes),
	)
}

// KeySuffix returns len(owner) || owner || BE64(height) || BE64(sequence).
func (id ID) KeySuffix() []byte {
	out := make([]byte, 0, 1+len(id.OwnerBytes)+16)
	out = append(out, byte(len(id.OwnerBytes)))
	out = append(out, id.OwnerBytes...)
	out = binary.BigEndian.AppendUint64(out, uint64(id.Height))

	return binary.BigEndian.AppendUint64(out, id.Sequence)
}

func ownerBytes(owner string) ([]byte, bool) {
	prefix, raw, err := bech32.DecodeAndConvert(owner)
	if err != nil || prefix != ownerPrefix || len(raw) == 0 || len(raw) > maxOwnerBytes {
		return nil, false
	}

	if again, err := bech32.ConvertAndEncode(prefix, raw); err != nil || again != owner {
		return nil, false
	}

	return raw, true
}

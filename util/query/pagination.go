package query

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"

	"pkg.akt.dev/node/util/validation"
)

var (
	ErrInvalidPaginationKey = fmt.Errorf("pagination: invalid key")
)

// DecodePaginationKey parses the pagination key and returns the states, prefix and key to be used by the FilteredPaginate
func DecodePaginationKey(key []byte) ([]byte, []byte, []byte, []byte, error) {
	if len(key) < 5 {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid key length", ErrInvalidPaginationKey)
	}

	expectedChecksum := binary.BigEndian.Uint32(key)

	key = key[4:]

	checksum := crc32.ChecksumIEEE(key)

	if expectedChecksum != checksum {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid checksum, 0x%08x != 0x%08x", ErrInvalidPaginationKey, expectedChecksum, checksum)
	}

	statesC := int(key[0])
	key = key[1:]

	if len(key) < statesC {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
	}

	states := make([]byte, 0, statesC)
	states = append(states, key[:statesC]...)

	key = key[len(states):]

	if len(key) < 1 {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
	}

	prefixLength := int(key[0])
	key = key[1:]
	if len(key) < prefixLength {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
	}

	prefix := key[:prefixLength]

	key = key[prefixLength:]

	if len(key) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
	}

	keyLength := int(key[0])
	key = key[1:]

	if len(key) < keyLength {
		return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
	}

	pkey := key[:keyLength]

	key = key[keyLength:]
	var unsolicited []byte

	if len(key) > 0 {
		keyLength = int(key[0])
		key = key[1:]

		if len(key) != keyLength {
			return nil, nil, nil, nil, fmt.Errorf("%w: invalid state length", ErrInvalidPaginationKey)
		}

		unsolicited = key
	}

	return states, prefix, pkey, unsolicited, nil
}

func EncodePaginationKey(states, prefix, key, unsolicited []byte) ([]byte, error) {
	if len(states) == 0 {
		return nil, fmt.Errorf("%w: states cannot be empty", ErrInvalidPaginationKey)
	}

	if len(prefix) == 0 {
		return nil, fmt.Errorf("%w: prefix cannot be empty", ErrInvalidPaginationKey)
	}

	if len(key) == 0 {
		return nil, fmt.Errorf("%w: key cannot be empty", ErrInvalidPaginationKey)
	}

	// 4 bytes for checksum
	// 1 byte for states count
	// len(states) bytes for states
	// 1 byte for prefix length
	// len(prefix) bytes for prefix
	// 1 byte for key length
	// len(key) bytes for key
	encLen := 4 + 1 + len(states) + 1 + len(prefix) + 1 + len(key)

	if len(unsolicited) > 0 {
		encLen += 1 + len(unsolicited)
	}

	buf := make([]byte, encLen)

	data := buf[4:]

	tmp := validation.MustEncodeWithLengthPrefix(states)
	copy(data, tmp)

	offset := len(tmp)
	tmp = validation.MustEncodeWithLengthPrefix(prefix)

	copy(data[offset:], tmp)
	offset += len(tmp)

	tmp = validation.MustEncodeWithLengthPrefix(key)
	copy(data[offset:], tmp)

	if len(unsolicited) > 0 {
		offset += len(tmp)
		tmp = validation.MustEncodeWithLengthPrefix(unsolicited)
		copy(data[offset:], tmp)
	}

	checksum := crc32.ChecksumIEEE(data)
	binary.BigEndian.PutUint32(buf, checksum)

	return buf, nil
}

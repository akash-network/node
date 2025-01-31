package validation

import (
	"fmt"
)

func KeyAtLeastLength(bz []byte, length int) error {
	if len(bz) < length {
		return fmt.Errorf("expected key of length at least %d, got %d", length, len(bz))
	}

	return nil
}

// AssertKeyAtLeastLength panics when store key length is less than the given length.
func AssertKeyAtLeastLength(bz []byte, length int) {
	err := KeyAtLeastLength(bz, length)
	if err != nil {
		panic(err)
	}
}

func KeyLength(bz []byte, length int) error {
	if len(bz) != length {
		return fmt.Errorf("unexpected key length; got: %d, expected: %d", len(bz), length)
	}

	return nil
}

// AssertKeyLength panics when store key length is not equal to the given length.
func AssertKeyLength(bz []byte, length int) {
	err := KeyLength(bz, length)
	if err != nil {
		panic(err)
	}
}

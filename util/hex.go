package util

import "encoding/hex"

// X encodes bytes to string
func X(val []byte) string {
	return hex.EncodeToString(val)
}

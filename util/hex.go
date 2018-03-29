package util

import "encoding/hex"

func X(val []byte) string {
	return hex.EncodeToString(val)
}

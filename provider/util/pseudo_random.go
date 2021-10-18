package util

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
)

func PseudoRandomUintFromAddr(addr string, cap uint) uint {
	if len(addr) == 0 {
		panic("address length cannot be zero")
	}
	h := sha256.New()
	_, err := io.WriteString(h, addr)
	if err != nil {
		panic(err)
	}
	buf := &bytes.Buffer{}
	_, err = buf.Write(h.Sum(nil))
	if err != nil {
		panic(err)
	}
	var result64 uint64
	err = binary.Read(buf, binary.BigEndian, &result64)
	if err != nil {
		panic(err)
	}
	result := uint(result64)
	result %= cap
	return result
}

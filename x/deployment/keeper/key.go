package keeper

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ovrclk/akash/x/deployment/types"
)

var (
	deploymentPrefix = []byte{0x01}
	groupPrefix      = []byte{0x02}
)

func deploymentKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(deploymentPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// groupKey provides prefixed key for a Group's marshalled data.
func groupKey(id types.GroupID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	if err := binary.Write(buf, binary.BigEndian, id.GSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func groupKeyToID(k []byte) types.GroupID {
	id := types.GroupID{}
	buf := bytes.NewReader(k)

	sizeOfLastTwo := binary.Size(id.DSeq) + binary.Size(id.GSeq)
	// Skip to the start of the last two fields
	_, err := buf.Seek(int64(len(k)-sizeOfLastTwo), 0)
	if err != nil {
		panic(err)
	}

	// Read DSeq
	if err = binary.Read(buf, binary.BigEndian, &id.DSeq); err != nil {
		panic(err)
	}

	// Read GSeq
	if err = binary.Read(buf, binary.BigEndian, &id.GSeq); err != nil {
		panic(err)
	}

	// Read bech32 address
	bech32AddrLength := len(k) - len(groupPrefix) - sizeOfLastTwo
	// Skip group prefix
	_, err = buf.Seek(int64(len(groupPrefix)), 0)
	if err != nil {
		panic(err)
	}

	addrRaw := make([]byte, bech32AddrLength)
	n, err := buf.Read(addrRaw)
	if err != nil {
		panic(err)
	}
	if n != bech32AddrLength {
		panic(fmt.Sprintf("Could not read %d bytes for address, read %d", addrRaw, n))
	}

	id.Owner = string(addrRaw)

	return id
}

// groupsKey provides default store Key for Group data.
func groupsKey(id types.DeploymentID) []byte {
	buf := bytes.NewBuffer(groupPrefix)
	buf.Write([]byte(id.Owner))
	if err := binary.Write(buf, binary.BigEndian, id.DSeq); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

package keeper

import (
	"bytes"
	"encoding/binary"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/address"
	types "pkg.akt.dev/go/node/provider/v1beta4"
)

var (
	providerRegistrationPrefix      = []byte{0x02}
	providerMaintenancePrefix       = []byte{0x03}
	providerMaintenanceOwnerPrefix  = []byte{0x04}
	providerActiveMaintenancePrefix = []byte{0x05}
	providerParamsKey               = []byte{0x06}
	providerNextMaintenanceIDKey    = []byte{0x07}
)

func ProviderKey(id sdk.Address) []byte {
	buf := bytes.NewBuffer(types.ProviderPrefix())
	buf.Write(address.MustLengthPrefix(id.Bytes()))

	return buf.Bytes()
}

func ProviderRegistrationKey(id sdk.Address) []byte {
	buf := bytes.NewBuffer(providerRegistrationPrefix)
	buf.Write(address.MustLengthPrefix(id.Bytes()))

	return buf.Bytes()
}

func ProviderMaintenanceKey(id uint64) []byte {
	buf := bytes.NewBuffer(providerMaintenancePrefix)
	_ = binary.Write(buf, binary.BigEndian, id)

	return buf.Bytes()
}

func ProviderMaintenanceOwnerPrefix(id sdk.Address) []byte {
	buf := bytes.NewBuffer(providerMaintenanceOwnerPrefix)
	buf.Write(address.MustLengthPrefix(id.Bytes()))

	return buf.Bytes()
}

func ProviderMaintenanceOwnerKey(id sdk.Address, maintenanceID uint64) []byte {
	buf := bytes.NewBuffer(ProviderMaintenanceOwnerPrefix(id))
	_ = binary.Write(buf, binary.BigEndian, maintenanceID)

	return buf.Bytes()
}

func ProviderActiveMaintenanceKey(id sdk.Address) []byte {
	buf := bytes.NewBuffer(providerActiveMaintenancePrefix)
	buf.Write(address.MustLengthPrefix(id.Bytes()))

	return buf.Bytes()
}

func uint64Bytes(val uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, val)
	return buf
}

func uint64FromBytes(buf []byte) uint64 {
	return binary.BigEndian.Uint64(buf)
}

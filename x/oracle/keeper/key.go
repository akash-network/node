package keeper

import (
	"bytes"
	"encoding/binary"
	"time"

	"cosmossdk.io/collections"
	"pkg.akt.dev/go/util/conv"
)

var (
	PricesPrefixLegacy     = collections.NewPrefix([]byte{0x11, 0x00})
	LatestPriceIDPrefix    = collections.NewPrefix([]byte{0x11, 0x01})
	AggregatedPricesPrefix = collections.NewPrefix([]byte{0x11, 0x02})
	PricesHealthPrefix     = collections.NewPrefix([]byte{0x11, 0x03})
	PricesPrefix           = collections.NewPrefix([]byte{0x11, 0x05})

	SourcesSeqPrefix = collections.NewPrefix([]byte{0x12, 0x00})
	PricesSeqPrefix  = collections.NewPrefix([]byte{0x12, 0x01})
	SourcesIDPrefix  = collections.NewPrefix([]byte{0x12, 0x02})

	ParamsKey = collections.NewPrefix(0x09) // key for oracle module params
)

func BuildPricePrefix(id uint32, denom string, ts time.Time) ([]byte, error) {
	buf := bytes.NewBuffer(PricesPrefix.Bytes())

	if id > 0 {
		val := make([]byte, 9)
		dataLen := binary.PutUvarint(val, uint64(id))
		buf.Write(val[:dataLen])

		if denom != "" {
			data := conv.UnsafeStrToBytes(denom)

			buf.WriteByte(byte(len(data)))
			buf.Write(data)

			if !ts.IsZero() {
				buf.Write(EncodeTimestamp(ts))
			}
		}
	}

	return buf.Bytes(), nil
}

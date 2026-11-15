package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"unicode/utf8"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func discriminator(namespace, name string) []byte {
	hash := sha256.Sum256([]byte(namespace + ":" + name))
	return hash[:8]
}

// A cursor advances through fields; account allocations may contain padding.
// Vector lengths are checked against available bytes before allocating.
type decoder struct {
	data []byte
	err  error
}

func accountDecoder(data []byte, name string) *decoder {
	d := &decoder{data: data}
	if !bytes.Equal(d.take(8), discriminator("account", name)) && d.err == nil {
		d.err = fmt.Errorf("invalid %s discriminator", name)
	}
	return d
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || n > len(d.data) {
		d.err = io.ErrUnexpectedEOF
		return nil
	}

	value := d.data[:n]
	d.data = d.data[n:]
	return value
}

func (d *decoder) u8() uint8 {
	b := d.take(1)
	if b == nil {
		return 0
	}

	return b[0]
}

func (d *decoder) u32() uint32 {
	b := d.take(4)
	if b == nil {
		return 0
	}

	return binary.LittleEndian.Uint32(b)
}

func (d *decoder) u64() uint64 {
	b := d.take(8)
	if b == nil {
		return 0
	}

	return binary.LittleEndian.Uint64(b)
}

func (d *decoder) boolean() bool {
	v := d.u8()
	if v > 1 && d.err == nil {
		d.err = fmt.Errorf("invalid Borsh boolean/option %d", v)
	}
	return v == 1
}

func (d *decoder) key() (v solana.PublicKey) {
	copy(v[:], d.take(32))
	return v
}

func (d *decoder) hash() (v [32]byte) {
	copy(v[:], d.take(32))
	return v
}

func (d *decoder) count(minSize int) int {
	n := d.u32()
	if uint64(n) > uint64(len(d.data)/minSize) && d.err == nil {
		d.err = io.ErrUnexpectedEOF
	}
	if d.err != nil {
		return 0
	}

	return int(n)
}

func (d *decoder) vector() []byte {
	return bytes.Clone(d.take(d.count(1)))
}

func (d *decoder) str() string {
	v := d.vector()
	if !utf8.Valid(v) && d.err == nil {
		d.err = fmt.Errorf("invalid UTF-8 Borsh string")
	}
	return string(v)
}

func (d *decoder) keys() []solana.PublicKey {
	v := make([]solana.PublicKey, d.count(32))
	for i := range v {
		v[i] = d.key()
	}
	return v
}

func (d *decoder) id() sink.ID {
	owner := d.vector()
	height := d.u64()
	sequence := d.u64()
	text, err := bech32.ConvertAndEncode("akash", owner)
	if d.err == nil {
		if err != nil {
			d.err = err
		} else if len(owner) == 0 || len(owner) > 255 || height == 0 || height > math.MaxInt64 || sequence == 0 {
			d.err = fmt.Errorf("invalid transfer identity")
		}
	}
	return sink.ID{
		Owner:    text,
		Height:   int64(height),
		Sequence: sequence,
	}
}

type encoder []byte

func instructionEncoder(name string) encoder {
	return encoder(discriminator("global", name))
}

func (e *encoder) u8(v uint8) { *e = append(*e, v) }

func (e *encoder) u32(v uint32) { *e = binary.LittleEndian.AppendUint32(*e, v) }

func (e *encoder) u64(v uint64) { *e = binary.LittleEndian.AppendUint64(*e, v) }

func (e *encoder) raw(v []byte) { *e = append(*e, v...) }

func (e *encoder) key(v solana.PublicKey) { e.raw(v[:]) }

func (e *encoder) hash(v [32]byte) { e.raw(v[:]) }

func (e *encoder) vector(v []byte) {
	e.u32(uint32(len(v)))
	e.raw(v)
}
func (e *encoder) str(v string) { e.vector([]byte(v)) }

func (e *encoder) boolean(v bool) {
	if v {
		e.u8(1)
	} else {
		e.u8(0)
	}
}

func ownerBytes(id sink.ID) ([]byte, error) {
	hrp, owner, err := bech32.DecodeAndConvert(id.Owner)
	if err != nil {
		return nil, fmt.Errorf("transfer owner: %w", err)
	}

	canonical, err := bech32.ConvertAndEncode("akash", owner)
	if err != nil {
		return nil, err
	}
	if hrp != "akash" || canonical != id.Owner || len(owner) == 0 || len(owner) > 255 || id.Height <= 0 || id.Sequence == 0 {
		return nil, fmt.Errorf("invalid transfer identity")
	}

	return owner, nil
}

func (e *encoder) id(id sink.ID) error {
	owner, err := ownerBytes(id)
	if err != nil {
		return err
	}

	e.vector(owner)
	e.u64(uint64(id.Height))
	e.u64(id.Sequence)
	return nil
}

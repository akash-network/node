package sinkid_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const errCanonical = "ID must be canonical owner/positive-int64-height/positive-uint64-sequence"

func owner(t *testing.T, prefix string, raw []byte) string {
	t.Helper()

	s, err := bech32.ConvertAndEncode(prefix, raw)
	require.NoError(t, err)

	return s
}

func ownerOf(t *testing.T, size int, fill byte) (string, []byte) {
	t.Helper()

	raw := bytes.Repeat([]byte{fill}, size)

	return owner(t, "akash", raw), raw
}

func TestParse(t *testing.T) {
	addr, raw := ownerOf(t, 20, 0x01)

	t.Run("valid", func(t *testing.T) {
		for _, tc := range []struct {
			text     string
			height   int64
			sequence uint64
		}{
			{addr + "/1/1", 1, 1},
			{fmt.Sprintf("%s/%d/%d", addr, int64(math.MaxInt64), uint64(math.MaxUint64)), math.MaxInt64, math.MaxUint64},
		} {
			id, err := sinkid.Parse(tc.text)
			require.NoError(t, err, tc.text)
			require.Equal(t, sinkid.ID{Owner: addr, OwnerBytes: raw, Height: tc.height, Sequence: tc.sequence}, id)
			require.Equal(t, tc.text, id.String())
		}
	})

	t.Run("invalid", func(t *testing.T) {
		upper := strings.ToUpper(addr)
		cosmos := owner(t, "cosmos", raw)
		empty := owner(t, "akash", nil)

		for name, text := range map[string]string{
			"leading zero height":   addr + "/01/1",
			"plus sign height":      addr + "/+1/1",
			"zero height":           addr + "/0/1",
			"zero sequence":         addr + "/1/0",
			"height over int64":     addr + "/9223372036854775808/1",
			"sequence over uint64":  addr + "/1/18446744073709551616",
			"negative height":       addr + "/-1/1",
			"uppercase owner":       upper + "/1/1",
			"cosmos prefix":         cosmos + "/1/1",
			"empty owner bytes":     empty + "/1/1",
			"256 byte owner":        owner(t, "akash", bytes.Repeat([]byte{0x01}, 256)) + "/1/1",
			"missing component":     addr + "/1",
			"extra component":       addr + "/1/1/1",
			"not bech32":            "not-an-address/1/1",
			"empty string":          "",
			"corrupted checksum":    addr[:len(addr)-1] + "q/1/1",
			"empty height":          addr + "//1",
			"whitespace in height":  addr + "/ 1/1",
			"leading zero sequence": addr + "/1/01",
		} {
			_, err := sinkid.Parse(text)
			require.EqualError(t, err, errCanonical, name)
		}
	})
}

func TestParseLongOwner(t *testing.T) {
	addr, raw := ownerOf(t, 255, 0xab)
	text := addr + "/42/7"

	id, err := sinkid.Parse(text)
	require.NoError(t, err)
	require.Equal(t, raw, id.OwnerBytes)
	require.Equal(t, text, id.String())
	require.Equal(t, sinkv1.ID{Owner: addr, Height: 42, Sequence: 7}, id.Proto())

	suffix := id.KeySuffix()
	require.Len(t, suffix, 1+255+16)
	require.Equal(t, byte(255), suffix[0])
	require.Equal(t, raw, suffix[1:256])

	back, err := sinkid.FromProto(id.Proto())
	require.NoError(t, err)
	require.Equal(t, id, back)
}

func TestFromProto(t *testing.T) {
	addr, raw := ownerOf(t, 20, 0x02)

	id, err := sinkid.FromProto(sinkv1.ID{Owner: addr, Height: 9, Sequence: 3})
	require.NoError(t, err)
	require.Equal(t, sinkid.ID{Owner: addr, OwnerBytes: raw, Height: 9, Sequence: 3}, id)

	for _, bad := range []sinkv1.ID{
		{Owner: addr, Height: 0, Sequence: 1},
		{Owner: addr, Height: -1, Sequence: 1},
		{Owner: addr, Height: 1, Sequence: 0},
		{Owner: "", Height: 1, Sequence: 1},
	} {
		_, err := sinkid.FromProto(bad)
		require.EqualError(t, err, errCanonical)
	}
}

func mustID(t *testing.T, size int, fill byte, height int64, sequence uint64) sinkid.ID {
	t.Helper()

	addr, _ := ownerOf(t, size, fill)
	id, err := sinkid.Parse(fmt.Sprintf("%s/%d/%d", addr, height, sequence))
	require.NoError(t, err)

	return id
}

func TestCompare(t *testing.T) {
	t.Run("height first", func(t *testing.T) {
		low := mustID(t, 20, 0xff, 1, 9)
		high := mustID(t, 20, 0x01, 2, 1)
		require.Equal(t, -1, sinkid.Compare(low, high))
		require.Equal(t, 1, sinkid.Compare(high, low))
	})

	t.Run("then sequence", func(t *testing.T) {
		low := mustID(t, 20, 0xff, 5, 1)
		high := mustID(t, 20, 0x01, 5, 2)
		require.Equal(t, -1, sinkid.Compare(low, high))
		require.Equal(t, 1, sinkid.Compare(high, low))
	})

	t.Run("then owner length", func(t *testing.T) {
		short := mustID(t, 20, 0xff, 5, 5)
		long := mustID(t, 32, 0x01, 5, 5)
		require.Equal(t, -1, sinkid.Compare(short, long))
		require.Equal(t, 1, sinkid.Compare(long, short))
	})

	t.Run("then owner bytes", func(t *testing.T) {
		low := mustID(t, 20, 0x01, 5, 5)
		high := mustID(t, 20, 0x02, 5, 5)
		require.Equal(t, -1, sinkid.Compare(low, high))
		require.Equal(t, 1, sinkid.Compare(high, low))
	})

	t.Run("equal", func(t *testing.T) {
		a := mustID(t, 20, 0x01, 5, 5)
		b := mustID(t, 20, 0x01, 5, 5)
		require.Zero(t, sinkid.Compare(a, b))
	})
}

func TestKeySuffix(t *testing.T) {
	id := mustID(t, 20, 0x01, 42, 7)

	want := []byte{0x14}
	want = append(want, bytes.Repeat([]byte{0x01}, 20)...)
	want = binary.BigEndian.AppendUint64(want, 42)
	want = binary.BigEndian.AppendUint64(want, 7)

	require.Equal(t, want, id.KeySuffix())
}

func TestProto(t *testing.T) {
	id := mustID(t, 20, 0x03, 11, 12)
	require.Equal(t, sinkv1.ID{Owner: id.Owner, Height: 11, Sequence: 12}, id.Proto())
}

package keeper

import (
	"fmt"
	"time"
)

const TimestampEncodedSize = 31

// EncodeTimestamp writes a time.Time as a 31-byte fixed-size string
// in the format YYYY.MM.DD.HH.MM.SS.MMM.UUU.NNN (UTC).
// This encoding preserves lexicographic ordering == chronological ordering.
func EncodeTimestamp(t time.Time) []byte {
	t = t.UTC()

	y, mo, d := t.Date()
	h, mi, s := t.Clock()
	ns := t.Nanosecond()

	ms := ns / 1_000_000
	us := (ns / 1_000) % 1000
	n := ns % 1000

	buf := make([]byte, TimestampEncodedSize)

	// YYYY.MM.DD.HH.MM.SS.MMM.UUU.NNN
	writeDigits4(buf[0:], y)
	buf[4] = '.'
	writeDigits2(buf[5:], int(mo))
	buf[7] = '.'
	writeDigits2(buf[8:], d)
	buf[10] = '.'
	writeDigits2(buf[11:], h)
	buf[13] = '.'
	writeDigits2(buf[14:], mi)
	buf[16] = '.'
	writeDigits2(buf[17:], s)
	buf[19] = '.'
	writeDigits3(buf[20:], ms)
	buf[23] = '.'
	writeDigits3(buf[24:], us)
	buf[27] = '.'
	writeDigits3(buf[28:], n)

	return buf
}

// DecodeTimestamp parses a 31-byte timestamp buffer back into time.Time.
func DecodeTimestamp(buf []byte) (time.Time, error) {
	if len(buf) < TimestampEncodedSize {
		return time.Time{}, fmt.Errorf("timestamp buffer too short: %d < %d", len(buf), TimestampEncodedSize)
	}

	y := readDigits(buf[0:4])
	mo := readDigits(buf[5:7])
	d := readDigits(buf[8:10])
	h := readDigits(buf[11:13])
	mi := readDigits(buf[14:16])
	s := readDigits(buf[17:19])
	ms := readDigits(buf[20:23])
	us := readDigits(buf[24:27])
	ns := readDigits(buf[28:31])

	totalNs := ms*1_000_000 + us*1_000 + ns

	return time.Date(y, time.Month(mo), d, h, mi, s, totalNs, time.UTC), nil
}

func writeDigits4(buf []byte, v int) {
	buf[0] = byte('0' + v/1000%10)
	buf[1] = byte('0' + v/100%10)
	buf[2] = byte('0' + v/10%10)
	buf[3] = byte('0' + v%10)
}

func writeDigits3(buf []byte, v int) {
	buf[0] = byte('0' + v/100%10)
	buf[1] = byte('0' + v/10%10)
	buf[2] = byte('0' + v%10)
}

func writeDigits2(buf []byte, v int) {
	buf[0] = byte('0' + v/10%10)
	buf[1] = byte('0' + v%10)
}

func readDigits(buf []byte) int {
	n := 0
	for _, b := range buf {
		n = n*10 + int(b-'0')
	}
	return n
}

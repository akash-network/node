// Package tx builds, simulates and prices the reporter's unordered transactions and derives their nonce slots
// (spec §6.1).
package tx

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"pkg.akt.dev/node/v2/operator/reporter/policy"
	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// Nonce scheme v1 slot counts. These are protocol values, not configuration.
const (
	VoteSlots   = 128
	SingleSlots = 1024
)

const nonceLabel = "akt-reporter/nonce/v1"

type timingValues struct {
	window     time.Duration
	spacing    time.Duration
	cutoffBase time.Duration
	flushLead  time.Duration
}

// timing holds the nonce scheme's durations. Only tests in this package change it, through scaleTiming.
var timing = timingValues{
	window:     2 * time.Minute,
	spacing:    time.Millisecond,
	cutoffBase: 20 * time.Second,
	flushLead:  10 * time.Second,
}

// SlotOf returns the nonce slot of a message: 0 to 127 for a vote, 128 to 1151 for a veto or a refund.
func SlotOf(id sinkid.ID, k policy.Key) int {
	buf := make([]byte, 0, len(nonceLabel)+1+1+len(id.OwnerBytes)+16+4+16)
	buf = append(buf, nonceLabel...)
	buf = append(buf, byte(k.Kind))
	buf = append(buf, id.KeySuffix()...)

	if k.Kind == policy.MsgVote {
		buf = binary.BigEndian.AppendUint32(buf, uint32(k.Outcome)) //nolint:gosec // the outcome enum is hashed as its 4-byte two's complement
		buf = binary.BigEndian.AppendUint64(buf, k.Round)
		buf = binary.BigEndian.AppendUint64(buf, k.PolicyVersion)
	}

	sum := sha256.Sum256(buf)
	h := binary.BigEndian.Uint64(sum[:8])

	if k.Kind == policy.MsgVote {
		return int(h % VoteSlots) //nolint:gosec // h % VoteSlots is below 128
	}

	return VoteSlots + int(h%SingleSlots) //nolint:gosec // h % SingleSlots is below 1024
}

// WindowOf returns the window that contains blockTime: ⌊t / W⌋ with t in Unix nanoseconds.
func WindowOf(blockTime time.Time) int64 {
	t := blockTime.UnixNano()
	w := int64(timing.window)

	q := t / w
	if t%w < 0 {
		q--
	}

	return q
}

// WindowStart returns the first instant of window w, in UTC.
func WindowStart(w int64) time.Time {
	return time.Unix(0, w*int64(timing.window)).UTC()
}

// SlotTimeout returns T(w, s) = (w + 1)·W − (s + 1)·spacing, in UTC.
func SlotTimeout(w int64, slot int) time.Time {
	return WindowStart(w + 1).Add(-time.Duration(slot+1) * timing.spacing)
}

// SendCutoff returns the least time a slot's timeout must lie ahead of the latest block time for its transaction
// to be signed: the cutoff base plus the signer timeout.
func SendCutoff(signerTimeout time.Duration) time.Duration {
	return timing.cutoffBase + signerTimeout
}

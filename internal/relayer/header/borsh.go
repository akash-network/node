package header

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"sort"
	"time"

	cmttypes "github.com/cometbft/cometbft/types"
)

type borshWriter []byte

func (w *borshWriter) u8(v byte) { *w = append(*w, v) }

func (w *borshWriter) u16(v uint16) { *w = binary.LittleEndian.AppendUint16(*w, v) }

func (w *borshWriter) u32(v uint32) { *w = binary.LittleEndian.AppendUint32(*w, v) }

func (w *borshWriter) u64(v uint64) { *w = binary.LittleEndian.AppendUint64(*w, v) }

func (w *borshWriter) raw(v []byte) { *w = append(*w, v...) }

func (w *borshWriter) vector(v []byte) {
	w.u32(uint32(len(v)))
	w.raw(v)
}

func (w *borshWriter) optional(v []byte) {
	if len(v) == 0 {
		w.u8(0)
		return
	}

	w.u8(1)
	w.vector(v)
}

func (w *borshWriter) time(t time.Time) {
	w.u64(uint64(t.Unix()))

	// Rust's signed remainder differs from Time.Nanosecond before the epoch.
	nanos := new(big.Int).Rem(timestamp(t), big.NewInt(1_000_000_000)).Int64()
	w.u32(uint32(int32(nanos)))
}

func (w *borshWriter) blockID(id cmttypes.BlockID) {
	w.vector(id.Hash)
	w.u32(id.PartSetHeader.Total)
	w.vector(id.PartSetHeader.Hash)
}

func (w *borshWriter) block(h *cmttypes.Header) {
	w.u64(h.Version.Block)
	w.u64(h.Version.App)
	w.vector([]byte(h.ChainID))
	w.u64(uint64(h.Height))
	w.time(h.Time)

	if h.LastBlockID.IsZero() {
		w.u8(0)
	} else {
		w.u8(1)
		w.blockID(h.LastBlockID)
	}

	w.optional(h.LastCommitHash)
	w.optional(h.DataHash)
	w.vector(h.ValidatorsHash)
	w.vector(h.NextValidatorsHash)
	w.vector(h.ConsensusHash)
	w.vector(h.AppHash)
	w.optional(h.LastResultsHash)
	w.optional(h.EvidenceHash)
	w.vector(h.ProposerAddress)
}

func (w *borshWriter) commit(c *cmttypes.Commit) {
	w.u64(uint64(c.Height))
	w.u16(uint16(c.Round))
	w.blockID(c.BlockID)

	signatures := append([]cmttypes.CommitSig(nil), c.Signatures...)
	sort.Slice(signatures, func(i, j int) bool {
		a, b := signatures[i], signatures[j]
		if a.BlockIDFlag == cmttypes.BlockIDFlagAbsent {
			return b.BlockIDFlag != cmttypes.BlockIDFlagAbsent
		}
		if b.BlockIDFlag == cmttypes.BlockIDFlagAbsent {
			return false
		}
		return bytes.Compare(a.ValidatorAddress, b.ValidatorAddress) < 0
	})

	w.u32(uint32(len(signatures)))
	for _, sig := range signatures {
		w.u8(byte(sig.BlockIDFlag) - 1)
		if sig.BlockIDFlag == cmttypes.BlockIDFlagAbsent {
			continue
		}

		w.raw(sig.ValidatorAddress)
		w.time(sig.Timestamp)
		var signature [64]byte
		copy(signature[:], sig.Signature)
		w.raw(signature[:])
	}
}

func (w *borshWriter) validators(values []*cmttypes.Validator, total uint64) {
	w.u32(uint32(len(values)))
	for _, v := range values {
		w.raw(v.Address)
		w.u8(0)
		w.raw(v.PubKey.Bytes())
		w.u64(uint64(v.VotingPower))
		w.u64(0)
	}

	w.u8(0)
	w.u64(total)
}

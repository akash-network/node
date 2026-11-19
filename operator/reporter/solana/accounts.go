// Package solana decodes the accounts and events of the AKT Solana program and attributes program logs.
package solana

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/gagliardetto/solana-go"
)

const (
	discriminatorSize = 8
	maxOwnerBytes     = 255

	outcomeMinted = 0
	outcomeVoided = 1
)

// Anchor discriminators, checked against contracts/solana/tests/fixtures/deployed-interface.json.
var (
	DiscLaunchConfig     = [8]byte{18, 161, 9, 224, 102, 145, 29, 94}
	DiscBridgeConfig     = [8]byte{40, 206, 51, 233, 246, 40, 178, 85}
	DiscTransferStatus   = [8]byte{32, 200, 30, 253, 33, 153, 127, 45}
	DiscTransferResolved = [8]byte{205, 27, 228, 215, 90, 3, 236, 176}
)

var errShort = errors.New("data too short")

// VoidReason says why a record was voided.
type VoidReason uint8

const (
	// VoidInvalidRecipient means the proven recipient cannot receive AKT.
	VoidInvalidRecipient VoidReason = iota
	// VoidGovernance means Akash governance wrote a void order for the record.
	VoidGovernance
)

// LaunchConfig is the account at ["config"].
type LaunchConfig struct {
	Version           uint8
	Initializer       solana.PublicKey
	Mint              solana.PublicKey
	BridgeEnabled     bool
	InitializedAtSlot uint64
	Bump              uint8
	MintAuthorityBump uint8
}

// BridgeConfig is the account at ["bridge"]. The mint policy and usage counters are validated but not kept.
type BridgeConfig struct {
	LightClientProgram solana.PublicKey
	ChainID            string
}

// TransferID is the x/sink record ID as the program stores it.
type TransferID struct {
	Owner    []byte
	Height   uint64
	Sequence uint64
}

// Minted is an outcome that minted Amount base units to the associated token account of Recipient.
type Minted struct {
	Amount    uint64
	Recipient solana.PublicKey
}

// Outcome is the Borsh enum TransferOutcome; exactly one field is set.
type Outcome struct {
	Minted *Minted
	Voided *VoidReason
}

// TransferStatus is the terminal record at ["transfer", SHA256(owner), LE64(height), LE64(sequence)].
type TransferStatus struct {
	ID             TransferID
	ProofHeight    uint64
	ResolvedAtSlot uint64
	Outcome        Outcome
}

// reader consumes Borsh data and keeps the first error.
type reader struct {
	data []byte
	err  error
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}

	if n < 0 || n > len(r.data) {
		r.err = errShort

		return nil
	}

	out := r.data[:n]
	r.data = r.data[n:]

	return out
}

func (r *reader) u8() uint8 {
	if b := r.take(1); b != nil {
		return b[0]
	}

	return 0
}

func (r *reader) u32() uint32 {
	if b := r.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}

	return 0
}

func (r *reader) u64() uint64 {
	if b := r.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}

	return 0
}

func (r *reader) key() solana.PublicKey {
	var k solana.PublicKey
	copy(k[:], r.take(solana.PublicKeyLength))

	return k
}

func (r *reader) bool() bool {
	switch v := r.u8(); v {
	case 0:
		return false
	case 1:
		return r.err == nil
	default:
		r.fail(fmt.Errorf("invalid bool %d", v))

		return false
	}
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// bytes reads a Borsh Vec<u8> whose length must lie within [minLen, maxLen].
func (r *reader) bytes(minLen, maxLen int) []byte {
	n := r.u32()
	if r.err != nil {
		return nil
	}

	if int64(n) < int64(minLen) || int64(n) > int64(maxLen) {
		r.fail(fmt.Errorf("byte vector length %d outside %d..%d", n, minLen, maxLen))

		return nil
	}

	return append([]byte(nil), r.take(int(n))...)
}

func (r *reader) string() string {
	n := r.u32()
	b := r.take(int(min(int64(n), int64(len(r.data)+1))))

	if r.err == nil && !utf8.Valid(b) {
		r.fail(errors.New("string is not valid UTF-8"))
	}

	return string(b)
}

// discriminator consumes and checks the 8-byte Anchor discriminator.
func (r *reader) discriminator(want [8]byte) {
	got := r.take(discriminatorSize)
	if r.err == nil && [8]byte(got) != want {
		r.fail(fmt.Errorf("discriminator %x, want %x", got, want))
	}
}

func (r *reader) transferID() TransferID {
	return TransferID{Owner: r.bytes(1, maxOwnerBytes), Height: r.u64(), Sequence: r.u64()}
}

func (r *reader) outcome() Outcome {
	switch tag := r.u8(); tag {
	case outcomeMinted:
		return Outcome{Minted: &Minted{Amount: r.u64(), Recipient: r.key()}}
	case outcomeVoided:
		switch reason := VoidReason(r.u8()); reason {
		case VoidInvalidRecipient, VoidGovernance:
			return Outcome{Voided: &reason}
		default:
			r.fail(fmt.Errorf("invalid void reason %d", reason))
		}
	default:
		r.fail(fmt.Errorf("invalid outcome tag %d", tag))
	}

	return Outcome{}
}

// transferStatus reads the fields shared by the TransferStatus account and the TransferResolved event.
func (r *reader) transferStatus() TransferStatus {
	return TransferStatus{
		ID:             r.transferID(),
		ProofHeight:    r.u64(),
		ResolvedAtSlot: r.u64(),
		Outcome:        r.outcome(),
	}
}

// DecodeLaunchConfig decodes a LaunchConfig account. Trailing bytes are account padding.
func DecodeLaunchConfig(data []byte) (LaunchConfig, error) {
	r := reader{data: data}
	r.discriminator(DiscLaunchConfig)

	cfg := LaunchConfig{
		Version:           r.u8(),
		Initializer:       r.key(),
		Mint:              r.key(),
		BridgeEnabled:     r.bool(),
		InitializedAtSlot: r.u64(),
		Bump:              r.u8(),
		MintAuthorityBump: r.u8(),
	}
	if r.err != nil {
		return LaunchConfig{}, fmt.Errorf("decode LaunchConfig: %w", r.err)
	}

	return cfg, nil
}

// DecodeBridgeConfig decodes a BridgeConfig account. Trailing bytes are account padding.
func DecodeBridgeConfig(data []byte) (BridgeConfig, error) {
	r := reader{data: data}
	r.discriminator(DiscBridgeConfig)

	cfg := BridgeConfig{LightClientProgram: r.key(), ChainID: r.string()}

	// MintPolicy{per_transfer_max, daily_mint_cap, vaults}, mint_day, minted_today.
	r.u64()
	r.u64()

	vaults := r.u32()
	r.take(int(min(int64(vaults)*solana.PublicKeyLength, int64(len(r.data)+1))))
	r.u64()
	r.u64()

	if r.err != nil {
		return BridgeConfig{}, fmt.Errorf("decode BridgeConfig: %w", r.err)
	}

	return cfg, nil
}

// DecodeTransferStatus decodes a TransferStatus account. Trailing bytes are account padding.
func DecodeTransferStatus(data []byte) (TransferStatus, error) {
	r := reader{data: data}
	r.discriminator(DiscTransferStatus)

	status := r.transferStatus()
	if r.err != nil {
		return TransferStatus{}, fmt.Errorf("decode TransferStatus: %w", r.err)
	}

	return status, nil
}

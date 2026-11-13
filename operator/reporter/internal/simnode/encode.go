package simnode

import (
	"encoding/base64"
	"encoding/binary"

	"github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

// These encoders are a test oracle, written from contracts/solana/programs/akt/src/state.rs and events.rs and the
// SPL Token Mint layout. They never call the production decoders, so tests can prove those decode what is here.

// Allocated account sizes, discriminator included (state.rs account_sizes_match_the_native_address_layout).
const (
	launchConfigSpace   = 84
	bridgeConfigSpace   = 656
	transferStatusSpace = 340
	mintSpace           = 82
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

// BridgeConfig is the account at ["bridge"], MintPolicy flattened into it.
type BridgeConfig struct {
	LightClientProgram solana.PublicKey
	ChainID            string
	PerTransferMax     uint64
	DailyMintCap       uint64
	Vaults             []solana.PublicKey
	MintDay            uint64
	MintedToday        uint64
}

// Mint is an SPL Token mint; a nil authority is COption::None.
type Mint struct {
	MintAuthority   *solana.PublicKey
	Supply          uint64
	Decimals        uint8
	Initialized     bool
	FreezeAuthority *solana.PublicKey
}

// VoidReason is the Borsh enum VoidReason.
type VoidReason uint8

// Void reasons in declaration order.
const (
	VoidInvalidRecipient VoidReason = iota
	VoidGovernance
)

// Minted is TransferOutcome::Minted.
type Minted struct {
	Amount    uint64
	Recipient solana.PublicKey
}

// Outcome is the Borsh enum TransferOutcome. Set exactly one field; with neither, the encoding carries an invalid
// variant tag, so that a scenario can serve a corrupt outcome.
type Outcome struct {
	Minted *Minted
	Voided *VoidReason
}

// TransferStatus is the account at ["transfer", SHA256(owner), LE64(height), LE64(sequence)], and the payload of
// the TransferResolved event.
type TransferStatus struct {
	ID             sinkid.ID
	ProofHeight    uint64
	ResolvedAtSlot uint64
	Outcome        Outcome
}

// invalidOutcomeTag marks an Outcome with no variant set.
const invalidOutcomeTag = 0xff

type borsh []byte

// anchor starts account or event data with its discriminator.
func anchor(disc [8]byte) borsh { return append(borsh(nil), disc[:]...) }

func (b borsh) u8(v uint8) borsh { return append(b, v) }

func (b borsh) u32(v uint32) borsh { return binary.LittleEndian.AppendUint32(b, v) }

func (b borsh) u64(v uint64) borsh { return binary.LittleEndian.AppendUint64(b, v) }

func (b borsh) key(k solana.PublicKey) borsh { return append(b, k[:]...) }

func (b borsh) bool(v bool) borsh {
	if v {
		return b.u8(1)
	}

	return b.u8(0)
}

func (b borsh) bytes(v []byte) borsh { return append(b.u32(uint32(len(v))), v...) } //nolint:gosec // test data

func (b borsh) coption(k *solana.PublicKey) borsh {
	if k == nil {
		return b.u32(0).key(solana.PublicKey{})
	}

	return b.u32(1).key(*k)
}

// pad zero-fills data to the allocated account size; an oversized encoding is kept whole.
func pad(data borsh, space int) []byte {
	if len(data) < space {
		data = append(data, make([]byte, space-len(data))...)
	}

	return data
}

// EncodeLaunchConfig returns LaunchConfig account data, padded to its allocated size.
func EncodeLaunchConfig(c LaunchConfig) []byte {
	b := anchor(aktsolana.DiscLaunchConfig).
		u8(c.Version).key(c.Initializer).key(c.Mint).bool(c.BridgeEnabled).
		u64(c.InitializedAtSlot).u8(c.Bump).u8(c.MintAuthorityBump)

	return pad(b, launchConfigSpace)
}

// EncodeBridgeConfig returns BridgeConfig account data, padded to its allocated size.
func EncodeBridgeConfig(c BridgeConfig) []byte {
	b := anchor(aktsolana.DiscBridgeConfig).
		key(c.LightClientProgram).bytes([]byte(c.ChainID)).
		u64(c.PerTransferMax).u64(c.DailyMintCap).u32(uint32(len(c.Vaults))) //nolint:gosec // test data
	for _, v := range c.Vaults {
		b = b.key(v)
	}

	return pad(b.u64(c.MintDay).u64(c.MintedToday), bridgeConfigSpace)
}

// EncodeMint returns SPL Token mint account data.
func EncodeMint(m Mint) []byte {
	b := borsh(nil).coption(m.MintAuthority).u64(m.Supply).u8(m.Decimals).bool(m.Initialized).coption(m.FreezeAuthority)

	return pad(b, mintSpace)
}

func (b borsh) transferStatus(s TransferStatus) borsh {
	b = b.bytes(s.ID.OwnerBytes).
		u64(uint64(s.ID.Height)). //nolint:gosec // the program stores the height as u64
		u64(s.ID.Sequence).u64(s.ProofHeight).u64(s.ResolvedAtSlot)

	switch o := s.Outcome; {
	case o.Minted != nil:
		return b.u8(0).u64(o.Minted.Amount).key(o.Minted.Recipient)
	case o.Voided != nil:
		return b.u8(1).u8(uint8(*o.Voided))
	default:
		return b.u8(invalidOutcomeTag)
	}
}

// EncodeTransferStatus returns TransferStatus account data, padded to its allocated size.
func EncodeTransferStatus(s TransferStatus) []byte {
	return pad(anchor(aktsolana.DiscTransferStatus).transferStatus(s), transferStatusSpace)
}

// EncodeTransferResolved returns the Anchor event data of TransferResolved, discriminator included.
func EncodeTransferResolved(s TransferStatus) []byte {
	return anchor(aktsolana.DiscTransferResolved).transferStatus(s)
}

// TransferResolvedLog returns the "Program data:" log line that emit! writes for the event.
func TransferResolvedLog(s TransferStatus) string {
	return "Program data: " + base64.StdEncoding.EncodeToString(EncodeTransferResolved(s))
}

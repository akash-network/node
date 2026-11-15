package bridge

import (
	"fmt"

	"github.com/gagliardetto/solana-go"
	addresslookuptable "github.com/gagliardetto/solana-go/programs/address-lookup-table"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type LaunchConfig struct {
	Version           uint8
	Initializer       solana.PublicKey
	Mint              solana.PublicKey
	BridgeEnabled     bool
	InitializedAtSlot uint64
	Bump              uint8
	MintAuthorityBump uint8
}

type MintPolicy struct {
	PerTransferMax uint64
	DailyMintCap   uint64
	Vaults         []solana.PublicKey
}

type BridgeConfig struct {
	LightClientProgram solana.PublicKey
	ChainID            string
	Policy             MintPolicy
	MintDay            uint64
	MintedToday        uint64
}

type ProofBuffer struct {
	Submitter   solana.PublicKey
	Nonce       [32]byte
	ExpectedLen uint32
	Data        []byte
	Allocated   int
	Lamports    uint64
}

type OutcomeKind uint8

const (
	OutcomeMinted OutcomeKind = iota
	OutcomeVoided
)

type VoidReason uint8

const (
	VoidInvalidRecipient VoidReason = iota
	VoidGovernance
)

type Outcome struct {
	Kind      OutcomeKind
	Amount    uint64
	Recipient solana.PublicKey
	Reason    VoidReason
}

type Status struct {
	ID             sink.ID
	ProofHeight    uint64
	ResolvedAtSlot uint64
	Outcome        Outcome
}

type Height struct {
	RevisionNumber uint64
	RevisionHeight uint64
}

type ClientState struct {
	ChainID               string
	TrustLevelNumerator   uint64
	TrustLevelDenominator uint64
	TrustingPeriod        uint64
	UnbondingPeriod       uint64
	MaxClockDrift         uint64
	FrozenHeight          Height
	LatestHeight          Height
}

type Consensus struct {
	Address            solana.PublicKey
	Height             uint64
	Timestamp          uint64
	Root               [32]byte
	NextValidatorsHash [32]byte
	Lamports           uint64
}

type AppState struct {
	AccessManager        solana.PublicKey
	PendingAccessManager *solana.PublicKey
	Reserved             [256]byte
}

type HeaderChunk struct {
	Submitter solana.PublicKey
	Data      []byte
	Lamports  uint64
}

type SignatureBatch struct {
	Version   uint8
	Submitter solana.PublicKey
	Session   [32]byte
	Bump      uint8
	Hashes    [][32]byte
	Lamports  uint64
}

type Role struct {
	ID      uint64
	Members []solana.PublicKey
}

type AuthorityTransfer struct {
	TargetProgram solana.PublicKey
	NewAuthority  solana.PublicKey
}

type AccessManager struct {
	Roles                     []Role
	WhitelistedPrograms       []solana.PublicKey
	PendingAuthorityTransfers []AuthorityTransfer
}

type Lookup struct {
	Address  solana.PublicKey
	State    addresslookuptable.AddressLookupTableState
	Lamports uint64
}

type Table = Lookup

func DecodeLaunch(data []byte) (LaunchConfig, error) {
	d := accountDecoder(data, "LaunchConfig")
	v := LaunchConfig{
		Version:           d.u8(),
		Initializer:       d.key(),
		Mint:              d.key(),
		BridgeEnabled:     d.boolean(),
		InitializedAtSlot: d.u64(),
		Bump:              d.u8(),
		MintAuthorityBump: d.u8(),
	}
	return v, d.err
}

func DecodeBridge(data []byte) (BridgeConfig, error) {
	d := accountDecoder(data, "BridgeConfig")
	v := BridgeConfig{
		LightClientProgram: d.key(),
		ChainID:            d.str(),
	}
	v.Policy = MintPolicy{
		PerTransferMax: d.u64(),
		DailyMintCap:   d.u64(),
		Vaults:         d.keys(),
	}
	v.MintDay = d.u64()
	v.MintedToday = d.u64()
	if d.err == nil && (len(v.ChainID) == 0 || len(v.ChainID) > 64) {
		d.err = fmt.Errorf("invalid bridge chain ID")
	}
	return v, d.err
}

func DecodeProofBuffer(data []byte) (ProofBuffer, error) {
	d := accountDecoder(data, "ProofBuffer")
	v := ProofBuffer{
		Submitter:   d.key(),
		Nonce:       d.hash(),
		ExpectedLen: d.u32(),
		Data:        d.vector(),
		Allocated:   len(data),
	}
	if d.err == nil && (v.ExpectedLen == 0 || v.ExpectedLen > 16384 || uint32(len(v.Data)) > v.ExpectedLen) {
		d.err = fmt.Errorf("invalid proof buffer length")
	}
	return v, d.err
}

func DecodeStatus(data []byte) (Status, error) {
	d := accountDecoder(data, "TransferStatus")
	v := Status{
		ID:             d.id(),
		ProofHeight:    d.u64(),
		ResolvedAtSlot: d.u64(),
	}
	v.Outcome.Kind = OutcomeKind(d.u8())
	switch v.Outcome.Kind {
	case OutcomeMinted:
		v.Outcome.Amount = d.u64()
		v.Outcome.Recipient = d.key()
	case OutcomeVoided:
		v.Outcome.Reason = VoidReason(d.u8())
		if v.Outcome.Reason > VoidGovernance && d.err == nil {
			d.err = fmt.Errorf("invalid void reason")
		}
	default:
		if d.err == nil {
			d.err = fmt.Errorf("invalid transfer outcome")
		}
	}
	return v, d.err
}

func (d *decoder) height() Height {
	return Height{
		RevisionNumber: d.u64(),
		RevisionHeight: d.u64(),
	}
}

func DecodeClient(data []byte) (ClientState, error) {
	d := accountDecoder(data, "ClientState")
	v := ClientState{
		ChainID:               d.str(),
		TrustLevelNumerator:   d.u64(),
		TrustLevelDenominator: d.u64(),
		TrustingPeriod:        d.u64(),
		UnbondingPeriod:       d.u64(),
		MaxClockDrift:         d.u64(),
		FrozenHeight:          d.height(),
		LatestHeight:          d.height(),
	}
	return v, d.err
}

func DecodeConsensus(data []byte) (Consensus, error) {
	d := accountDecoder(data, "ConsensusStateStore")
	v := Consensus{
		Height:             d.u64(),
		Timestamp:          d.u64(),
		Root:               d.hash(),
		NextValidatorsHash: d.hash(),
	}
	return v, d.err
}

func DecodeApp(data []byte) (AppState, error) {
	d := accountDecoder(data, "AppState")
	v := AppState{AccessManager: d.key()}
	if d.boolean() {
		key := d.key()
		v.PendingAccessManager = &key
	}
	copy(v.Reserved[:], d.take(len(v.Reserved)))
	return v, d.err
}

func DecodeChunk(data []byte) (HeaderChunk, error) {
	d := accountDecoder(data, "HeaderChunk")
	v := HeaderChunk{
		Submitter: d.key(),
		Data:      d.vector(),
	}
	return v, d.err
}

func DecodeBatch(data []byte) (SignatureBatch, error) {
	d := accountDecoder(data, "SignatureBatch")
	v := SignatureBatch{
		Version:   d.u8(),
		Submitter: d.key(),
		Session:   d.hash(),
		Bump:      d.u8(),
	}
	v.Hashes = make([][32]byte, d.count(32))
	for i := range v.Hashes {
		v.Hashes[i] = d.hash()
	}
	return v, d.err
}

func DecodeAccess(data []byte) (AccessManager, error) {
	d := accountDecoder(data, "AccessManager")
	v := AccessManager{Roles: make([]Role, d.count(12))}
	for i := range v.Roles {
		v.Roles[i] = Role{
			ID:      d.u64(),
			Members: d.keys(),
		}
	}
	v.WhitelistedPrograms = d.keys()
	v.PendingAuthorityTransfers = make([]AuthorityTransfer, d.count(64))
	for i := range v.PendingAuthorityTransfers {
		v.PendingAuthorityTransfers[i] = AuthorityTransfer{
			TargetProgram: d.key(),
			NewAuthority:  d.key(),
		}
	}
	return v, d.err
}

func (e *encoder) height(v Height) {
	e.u64(v.RevisionNumber)
	e.u64(v.RevisionHeight)
}

func (e *encoder) client(v ClientState) {
	e.str(v.ChainID)
	e.u64(v.TrustLevelNumerator)
	e.u64(v.TrustLevelDenominator)
	e.u64(v.TrustingPeriod)
	e.u64(v.UnbondingPeriod)
	e.u64(v.MaxClockDrift)
	e.height(v.FrozenHeight)
	e.height(v.LatestHeight)
}

func (e *encoder) policy(v MintPolicy) {
	e.u64(v.PerTransferMax)
	e.u64(v.DailyMintCap)
	e.u32(uint32(len(v.Vaults)))
	for _, key := range v.Vaults {
		e.key(key)
	}
}

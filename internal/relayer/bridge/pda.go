package bridge

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/gagliardetto/solana-go"

	sink "pkg.akt.dev/go/node/sink/v1"
)

type Deployment struct {
	Program       solana.PublicKey
	Config        solana.PublicKey
	Bridge        solana.PublicKey
	Mint          solana.PublicKey
	MintAuthority solana.PublicKey
}

type Followed struct {
	Program          solana.PublicKey
	AccessManager    solana.PublicKey
	ClientState      solana.PublicKey
	AppState         solana.PublicKey
	AccessState      solana.PublicKey
	Slot             uint64
	ChainID          string
	Revision         uint64
	Latest           uint64
	TrustNumerator   uint64
	TrustDenominator uint64
	TrustingPeriod   uint64
	UnbondingPeriod  uint64
	MaxClockDrift    uint64
	FrozenRevision   uint64
	FrozenHeight     uint64
}

func pda(program solana.PublicKey, seed string, extra ...[]byte) (solana.PublicKey, error) {
	seeds := append([][]byte{[]byte(seed)}, extra...)
	key, _, err := solana.FindProgramAddress(seeds, program)
	return key, err
}

func le64(v uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, v)
}

func Derive(program solana.PublicKey) (Deployment, error) {
	d := Deployment{Program: program}
	var err error
	if d.Config, err = pda(program, "config"); err != nil {
		return Deployment{}, err
	}
	if d.Bridge, err = pda(program, "bridge"); err != nil {
		return Deployment{}, err
	}
	if d.Mint, err = pda(program, "mint"); err != nil {
		return Deployment{}, err
	}
	if d.MintAuthority, err = pda(program, "mint_authority"); err != nil {
		return Deployment{}, err
	}

	return d, nil
}

func (d Deployment) Status(id sink.ID) (solana.PublicKey, error) {
	owner, err := ownerBytes(id)
	if err != nil {
		return solana.PublicKey{}, err
	}

	hash := sha256.Sum256(owner)
	return pda(d.Program, "transfer", hash[:], le64(uint64(id.Height)), le64(id.Sequence))
}

func (d Deployment) ProofBuffer(payer solana.PublicKey, key []byte) (solana.PublicKey, error) {
	hash := sha256.Sum256(key)
	return pda(d.Program, "proof", payer[:], hash[:])
}

func DeriveClient(program, accessManager solana.PublicKey) (Followed, error) {
	f := Followed{
		Program:       program,
		AccessManager: accessManager,
	}
	var err error
	if f.ClientState, err = pda(program, "client"); err != nil {
		return Followed{}, err
	}
	if f.AppState, err = pda(program, "app_state"); err != nil {
		return Followed{}, err
	}
	if f.AccessState, err = pda(accessManager, "access_manager"); err != nil {
		return Followed{}, err
	}

	return f, nil
}

func (f Followed) ConsensusAt(height uint64) (solana.PublicKey, error) {
	return pda(f.Program, "consensus_state", le64(height))
}

func (f Followed) BatchAt(payer solana.PublicKey, session [32]byte) (solana.PublicKey, error) {
	return pda(f.Program, "sig_batch", payer[:], session[:])
}

func (f Followed) ChunkAt(payer solana.PublicKey, target uint64, index uint8) (solana.PublicKey, error) {
	return pda(f.Program, "header_chunk", payer[:], le64(target), []byte{index})
}

func ProgramData(program solana.PublicKey) (solana.PublicKey, error) {
	key, _, err := solana.FindProgramAddress([][]byte{program[:]}, solana.BPFLoaderUpgradeableProgramID)
	return key, err
}

func AssociatedToken(recipient, mint solana.PublicKey) (solana.PublicKey, error) {
	key, _, err := solana.FindAssociatedTokenAddress(recipient, mint)
	return key, err
}

// Package header encodes the pinned Solana IBC Borsh header without an external encoder.
package header

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"sort"
	"time"

	cmted "github.com/cometbft/cometbft/crypto/ed25519"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	cmttypes "github.com/cometbft/cometbft/types"
)

var ErrUnsupported = errors.New("unsupported header")

// UnsupportedError retains the operator-facing reason without changing its text.
type UnsupportedError struct {
	Reason string
}

func (e *UnsupportedError) Error() string { return e.Reason }

func (e *UnsupportedError) Unwrap() error { return ErrUnsupported }

// LayoutRevision binds this encoding to both Rust dependency manifests.
const LayoutRevision = "0da3165d8428a1b7792cc17551ecd1d3985761e6"

type Source struct {
	Signed        *cmttypes.SignedHeader
	Validators    []*cmttypes.Validator
	TrustedNext   []*cmttypes.Validator
	TrustedHeight uint64
	Revision      uint64
}

type Fraction struct {
	Numerator   uint64
	Denominator uint64
}

type Selection struct {
	Trigger int
	Limit   int
	Trust   *Fraction
}

type SelectionReport struct {
	Present int
	Kept    int
}

type Receipt struct {
	PubKey    [32]byte
	Message   []byte
	Signature [64]byte
	Hash      [32]byte
}

type Encoding struct {
	ChainID            string
	Header             []byte
	Height             uint64
	Timestamp          string
	Root               []byte
	NextValidatorsHash [32]byte
	BlockHash          [32]byte
	Receipts           []Receipt
}

type Packet struct {
	Header             []byte
	Timestamp          uint64
	Root               [32]byte
	NextValidatorsHash [32]byte
	Receipts           []Receipt
}

func ParseSource(raw []byte) (Source, error) {
	var input struct {
		Signed     json.RawMessage `json:"signed_header"`
		Validators json.RawMessage `json:"validators"`
		Trusted    json.RawMessage `json:"trusted_validators"`
		Height     *uint64         `json:"trusted_height"`
		Revision   *uint64         `json:"revision"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return Source{}, err
	}
	if input.Height == nil || input.Revision == nil {
		return Source{}, errors.New("missing trusted height or revision")
	}

	result := Source{
		TrustedHeight: *input.Height,
		Revision:      *input.Revision,
	}
	var err error
	result.Signed, err = parseSignedHeader(input.Signed)
	if err != nil {
		return Source{}, err
	}

	result.Validators, err = parseValidators(input.Validators)
	if err != nil {
		return Source{}, err
	}

	result.TrustedNext, err = parseValidators(input.Trusted)
	if err != nil {
		return Source{}, err
	}
	if result.Signed == nil || result.Signed.Header == nil || result.Signed.Commit == nil {
		return Source{}, errors.New("missing signed header")
	}

	return result, nil
}

func parseValidators(raw []byte) ([]*cmttypes.Validator, error) {
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	if values == nil {
		return nil, errors.New("missing validators")
	}

	result := make([]*cmttypes.Validator, len(values))
	for i, value := range values {
		if value == nil {
			return nil, errors.New("invalid validator")
		}

		var power json.RawMessage
		for _, name := range []string{"voting_power", "power", "total_voting_power"} {
			if raw, ok := value[name]; ok {
				if power != nil {
					return nil, errors.New("duplicate validator power")
				}
				power = raw
			}
		}
		if power == nil || bytes.Equal(bytes.TrimSpace(power), []byte("null")) {
			return nil, errors.New("missing validator power")
		}
		power, err := normalizeSourcePower(power)
		if err != nil {
			return nil, err
		}
		value["voting_power"] = power

		if raw, ok := value["name"]; ok {
			var name *string
			if err := json.Unmarshal(raw, &name); err != nil {
				return nil, err
			}
		}

		// Rust serde skips priority altogether, including values outside i64.
		delete(value, "proposer_priority")
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if err := cmtjson.Unmarshal(encoded, &result[i]); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func cloneSource(s Source) Source {
	out := s
	if s.Signed != nil {
		signed := *s.Signed
		if s.Signed.Header != nil {
			h := *s.Signed.Header
			signed.Header = &h
		}

		if s.Signed.Commit != nil {
			commit := *s.Signed.Commit
			commit.Signatures = append([]cmttypes.CommitSig(nil), commit.Signatures...)
			signed.Commit = &commit
		}
		out.Signed = &signed
	}

	out.Validators = cloneValidators(s.Validators)
	out.TrustedNext = cloneValidators(s.TrustedNext)

	return out
}

func cloneValidators(values []*cmttypes.Validator) []*cmttypes.Validator {
	result := make([]*cmttypes.Validator, len(values))
	for i, v := range values {
		if v != nil {
			result[i] = v.Copy()
		}
	}
	return result
}

func sortedValidators(values []*cmttypes.Validator) ([]*cmttypes.Validator, uint64, error) {
	result := cloneValidators(values)
	seen := make(map[string]bool)
	var total uint64
	for _, v := range result {
		if v == nil || len(v.Address) != 20 || v.PubKey == nil || v.VotingPower < 0 {
			return nil, 0, errors.New("invalid validator")
		}
		if _, ok := v.PubKey.(cmted.PubKey); !ok || len(v.PubKey.Bytes()) != 32 {
			return nil, 0, errors.New("only Ed25519 is supported")
		}
		if seen[string(v.Address)] {
			return nil, 0, errors.New("duplicate validator address")
		}
		seen[string(v.Address)] = true
		if uint64(v.VotingPower) > uint64(cmttypes.MaxTotalVotingPower)-total {
			return nil, 0, errors.New("validator power exceeds limit")
		}
		total += uint64(v.VotingPower)

		// tendermint-rs skips this field when decoding JSON.
		v.ProposerPriority = 0
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].VotingPower != result[j].VotingPower {
			return result[i].VotingPower > result[j].VotingPower
		}
		return bytes.Compare(result[i].Address, result[j].Address) < 0
	})

	return result, total, nil
}

func timestamp(t time.Time) *big.Int {
	n := new(big.Int).Mul(big.NewInt(t.Unix()), big.NewInt(1_000_000_000))
	return n.Add(n, big.NewInt(int64(t.Nanosecond())))
}

func Encode(source Source) (Encoding, error) {
	if source.Signed == nil || source.Signed.Header == nil || source.Signed.Commit == nil {
		return Encoding{}, errors.New("missing signed header")
	}

	block, commit := source.Signed.Header, source.Signed.Commit
	if block.Height <= 0 || commit.Height != block.Height || source.TrustedHeight == 0 {
		return Encoding{}, errors.New("invalid header or trusted height")
	}
	if commit.Round < 0 || commit.Round > math.MaxUint16 {
		return Encoding{}, errors.New("round exceeds the pinned Borsh header format")
	}
	if len(block.ChainID) == 0 || len(block.ChainID) > 50 || block.Time.Year() < 1 || block.Time.Year() > 9999 {
		return Encoding{}, errors.New("invalid chain ID or time")
	}

	for _, hash := range [][]byte{block.ValidatorsHash, block.NextValidatorsHash, commit.BlockID.Hash} {
		if len(hash) != 32 {
			return Encoding{}, errors.New("invalid required hash length")
		}
	}
	for _, hash := range [][]byte{block.LastCommitHash, block.DataHash, block.LastResultsHash, block.EvidenceHash, block.ConsensusHash} {
		if len(hash) != 0 && len(hash) != 32 {
			return Encoding{}, errors.New("invalid optional hash length")
		}
	}

	if len(block.ProposerAddress) != 20 {
		return Encoding{}, errors.New("invalid proposer address")
	}
	if err := block.LastBlockID.ValidateBasic(); err != nil {
		return Encoding{}, err
	}
	if err := commit.BlockID.ValidateBasic(); err != nil {
		return Encoding{}, err
	}
	if block.Height == 1 && !block.LastBlockID.IsZero() {
		return Encoding{}, errors.New("last block ID on first header")
	}

	validators, total, err := sortedValidators(source.Validators)
	if err != nil {
		return Encoding{}, err
	}
	trusted, trustedTotal, err := sortedValidators(source.TrustedNext)
	if err != nil {
		return Encoding{}, err
	}

	set := &cmttypes.ValidatorSet{Validators: validators}
	if !bytes.Equal(set.Hash(), block.ValidatorsHash) {
		return Encoding{}, errors.New("validator hash mismatch")
	}

	hash := block.Hash()
	if !bytes.Equal(hash, commit.BlockID.Hash) {
		return Encoding{}, errors.New("commit does not bind header")
	}

	out := Encoding{
		ChainID:   block.ChainID,
		Height:    uint64(block.Height),
		Timestamp: timestamp(block.Time).String(),
		Receipts:  make([]Receipt, 0),
	}
	out.Root = bytes.Clone(block.AppHash)
	copy(out.NextValidatorsHash[:], block.NextValidatorsHash)
	copy(out.BlockHash[:], hash)

	seen := make(map[string]bool)
	for i, sig := range commit.Signatures {
		if sig.BlockIDFlag == cmttypes.BlockIDFlagAbsent {
			if len(sig.Signature) != 0 {
				return Encoding{}, errors.New("expected empty signature for absent commitsig")
			}
			if !sig.Timestamp.IsZero() {
				return Encoding{}, errors.New("absent commitsig has non-zero timestamp")
			}
			continue
		}

		if sig.BlockIDFlag != cmttypes.BlockIDFlagCommit && sig.BlockIDFlag != cmttypes.BlockIDFlagNil {
			return Encoding{}, errors.New("invalid vote kind")
		}
		if len(sig.ValidatorAddress) != 20 || (len(sig.Signature) != 0 && len(sig.Signature) != 64) || sig.Timestamp.Year() < 1 || sig.Timestamp.Year() > 9999 {
			return Encoding{}, errors.New("invalid vote")
		}

		if seen[string(sig.ValidatorAddress)] {
			return Encoding{}, errors.New("duplicate non-absent signature address")
		}
		seen[string(sig.ValidatorAddress)] = true

		if sig.BlockIDFlag == cmttypes.BlockIDFlagNil {
			if len(sig.Signature) != 64 {
				return Encoding{}, errors.New("nil commitsig has no signature")
			}
			continue
		}
		if len(sig.Signature) != 64 {
			return Encoding{}, errors.New("missing commit signature")
		}

		var validator *cmttypes.Validator
		for _, v := range validators {
			if bytes.Equal(v.Address, sig.ValidatorAddress) {
				validator = v
				break
			}
		}
		if validator == nil {
			return Encoding{}, errors.New("signature references unknown validator")
		}

		receipt := Receipt{Message: cmttypes.VoteSignBytes(block.ChainID, commit.GetVote(int32(i)).ToProto())}
		copy(receipt.PubKey[:], validator.PubKey.Bytes())
		copy(receipt.Signature[:], sig.Signature)

		digest := sha256.New()
		digest.Write(receipt.PubKey[:])
		digest.Write(receipt.Message)
		digest.Write(receipt.Signature[:])
		copy(receipt.Hash[:], digest.Sum(nil))
		out.Receipts = append(out.Receipts, receipt)
	}

	w := borshWriter{}
	w.block(block)
	w.commit(commit)
	w.validators(validators, total)
	w.u64(source.Revision)
	w.u64(source.TrustedHeight)
	w.validators(trusted, trustedTotal)

	out.Header = w
	return out, nil
}

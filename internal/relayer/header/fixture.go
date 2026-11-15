package header

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	cmted "github.com/cometbft/cometbft/crypto/ed25519"
	cmttypes "github.com/cometbft/cometbft/types"
)

// Fixture replaces validators with deterministic LOCAL TEST keys only.
func Fixture(input Source, count, signers int, weighted bool) (Source, error) {
	if count < 1 || count > 256 || signers < 0 || signers > count {
		return Source{}, errors.New("invalid fixture size")
	}
	if input.Signed == nil || input.Signed.Header == nil || input.Signed.Commit == nil {
		return Source{}, errors.New("missing fixture header")
	}

	result := cloneSource(input)
	values := make([]*cmttypes.Validator, count)
	keys := make(map[string]ed25519.PrivateKey, count)
	for i := range values {
		seed := sha256.Sum256([]byte(fmt.Sprintf("AKT LOCAL BENCHMARK ONLY %d", i)))
		key := ed25519.NewKeyFromSeed(seed[:])
		pub := cmted.PubKey(key.Public().(ed25519.PublicKey))
		power := int64(10000)
		if weighted {
			power = 1000
			if i < 30 {
				power = 100000
			}
		}

		values[i] = cmttypes.NewValidator(pub, power)
		keys[string(values[i].Address)] = key
	}

	values, _, err := sortedValidators(values)
	if err != nil {
		return Source{}, err
	}

	set := &cmttypes.ValidatorSet{Validators: values}
	block, commit := result.Signed.Header, result.Signed.Commit
	block.ChainID = "akash-scale-1"
	block.ValidatorsHash = set.Hash()
	block.NextValidatorsHash = set.Hash()
	block.ProposerAddress = values[0].Address

	commit.Height = block.Height
	commit.BlockID.Hash = block.Hash()

	commit.Signatures = make([]cmttypes.CommitSig, count)
	for i, v := range values {
		commit.Signatures[i] = cmttypes.NewCommitSigAbsent()
		if i >= signers {
			continue
		}

		commit.Signatures[i] = cmttypes.CommitSig{
			BlockIDFlag:      cmttypes.BlockIDFlagCommit,
			ValidatorAddress: v.Address,
			Timestamp:        block.Time,
		}

		message := cmttypes.VoteSignBytes(block.ChainID, commit.GetVote(int32(i)).ToProto())
		commit.Signatures[i].Signature = ed25519.Sign(keys[string(v.Address)], message)
	}

	result.Validators = values
	result.TrustedNext = cloneValidators(values)

	return result, nil
}

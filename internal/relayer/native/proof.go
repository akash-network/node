package native

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"

	cmtcrypto "github.com/cometbft/cometbft/proto/tendermint/crypto"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	ics23 "github.com/cosmos/ics23/go"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/x/sink/keeper"
)

type Proof struct {
	Transfer sink.TransferState
	Height   int64
	Key      []byte
	Value    []byte
	Merkle   []byte
}

// ProofValue returns the canonical node key and value at the queried state height.
func ProofValue(transfer sink.TransferState, height int64) ([]byte, []byte, error) {
	if err := validTransfer(transfer); err != nil {
		return nil, nil, err
	}

	authorization := transfer.Settlement.VoidAuthorization
	if authorization == nil {
		key, err := keeper.TransferKey(transfer.ID)
		if err != nil {
			return nil, nil, err
		}
		value, err := transfer.Data.Marshal()
		return key, value, err
	}

	if height <= 0 || authorization.RequestedHeight < transfer.ID.Height || authorization.RequestedHeight > height || authorization.RequestedRound == 0 || authorization.RequestedRound > transfer.Settlement.Round {
		return nil, nil, errors.New("invalid void authorization height or round")
	}
	key, err := keeper.SettlementKey(*sink.NewSettlementID(transfer.Status, transfer.ID))
	if err != nil {
		return nil, nil, err
	}
	value, err := transfer.Settlement.Marshal()
	return key, value, err
}

// membershipProof checks the wire envelope, leaving cryptographic verification to the contract.
func membershipProof(key, value []byte, ops *cmtcrypto.ProofOps) ([]byte, error) {
	if ops == nil || len(ops.Ops) != 2 || ops.Ops[0].Type != "ics23:iavl" || ops.Ops[1].Type != "ics23:simple" {
		return nil, errors.New("membership proof needs both store levels")
	}

	result := commitment.MerkleProof{Proofs: make([]*ics23.CommitmentProof, 2)}
	for i, op := range ops.Ops {
		shape, err := fields(op.Data)
		if err != nil {
			return nil, err
		}
		if len(shape[1]) != 1 || len(shape[2])+len(shape[3])+len(shape[4]) != 0 {
			return nil, errors.New("ambiguous membership proof")
		}

		proof := new(ics23.CommitmentProof)
		if err := proof.Unmarshal(op.Data); err != nil {
			return nil, err
		}
		existence := proof.GetExist()
		if existence == nil {
			return nil, errors.New("membership proof needs existence proof")
		}
		if i == 0 && (!bytes.Equal(existence.Key, key) || !bytes.Equal(existence.Value, value)) {
			return nil, errors.New("proof key or value mismatch")
		}
		if i == 1 && !bytes.Equal(existence.Key, []byte("sink")) {
			return nil, errors.New("wrong proof store")
		}
		result.Proofs[i] = proof
	}

	encoded, err := result.Marshal()
	if err != nil {
		return nil, err
	}
	if len(encoded) == 0 || len(encoded) > 16*1024 {
		return nil, errors.New("proof exceeds upload bound")
	}

	return encoded, nil
}

func (c *Client) Prove(ctx context.Context, transfer sink.TransferState, height int64) (Proof, error) {
	if height <= transfer.ID.Height || height <= 1 || height > maxSafeHeight {
		return Proof{}, errors.New("proof predates identity or has invalid height")
	}

	key, value, err := ProofValue(transfer, height-1)
	if err != nil {
		return Proof{}, err
	}

	reply, err := c.query(ctx, "/store/sink/key", key, height-1, true)
	if err != nil {
		return Proof{}, err
	}
	if !bytes.Equal(reply.Key, key) || !bytes.Equal(reply.Value, value) {
		return Proof{}, errors.New("proof differs from snapshot")
	}

	merkle, err := membershipProof(key, value, reply.ProofOps)
	if err != nil {
		return Proof{}, err
	}

	return Proof{
		Transfer: transfer,
		Height:   height,
		Key:      key,
		Value:    value,
		Merkle:   merkle,
	}, nil
}

// Retained's height is the proof header height; its state is queried at height-1.
func (c *Client) Retained(ctx context.Context, key []byte, height int64, valueHash [32]byte) (bool, error) {
	if height <= 1 || height > maxSafeHeight || len(key) == 0 {
		return false, errors.New("invalid retained checkpoint")
	}

	reply, err := c.query(ctx, "/store/sink/key", key, height-1, false)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(reply.Key, key) {
		return false, errors.New("checkpoint key mismatch")
	}

	if reply.Log == "version does not exist" {
		if reply.Codespace != "" || len(reply.Value) != 0 || reply.ProofOps != nil {
			return false, errors.New("invalid missing version response")
		}
		return false, nil
	}
	if reply.Log != "" {
		return false, errors.New("unrecognized checkpoint response")
	}
	if sha256.Sum256(reply.Value) != valueHash {
		return false, errors.New("checkpoint value mismatch")
	}

	return true, nil
}

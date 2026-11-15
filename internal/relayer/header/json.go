package header

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
)

type receiptJSON struct {
	PubKey    string `json:"pubkey"`
	Message   string `json:"message"`
	Signature string `json:"signature"`
	Hash      string `json:"hash"`
}

type encodingJSON struct {
	ChainID   string `json:"chainId"`
	Header    string `json:"header"`
	Consensus struct {
		Height             string  `json:"height"`
		Timestamp          string  `json:"timestamp"`
		Root               *string `json:"root"`
		NextValidatorsHash string  `json:"nextValidatorsHash"`
		BlockHash          string  `json:"blockHash"`
	} `json:"consensus"`
	Signatures []receiptJSON `json:"signatures"`
}

func (e Encoding) MarshalJSON() ([]byte, error) {
	out := encodingJSON{
		ChainID:    e.ChainID,
		Header:     hex.EncodeToString(e.Header),
		Signatures: make([]receiptJSON, len(e.Receipts)),
	}
	out.Consensus.Height = strconv.FormatUint(e.Height, 10)
	out.Consensus.Timestamp = e.Timestamp
	root := hex.EncodeToString(e.Root)
	out.Consensus.Root = &root
	out.Consensus.NextValidatorsHash = hex.EncodeToString(e.NextValidatorsHash[:])
	out.Consensus.BlockHash = hex.EncodeToString(e.BlockHash[:])

	for i, r := range e.Receipts {
		out.Signatures[i] = receiptJSON{
			PubKey:    hex.EncodeToString(r.PubKey[:]),
			Message:   hex.EncodeToString(r.Message),
			Signature: hex.EncodeToString(r.Signature[:]),
			Hash:      hex.EncodeToString(r.Hash[:]),
		}
	}

	return json.Marshal(out)
}

func decodeHex(value string, length int) ([]byte, error) {
	result, err := hex.DecodeString(value)
	if err != nil || hex.EncodeToString(result) != value || (length >= 0 && len(result) != length) || (length < 0 && len(result) == 0) {
		return nil, errors.New("invalid canonical packet hex")
	}

	return result, nil
}

func (e *Encoding) UnmarshalJSON(raw []byte) error {
	var input encodingJSON
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}

	result := Encoding{
		ChainID:   input.ChainID,
		Timestamp: input.Consensus.Timestamp,
		Receipts:  make([]Receipt, len(input.Signatures)),
	}

	var err error
	result.Height, err = strconv.ParseUint(input.Consensus.Height, 10, 64)
	if err != nil || result.Height == 0 || strconv.FormatUint(result.Height, 10) != input.Consensus.Height {
		return errors.New("invalid consensus height")
	}

	n, ok := new(big.Int).SetString(result.Timestamp, 10)
	if !ok || n.String() != result.Timestamp {
		return errors.New("invalid consensus timestamp")
	}
	if result.ChainID == "" || input.Signatures == nil {
		return errors.New("missing packet fields")
	}

	result.Header, err = decodeHex(input.Header, -1)
	if err != nil {
		return err
	}

	if input.Consensus.Root == nil {
		return errors.New("missing consensus root")
	}
	result.Root, err = decodeHex(*input.Consensus.Root, len(*input.Consensus.Root)/2)
	if err != nil {
		return err
	}

	for _, field := range []struct {
		value  string
		target []byte
	}{
		{
			value:  input.Consensus.NextValidatorsHash,
			target: result.NextValidatorsHash[:],
		},
		{
			value:  input.Consensus.BlockHash,
			target: result.BlockHash[:],
		},
	} {
		value, err := decodeHex(field.value, len(field.target))
		if err != nil {
			return err
		}
		copy(field.target, value)
	}

	for i, r := range input.Signatures {
		receipt := &result.Receipts[i]
		for _, field := range []struct {
			value  string
			target []byte
		}{
			{
				value:  r.PubKey,
				target: receipt.PubKey[:],
			},
			{
				value:  r.Signature,
				target: receipt.Signature[:],
			},
			{
				value:  r.Hash,
				target: receipt.Hash[:],
			},
		} {
			value, err := decodeHex(field.value, len(field.target))
			if err != nil {
				return err
			}
			copy(field.target, value)
		}

		receipt.Message, err = decodeHex(r.Message, -1)
		if err != nil {
			return err
		}
	}

	*e = result
	return nil
}

func RuntimePacket(e Encoding) (Packet, error) {
	if len(e.Root) != 32 {
		return Packet{}, errors.New("runtime root must be exactly 32 bytes")
	}

	n, ok := new(big.Int).SetString(e.Timestamp, 10)
	if !ok || n.String() != e.Timestamp || !n.IsUint64() {
		return Packet{}, fmt.Errorf("invalid runtime timestamp %q", e.Timestamp)
	}

	receipts := make([]Receipt, len(e.Receipts))
	for i, r := range e.Receipts {
		receipts[i] = r
		receipts[i].Message = append([]byte(nil), r.Message...)
	}

	return Packet{
		Header:             append([]byte(nil), e.Header...),
		Timestamp:          n.Uint64(),
		Root:               [32]byte(e.Root),
		NextValidatorsHash: e.NextValidatorsHash,
		Receipts:           receipts,
	}, nil
}

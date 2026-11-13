package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gagliardetto/solana-go"
)

// Transaction versions as the JSON encoding reports them.
const (
	versionLegacy = "legacy"
	version0      = "0"
	version1      = "1"
)

// Tx is a transaction decoded from the JSON encoding. Keys lists the static account keys, then
// meta.loadedAddresses.writable, then meta.loadedAddresses.readonly; Writable holds one flag per key.
type Tx struct {
	Slot       uint64
	Version    string
	Signatures []string
	Failed     bool
	Logs       []string
	HasLogs    bool
	Keys       []solana.PublicKey
	Writable   []bool
}

type txJSON struct {
	Version json.RawMessage `json:"version"`
	Meta    *struct {
		Err             json.RawMessage `json:"err"`
		LogMessages     *[]string       `json:"logMessages"`
		LoadedAddresses *struct {
			Writable []string `json:"writable"`
			Readonly []string `json:"readonly"`
		} `json:"loadedAddresses"`
	} `json:"meta"`
	Transaction struct {
		Signatures []string `json:"signatures"`
		Message    struct {
			AccountKeys []string `json:"accountKeys"`
			Header      struct {
				NumRequiredSignatures       int `json:"numRequiredSignatures"`
				NumReadonlySignedAccounts   int `json:"numReadonlySignedAccounts"`
				NumReadonlyUnsignedAccounts int `json:"numReadonlyUnsignedAccounts"`
			} `json:"header"`
			AddressTableLookups []struct {
				WritableIndexes []int `json:"writableIndexes"`
				ReadonlyIndexes []int `json:"readonlyIndexes"`
			} `json:"addressTableLookups"`
		} `json:"message"`
	} `json:"transaction"`
}

// version returns "legacy", the decimal text of a numeric version, or "" when the field is absent.
func (j *txJSON) version() string {
	var s string
	if json.Unmarshal(j.Version, &s) == nil {
		return s
	}

	return string(bytes.TrimSpace(j.Version))
}

func parseKeys(dst []solana.PublicKey, encoded []string) ([]solana.PublicKey, error) {
	for _, e := range encoded {
		k, err := solana.PublicKeyFromBase58(e)
		if err != nil {
			return nil, fmt.Errorf("account key %q: %w", e, err)
		}
		dst = append(dst, k)
	}

	return dst, nil
}

// decode converts the JSON form, porting web3.js getAccountKeys and isAccountWritable for legacy, v0
// and v1 messages. A transaction web3.js could not resolve, or of another version, gets no writable key.
func (j *txJSON) decode(slot uint64) (Tx, error) {
	msg := &j.Transaction.Message
	tx := Tx{
		Slot:       slot,
		Version:    j.version(),
		Signatures: j.Transaction.Signatures,
		Failed:     j.Meta == nil || !isJSONNull(j.Meta.Err),
	}
	if j.Meta != nil && j.Meta.LogMessages != nil {
		tx.Logs = *j.Meta.LogMessages
		tx.HasLogs = true
	}

	static, err := parseKeys(nil, msg.AccountKeys)
	if err != nil {
		return Tx{}, err
	}
	tx.Keys = static

	var loadedWritable int
	switch tx.Version {
	case versionLegacy, version1:
		// web3.js ignores loaded addresses for these messages.
	case version0:
		var lookupWritable, lookupTotal int
		for _, l := range msg.AddressTableLookups {
			lookupWritable += len(l.WritableIndexes)
			lookupTotal += len(l.WritableIndexes) + len(l.ReadonlyIndexes)
		}
		var loaded []string
		if j.Meta != nil && j.Meta.LoadedAddresses != nil {
			loaded = append(append(loaded, j.Meta.LoadedAddresses.Writable...), j.Meta.LoadedAddresses.Readonly...)
		} else if lookupTotal > 0 {
			return tx.readonly(), nil
		}
		if len(loaded) != lookupTotal {
			return tx.readonly(), nil
		}
		if tx.Keys, err = parseKeys(tx.Keys, loaded); err != nil {
			return Tx{}, err
		}
		loadedWritable = lookupWritable
	default:
		return tx.readonly(), nil
	}

	h := msg.Header
	numStatic := len(static)
	tx.Writable = make([]bool, len(tx.Keys))
	for i := range tx.Keys {
		switch {
		case i >= numStatic:
			tx.Writable[i] = i-numStatic < loadedWritable
		case i >= h.NumRequiredSignatures:
			tx.Writable[i] = i-h.NumRequiredSignatures < numStatic-h.NumRequiredSignatures-h.NumReadonlyUnsignedAccounts
		default:
			tx.Writable[i] = i < h.NumRequiredSignatures-h.NumReadonlySignedAccounts
		}
	}

	return tx, nil
}

// readonly marks every key read-only, so the transaction can never supply evidence.
func (tx Tx) readonly() Tx {
	tx.Writable = make([]bool, len(tx.Keys))
	return tx
}

// isJSONNull reports an explicit null; an absent field is not null, as in web3.js (`undefined !== null`).
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// StreamBlock fetches the finalized block at slot and calls each for its transactions in order,
// decoding one transaction at a time. A getBlock error, a null result or a malformed block gives
// ErrBlockUnavailable, possibly after some calls to each; an error from each is returned as is.
func (r *RPC) StreamBlock(ctx context.Context, slot uint64, each func(Tx) error) error {
	body, release, err := r.post(ctx, r.blocks, "getBlock", slot, rpcConfig{
		Commitment:                     commitment,
		Encoding:                       encodingJSON,
		TransactionDetails:             "full",
		Rewards:                        ptr(false),
		MaxSupportedTransactionVersion: ptr(maxTxVersion),
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return blockUnavailable(err)
	}

	var eachErr error
	err = streamBlock(json.NewDecoder(body), slot, func(tx Tx) error {
		eachErr = each(tx)
		return eachErr
	})
	release(err)

	switch {
	case err == nil:
		return nil
	case eachErr != nil:
		return eachErr
	case ctx.Err() != nil:
		return ctx.Err()
	default:
		return blockUnavailable(err)
	}
}

func streamBlock(dec *json.Decoder, slot uint64, each func(Tx) error) error {
	if err := expectDelim(dec, '{'); err != nil {
		return err
	}

	seen := false
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return err
		}
		switch key {
		case "error":
			var e *rpcError
			if err := dec.Decode(&e); err != nil {
				return err
			}
			if e != nil {
				return fmt.Errorf("getBlock: %w", e)
			}
		case "result":
			if err := streamResult(dec, slot, each); err != nil {
				return err
			}
			seen = true
		default:
			if err := skipValue(dec); err != nil {
				return err
			}
		}
	}
	if err := expectDelim(dec, '}'); err != nil {
		return err
	}
	if !seen {
		return errors.New("getBlock: response has no result")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("getBlock: trailing data after response")
	}

	return nil
}

func streamResult(dec *json.Decoder, slot uint64, each func(Tx) error) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return errors.New("getBlock: null result")
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("getBlock: result is %v, not an object", tok)
	}

	seen := false
	for dec.More() {
		key, err := objectKey(dec)
		if err != nil {
			return err
		}
		if key != "transactions" {
			if err := skipValue(dec); err != nil {
				return err
			}
			continue
		}
		if err := expectDelim(dec, '['); err != nil {
			return err
		}
		for i := 0; dec.More(); i++ {
			var j txJSON
			if err := dec.Decode(&j); err != nil {
				return fmt.Errorf("getBlock: transaction %d: %w", i, err)
			}
			tx, err := j.decode(slot)
			if err != nil {
				return fmt.Errorf("getBlock: transaction %d: %w", i, err)
			}
			if err := each(tx); err != nil {
				return err
			}
		}
		if err := expectDelim(dec, ']'); err != nil {
			return err
		}
		seen = true
	}
	if err := expectDelim(dec, '}'); err != nil {
		return err
	}
	if !seen {
		return errors.New("getBlock: result has no transactions")
	}

	return nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != want {
		return fmt.Errorf("getBlock: got %v, want %v", tok, want)
	}

	return nil
}

func objectKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("getBlock: got %v, want an object key", tok)
	}

	return key, nil
}

func skipValue(dec *json.Decoder) error {
	var skip json.RawMessage
	return dec.Decode(&skip)
}

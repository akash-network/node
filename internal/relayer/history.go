package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
)

// Only explicit absence returns nil. A receipt is terminal evidence for the
// original signed transaction, including when execution failed and paid a fee.
func (p *payerSession) finalizedHistory(ctx context.Context, doc *intent) (*txResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var raw json.RawMessage
	options := rpc.M{
		"commitment":                     rpc.CommitmentFinalized,
		"encoding":                       solana.EncodingBase64,
		"maxSupportedTransactionVersion": 0,
	}
	err := p.service.solana.RPCCallWithCallback(ctx, "getTransaction", []interface{}{doc.Signature, options}, func(request *http.Request, response *http.Response) error {
		var err error
		raw, err = historyResponse(request, response)
		return err
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("finalized transaction history lookup: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	result, err := decodeFinalizedHistory(doc, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid finalized transaction receipt: %w", err)
	}
	// Polling may outlive the genesis check at entry to settle. Recheck before
	// allowing recovered metadata to reach the durable observer.
	if err := p.checkGenesis(ctx); err != nil {
		return nil, err
	}
	return &result, nil
}

// RPCCallForInto substitutes null for a missing result. The existing callback
// API keeps the configured transport while preserving the response envelope.
func historyResponse(request *http.Request, response *http.Response) (json.RawMessage, error) {
	decoder := json.NewDecoder(response.Body)
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		return nil, fmt.Errorf("malformed finalized history response (HTTP %d)", response.StatusCode)
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return nil, fmt.Errorf("trailing finalized history response data")
	}
	var protocol string
	if err := historyField(fields, "jsonrpc", &protocol); err != nil || protocol != "2.0" {
		return nil, fmt.Errorf("invalid finalized history RPC version")
	}
	if request.GetBody == nil {
		return nil, fmt.Errorf("finalized history request identity unavailable")
	}
	body, err := request.GetBody()
	if err != nil {
		return nil, fmt.Errorf("finalized history request identity unavailable")
	}
	defer body.Close()
	var sent struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.NewDecoder(body).Decode(&sent); err != nil || len(sent.ID) == 0 || !bytes.Equal(bytes.TrimSpace(fields["id"]), bytes.TrimSpace(sent.ID)) {
		return nil, fmt.Errorf("finalized history response ID mismatch")
	}
	result, hasResult := fields["result"]
	failure, hasError := fields["error"]
	if hasError && !bytes.Equal(bytes.TrimSpace(failure), []byte("null")) {
		if hasResult {
			return nil, fmt.Errorf("conflicting finalized history result and error")
		}
		errorFields, err := jsonObject(failure)
		if err != nil || requireFields(errorFields, "code", "message") != nil {
			return nil, fmt.Errorf("malformed finalized history RPC error")
		}
		var rpcError jsonrpc.RPCError
		decoder := json.NewDecoder(bytes.NewReader(failure))
		decoder.UseNumber()
		if err := decoder.Decode(&rpcError); err != nil {
			return nil, fmt.Errorf("malformed finalized history RPC error")
		}
		return nil, &rpcError
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("finalized history HTTP status %d", response.StatusCode)
	}
	if !hasResult {
		return nil, fmt.Errorf("missing finalized history result")
	}
	return result, nil
}

// Select exact JSON names and reject missing/null fields before decoding. Do
// not echo provider-controlled field values into schema error diagnostics.
func historyField(fields map[string]json.RawMessage, name string, value any) error {
	if err := requireFields(fields, name); err != nil {
		return err
	}
	if err := json.Unmarshal(fields[name], value); err != nil {
		return fmt.Errorf("invalid receipt field %s", name)
	}
	return nil
}

func historyUint(raw json.RawMessage) (uint64, error) {
	var value *uint64
	if err := json.Unmarshal(raw, &value); err != nil || value == nil || *value > uint64(maxSafeInteger) {
		return 0, fmt.Errorf("receipt number must be a nonnegative safe integer")
	}
	return *value, nil
}

func historyBalances(fields map[string]json.RawMessage, name string, accounts int) ([]uint64, error) {
	var values []json.RawMessage
	if err := historyField(fields, name, &values); err != nil {
		return nil, err
	}
	if len(values) != accounts {
		return nil, fmt.Errorf("receipt %s account count mismatch", name)
	}
	balances := make([]uint64, len(values))
	for i, raw := range values {
		value, err := historyUint(raw)
		if err != nil {
			return nil, fmt.Errorf("receipt %s: %w", name, err)
		}
		balances[i] = value
	}
	return balances, nil
}

func decodeFinalizedHistory(doc *intent, raw json.RawMessage) (txResult, error) {
	fields, err := jsonObject(raw)
	if err != nil {
		return txResult{}, fmt.Errorf("receipt must be an object")
	}
	if _, err := historyUint(fields["slot"]); err != nil {
		return txResult{}, fmt.Errorf("invalid receipt slot")
	}
	var encoded []*string
	if err := historyField(fields, "transaction", &encoded); err != nil {
		return txResult{}, err
	}
	if len(encoded) != 2 || encoded[0] == nil || encoded[1] == nil || *encoded[1] != string(solana.EncodingBase64) || *encoded[0] != doc.Raw {
		return txResult{}, fmt.Errorf("receipt transaction must match original base64 bytes")
	}
	original, _, err := doc.transaction()
	if err != nil {
		return txResult{}, err
	}
	tx, err := solana.TransactionFromBytes(original)
	if err != nil {
		return txResult{}, fmt.Errorf("invalid receipt transaction")
	}
	switch tx.Message.GetVersion() {
	case solana.MessageVersionV0:
		version, err := historyUint(fields["version"])
		if err != nil || version != 0 {
			return txResult{}, fmt.Errorf("receipt transaction version mismatch")
		}
	case solana.MessageVersionLegacy:
		var version string
		if err := historyField(fields, "version", &version); err != nil || version != "legacy" {
			return txResult{}, fmt.Errorf("receipt transaction version mismatch")
		}
	default:
		return txResult{}, fmt.Errorf("unsupported receipt transaction version")
	}
	canonical, err := tx.MarshalBinary()
	if err != nil || !bytes.Equal(canonical, original) {
		return txResult{}, fmt.Errorf("noncanonical receipt transaction")
	}
	header := tx.Message.Header
	static := len(tx.Message.AccountKeys)
	signers := int(header.NumRequiredSignatures)
	if signers == 0 || signers > static || len(tx.Signatures) != signers || int(header.NumReadonlySignedAccounts) >= signers || int(header.NumReadonlyUnsignedAccounts) > static-signers {
		return txResult{}, fmt.Errorf("invalid receipt transaction account header")
	}
	if err := tx.VerifySignatures(); err != nil {
		return txResult{}, fmt.Errorf("invalid receipt transaction signature")
	}

	meta, err := jsonObject(fields["meta"])
	if err != nil {
		return txResult{}, fmt.Errorf("receipt metadata must be an object")
	}
	loaded, err := jsonObject(meta["loadedAddresses"])
	if err != nil {
		return txResult{}, fmt.Errorf("receipt loaded addresses must be an object")
	}
	var writable, readonly []*solana.PublicKey
	if err := historyField(loaded, "writable", &writable); err != nil {
		return txResult{}, err
	}
	if err := historyField(loaded, "readonly", &readonly); err != nil {
		return txResult{}, err
	}
	wantWritable, wantReadonly := 0, 0
	for _, lookup := range tx.Message.AddressTableLookups {
		wantWritable += len(lookup.WritableIndexes)
		wantReadonly += len(lookup.ReadonlyIndexes)
	}
	if len(writable) != wantWritable || len(readonly) != wantReadonly {
		return txResult{}, fmt.Errorf("receipt loaded address count mismatch")
	}
	for _, addresses := range [][]*solana.PublicKey{writable, readonly} {
		for _, address := range addresses {
			if address == nil {
				return txResult{}, fmt.Errorf("invalid receipt loaded address")
			}
		}
	}
	accounts := static + wantWritable + wantReadonly
	if accounts < 1 || accounts > 64 || accounts != doc.Accounts {
		return txResult{}, fmt.Errorf("receipt transaction account count mismatch")
	}
	for _, instruction := range tx.Message.Instructions {
		if int(instruction.ProgramIDIndex) >= static {
			return txResult{}, fmt.Errorf("receipt program index outside static accounts")
		}
		for _, index := range instruction.Accounts {
			if int(index) >= accounts {
				return txResult{}, fmt.Errorf("receipt instruction account index out of bounds")
			}
		}
	}

	failure := bytes.TrimSpace(meta["err"])
	if !validHistoryError(failure, len(tx.Message.Instructions)) {
		return txResult{}, fmt.Errorf("invalid or missing receipt execution error")
	}
	fee, err := historyUint(meta["fee"])
	if err != nil {
		return txResult{}, fmt.Errorf("invalid receipt fee: %w", err)
	}
	pre, err := historyBalances(meta, "preBalances", accounts)
	if err != nil {
		return txResult{}, err
	}
	post, err := historyBalances(meta, "postBalances", accounts)
	if err != nil {
		return txResult{}, err
	}
	delta := int64(pre[0]) - int64(post[0])
	compute := uint64(0)
	if raw, ok := meta["computeUnitsConsumed"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		compute, err = historyUint(raw)
		if err != nil {
			return txResult{}, fmt.Errorf("invalid receipt compute units: %w", err)
		}
	}
	var messages []*string
	if err := historyField(meta, "logMessages", &messages); err != nil {
		return txResult{}, err
	}
	logs := make([]string, len(messages))
	for i, message := range messages {
		if message == nil {
			return txResult{}, fmt.Errorf("invalid receipt log message")
		}
		logs[i] = *message
	}
	return txResult{
		stage:         doc.Stage,
		signature:     doc.Signature,
		bytes:         len(original),
		accounts:      accounts,
		fee:           fee,
		compute:       compute,
		failure:       failure,
		logs:          logs,
		payerLamports: &delta,
	}, nil
}

// Solana execution errors are null, a unit variant string, or a tagged object.
// Preserve the payload verbatim; unknown fields must never default to success.
func validHistoryError(raw json.RawMessage, instructions int) bool {
	if bytes.Equal(raw, []byte("null")) {
		return true
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name != ""
	}
	fields, err := jsonObject(raw)
	if err != nil || len(fields) != 1 {
		return false
	}
	for name, value := range fields {
		if name == "InstructionError" {
			var parts []json.RawMessage
			if json.Unmarshal(value, &parts) != nil || len(parts) != 2 {
				return false
			}
			index, err := historyUint(parts[0])
			if err != nil || index >= uint64(instructions) {
				return false
			}
			var reason string
			if json.Unmarshal(parts[1], &reason) == nil {
				return reason != ""
			}
			detail, err := jsonObject(parts[1])
			if err != nil || len(detail) != 1 {
				return false
			}
			if custom, ok := detail["Custom"]; ok {
				code, err := historyUint(custom)
				return err == nil && code <= uint64(^uint32(0))
			}
			if message, ok := detail["BorshIoError"]; ok {
				return json.Unmarshal(message, &reason) == nil && !bytes.Equal(bytes.TrimSpace(message), []byte("null"))
			}
			return false
		}
		payload, err := jsonObject(value)
		return name != "" && err == nil && len(payload) != 0
	}
	return false
}

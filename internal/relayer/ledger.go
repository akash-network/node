package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type retryState struct {
	Failures int64 `json:"failures"`
	RetryAt  int64 `json:"retryAt"`
}

type spendEntry struct {
	SpentLamports           int64       `json:"spentLamports"`
	Attempts                int64       `json:"attempts"`
	BufferFundedLamports    *int64      `json:"bufferFundedLamports,omitempty"`
	BufferReclaimedLamports *int64      `json:"bufferReclaimedLamports,omitempty"`
	Signatures              []string    `json:"signatures,omitempty"`
	Completed               *bool       `json:"completed,omitempty"`
	Retry                   *retryState `json:"retry,omitempty"`
}

func (entry *spendEntry) UnmarshalJSON(data []byte) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	type plain spendEntry
	return decodeExactFields(fields, (*plain)(entry), "spentLamports", "attempts", "bufferFundedLamports", "bufferReclaimedLamports")
}

func (retry *retryState) UnmarshalJSON(data []byte) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	type plain retryState
	return decodeExactFields(fields, (*plain)(retry), "failures", "retryAt")
}

// A present empty signature list is distinct from an omitted list in legacy
// JSON, even though both admit the first charge.
func (entry spendEntry) MarshalJSON() ([]byte, error) {
	type plain spendEntry
	data, err := json.Marshal(plain(entry))
	if err != nil || entry.Signatures == nil || len(entry.Signatures) != 0 {
		return data, err
	}
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	fields["signatures"] = json.RawMessage(`[]`)
	return json.Marshal(fields)
}

type spendLedger struct {
	entries map[sink.ID]spendEntry
	order   []sink.ID
}

type spendLedgerDoc struct {
	Version   int                   `json:"version"`
	Genesis   string                `json:"genesis"`
	ProgramID string                `json:"programId"`
	Transfers map[string]spendEntry `json:"transfers"`
}

func (l *spendLedger) ids() []sink.ID { return append([]sink.ID{}, l.order...) }

func (l *spendLedger) set(id sink.ID, entry spendEntry) {
	if _, exists := l.entries[id]; !exists {
		l.order = append(l.order, id)
	}
	l.entries[id] = entry
}

// JSON.parse keeps the first insertion position of a duplicate property while
// its final value wins. Sink ID properties cannot be JS array-index keys.
func orderedJSONFields(data []byte) ([]string, map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, nil, fmt.Errorf("spend ledger transfers must be a JSON object")
	}
	order := []string{}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid spend ledger property")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, nil, err
		}
		if _, exists := fields[name]; !exists {
			order = append(order, name)
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, nil, fmt.Errorf("trailing spend ledger transfers data")
	}
	return order, fields, nil
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func safeSum(a, b int64) (int64, error) {
	if !safeInteger(a) || !safeInteger(b) || !safeInteger(a+b) {
		return 0, fmt.Errorf("spend total exceeds safe integer")
	}
	return a + b, nil
}

func (l *spendLedger) ready(id sink.ID, now int64) bool {
	retry := l.entries[id].Retry
	return retry == nil || now >= retry.RetryAt
}

func (l *spendLedger) failed(id sink.ID, now, maxMs int64) (int64, int64, error) {
	if now < 0 || maxMs < 0 || !safeInteger(now) {
		return 0, 0, fmt.Errorf("invalid retry clock or maximum")
	}
	entry := l.entries[id]
	failures := int64(1)
	if entry.Retry != nil {
		var err error
		failures, err = safeSum(entry.Retry.Failures, 1)
		if err != nil {
			return 0, 0, err
		}
	}
	delay := min(maxMs, int64(6000)<<min(failures-1, 30))
	retryAt, err := safeSum(now, delay)
	if err != nil {
		return 0, 0, err
	}
	entry.Retry = &retryState{
		Failures: failures,
		RetryAt:  retryAt,
	}
	l.set(id, entry)
	return failures, delay, nil
}

func (l *spendLedger) clearRetry(id sink.ID) {
	if entry, ok := l.entries[id]; ok {
		entry.Retry = nil
		l.set(id, entry)
	}
}

func (l *spendLedger) complete(id sink.ID) {
	if entry, ok := l.entries[id]; ok {
		completed := true
		entry.Completed = &completed
		entry.Retry = nil
		l.set(id, entry)
	}
}

func (l *spendLedger) spent(id sink.ID) int64                   { return l.entries[id].SpentLamports }
func (l *spendLedger) exhausted(id sink.ID, maximum int64) bool { return l.spent(id) > maximum }

func (l *spendLedger) attempt(id sink.ID) error {
	entry := l.entries[id]
	attempts, err := safeSum(entry.Attempts, 1)
	if err != nil {
		return err
	}
	entry.Attempts = attempts
	entry.Completed = nil
	l.set(id, entry)
	return nil
}

func (l *spendLedger) charge(id sink.ID, lamports int64, signature string, bufferChange int64) error {
	if !safeInteger(lamports) || !safeInteger(bufferChange) {
		return fmt.Errorf("spend must be a safe integer")
	}
	entry := l.entries[id]
	for _, charged := range entry.Signatures {
		if signature != "" && charged == signature {
			return nil
		}
	}
	spent, err := safeSum(entry.SpentLamports, lamports)
	if err != nil {
		return err
	}
	funded, err := safeSum(int64Value(entry.BufferFundedLamports), max(0, bufferChange))
	if err != nil {
		return err
	}
	reclaimed, err := safeSum(int64Value(entry.BufferReclaimedLamports), max(0, -bufferChange))
	if err != nil {
		return err
	}
	if signature != "" {
		entry.Signatures = append(append([]string{}, entry.Signatures...), signature)
	}
	entry.SpentLamports = spent
	entry.BufferFundedLamports = &funded
	entry.BufferReclaimedLamports = &reclaimed
	l.set(id, entry)
	return nil
}

func (l *spendLedger) reset(value string) (bool, error) {
	if value == "all" {
		changed := len(l.entries) > 0
		clear(l.entries)
		l.order = nil
		return changed, nil
	}
	id, err := native.ParseID(value)
	if err != nil {
		return false, err
	}
	_, exists := l.entries[id]
	delete(l.entries, id)
	if exists {
		for i, orderedID := range l.order {
			if orderedID == id {
				l.order = append(l.order[:i], l.order[i+1:]...)
				break
			}
		}
	}
	return exists, nil
}

func (l *spendLedger) document(genesis, program string) spendLedgerDoc {
	entries := make(map[string]spendEntry, len(l.entries))
	for _, id := range l.order {
		entries[id.String()] = l.entries[id]
	}
	return spendLedgerDoc{
		Version:   2,
		Genesis:   genesis,
		ProgramID: program,
		Transfers: entries,
	}
}

func parseSpendLedger(data []byte, genesis, program string) (*spendLedger, error) {
	ledger := &spendLedger{entries: make(map[sink.ID]spendEntry)}
	if data == nil {
		return ledger, nil
	}
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Version   int    `json:"version"`
		Genesis   string `json:"genesis"`
		ProgramID string `json:"programId"`
	}
	if err := decodeExactFields(fields, &doc, "version"); err != nil {
		return nil, err
	}
	if doc.Version != 2 {
		return nil, fmt.Errorf("unsupported scalar-ID spend ledger; preserve it and choose a fresh v2 state directory")
	}
	if doc.Genesis != genesis || doc.ProgramID != program {
		return nil, fmt.Errorf("spend ledger belongs to another deployment")
	}
	order, entries, err := orderedJSONFields(fields["transfers"])
	if err != nil {
		return nil, err
	}
	for _, key := range order {
		data := entries[key]
		id, err := native.ParseID(key)
		if err != nil {
			return nil, err
		}
		fields, err := jsonObject(data)
		if err != nil {
			return nil, err
		}
		if err := requireFields(fields, "spentLamports", "attempts"); err != nil {
			return nil, err
		}
		var entry spendEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, err
		}
		if raw, present := fields["signatures"]; present {
			var signatures []json.RawMessage
			if err := json.Unmarshal(raw, &signatures); err != nil {
				return nil, err
			}
			for _, signature := range signatures {
				if len(signature) == 0 || signature[0] != '"' {
					return nil, fmt.Errorf("invalid charged signature")
				}
			}
		}
		if !safeInteger(entry.SpentLamports) || !safeInteger(entry.Attempts) || entry.Attempts < 0 {
			return nil, fmt.Errorf("invalid spend ledger entry %s", key)
		}
		for _, name := range documentFields(entry) {
			if bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
				return nil, fmt.Errorf("invalid null ledger field %s", name)
			}
		}
		for _, amount := range []*int64{entry.BufferFundedLamports, entry.BufferReclaimedLamports} {
			if amount != nil && (*amount < 0 || !safeInteger(*amount)) {
				return nil, fmt.Errorf("invalid buffer liquidity")
			}
		}
		if entry.Retry != nil {
			retryFields, err := jsonObject(fields["retry"])
			if err != nil {
				return nil, err
			}
			if err := requireFields(retryFields, "failures", "retryAt"); err != nil {
				return nil, err
			}
			if !positiveSafe(entry.Retry.Failures) || !safeInteger(entry.Retry.RetryAt) || entry.Retry.RetryAt < 0 {
				return nil, fmt.Errorf("invalid retry state")
			}
		}
		funded, reclaimed := int64Value(entry.BufferFundedLamports), int64Value(entry.BufferReclaimedLamports)
		entry.BufferFundedLamports = &funded
		entry.BufferReclaimedLamports = &reclaimed
		ledger.set(id, entry)
	}
	return ledger, nil
}

func (p *payerSession) chargeResult(ctx context.Context, id sink.ID, result txResult) error {
	liquidity := result.stage == "proof-create" || result.stage == "proof-grow" || result.stage == "proof-close"
	if (liquidity || result.stage == "resolve") && (result.payerLamports == nil || !safeInteger(*result.payerLamports)) {
		return fmt.Errorf("missing payer balance change")
	}
	if result.fee > uint64(maxSafeInteger) {
		return fmt.Errorf("fee exceeds safe integer")
	}
	spent := int64(result.fee)
	bufferChange := int64(0)
	if result.stage == "resolve" {
		spent = *result.payerLamports
	}
	if liquidity {
		var err error
		bufferChange, err = safeSum(*result.payerLamports, -int64(result.fee))
		if err != nil {
			return err
		}
	}
	_, err := p.service.store.updateLedger(ctx, func(ledger *spendLedger) (bool, error) {
		return true, ledger.charge(id, spent, result.signature, bufferChange)
	})
	if err != nil {
		return err
	}
	payerLamports := int64(result.fee)
	if result.payerLamports != nil {
		payerLamports = *result.payerLamports
	}
	p.service.emit(false, map[string]any{
		"event":                "relay-payment",
		"id":                   id.String(),
		"stage":                result.stage,
		"signature":            result.signature,
		"fee":                  result.fee,
		"payerLamports":        payerLamports,
		"bufferChangeLamports": bufferChange,
		"spentLamports":        spent,
	})
	return nil
}

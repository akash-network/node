package relayer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// Extra holds fields understood by another version of the header lifecycle.
// Typed fields always replace their previous values, including omissions.
type activeHeaderDoc struct {
	ChainID string                     `json:"chainId"`
	Genesis string                     `json:"genesis"`
	Client  string                     `json:"client"`
	Target  int64                      `json:"target"`
	Trusted int64                      `json:"trusted"`
	Packet  json.RawMessage            `json:"packet"`
	Extra   map[string]json.RawMessage `json:"-"`
}

type headerPacketDoc struct {
	ChainID    string                     `json:"chainId"`
	Header     string                     `json:"header"`
	Consensus  headerConsensusDoc         `json:"consensus"`
	Signatures []headerSignatureDoc       `json:"signatures"`
	Extra      map[string]json.RawMessage `json:"-"`
}

type headerConsensusDoc struct {
	Timestamp          string                     `json:"timestamp"`
	Root               string                     `json:"root"`
	NextValidatorsHash string                     `json:"nextValidatorsHash"`
	Extra              map[string]json.RawMessage `json:"-"`
}

type headerSignatureDoc struct {
	Hash      string                     `json:"hash"`
	Pubkey    string                     `json:"pubkey"`
	Message   string                     `json:"message"`
	Signature string                     `json:"signature"`
	Extra     map[string]json.RawMessage `json:"-"`
}

type transactionDoc struct {
	Stage         string                     `json:"stage"`
	Signature     string                     `json:"signature"`
	Bytes         int                        `json:"bytes"`
	Accounts      int                        `json:"accounts"`
	Fee           uint64                     `json:"fee"`
	Compute       uint64                     `json:"compute"`
	Error         json.RawMessage            `json:"error"`
	Logs          []string                   `json:"logs"`
	PayerLamports *int64                     `json:"payerLamports,omitempty"`
	Extra         map[string]json.RawMessage `json:"-"`
}

type sessionDoc struct {
	Version                 int                        `json:"version"`
	Genesis                 string                     `json:"genesis"`
	Payer                   string                     `json:"payer"`
	Client                  string                     `json:"client"`
	Target                  int64                      `json:"target"`
	Trusted                 int64                      `json:"trusted"`
	Session                 string                     `json:"session"`
	Lookup                  *string                    `json:"lookup,omitempty"`
	LookupSlot              *int64                     `json:"lookupSlot,omitempty"`
	Assembly                *transactionDoc            `json:"assembly,omitempty"`
	LookupClosed            *bool                      `json:"lookupClosed,omitempty"`
	LookupClosureUnobserved *bool                      `json:"lookupClosureUnobserved,omitempty"`
	LookupRentClosing       *int64                     `json:"lookupRentClosing,omitempty"`
	LookupRentReturned      *int64                     `json:"lookupRentReturned,omitempty"`
	LookupCloseTransaction  *string                    `json:"lookupCloseTransaction,omitempty"`
	LookupCloseFailure      *transactionDoc            `json:"lookupCloseFailure,omitempty"`
	Extra                   map[string]json.RawMessage `json:"-"`
}

func jsonObject(data []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("journal must be a JSON object")
	}
	return fields, nil
}

func requireFields(fields map[string]json.RawMessage, names ...string) error {
	for _, name := range names {
		if raw, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("missing journal field %s", name)
		}
	}
	return nil
}

func documentFields(value any) []string {
	typ := reflect.TypeOf(value)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	var names []string
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "-" && name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Select exact property names after JSON.parse-style last-value resolution.
// Only declared Number fields pass through binary64: JavaScript rounds before
// testing safe integers. Opaque JSON and bigint strings must not be converted.
func decodeExactFields(fields map[string]json.RawMessage, value any, numbers ...string) error {
	known := make(map[string]json.RawMessage)
	for _, name := range documentFields(value) {
		if raw, present := fields[name]; present {
			known[name] = raw
		}
	}
	for _, name := range numbers {
		raw, present := known[name]
		if !present {
			continue
		}
		number, err := strconv.ParseFloat(string(bytes.TrimSpace(raw)), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < -maxSafeInteger || number > maxSafeInteger {
			return fmt.Errorf("journal field %s must be a safe integer number", name)
		}
		known[name] = json.RawMessage(strconv.FormatInt(int64(number), 10))
	}
	data, err := json.Marshal(known)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (v *intent) UnmarshalJSON(data []byte) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	type plain intent
	return decodeExactFields(fields, (*plain)(v), "lastValidBlockHeight", "accounts")
}

func decodeDocument(data []byte, value any, extra *map[string]json.RawMessage) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return err
	}
	for _, name := range documentFields(value) {
		if name != "error" && bytes.Equal(bytes.TrimSpace(fields[name]), []byte("null")) {
			return fmt.Errorf("journal field %s cannot be null", name)
		}
		delete(fields, name)
	}
	*extra = fields
	return nil
}

func encodeDocument(value any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	fields, err := jsonObject(data)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for _, name := range documentFields(value) {
		known[name] = true
	}
	for name, raw := range extra {
		if !known[name] {
			fields[name] = raw
		}
	}
	return json.Marshal(fields)
}

func (v *activeHeaderDoc) UnmarshalJSON(data []byte) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	if err := requireFields(fields, "chainId", "genesis", "client", "target", "trusted"); err != nil {
		return err
	}
	type plain activeHeaderDoc
	if err := json.Unmarshal(data, (*plain)(v)); err != nil {
		return err
	}
	for _, name := range documentFields(v) {
		delete(fields, name)
	}
	v.Extra = fields
	return nil
}
func (v activeHeaderDoc) MarshalJSON() ([]byte, error) {
	type plain activeHeaderDoc
	return encodeDocument(plain(v), v.Extra)
}

// A repinned upload is archived before interpreting its former client's packet.
func decodeActivePacket(raw json.RawMessage, chainID string) (headerPacketDoc, error) {
	var packet headerPacketDoc
	if err := json.Unmarshal(raw, &packet); err != nil {
		return headerPacketDoc{}, err
	}
	if err := packet.validate(chainID); err != nil {
		return headerPacketDoc{}, err
	}
	if packet.Header == "" {
		return headerPacketDoc{}, fmt.Errorf("hex bytes required for header")
	}
	for _, signature := range packet.Signatures {
		if signature.Message == "" {
			return headerPacketDoc{}, fmt.Errorf("hex bytes required for signature message")
		}
	}
	return packet, nil
}

func (v *headerPacketDoc) UnmarshalJSON(data []byte) error {
	type plain headerPacketDoc
	return decodeDocument(data, (*plain)(v), &v.Extra)
}
func (v headerPacketDoc) MarshalJSON() ([]byte, error) {
	type plain headerPacketDoc
	return encodeDocument(plain(v), v.Extra)
}
func (v *headerConsensusDoc) UnmarshalJSON(data []byte) error {
	type plain headerConsensusDoc
	return decodeDocument(data, (*plain)(v), &v.Extra)
}
func (v headerConsensusDoc) MarshalJSON() ([]byte, error) {
	type plain headerConsensusDoc
	return encodeDocument(plain(v), v.Extra)
}
func (v *headerSignatureDoc) UnmarshalJSON(data []byte) error {
	type plain headerSignatureDoc
	return decodeDocument(data, (*plain)(v), &v.Extra)
}
func (v headerSignatureDoc) MarshalJSON() ([]byte, error) {
	type plain headerSignatureDoc
	return encodeDocument(plain(v), v.Extra)
}
func (v *sessionDoc) UnmarshalJSON(data []byte) error {
	type plain sessionDoc
	return decodeDocument(data, (*plain)(v), &v.Extra)
}
func (v sessionDoc) MarshalJSON() ([]byte, error) {
	type plain sessionDoc
	return encodeDocument(plain(v), v.Extra)
}
func (v *transactionDoc) UnmarshalJSON(data []byte) error {
	fields, err := jsonObject(data)
	if err != nil {
		return err
	}
	if err := requireFields(fields, "stage", "signature"); err != nil {
		return err
	}
	if _, ok := fields["error"]; !ok {
		return fmt.Errorf("missing transaction error field")
	}
	type plain transactionDoc
	return decodeDocument(data, (*plain)(v), &v.Extra)
}
func (v transactionDoc) MarshalJSON() ([]byte, error) {
	type plain transactionDoc
	if v.Logs == nil {
		v.Logs = []string{}
	}
	return encodeDocument(plain(v), v.Extra)
}

func transactionDocument(v txResult) *transactionDoc {
	return &transactionDoc{
		Stage:         v.stage,
		Signature:     v.signature,
		Bytes:         v.bytes,
		Accounts:      v.accounts,
		Fee:           v.fee,
		Compute:       v.compute,
		Error:         v.failure,
		Logs:          v.logs,
		PayerLamports: v.payerLamports,
	}
}

func (v transactionDoc) result() txResult {
	return txResult{
		stage:         v.Stage,
		signature:     v.Signature,
		bytes:         v.Bytes,
		accounts:      v.Accounts,
		fee:           v.Fee,
		compute:       v.Compute,
		failure:       v.Error,
		logs:          v.Logs,
		payerLamports: v.PayerLamports,
	}
}

func safeInteger(value int64) bool  { return value >= -maxSafeInteger && value <= maxSafeInteger }
func positiveSafe(value int64) bool { return value > 0 && value <= maxSafeInteger }

func validHex(value string, size int, lowercase bool) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && (size < 0 || len(decoded) == size) && (!lowercase || strings.ToLower(value) == value)
}

func (v *headerPacketDoc) validate(chainID string) error {
	if v.ChainID != chainID || !validHex(v.Header, -1, false) || v.Signatures == nil {
		return fmt.Errorf("invalid header packet")
	}
	if v.Consensus.Timestamp == "" || strings.IndexFunc(v.Consensus.Timestamp, func(c rune) bool { return c < '0' || c > '9' }) != -1 {
		return fmt.Errorf("invalid header timestamp")
	}
	if _, err := strconv.ParseUint(v.Consensus.Timestamp, 10, 64); err != nil {
		return fmt.Errorf("header timestamp: %w", err)
	}
	if !validHex(v.Consensus.Root, 32, false) || !validHex(v.Consensus.NextValidatorsHash, 32, false) {
		return fmt.Errorf("invalid header consensus hashes")
	}
	for _, sig := range v.Signatures {
		if !validHex(sig.Hash, 32, false) || !validHex(sig.Pubkey, 32, false) || !validHex(sig.Message, -1, false) || !validHex(sig.Signature, 64, false) {
			return fmt.Errorf("invalid header signature")
		}
	}
	return nil
}

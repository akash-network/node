package header

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	cmtjson "github.com/cometbft/cometbft/libs/json"
	cmttypes "github.com/cometbft/cometbft/types"
)

// time 0.3.37 consumes any single separator byte, but requires two-digit clock
// fields, dot fractions and bounded offsets. A valid UTF-8 single byte is ASCII.
var sourceTimePattern = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2})[\x00-\x7f]([0-9]{2}:[0-9]{2}:)([0-9]{2})(\.[0-9]+)?([Zz]|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

func normalizeSourceTime(raw json.RawMessage, absent bool) (json.RawMessage, error) {
	if absent && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return json.Marshal(time.Time{})
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	parts := sourceTimePattern.FindStringSubmatch(value)
	if parts == nil {
		return nil, fmt.Errorf("invalid RFC3339 timestamp %q", value)
	}

	date, clock, second, fraction, zone := parts[1], parts[2], parts[3], parts[4], parts[5]
	leap := second == "60"
	if leap {
		// The pinned Rust parser represents a leap second by the preceding
		// nanosecond, regardless of the supplied fractional digits.
		second, fraction = "59", ".999999999"
	}

	parsed, err := time.Parse(time.RFC3339Nano, date+"T"+clock+second+fraction+strings.ToUpper(zone))
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	if parsed.Year() < 1 || parsed.Year() > 9999 {
		return nil, errors.New("timestamp year out of range")
	}
	if leap && (parsed.Hour() != 23 || parsed.Minute() != 59 || parsed.Second() != 59 || parsed.AddDate(0, 0, 1).Day() != 1) {
		return nil, errors.New("leap second is not at UTC month end")
	}

	return json.Marshal(parsed)
}

func normalizeSourcePower(raw json.RawMessage) (json.RawMessage, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	power, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil, err
	}
	if power < 0 {
		return nil, errors.New("negative validator power")
	}

	return json.Marshal(strconv.FormatInt(power, 10))
}

func parseSignedHeader(raw json.RawMessage) (*cmttypes.SignedHeader, error) {
	var input struct {
		Header map[string]json.RawMessage `json:"header"`
		Commit map[string]json.RawMessage `json:"commit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Header == nil || input.Commit == nil {
		return nil, errors.New("missing signed header")
	}

	var err error
	input.Header["time"], err = normalizeSourceTime(input.Header["time"], false)
	if err != nil {
		return nil, fmt.Errorf("header time: %w", err)
	}

	var signatures []map[string]json.RawMessage
	if err := json.Unmarshal(input.Commit["signatures"], &signatures); err != nil {
		return nil, err
	}
	for i, sig := range signatures {
		// Rust requires each JSON field even when an Absent vote permits null.
		for _, name := range []string{"block_id_flag", "validator_address", "timestamp", "signature"} {
			if _, ok := sig[name]; !ok {
				return nil, fmt.Errorf("vote %d: missing %s", i, name)
			}
		}

		var flag cmttypes.BlockIDFlag
		if err := json.Unmarshal(sig["block_id_flag"], &flag); err != nil {
			return nil, err
		}
		sig["timestamp"], err = normalizeSourceTime(sig["timestamp"], flag == cmttypes.BlockIDFlagAbsent)
		if err != nil {
			return nil, fmt.Errorf("vote %d timestamp: %w", i, err)
		}
	}

	input.Commit["signatures"], err = json.Marshal(signatures)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var result cmttypes.SignedHeader
	if err := cmtjson.Unmarshal(normalized, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

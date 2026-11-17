package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"time"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

const legacyConfigLimit = 1 << 20

// Legacy selects the packaged JSON input and single-Comet query transport.
// It is set only by LoadLegacy, never by TOML, environment or canonical flags.
type Legacy struct {
	KeyFile        string
	ExternalSigner *ExternalSigner
}

// ExternalSigner holds the legacy JSON command settings and an optional validated public key.
// A missing key is looked up only during broadcast startup.
type ExternalSigner struct {
	Options   signer.CommandOptions
	PublicKey cryptotypes.PubKey
}

// LoadLegacy reads the old packaged JSON without inspecting keys or contacting services.
// Resolve applies the same identity, fee, gas and broadcast safety rules as canonical input.
func LoadLegacy(path, network string, broadcast bool) (Config, error) {
	if !filepath.IsAbs(path) {
		return Config{}, errors.New("--config must be an absolute path")
	}

	file, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("cannot open legacy config")
	}

	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, legacyConfigLimit+1))
	if err != nil || len(data) > legacyConfigLimit {
		return Config{}, errors.New("legacy config is unreadable or exceeds 1 MiB")
	}

	return parseLegacy(data, network, broadcast)
}

func parseLegacy(data []byte, network string, broadcast bool) (Config, error) {
	fields, err := legacyFields(data)
	if err != nil {
		return Config{}, err
	}

	if _, ok := fields["externalSigner"]; ok {
		if _, raw := fields["keyFile"]; raw {
			return Config{}, errors.New("configure keyFile or externalSigner, not both")
		}
	}

	var input struct {
		AkashRPC    string `json:"akashRpc"`
		SolanaRPC   string `json:"solanaRpc"`
		ChainID     string `json:"akashChainId"`
		Reporter    string `json:"reporter"`
		Genesis     string `json:"solanaGenesisHash"`
		Program     string `json:"programId"`
		Mint        string `json:"mint"`
		LightClient string `json:"lightClientProgram"`
		Fee         string `json:"feeAmountUakt"`
		Gas         string `json:"gasLimit"`
		KeyFile     string `json:"keyFile"`
	}

	if err := json.Unmarshal(data, &input); err != nil {
		return Config{}, errors.New("legacy string fields must be JSON strings")
	}

	for _, key := range []string{"akashRpc", "solanaRpc", "akashChainId", "reporter", "solanaGenesisHash", "programId", "mint", "feeAmountUakt", "gasLimit", "keyFile", "lightClientProgram"} {
		if raw, ok := fields[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || value == "" {
				return Config{}, fmt.Errorf("%s must be a nonempty string", key)
			}
		}
	}

	for _, key := range []string{"akashRpc", "solanaRpc"} {
		var raw string
		_ = json.Unmarshal(fields[key], &raw)
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
			return Config{}, fmt.Errorf("%s must be an http/https URL without credentials or fragment", key)
		}
	}

	if input.KeyFile != "" && !filepath.IsAbs(input.KeyFile) {
		return Config{}, errors.New("keyFile must be absolute")
	}

	if !gasAmount.MatchString(input.Gas) || !uaktAmount.MatchString(input.Fee+uaktDenom) {
		return Config{}, errors.New("gasLimit and feeAmountUakt must be canonical decimal strings")
	}

	age, err := legacyInteger(fields, "maxAgeSeconds", 600, 0)
	if err != nil {
		return Config{}, err
	}

	poll, err := legacyInteger(fields, "pollSeconds", 60, 0)
	if err != nil {
		return Config{}, err
	}

	audit, err := legacyInteger(fields, "auditPerScan", 10000, 100)
	if err != nil {
		return Config{}, err
	}

	lead, err := legacyInteger(fields, "maxSlotLead", 100000, 0)
	if err != nil {
		return Config{}, err
	}

	c := Default()
	c.Legacy = &Legacy{KeyFile: input.KeyFile}
	if raw, ok := fields["externalSigner"]; ok {
		c.Legacy.ExternalSigner, err = parseExternalSigner(raw, input.Reporter)
		if err != nil {
			return Config{}, err
		}
	}

	c.Network = network
	c.Broadcast = broadcast
	c.Reporter = input.Reporter
	c.ChainID = input.ChainID
	c.Node = input.AkashRPC
	c.GRPCAddr = ""
	c.Solana = Solana{
		RPC:                input.SolanaRPC,
		GenesisHash:        input.Genesis,
		ProgramID:          input.Program,
		Mint:               input.Mint,
		LightClientProgram: input.LightClient,
		MaxSlotLead:        uint64(lead),
	}

	c.MaxAge = time.Duration(age) * time.Second
	c.Scan.Interval = time.Duration(poll) * time.Second
	c.Scan.AuditPerScan = audit
	c.Tx.MaxMsgs = 1
	c.Gas = input.Gas
	c.GasPrices = ""
	c.Fees = input.Fee + uaktDenom
	return c, nil
}

func legacyInteger(fields map[string]json.RawMessage, key string, max, fallback int) (int, error) {
	raw, ok := fields[key]
	if !ok {
		return fallback, nil
	}

	var n float64
	if json.Unmarshal(raw, &n) != nil || n < 1 || n > float64(max) || math.Trunc(n) != n {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", key, max)
	}

	return int(n), nil
}

// Decode keys explicitly: encoding/json's struct decoder accepts case-folded and duplicate keys.
func legacyFields(data []byte) (map[string]json.RawMessage, error) {
	required := map[string]bool{
		"akashRpc":           true,
		"solanaRpc":          true,
		"akashChainId":       true,
		"reporter":           true,
		"solanaGenesisHash":  true,
		"programId":          true,
		"mint":               true,
		"maxAgeSeconds":      true,
		"pollSeconds":        true,
		"gasLimit":           true,
		"feeAmountUakt":      true,
		"lightClientProgram": false,
		"maxSlotLead":        false,
		"auditPerScan":       false,
		"keyFile":            false,
		"externalSigner":     false,
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("legacy config must be a JSON object")
	}

	fields := make(map[string]json.RawMessage)
	for dec.More() {
		token, err = dec.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, errors.New("malformed legacy config")
		}

		if _, known := required[key]; !known {
			return nil, errors.New("unknown legacy config field")
		}

		if _, duplicate := fields[key]; duplicate {
			return nil, fmt.Errorf("duplicate legacy config field %s", key)
		}

		var raw json.RawMessage
		if dec.Decode(&raw) != nil || bytes.Equal(raw, []byte("null")) {
			return nil, fmt.Errorf("invalid or null legacy config field %s", key)
		}

		fields[key] = raw
	}

	if token, err = dec.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("malformed legacy config")
	}

	if _, err = dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after legacy config")
	}

	for key, needed := range required {
		if _, ok := fields[key]; needed && !ok {
			return nil, fmt.Errorf("missing legacy config field %s", key)
		}
	}

	return fields, nil
}

func parseExternalSigner(data []byte, reporter string) (*ExternalSigner, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("externalSigner must be an object")
	}

	fields := make(map[string]json.RawMessage)
	for dec.More() {
		token, err = dec.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return nil, errors.New("malformed externalSigner")
		}

		switch key {
		case "command", "args", "timeoutSeconds", "publicKeyBase64":
		default:
			return nil, errors.New("unknown externalSigner field")
		}

		if _, duplicate := fields[key]; duplicate {
			return nil, errors.New("duplicate externalSigner field")
		}

		var raw json.RawMessage
		if dec.Decode(&raw) != nil {
			return nil, errors.New("malformed externalSigner")
		}
		fields[key] = raw
	}

	if token, err = dec.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("malformed externalSigner")
	}

	var command string
	if json.Unmarshal(fields["command"], &command) != nil || !filepath.IsAbs(command) {
		return nil, errors.New("externalSigner.command must be an absolute path")
	}

	var args []string
	if raw, ok := fields["args"]; ok && !bytes.Equal(raw, []byte("null")) {
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || len(values) > 32 {
			return nil, errors.New("externalSigner.args must be an array of at most 32 strings")
		}

		for _, value := range values {
			var arg *string
			if json.Unmarshal(value, &arg) != nil || arg == nil {
				return nil, errors.New("externalSigner.args must contain strings")
			}
			args = append(args, *arg)
		}
	}

	// The TS nullish defaults accept null for args and timeoutSeconds.
	if bytes.Equal(fields["timeoutSeconds"], []byte("null")) {
		delete(fields, "timeoutSeconds")
	}
	seconds, err := legacyInteger(fields, "timeoutSeconds", 120, 10)
	if err != nil {
		return nil, fmt.Errorf("externalSigner: %w", err)
	}

	out := &ExternalSigner{
		Options: signer.CommandOptions{
			Command: command,
			Args:    args,
			Timeout: time.Duration(seconds) * time.Second,
		},
	}
	if raw, ok := fields["publicKeyBase64"]; ok {
		var encoded *string
		if json.Unmarshal(raw, &encoded) != nil || encoded == nil {
			return nil, errors.New("externalSigner.publicKeyBase64 must be a string")
		}

		hrp, address, err := bech32.DecodeAndConvert(reporter)
		if err != nil || hrp != "akash" || len(address) != 20 {
			return nil, errors.New("externalSigner requires a valid reporter address")
		}
		out.PublicKey, err = signer.ParseCommandPublicKey(*encoded, sdk.AccAddress(address))
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

package header

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	cmtsecp "github.com/cometbft/cometbft/crypto/secp256k1"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

type sourceChange struct {
	Path   []string
	Value  any
	Remove bool
}

func sourceWithChanges(t *testing.T, original []byte, changes []sourceChange) []byte {
	t.Helper()
	var input any
	require.NoError(t, json.Unmarshal(original, &input))
	for _, change := range changes {
		target := input
		for _, key := range change.Path[:len(change.Path)-1] {
			if array, ok := target.([]any); ok {
				index, err := strconv.Atoi(key)
				require.NoError(t, err)
				target = array[index]
			} else {
				target = target.(map[string]any)[key]
			}
		}

		key := change.Path[len(change.Path)-1]
		if array, ok := target.([]any); ok {
			index, err := strconv.Atoi(key)
			require.NoError(t, err)
			array[index] = change.Value
		} else if change.Remove {
			delete(target.(map[string]any), key)
		} else {
			target.(map[string]any)[key] = change.Value
		}
	}

	raw, err := json.Marshal(input)
	require.NoError(t, err)
	return raw
}

// These mutations were run against the locked Rust encoder. Every accepted
// input produces the complete normal-mixed oracle, including every receipt.
func TestSourceRustParity(t *testing.T) {
	var cases []struct {
		Name    string
		Accept  bool
		Changes []sourceChange
	}
	raw, err := os.ReadFile("testdata/source-parity.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &cases))

	original, err := os.ReadFile("testdata/normal-mixed.input.json")
	require.NoError(t, err)
	for _, test := range cases {
		t.Run(test.Name, func(t *testing.T) {
			raw := sourceWithChanges(t, original, test.Changes)
			source, err := ParseSource(raw)
			if !test.Accept {
				if err == nil {
					_, err = Encode(source)
				}
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrUnsupported)
				return
			}

			require.NoError(t, err)
			compareOracle(t, source, "normal-mixed.output.json", false)
		})
	}
}

// The fixture retains actual Rust output differences from normal-mixed once per
// distinct result. Reconstruct and compare the entire encoding and JSON output.
func TestSourceDiagnosticRustParity(t *testing.T) {
	var fixture struct {
		Base    string
		Outputs map[string]map[string]json.RawMessage
		Cases   []struct {
			Name    string
			Accept  bool
			Changes []sourceChange
			Output  string
		}
	}
	raw, err := os.ReadFile("testdata/source-diagnostic-parity.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixture))
	original, err := os.ReadFile(filepath.Join("testdata", fixture.Base+".input.json"))
	require.NoError(t, err)
	normal, err := os.ReadFile(filepath.Join("testdata", fixture.Base+".output.json"))
	require.NoError(t, err)

	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			source, err := ParseSource(sourceWithChanges(t, original, test.Changes))
			if !test.Accept {
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrUnsupported)
				return
			}
			require.NoError(t, err)

			var output map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(normal, &output))
			if test.Output != "" {
				overrides, ok := fixture.Outputs[test.Output]
				require.True(t, ok, "missing Rust output %q", test.Output)
				for key, value := range overrides {
					output[key] = value
				}
			}
			wantJSON, err := json.Marshal(output)
			require.NoError(t, err)
			var want Encoding
			require.NoError(t, json.Unmarshal(wantJSON, &want))

			got, err := Encode(source)
			require.NoError(t, err)
			require.Equal(t, want, got)
			gotJSON, err := json.Marshal(got)
			require.NoError(t, err)
			require.JSONEq(t, string(wantJSON), string(gotJSON))
		})
	}
}

func TestEncodeVoteBoundaries(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*cmttypes.CommitSig)
	}{
		{
			name: "nil-missing-signature",
			change: func(sig *cmttypes.CommitSig) {
				sig.Signature = nil
			},
		},
		{
			name: "absent-signature",
			change: func(sig *cmttypes.CommitSig) {
				sig.BlockIDFlag = cmttypes.BlockIDFlagAbsent
				sig.Timestamp = time.Time{}
			},
		},
		{
			name: "absent-timestamp",
			change: func(sig *cmttypes.CommitSig) {
				sig.BlockIDFlag = cmttypes.BlockIDFlagAbsent
				sig.Signature = nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := readSource(t, "normal-mixed.input.json")
			test.change(&source.Signed.Commit.Signatures[3])
			_, err := Encode(source)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrUnsupported)
		})
	}
}

func TestEncoderFailuresRetryable(t *testing.T) {
	for _, set := range []string{"validators", "trusted"} {
		t.Run(set, func(t *testing.T) {
			source := readSource(t, "normal-mixed.input.json")
			values := source.Validators
			if set == "trusted" {
				values = source.TrustedNext
			}
			values[0].PubKey = cmtsecp.PubKey(make([]byte, 33))

			_, err := Encode(source)
			require.ErrorContains(t, err, "only Ed25519 is supported")
			require.NotErrorIs(t, err, ErrUnsupported)
		})
	}

	for _, round := range []int32{-1, 65536} {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			source := readSource(t, "normal-mixed.input.json")
			source.Signed.Commit.Round = round

			_, err := Encode(source)
			require.ErrorContains(t, err, "round exceeds the pinned Borsh header format")
			require.NotErrorIs(t, err, ErrUnsupported)
		})
	}
}

func TestSelectionUnsupportedReasons(t *testing.T) {
	for _, test := range []struct {
		name    string
		count   int
		signers int
		limit   int
		trust   *Fraction
		reason  string
	}{
		{
			name:    "fresh-quorum",
			count:   100,
			signers: 66,
			limit:   128,
			reason:  "signers hold 660000 of 1000000 voting power; the verifier needs more than 2/3",
		},
		{
			name:    "signature-limit",
			count:   200,
			signers: 200,
			limit:   128,
			reason:  "a quorum needs 134 of 200 signatures; at most 128 fit one header update",
		},
		{
			name:    "trusted-quorum",
			count:   4,
			signers: 3,
			limit:   4,
			trust: &Fraction{
				Numerator:   999,
				Denominator: 1000,
			},
			reason: "signers hold 30000 of 40000 trusted voting power; the verifier needs more than 999/1000",
		},
		{
			name:    "zero-denominator",
			count:   4,
			signers: 4,
			limit:   4,
			trust: &Fraction{
				Numerator:   1,
				Denominator: 0,
			},
			reason: "signers hold 40000 of 40000 trusted voting power; the verifier needs more than 1/0",
		},
		{
			name:    "zero-fraction",
			count:   4,
			signers: 4,
			limit:   4,
			trust: &Fraction{
				Numerator:   0,
				Denominator: 0,
			},
			reason: "signers hold 40000 of 40000 trusted voting power; the verifier needs more than 0/0",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, err := Fixture(readSource(t, "template.json"), test.count, test.signers, false)
			require.NoError(t, err)

			_, _, err = Select(source, Selection{
				Trigger: 1,
				Limit:   test.limit,
				Trust:   test.trust,
			})
			require.EqualError(t, err, test.reason)
			require.ErrorIs(t, err, ErrUnsupported)
			wrapped := fmt.Errorf("outer: %w", err)
			require.ErrorIs(t, wrapped, ErrUnsupported)
			var unsupported *UnsupportedError
			require.ErrorAs(t, wrapped, &unsupported)
			require.Equal(t, test.reason, unsupported.Reason)
		})
	}
}

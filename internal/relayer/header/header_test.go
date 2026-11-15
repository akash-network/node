package header

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"
)

func readSource(t *testing.T, name string) Source {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	source, err := ParseSource(raw)
	require.NoError(t, err)

	return source
}

func TestRustOracle(t *testing.T) {
	raw, err := os.ReadFile("testdata/manifest.json")
	require.NoError(t, err)
	var manifest struct {
		Cases []struct {
			Name      string
			Arguments []string
		}
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))

	for _, c := range manifest.Cases {
		t.Run(c.Name, func(t *testing.T) {
			count, err := strconv.Atoi(c.Arguments[1])
			require.NoError(t, err)
			signers, err := strconv.Atoi(c.Arguments[2])
			require.NoError(t, err)
			source, err := Fixture(readSource(t, "template.json"), count, signers, c.Arguments[3] == "weighted")
			require.NoError(t, err)
			compareOracle(t, source, c.Name+".json", true)
		})
	}
	t.Run("normal-mixed", func(t *testing.T) {
		compareOracle(t, readSource(t, "normal-mixed.input.json"), "normal-mixed.output.json", false)
	})
	t.Run("zero-priorities", func(t *testing.T) {
		compareOracle(t, readSource(t, "normal-mixed.zero-priorities.input.json"), "normal-mixed.output.json", false)
	})
}

func compareOracle(t *testing.T, source Source, file string, verify bool) {
	t.Helper()
	before, err := json.Marshal(source)
	require.NoError(t, err)

	got, err := Encode(source)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	var want Encoding
	require.NoError(t, json.Unmarshal(raw, &want))
	require.Equal(t, want, got)

	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, string(raw), string(encoded))

	after, err := json.Marshal(source)
	require.NoError(t, err)
	require.Equal(t, before, after)

	if verify {
		for _, receipt := range got.Receipts {
			require.True(t, ed25519.Verify(receipt.PubKey[:], receipt.Message, receipt.Signature[:]))
		}
	}
}

func TestLayoutRevision(t *testing.T) {
	for _, file := range []string{"programs/akt/Cargo.toml", "tools/header-encoder/Cargo.toml"} {
		raw, err := os.ReadFile(filepath.Join("../../..", "contracts/solana", file))
		require.NoError(t, err)
		found := false
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "ibc-contracts") {
				require.Contains(t, line, LayoutRevision)
				found = true
			}
		}
		require.True(t, found, file)
	}
}

func TestSelection(t *testing.T) {
	for _, test := range []struct {
		count, signers, trigger, limit, kept int
		weighted, failure                    bool
	}{
		{
			count:   100,
			signers: 100,
			trigger: 100,
			limit:   1,
			kept:    100,
		},
		{
			count:   100,
			signers: 100,
			trigger: 99,
			limit:   128,
			kept:    68,
		},
		{
			count:   100,
			signers: 67,
			trigger: 1,
			limit:   128,
			kept:    67,
		},
		{
			count:   100,
			signers: 66,
			trigger: 1,
			limit:   128,
			failure: true,
		},
		{
			count:   200,
			signers: 200,
			trigger: 100,
			limit:   128,
			failure: true,
		},
		{
			count:    100,
			signers:  40,
			trigger:  1,
			limit:    128,
			kept:     21,
			weighted: true,
		},
	} {
		t.Run(fmt.Sprint(test), func(t *testing.T) {
			source, err := Fixture(readSource(t, "template.json"), test.count, test.signers, test.weighted)
			require.NoError(t, err)

			before, err := json.Marshal(source)
			require.NoError(t, err)

			result, report, err := Select(source, Selection{
				Trigger: test.trigger,
				Limit:   test.limit,
			})
			if test.failure {
				require.ErrorIs(t, err, ErrUnsupported)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.signers, report.Present)
			require.Equal(t, test.kept, report.Kept)

			_, err = Encode(result)
			require.NoError(t, err)

			after, err := json.Marshal(source)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}

	source, err := Fixture(readSource(t, "template.json"), 4, 4, false)
	require.NoError(t, err)
	for _, v := range source.Validators {
		v.VotingPower = cmttypes.MaxTotalVotingPower / 4
	}
	source.TrustedNext = cloneValidators(source.Validators)
	selected, report, err := Select(source, Selection{
		Trigger: 1,
		Limit:   4,
		Trust: &Fraction{
			Numerator:   math.MaxUint64 / 3,
			Denominator: math.MaxUint64,
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, report.Kept)
	require.NotNil(t, selected.Signed)

	source.TrustedNext = source.TrustedNext[:1]
	source.Signed.Commit.Signatures[0] = cmttypes.NewCommitSigAbsent()
	_, _, err = Select(source, Selection{
		Trigger: 1,
		Limit:   4,
		Trust: &Fraction{
			Numerator:   1,
			Denominator: 3,
		},
	})
	require.ErrorIs(t, err, ErrUnsupported)
}

func TestBoundaries(t *testing.T) {
	source := readSource(t, "normal-mixed.input.json")
	for _, round := range []int32{-1, 65536} {
		copy := cloneSource(source)
		copy.Signed.Commit.Round = round
		_, err := Encode(copy)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrUnsupported)
	}

	copy := cloneSource(source)
	copy.Signed.Commit.Signatures = append(copy.Signed.Commit.Signatures, copy.Signed.Commit.Signatures[0])
	_, err := Encode(copy)
	require.Error(t, err)

	for _, timestamp := range []string{"-1", "18446744073709551616", "00", "+1"} {
		_, err := RuntimePacket(Encoding{
			Timestamp: timestamp,
			Root:      make([]byte, 32),
		})
		require.Error(t, err)
	}

	for _, timestamp := range []string{"0", "18446744073709551615"} {
		_, err := RuntimePacket(Encoding{
			Timestamp: timestamp,
			Root:      make([]byte, 32),
		})
		require.NoError(t, err)
	}

	require.Equal(t, "-1", timestamp(time.Unix(-1, 999999999)).String())
	require.Equal(t, "253402300799000000000", timestamp(time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)).String())

	got, err := Encode(source)
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	var decoded Encoding
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.True(t, reflect.DeepEqual(got, decoded))
	require.Error(t, json.Unmarshal([]byte(`{"header":"00"}`), &decoded))
}

func TestRustBoundaryOracles(t *testing.T) {
	for _, name := range []string{"preepoch", "year9999", "genesis-none", "zero-hashes"} {
		t.Run(name, func(t *testing.T) {
			compareOracle(t, readSource(t, name+".input.json"), name+".output.json", false)
		})
	}
}

func TestSkippedPriorityAndInputBoundaries(t *testing.T) {
	raw, err := os.ReadFile("testdata/normal-mixed.input.json")
	require.NoError(t, err)
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &object))

	for _, key := range []string{"validators", "trusted_validators"} {
		var validators []map[string]any
		require.NoError(t, json.Unmarshal(object[key], &validators))
		for _, validator := range validators {
			validator["proposer_priority"] = map[string]any{"ignored": true}
		}
		object[key], err = json.Marshal(validators)
		require.NoError(t, err)
	}

	modified, err := json.Marshal(object)
	require.NoError(t, err)
	source, err := ParseSource(modified)
	require.NoError(t, err)
	compareOracle(t, source, "normal-mixed.output.json", false)

	for _, raw := range []string{`{}`, `{"trusted_height":"1","revision":1}`, `{"trusted_height":1,"revision":1,"signed_header":null,"validators":[],"trusted_validators":[]}`} {
		_, err := ParseSource([]byte(raw))
		require.Error(t, err)
	}

	for _, count := range []int{0, 257} {
		_, err := Fixture(source, count, 0, false)
		require.Error(t, err)
	}

	_, err = Fixture(source, 1, 2, false)
	require.Error(t, err)
}

func TestDiagnosticAppHashRustOracles(t *testing.T) {
	for _, name := range []string{"apphash-empty", "apphash-short", "apphash-long"} {
		t.Run(name, func(t *testing.T) {
			source := readSource(t, name+".input.json")
			compareOracle(t, source, name+".output.json", false)
			encoded, err := Encode(source)
			require.NoError(t, err)
			require.Equal(t, []byte(source.Signed.Header.AppHash), encoded.Root)
			_, err = RuntimePacket(encoded)
			require.ErrorContains(t, err, "exactly 32 bytes")
		})
	}

	encoded, err := Encode(readSource(t, "normal-mixed.input.json"))
	require.NoError(t, err)
	packet, err := RuntimePacket(encoded)
	require.NoError(t, err)
	require.Equal(t, encoded.Root, packet.Root[:])
	encoded.Root[0] ^= 255
	require.NotEqual(t, encoded.Root, packet.Root[:])
}

func TestDiagnosticRootJSONPresence(t *testing.T) {
	encoded, err := Encode(readSource(t, "normal-mixed.input.json"))
	require.NoError(t, err)
	raw, err := json.Marshal(encoded)
	require.NoError(t, err)
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &object))
	var consensus map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(object["consensus"], &consensus))
	delete(consensus, "root")
	object["consensus"], err = json.Marshal(consensus)
	require.NoError(t, err)
	raw, err = json.Marshal(object)
	require.NoError(t, err)
	var decoded Encoding
	require.ErrorContains(t, json.Unmarshal(raw, &decoded), "missing consensus root")
}

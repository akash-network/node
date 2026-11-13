package solana_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	errTruncated  = "inconsistent truncated logs"
	errStack      = "inconsistent program log stack"
	dataPrefix    = "Program data: "
	truncatedLine = "Log truncated"
)

func TestProgramEvents(t *testing.T) {
	program := key(0x11)
	wrapper := key(0x22).String()
	token := key(0x33).String()
	budget := key(0x44).String()
	other := key(0x55).String()
	p := program.String()

	owner := bytes.Repeat([]byte{0x02}, 20)
	minted := outcomes()[0]
	payload := withDisc(aktsolana.DiscTransferResolved, statusBody(owner, 100, 5, minted.encoded))
	e := dataPrefix + base64.StdEncoding.EncodeToString(payload)

	want := aktsolana.TransferResolved{
		ID:             aktsolana.TransferID{Owner: owner, Height: 100, Sequence: 5},
		ProofHeight:    101,
		ResolvedAtSlot: 99,
		Outcome:        minted.want,
	}

	inv := func(id string, depth int) string { return fmt.Sprintf("Program %s invoke [%d]", id, depth) }
	ok := func(id string) string { return fmt.Sprintf("Program %s success", id) }

	t.Run("events emitted inside a CPI count only from an AKT program frame", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			logs     []string
			accepted bool
		}{
			{"AKT frame inside a wrapper CPI", []string{
				inv(wrapper, 1), inv(p, 2), inv(token, 3), ok(token), e,
				"Program " + p + " consumed 1 of 2 compute units", ok(p), ok(wrapper),
			}, true},
			{"AKT frame after a nested call returns", []string{
				inv(p, 1), inv(token, 2), ok(token), e, ok(p),
			}, true},
			{"wrapper frame replaying AKT event bytes", []string{
				inv(wrapper, 1), inv(p, 2), ok(p), e, ok(wrapper),
			}, false},
			{"nested program frame", []string{
				inv(p, 1), inv(token, 2), e, ok(token), ok(p),
			}, false},
			{"program log line", []string{
				inv(p, 1), "Program log: " + e[len(dataPrefix):], ok(p),
			}, false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := aktsolana.ProgramEvents(tc.logs, program)
				require.NoError(t, err)

				if tc.accepted {
					require.Equal(t, []aktsolana.TransferResolved{want}, got)
				} else {
					require.Empty(t, got)
				}
			})
		}
	})

	t.Run("truncated or unbalanced logs never supply evidence", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			logs []string
			err  string
		}{
			{nameTruncated, []string{inv(p, 1), e, truncatedLine}, ""},
			{"unterminated", []string{inv(p, 1), e}, errStack},
			{"wrong depth", []string{inv(p, 2), e, ok(p)}, errStack},
			{"mismatched exit", []string{inv(p, 1), e, ok(other)}, errStack},
			{"failed frame", []string{inv(p, 1), e, "Program " + p + " failed: custom program error: 0x1"}, errStack},
			{"data outside a frame", []string{inv(p, 1), ok(p), e}, ""},
			{"exit without invoke", []string{ok(p)}, errStack},
			{"huge depth", []string{"Program " + p + " invoke [99999999999999999999]"}, errStack},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := aktsolana.ProgramEvents(tc.logs, program)
				if tc.err != "" {
					require.EqualError(t, err, tc.err)
				} else {
					require.NoError(t, err)
				}

				require.Empty(t, got)
			})
		}
	})

	t.Run("log truncation keeps only top-level AKT frames that already succeeded", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			logs     []string
			accepted bool
			err      string
		}{
			{"after a completed AKT frame", []string{
				inv(budget, 1), ok(budget),
				inv(p, 1), inv(other, 2), ok(other), e,
				"Program " + p + " consumed 1 of 2 compute units", ok(p),
				inv(other, 1), "Program log: padding", truncatedLine,
			}, true, ""},
			{"inside the AKT frame", []string{inv(p, 1), e, inv(other, 2), truncatedLine}, false, ""},
			{"before the AKT frame", []string{inv(budget, 1), ok(budget), inv(other, 1), truncatedLine}, false, ""},
			{"lines after the truncation marker", []string{
				inv(budget, 1), ok(budget), inv(other, 1), truncatedLine, inv(p, 1), e, ok(p),
			}, false, errTruncated},
			{"completed AKT frame without the event", []string{inv(p, 1), ok(p), truncatedLine}, false, ""},
			{"AKT event inside a completed wrapper frame", []string{
				inv(other, 1), inv(p, 2), e, ok(p), ok(other), truncatedLine,
			}, false, ""},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := aktsolana.ProgramEvents(tc.logs, program)
				if tc.err != "" {
					require.EqualError(t, err, tc.err)
					require.Empty(t, got)

					return
				}

				require.NoError(t, err)

				if tc.accepted {
					require.Equal(t, []aktsolana.TransferResolved{want}, got)
				} else {
					require.Empty(t, got)
				}
			})
		}
	})

	t.Run("events keep log order across top-level frames", func(t *testing.T) {
		second := withDisc(aktsolana.DiscTransferResolved, statusBody(owner, 100, 6, []byte{outcomeTagVoided, 1}))
		logs := []string{
			inv(p, 1), e, ok(p),
			inv(other, 1), ok(other),
			inv(p, 1), dataPrefix + base64.StdEncoding.EncodeToString(second), ok(p),
		}

		got, err := aktsolana.ProgramEvents(logs, program)
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, uint64(5), got[0].ID.Sequence)
		require.Equal(t, uint64(6), got[1].ID.Sequence)
		require.NotNil(t, got[1].Outcome.Voided)
		require.Equal(t, aktsolana.VoidGovernance, *got[1].Outcome.Voided)
	})

	t.Run("other discriminators are ignored", func(t *testing.T) {
		unrelated := dataPrefix + base64.StdEncoding.EncodeToString(append(aktsolana.DiscTransferStatus[:], 1, 2, 3))
		short := dataPrefix + base64.StdEncoding.EncodeToString([]byte{1, 2, 3})

		got, err := aktsolana.ProgramEvents([]string{inv(p, 1), unrelated, short, dataPrefix + "!!not base64!!", ok(p)}, program)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("a malformed TransferResolved is an error", func(t *testing.T) {
		for name, data := range map[string][]byte{
			"trailing byte": append(append([]byte{}, payload...), 0),
			nameTruncated:   payload[:len(payload)-1],
			"bad outcome":   withDisc(aktsolana.DiscTransferResolved, statusBody(owner, 100, 5, []byte{2})),
		} {
			t.Run(name, func(t *testing.T) {
				logs := []string{inv(p, 1), dataPrefix + base64.StdEncoding.EncodeToString(data), ok(p)}

				got, err := aktsolana.ProgramEvents(logs, program)
				require.Error(t, err)
				require.Empty(t, got)
			})
		}
	})
}

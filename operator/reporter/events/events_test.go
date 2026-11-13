package events_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/events"
)

const owner = "akash1reporterowner"

var signature = []byte{0xfb, 0xff, 0x00, 0x01, 0x02}

const (
	signatureBase64 = "+/8AAQI="
	kindResolved    = "resolved"
)

func emit(t *testing.T, v any) string {
	t.Helper()

	var buf bytes.Buffer

	w := events.NewWriter(&buf)
	w.Emit(v)
	require.NoError(t, w.Close())

	out := buf.String()
	require.True(t, strings.HasSuffix(out, "\n"))
	require.Equal(t, 1, strings.Count(out, "\n"))

	return out
}

func TestEventsMatchAppendixA(t *testing.T) {
	const (
		contextSlot = 350000000
		slotLimit   = 350000600
		slot        = 349999000
	)

	observation := sinkv1.Observation{
		Outcome:              sinkv1.OutcomeMinted,
		Slot:                 slot,
		TransactionSignature: signature,
	}

	report, err := events.MessageOf(&sinkv1.MsgReportOutcome{
		Reporter:      owner,
		ID:            sinkv1.ID{Owner: owner, Height: 1200, Sequence: 4},
		PolicyVersion: 2,
		Round:         1,
		Observation:   observation,
	})
	require.NoError(t, err)

	veto, err := events.MessageOf(&sinkv1.MsgVetoVoid{
		Signer: owner,
		ID:     sinkv1.ID{Owner: owner, Height: 1200, Sequence: 5},
		Reason: "contradicts",
	})
	require.NoError(t, err)

	refund, err := events.MessageOf(&sinkv1.MsgFinalizeRefund{
		Executor: owner,
		ID:       sinkv1.ID{Owner: owner, Height: 1200, Sequence: 6},
	})
	require.NoError(t, err)

	tests := []struct {
		name  string
		event any
		want  string
	}{
		{
			name: "absent review",
			event: events.Review{
				ID:   owner + "/1200/3",
				Mode: "preview",
				Observed: events.Absent{
					Kind:        "absent",
					ContextSlot: events.U64(contextSlot),
					SlotLimit:   events.U64(slotLimit),
				},
				Messages: []events.Message{},
				Alerts:   []string{},
			},
			want: `{"id":"akash1reporterowner/1200/3","mode":"preview","audit":false,"observed":{"kind":"absent","contextSlot":"350000000","slotLimit":"350000600"},"messages":[],"alerts":[]}`,
		},
		{
			name: "resolved broadcast review",
			event: events.Review{
				ID:   owner + "/1200/4",
				Mode: "broadcast",
				Observed: events.Resolved{
					Kind:        kindResolved,
					ContextSlot: events.U64(contextSlot),
					Observation: events.ObservationOf(observation),
				},
				Messages: []events.Message{report},
				Alerts:   []string{},
			},
			want: `{"id":"akash1reporterowner/1200/4","mode":"broadcast","audit":false,"observed":{"kind":"resolved","contextSlot":"350000000","observation":{"outcome":1,"slot":"349999000","transactionSignature":"` + signatureBase64 + `"}},"messages":[{"typeUrl":"/akash.sink.v1.MsgReportOutcome","value":{"reporter":"akash1reporterowner","id":{"owner":"akash1reporterowner","height":"1200","sequence":"4"},"policyVersion":"2","round":"1","observation":{"outcome":1,"slot":"349999000","transactionSignature":"` + signatureBase64 + `"}}}],"alerts":[]}`,
		},
		{
			name: "audit early exit has no mode",
			event: events.Review{
				ID:       owner + "/1100/1",
				Audit:    true,
				Observed: kindResolved,
				Messages: []events.Message{},
				Alerts:   []string{},
			},
			want: `{"id":"akash1reporterowner/1100/1","audit":true,"observed":"resolved","messages":[],"alerts":[]}`,
		},
		{
			name: "review with veto, refund and plan alert",
			event: events.Review{
				ID:       owner + "/1200/5",
				Mode:     "preview",
				Observed: kindResolved,
				Messages: []events.Message{veto, refund},
				Alerts:   []string{"Transfer requires governance review"},
			},
			want: `{"id":"akash1reporterowner/1200/5","mode":"preview","audit":false,"observed":"resolved","messages":[{"typeUrl":"/akash.sink.v1.MsgVetoVoid","value":{"signer":"akash1reporterowner","id":{"owner":"akash1reporterowner","height":"1200","sequence":"5"},"reason":"contradicts"}},{"typeUrl":"/akash.sink.v1.MsgFinalizeRefund","value":{"executor":"akash1reporterowner","id":{"owner":"akash1reporterowner","height":"1200","sequence":"6"}}}],"alerts":["Transfer requires governance review"]}`,
		},
		{
			name:  "transaction hash",
			event: events.TxHash{ID: owner + "/1200/4", TransactionHash: "ABCDEF0123"},
			want:  `{"id":"akash1reporterowner/1200/4","transactionHash":"ABCDEF0123"}`,
		},
		{
			name:  "transfer error",
			event: events.TransferError{ID: owner + "/1200/5", Error: "resolution evidence unavailable in its block"},
			want:  `{"id":"akash1reporterowner/1200/5","error":"resolution evidence unavailable in its block"}`,
		},
		{
			name: "light client changed",
			event: events.LightClientChanged{
				Alert:    "light-client-changed",
				Previous: "Prev1111",
				Current:  "Curr2222",
				Slot:     events.U64(contextSlot),
			},
			want: `{"alert":"light-client-changed","previous":"Prev1111","current":"Curr2222","slot":"350000000"}`,
		},
		{
			name:  "light client mismatch",
			event: events.LightClientMismatch{Alert: "light-client-mismatch", Pinned: "Pin1111", Current: "Curr2222"},
			want:  `{"alert":"light-client-mismatch","pinned":"Pin1111","current":"Curr2222"}`,
		},
		{
			name:  "audit unavailable",
			event: events.AuditUnavailable{Alert: "audit-unavailable", Error: "audit query failed"},
			want:  `{"alert":"audit-unavailable","error":"audit query failed"}`,
		},
		{
			name:  "pass error",
			event: events.PassError{Error: "not a current reporter"},
			want:  `{"error":"not a current reporter"}`,
		},
		{
			name: "summary",
			event: events.Summary{
				Scanned:            100000,
				Audited:            100,
				Index:              "canonical",
				Failures:           0,
				LightClientProgram: "LightClient1111",
			},
			want: `{"scanned":100000,"audited":100,"index":"canonical","failures":0,"lightClientProgram":"LightClient1111"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.JSONEq(t, tt.want, emit(t, tt.event))
		})
	}
}

func TestAuditEarlyExitOmitsMode(t *testing.T) {
	line := emit(t, events.Review{ID: owner + "/1100/1", Audit: true, Observed: kindResolved})

	var got map[string]json.RawMessage

	require.NoError(t, json.Unmarshal([]byte(line), &got))
	require.NotContains(t, got, "mode")
}

func TestEmptySlicesRenderAsArrays(t *testing.T) {
	var review events.Review

	for _, v := range []any{review, &review} {
		line := emit(t, v)

		require.Contains(t, line, `"messages":[]`)
		require.Contains(t, line, `"alerts":[]`)
	}
}

func TestMessageOfUnsupported(t *testing.T) {
	_, err := events.MessageOf(&sinkv1.MsgUpdateParams{})
	require.Error(t, err)

	_, err = events.MessageOf(nil)
	require.Error(t, err)

	_, err = events.MessageOf((*sinkv1.MsgVetoVoid)(nil))
	require.Error(t, err)
}

func TestObservationOfEmptySignature(t *testing.T) {
	require.Equal(t, events.Observation{Outcome: 2, Slot: "18446744073709551615"},
		events.ObservationOf(sinkv1.Observation{Outcome: sinkv1.OutcomeVoided, Slot: 1<<64 - 1}))
}

func TestWriterDoesNotEscapeHTML(t *testing.T) {
	line := emit(t, events.PassError{Error: "a<b>&c"})
	require.Equal(t, `{"error":"a<b>&c"}`+"\n", line)
}

func TestWriterEmitAfterClose(t *testing.T) {
	var buf bytes.Buffer

	w := events.NewWriter(&buf)
	w.Emit(events.PassError{Error: "first"})
	require.NoError(t, w.Close())
	require.NotPanics(t, func() { w.Emit(events.PassError{Error: "late"}) })
	require.NoError(t, w.Close())
	require.Equal(t, `{"error":"first"}`+"\n", buf.String())
}

func TestWriterConcurrent(t *testing.T) {
	const (
		goroutines = 100
		perRoutine = 100
	)

	var buf bytes.Buffer

	w := events.NewWriter(&buf)

	var wg sync.WaitGroup

	for g := range goroutines {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range perRoutine {
				w.Emit(events.TransferError{ID: owner + "/1/1", Error: strings.Repeat("x", g+1)})
			}
		}()
	}

	wg.Wait()
	require.NoError(t, w.Close())

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, goroutines*perRoutine)

	for _, line := range lines {
		var got events.TransferError

		require.NoError(t, json.Unmarshal([]byte(line), &got), line)
		require.Equal(t, owner+"/1/1", got.ID)
	}
}

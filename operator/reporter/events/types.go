package events

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
)

// Review is the per-transfer review event. Mode is empty for the audit early exit.
type Review struct {
	ID       string    `json:"id"`
	Mode     string    `json:"mode,omitempty"`
	Audit    bool      `json:"audit"`
	Observed any       `json:"observed"`
	Messages []Message `json:"messages"`
	Alerts   []string  `json:"alerts"`
}

// MarshalJSON renders nil Messages and Alerts as empty arrays, like the TS reporter.
func (r Review) MarshalJSON() ([]byte, error) {
	type plain Review

	if r.Messages == nil {
		r.Messages = []Message{}
	}

	if r.Alerts == nil {
		r.Alerts = []string{}
	}

	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(plain(r)); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Absent is the observed value when Solana holds no finalized status for the transfer.
type Absent struct {
	Kind        string `json:"kind"`
	ContextSlot string `json:"contextSlot"`
	SlotLimit   string `json:"slotLimit"`
}

// Resolved is the observed value when Solana holds a finalized status for the transfer.
type Resolved struct {
	Kind        string      `json:"kind"`
	ContextSlot string      `json:"contextSlot"`
	Observation Observation `json:"observation"`
}

// Observation is the JSON form of a sink observation.
type Observation struct {
	Outcome              int32  `json:"outcome"`
	Slot                 string `json:"slot"`
	TransactionSignature string `json:"transactionSignature"`
}

// Message is a planned sink message: its type URL and its TS-shaped value.
type Message struct {
	TypeURL string `json:"typeUrl"`
	Value   any    `json:"value"`
}

// TxHash reports the hash of a transaction carrying a message of the transfer.
type TxHash struct {
	ID              string `json:"id"`
	TransactionHash string `json:"transactionHash"`
}

// TransferError reports a failure while reviewing or sending for one transfer.
type TransferError struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// PassError reports a failure of the whole pass.
type PassError struct {
	Error string `json:"error"`
}

// LightClientChanged alerts that the Solana light client program changed.
type LightClientChanged struct {
	Alert    string `json:"alert"`
	Previous string `json:"previous"`
	Current  string `json:"current"`
	Slot     string `json:"slot"`
}

// LightClientMismatch alerts that the light client program differs from the pinned one.
type LightClientMismatch struct {
	Alert   string `json:"alert"`
	Pinned  string `json:"pinned"`
	Current string `json:"current"`
}

// AuditUnavailable alerts that the audit discovery query failed.
type AuditUnavailable struct {
	Alert string `json:"alert"`
	Error string `json:"error"`
}

// Summary is the last event of a pass.
type Summary struct {
	Scanned            int    `json:"scanned"`
	Audited            int    `json:"audited"`
	Index              string `json:"index"`
	Failures           int    `json:"failures"`
	LightClientProgram string `json:"lightClientProgram"`
}

type messageID struct {
	Owner    string `json:"owner"`
	Height   string `json:"height"`
	Sequence string `json:"sequence"`
}

type reportOutcomeValue struct {
	Reporter      string      `json:"reporter"`
	ID            messageID   `json:"id"`
	PolicyVersion string      `json:"policyVersion"`
	Round         string      `json:"round"`
	Observation   Observation `json:"observation"`
}

type vetoVoidValue struct {
	Signer string    `json:"signer"`
	ID     messageID `json:"id"`
	Reason string    `json:"reason"`
}

type finalizeRefundValue struct {
	Executor string    `json:"executor"`
	ID       messageID `json:"id"`
}

// U64 renders a 64-bit value as a decimal string.
func U64(v uint64) string {
	return strconv.FormatUint(v, 10)
}

// ObservationOf renders a chain observation with its slot as a decimal string and its signature as base64.
func ObservationOf(o sinkv1.Observation) Observation {
	return Observation{
		Outcome:              int32(o.Outcome),
		Slot:                 U64(o.Slot),
		TransactionSignature: base64.StdEncoding.EncodeToString(o.TransactionSignature),
	}
}

// MessageOf renders a sink message in the TS reporter's shape.
func MessageOf(msg sdk.Msg) (Message, error) {
	switch m := msg.(type) {
	case *sinkv1.MsgReportOutcome:
		if m == nil {
			break
		}

		return Message{TypeURL: sdk.MsgTypeURL(m), Value: reportOutcomeValue{
			Reporter:      m.Reporter,
			ID:            idOf(m.ID),
			PolicyVersion: U64(m.PolicyVersion),
			Round:         U64(m.Round),
			Observation:   ObservationOf(m.Observation),
		}}, nil
	case *sinkv1.MsgVetoVoid:
		if m == nil {
			break
		}

		return Message{TypeURL: sdk.MsgTypeURL(m), Value: vetoVoidValue{
			Signer: m.Signer,
			ID:     idOf(m.ID),
			Reason: m.Reason,
		}}, nil
	case *sinkv1.MsgFinalizeRefund:
		if m == nil {
			break
		}

		return Message{TypeURL: sdk.MsgTypeURL(m), Value: finalizeRefundValue{
			Executor: m.Executor,
			ID:       idOf(m.ID),
		}}, nil
	}

	return Message{}, fmt.Errorf("unsupported sink message %T", msg)
}

func idOf(id sinkv1.ID) messageID {
	return messageID{
		Owner:    id.Owner,
		Height:   strconv.FormatInt(id.Height, 10),
		Sequence: U64(id.Sequence),
	}
}

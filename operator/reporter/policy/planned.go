package policy

import (
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

// MsgKind identifies the kind of a sink message. The values are the kind bytes of the nonce slot hash.
type MsgKind uint8

const (
	MsgVote   MsgKind = 1
	MsgVeto   MsgKind = 2
	MsgRefund MsgKind = 3
)

// Key identifies a message for the purpose of executing it at most once.
// A vote is keyed by outcome, round and policy version. A veto and a refund are keyed by the transfer alone:
// MsgVetoVoid carries no round, so a veto planned in one round and one planned after a governance resolution share a key.
type Key struct {
	ID            string
	Kind          MsgKind
	Outcome       sinkv1.Outcome
	Round         uint64
	PolicyVersion uint64
}

// KeyOf returns the key of a message the policy plans for the transfer. A message of any other type gets a key with kind zero.
func KeyOf(id sinkid.ID, msg sdk.Msg) Key {
	key := Key{ID: id.String()}

	switch m := msg.(type) {
	case *sinkv1.MsgReportOutcome:
		key.Kind = MsgVote
		key.Outcome = m.Observation.Outcome
		key.Round = m.Round
		key.PolicyVersion = m.PolicyVersion
	case *sinkv1.MsgVetoVoid:
		key.Kind = MsgVeto
	case *sinkv1.MsgFinalizeRefund:
		key.Kind = MsgRefund
	}

	return key
}

// Planned is a message the policy planned from one snapshot.
type Planned struct {
	ID  sinkid.ID
	Msg sdk.Msg
	Key Key
	// PlanHeight and PlanTime are the height and block time of the snapshot the message was planned from.
	PlanHeight int64
	PlanTime   time.Time
}

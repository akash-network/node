package tx

import (
	"regexp"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"pkg.akt.dev/node/v2/operator/reporter/policy"
)

// pruneRounds is the number of failing simulations after which the remaining messages are deferred.
const pruneRounds = 3

var messageIndex = regexp.MustCompile(`message index: (\d+)`)

// Dropped is a message that simulation rejected, with the rejection.
type Dropped struct {
	Planned policy.Planned
	Err     error
}

// PruneResult splits a batch by simulation (spec §6.4).
type PruneResult struct {
	// Kept simulated successfully together, using GasUsed.
	Kept    []policy.Planned
	GasUsed uint64
	Dropped []Dropped
	// Deferred holds the messages still left after three failing rounds.
	Deferred []policy.Planned
	// Unavailable is a simulation error that is not the chain's answer, such as a transport failure. Pruning stopped
	// at it, and Retry holds the messages it had not decided yet.
	Unavailable error
	Retry       []policy.Planned
}

// Prune simulates batch and drops the message each failure names, for up to three failing rounds. A failure
// without a message index, or with one outside the batch, drops every remaining message. An error that is not the
// chain's answer stops pruning without dropping anything more.
func Prune(sim func([]sdk.Msg) (uint64, error), batch []policy.Planned) PruneResult {
	var res PruneResult

	remaining := append([]policy.Planned(nil), batch...)

	for round := 0; round < pruneRounds && len(remaining) > 0; round++ {
		msgs := make([]sdk.Msg, len(remaining))
		for i, p := range remaining {
			msgs[i] = p.Msg
		}

		gas, err := sim(msgs)
		if err == nil {
			res.Kept = remaining
			res.GasUsed = gas

			return res
		}

		if unavailable(err) {
			res.Unavailable = err
			res.Retry = remaining

			return res
		}

		idx, ok := failedIndex(err, len(remaining))
		if !ok {
			for _, p := range remaining {
				res.Dropped = append(res.Dropped, Dropped{Planned: p, Err: err})
			}

			return res
		}

		res.Dropped = append(res.Dropped, Dropped{Planned: remaining[idx], Err: err})
		remaining = append(remaining[:idx], remaining[idx+1:]...)
	}

	if len(remaining) > 0 {
		res.Deferred = remaining
	}

	return res
}

// unavailable reports a simulation error the chain did not return: one without a gRPC status, or a status that a
// transport, a deadline, a cancellation or an overloaded node produces.
func unavailable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return true
	}

	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.ResourceExhausted:
		return true
	}

	return false
}

func failedIndex(err error, n int) (int, bool) {
	m := messageIndex.FindStringSubmatch(err.Error())
	if m == nil {
		return 0, false
	}

	idx, perr := strconv.Atoi(m[1])
	if perr != nil || idx >= n {
		return 0, false
	}

	return idx, true
}

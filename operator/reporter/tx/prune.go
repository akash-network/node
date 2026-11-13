package tx

import (
	"regexp"
	"strconv"

	sdk "github.com/cosmos/cosmos-sdk/types"

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
}

// Prune simulates batch and drops the message each failure names, for up to three failing rounds. A failure
// without a message index, or with one outside the batch, drops every remaining message.
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

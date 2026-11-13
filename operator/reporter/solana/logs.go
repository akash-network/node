package solana

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/gagliardetto/solana-go"
)

const (
	logKey        = "[1-9A-HJ-NP-Za-km-z]{32,44}"
	logDataPrefix = "Program data: "
	logTruncated  = "Log truncated"
)

var (
	errTruncatedLogs = errors.New("inconsistent truncated logs")
	errLogStack      = errors.New("inconsistent program log stack")

	logInvoke = regexp.MustCompile(`^Program (` + logKey + `) invoke \[([1-9][0-9]*)\]$`)
	logExit   = regexp.MustCompile(`^Program (` + logKey + `) (success|failed: .*)$`)
)

type closedFrame struct {
	root   string
	events []TransferResolved
}

// ProgramEvents returns the TransferResolved events emitted by program frames, in log order.
//
// Anchor's event parser ignores events emitted inside a CPI, so each "Program data:" line is attributed to the
// executing frame from the invoke and exit lines instead. Unbalanced logs cannot be attributed and are an error.
// A later instruction can exhaust the log limit; then only top-level program frames that already closed with
// success count. Data lines with other discriminators are ignored; a TransferResolved that does not decode is an error.
func ProgramEvents(logs []string, program solana.PublicKey) ([]TransferResolved, error) {
	id := program.String()

	var (
		stack  []string
		closed []closedFrame
		events []TransferResolved
	)

	for index, line := range logs {
		if line == logTruncated {
			if index != len(logs)-1 {
				return nil, errTruncatedLogs
			}

			return flatten(closed, id), nil
		}

		if m := logInvoke.FindStringSubmatch(line); m != nil {
			depth, err := strconv.Atoi(m[2])
			if err != nil || depth != len(stack)+1 {
				return nil, errLogStack
			}

			stack = append(stack, m[1])

			continue
		}

		if m := logExit.FindStringSubmatch(line); m != nil {
			// A successful transaction cannot contain a failed invocation.
			if m[2] != "success" || len(stack) == 0 || stack[len(stack)-1] != m[1] {
				return nil, errLogStack
			}

			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				closed = append(closed, closedFrame{root: m[1], events: events})
				events = nil
			}

			continue
		}

		if !strings.HasPrefix(line, logDataPrefix) || len(stack) == 0 || stack[len(stack)-1] != id {
			continue
		}

		event, ok, err := decodeLogEvent(line[len(logDataPrefix):])
		if err != nil {
			return nil, err
		}

		if ok {
			events = append(events, event)
		}
	}

	if len(stack) != 0 {
		return nil, errLogStack
	}

	return flatten(closed, ""), nil
}

// flatten joins the events of closed frames, keeping only frames rooted at root when it is not empty.
func flatten(closed []closedFrame, root string) []TransferResolved {
	var out []TransferResolved

	for _, frame := range closed {
		if root == "" || frame.root == root {
			out = append(out, frame.events...)
		}
	}

	return out
}

// decodeLogEvent decodes base64 event data. Data that is not base64 or lacks the TransferResolved discriminator is not
// an AKT event; data with that discriminator must decode.
func decodeLogEvent(encoded string) (TransferResolved, bool, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(data) < discriminatorSize || [8]byte(data) != DiscTransferResolved {
		return TransferResolved{}, false, nil
	}

	event, err := DecodeTransferResolved(data)
	if err != nil {
		return TransferResolved{}, false, err
	}

	return event, true, nil
}

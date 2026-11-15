package relayer

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func (s *service) emit(alert bool, fields map[string]any) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()
	writer := s.deps.streams.Out
	if alert {
		writer = s.deps.streams.Err
	}
	if writer == nil {
		writer = io.Discard
	}
	if err := json.NewEncoder(writer).Encode(fields); err != nil && !alert && s.deps.streams.Err != nil {
		fmt.Fprintf(s.deps.streams.Err, "write relayer event: %v\n", err)
	}
}

func timed[T any](s *service, stage string, id *sink.ID, action func() (T, error)) (result T, err error) {
	started := time.Now()
	defer func() {
		fields := map[string]any{
			"event":     "relay-stage-duration",
			"stage":     stage,
			"elapsedMs": math.Round(float64(time.Since(started)) / float64(time.Millisecond)),
			"completed": err == nil,
		}
		if id != nil {
			fields["id"] = id.String()
		}
		s.emit(false, fields)
	}()
	return action()
}

func timedAction(s *service, stage string, id *sink.ID, action func() error) error {
	_, err := timed(s, stage, id, func() (struct{}, error) {
		return struct{}{}, action()
	})
	return err
}

func (s *service) alertDue(key string) bool {
	now := s.deps.now().UnixMilli()
	previous, exists := s.alerts[key]
	if exists && now-previous < 15*60_000 {
		return false
	}
	if s.alerts == nil {
		s.alerts = make(map[string]int64)
	}
	s.alerts[key] = now
	return true
}

func (s *service) clearAlert(key string) { delete(s.alerts, key) }

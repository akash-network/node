package relayer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func (s *service) runLocal(ctx context.Context, cmd command) error {
	if !s.cfg.local {
		return fmt.Errorf("local command requires an isolated local configuration")
	}
	work := context.WithoutCancel(ctx)
	switch cmd.localArgs[0] {
	case "reset-budget":
		return s.resetBudget(work, cmd.reset)
	case "bootstrap":
		return s.bootstrap(work)
	case "check-invalid-header":
		return s.checkInvalidHeader(work)
	case "tick":
		return s.tick(work)
	case "run":
		return s.run(ctx, cmd.watch)
	case "update":
		height, err := parseHeight(cmd.localArgs[1])
		if err != nil {
			return err
		}
		return s.update(work, height, cmd.tamper, nil)
	case "relay":
		id, err := native.ParseID(cmd.localArgs[1])
		if err != nil {
			return err
		}
		var height headerHeight
		if len(cmd.localArgs) == 3 {
			height, err = parseHeight(cmd.localArgs[2])
		} else {
			followed, followErr := s.follow(work)
			if followErr != nil {
				return followErr
			}
			client, clientErr := s.reader.Client(work, followed)
			if clientErr != nil {
				return clientErr
			}
			height, err = parseHeight(strconv.FormatUint(client.LatestHeight.RevisionHeight, 10))
		}
		if err != nil {
			return err
		}
		result, err := s.relay(work, id, height)
		if err != nil {
			return err
		}
		s.emit(false, map[string]any{
			"event":  "relay-result",
			"id":     id.String(),
			"result": result,
		})
		return nil
	default:
		return fmt.Errorf("unknown local command")
	}
}

func parseHeight(value string) (headerHeight, error) {
	height, err := strconv.ParseInt(value, 10, 64)
	if err != nil || height <= 0 || height > maxSafeInteger || strconv.FormatInt(height, 10) != value {
		return 0, fmt.Errorf("expected a positive safe integer header height")
	}
	return headerHeight(height), nil
}

func (s *service) checkInvalidHeader(ctx context.Context) error {
	followed, err := s.follow(ctx)
	if err != nil {
		return err
	}
	before, err := s.reader.Client(ctx, followed)
	if err != nil {
		return err
	}
	trusted := before.LatestHeight.RevisionHeight
	deadline := s.deps.now().Add(30 * time.Second)
	var target headerHeight
	for {
		snapshot, err := s.native.Snapshot(ctx)
		if err != nil {
			return err
		}
		target = headerHeight(snapshot.Height - 1)
		if target > 0 && uint64(target) > trusted {
			break
		}
		if !s.deps.now().Before(deadline) {
			return fmt.Errorf("Akash did not produce a newer canonical header")
		}
		if err := s.deps.sleep(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	err = s.update(ctx, target, true, nil)
	if err == nil || !strings.Contains(err.Error(), "UpdateClientFailed") {
		return fmt.Errorf("expected UpdateClientFailed for altered app root; received %v", err)
	}
	after, err := s.reader.Client(ctx, followed)
	if err != nil {
		return err
	}
	if after.LatestHeight.RevisionHeight != trusted {
		return fmt.Errorf("rejected header changed trusted height")
	}
	consensus, err := s.reader.ConsensusConfirmed(ctx, followed, uint64(target))
	if err != nil {
		return err
	}
	if consensus != nil {
		return fmt.Errorf("rejected header left a trusted consensus state")
	}
	if err := s.update(ctx, target, false, nil); err != nil {
		return err
	}
	_, err = fmt.Fprintln(s.deps.streams.Out, "Altered app root rejected; authentic signed header accepted.")
	return err
}

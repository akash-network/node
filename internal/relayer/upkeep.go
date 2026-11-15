package relayer

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

type relayHeader struct {
	followed bridge.Followed
	height   headerHeight
}

// headerUpkeep belongs to one tick. Checkpoints run only between serial work
// units, after proof workers join and outside every payer action.
type headerUpkeep struct {
	service       *service
	expected      bridge.Followed
	current       relayHeader
	state         native.Snapshot
	nextCheck     time.Time
	paymentsDirty bool
}

func (u *headerUpkeep) validateFollow(found bridge.Followed) error {
	if found.Program != u.expected.Program || found.AccessManager != u.expected.AccessManager || found.ChainID != u.expected.ChainID {
		return fmt.Errorf("light client identity changed during tick")
	}
	return nil
}

func (u *headerUpkeep) check(ctx context.Context) error {
	if u.service.deps.now().Before(u.nextCheck) {
		return nil
	}
	return u.refresh(ctx, nil)
}

func (u *headerUpkeep) header(ctx context.Context) (relayHeader, error) {
	if err := u.check(ctx); err != nil {
		return relayHeader{}, err
	}
	return u.current, nil
}

func (u *headerUpkeep) retryReady() bool {
	now := u.service.deps.now().UnixMilli()
	return now >= u.service.headerRetry.RetryAt && now >= u.service.unsupported.RetryAt
}

// Recovery errors must escape the tick, not become header retry events.
// Call this only for an admitted resume/update, after its retry/access gates.
func (u *headerUpkeep) beforeHeader(ctx context.Context) (bool, error) {
	if !u.paymentsDirty {
		return true, nil
	}
	s := u.service
	if err := s.recoverPayments(ctx); err != nil {
		return false, err
	}
	u.paymentsDirty = false
	// Recovery may take time or re-follow a deployment. Recheck identity and
	// authority before admitting another paid operation.
	found, err := s.follow(ctx)
	if err != nil {
		return false, err
	}
	if err := u.validateFollow(found); err != nil {
		return false, err
	}
	allowed, err := s.relayerAccess(ctx, found)
	return allowed != bridge.AccessNone, err
}

func (u *headerUpkeep) observe(ctx context.Context, followed bridge.Followed) (bridge.ClientState, error) {
	if u.service.followed != nil {
		if err := u.validateFollow(*u.service.followed); err != nil {
			return bridge.ClientState{}, err
		}
	}
	client, err := u.service.reader.Client(ctx, followed)
	if err != nil {
		return bridge.ClientState{}, err
	}
	if client.LatestHeight.RevisionHeight > maxSafeInteger || client.TrustingPeriod > maxSafeInteger {
		return bridge.ClientState{}, fmt.Errorf("light client height or trusting period exceeds safe integer")
	}
	u.current = relayHeader{
		followed: resolutionClient(followed, client),
		height:   headerHeight(client.LatestHeight.RevisionHeight),
	}
	return client, nil
}

func (u *headerUpkeep) refresh(ctx context.Context, required []headerHeight) error {
	s := u.service
	defer func() { u.nextCheck = s.deps.now().Add(s.cfg.poll) }()
	followed, err := s.follow(ctx)
	if err != nil {
		return err
	}
	if err := u.validateFollow(followed); err != nil {
		return err
	}
	allowed, err := s.relayerAccess(ctx, followed)
	if err != nil {
		return err
	}
	if allowed != bridge.AccessNone && u.retryReady() {
		pending, err := s.store.loadActive()
		if err != nil {
			return err
		}
		if pending != nil {
			admitted, err := u.beforeHeader(ctx)
			if err != nil {
				return err
			}
			if admitted {
				if _, err := s.headerStep(func() error { return s.resumeHeader(ctx, &u.expected) }, 0); err != nil {
					return err
				}
			} else {
				allowed = bridge.AccessNone
			}
		}
	}
	if s.followed != nil {
		if err := u.validateFollow(*s.followed); err != nil {
			return err
		}
	}
	if err := s.reader.Active(ctx, followed); err != nil {
		if allowed == bridge.AccessMember {
			if pruneErr := s.pruneConsensus(ctx, followed); pruneErr != nil {
				return pruneErr
			}
		}
		if reclaimErr := s.reclaimLookups(ctx); reclaimErr != nil {
			return reclaimErr
		}
		return err
	}
	client, err := u.observe(ctx, followed)
	if err != nil {
		return err
	}
	state, err := timed(s, "state-query", nil, func() (native.Snapshot, error) {
		return s.native.Snapshot(ctx)
	})
	if err != nil {
		return err
	}
	u.state = state
	current := u.current.height
	target := headerHeight(state.Height - 1)
	consensus, err := s.reader.ConsensusConfirmed(ctx, followed, uint64(current))
	if err != nil {
		return err
	}
	if consensus == nil {
		return fmt.Errorf("latest consensus unavailable")
	}
	now, err := s.reader.Clock(ctx)
	if err != nil {
		return err
	}
	age := new(big.Int).Sub(big.NewInt(now), new(big.Int).SetUint64(consensus.Timestamp/1_000_000_000))
	ageSeconds, _ := new(big.Float).SetInt(age).Float64()
	reason := headerRefresh(refreshInput{
		Latest:          current,
		Target:          target,
		Required:        required,
		AgeSeconds:      ageSeconds,
		TrustingSeconds: float64(client.TrustingPeriod),
		Fraction:        s.cfg.refreshFraction,
	})
	if reason == "" {
		return nil
	}
	if allowed == bridge.AccessNone {
		if s.alertDue("header-blocked") {
			s.emit(true, map[string]any{
				"event":       "relay-header-blocked",
				"reason":      reason,
				"payer":       s.payer.PublicKey().String(),
				"lightClient": followed.Program.String(),
				"trusted":     current,
				"target":      target,
			})
		}
		return nil
	}
	s.clearAlert("header-blocked")
	if u.retryReady() {
		admitted, err := u.beforeHeader(ctx)
		if err != nil {
			return err
		}
		if admitted {
			updated, err := s.headerStep(func() error {
				s.emit(false, map[string]any{
					"event":   "header-refresh",
					"reason":  reason,
					"trusted": current,
					"target":  target,
				})
				return timedAction(s, "header-update", nil, func() error {
					return s.update(ctx, target, false, &u.expected)
				})
			}, target)
			if err != nil {
				return err
			}
			if updated {
				s.headerRetry = retryState{}
			}
		}
	}
	// A failed/skipped attempt is not progress; another operator may have won.
	_, err = u.observe(ctx, followed)
	return err
}

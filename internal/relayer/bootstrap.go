package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	solana "github.com/gagliardetto/solana-go"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

const localClientProgram = "HqPcGpVHxNNFfVatjhG78dFVMwjyZixoKPdZSt3d3TdD"
const localAccessProgram = "4fMih2CidrXPeRx77kj3QcuBZwREYtxEbXjURUgadoe1"

func (s *service) bootstrap(ctx context.Context) error {
	if !s.cfg.local {
		return fmt.Errorf("only the local stack bootstraps")
	}
	err := s.withRequiredPayer(ctx, func(p *payerSession) error {
		return p.bootstrap(ctx)
	})
	return err
}

func (p *payerSession) bootstrap(ctx context.Context) error {
	s := p.service
	local, err := bridge.DeriveClient(solana.MustPublicKeyFromBase58(localClientProgram), solana.MustPublicKeyFromBase58(localAccessProgram))
	if err != nil {
		return err
	}

	payer := s.payer.PublicKey()
	send := func(ix solana.Instruction, err error) error {
		if err != nil {
			return err
		}
		result, err := p.sendBootstrap(ctx, []solana.Instruction{ix})
		if err != nil {
			return err
		}
		if transactionFailed(result) {
			return fmt.Errorf("bootstrap failed: %s", result.failure)
		}
		return nil
	}

	manager, err := s.reader.AccessState(ctx, local)
	if err != nil {
		return err
	}
	if manager == nil {
		if err := send(bridge.InitializeAccess(local.AccessManager, payer, payer, payer)); err != nil {
			return err
		}
		if err := send(bridge.GrantRole(local.AccessManager, payer, payer, 1)); err != nil {
			return err
		}
	}

	launch, err := s.reader.Launch(ctx)
	if err != nil {
		return err
	}
	if launch == nil {
		if err := send(s.deployment.Initialize(payer)); err != nil {
			return err
		}
	}

	current, err := s.reader.ClientNullable(ctx, local)
	if err != nil {
		return err
	}
	if current == nil {
		state, err := s.native.Snapshot(ctx)
		if err != nil {
			return err
		}
		for state.Height < 4 {
			if err := s.deps.sleep(ctx, 500*time.Millisecond); err != nil {
				return err
			}
			state, err = s.native.Snapshot(ctx)
			if err != nil {
				return err
			}
		}

		checkpoint := headerHeight(state.Height - 1)
		packet, blockHash, err := s.headerPacket(ctx, checkpoint-1, checkpoint, 1, nil)
		if err != nil {
			return err
		}

		initial := bridge.ClientState{
			ChainID:               s.cfg.chainID,
			TrustLevelNumerator:   1,
			TrustLevelDenominator: 3,
			TrustingPeriod:        86400,
			UnbondingPeriod:       1814400,
			MaxClockDrift:         10,
			LatestHeight: bridge.Height{
				RevisionNumber: 1,
				RevisionHeight: uint64(checkpoint),
			},
		}

		if err := send(local.Initialize(payer, payer, initial, packet)); err != nil {
			return err
		}
		if err := s.store.saveCheckpoint(int64(checkpoint), blockHash, hex.EncodeToString(packet.Root[:])); err != nil {
			return err
		}

		s.emit(false, map[string]any{
			"event":     "trusted-local-checkpoint",
			"height":    checkpoint,
			"blockHash": blockHash,
		})
	}

	state, err := s.reader.Client(ctx, local)
	if err != nil {
		return err
	}
	if state.ChainID != s.cfg.chainID {
		return fmt.Errorf("wrong local client chain")
	}
	local.Latest = state.LatestHeight.RevisionHeight

	configured, err := s.reader.Bridge(ctx)
	if err != nil {
		return err
	}
	if configured == nil {
		consensus, err := s.reader.ConsensusConfirmed(ctx, local, local.Latest)
		if err != nil {
			return err
		}
		if consensus == nil {
			return fmt.Errorf("local checkpoint consensus unavailable")
		}

		deadline := s.deps.now().Add(540 * time.Second)
		announced := false
		for {
			now, err := s.reader.Clock(ctx)
			if err != nil {
				return err
			}
			clockWithDrift := new(big.Int).Add(big.NewInt(now), new(big.Int).SetUint64(state.MaxClockDrift))
			clockWithDrift.Mul(clockWithDrift, big.NewInt(1_000_000_000))
			if new(big.Int).SetUint64(consensus.Timestamp).Cmp(clockWithDrift) <= 0 {
				break
			}
			if !s.deps.now().Before(deadline) {
				return fmt.Errorf("Solana clock has not reached the existing checkpoint; leave the validator running and retry")
			}
			if !announced {
				fmt.Fprintln(s.deps.streams.Out, "Waiting for the retained Solana clock to reach the existing checkpoint.")
				announced = true
			}
			if err := s.deps.sleep(ctx, time.Second); err != nil {
				return err
			}
		}

		if err := send(s.deployment.ConfigureBridge(payer, local, s.cfg.chainID, bridge.MintPolicy{
			PerTransferMax: 1_000_000_000_000,
			DailyMintCap:   10_000_000_000_000,
			Vaults:         []solana.PublicKey{},
		})); err != nil {
			return err
		}
		if err := send(s.deployment.SetBridgeEnabled(payer, true)); err != nil {
			return err
		}
	}

	launch, err = s.reader.Launch(ctx)
	if err != nil {
		return err
	}
	if launch == nil {
		return fmt.Errorf("launch config unavailable after bootstrap")
	}

	s.emit(false, map[string]any{
		"event":   "bridge-state",
		"enabled": launch.BridgeEnabled,
		"mint":    s.deployment.Mint.String(),
		"height":  fmt.Sprint(state.LatestHeight.RevisionHeight),
	})
	return nil
}

package relayer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os"
	"strconv"
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	commitment "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	ics23 "github.com/cosmos/ics23/go"
	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
	"pkg.akt.dev/node/v2/internal/relayer/native"
)

func planningHex(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	require.NoError(t, err)
	return raw
}

func TestPlanningPolicyTSOracle(t *testing.T) {
	raw, err := os.ReadFile("testdata/planning-policy.json")
	require.NoError(t, err)
	var fixture struct {
		Checks []struct{ Name, Before, After, Reason, Unbacked string }
		Order  []struct {
			Payer   string
			Entries []struct {
				ID                   string
				Status               sink.Status
				Hold                 sink.Hold
				Retained, Authorized bool
			}
			Result []string
		}
		Refresh []struct {
			Latest, Target                                     headerHeight
			Required                                           []headerHeight
			AgeSeconds, TrustingPeriodSeconds, RefreshFraction float64
			Reason                                             string
		}
		Waits []struct {
			Enabled, Mints                                                                    bool
			Now, Amount, Reserved, PerTransferMax, DailyMintCap, MintDay, MintedToday, Reason string
		}
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))
	for _, test := range fixture.Checks {
		t.Run("check/"+test.Name, func(t *testing.T) {
			var before, after sink.TransferState
			require.NoError(t, before.Unmarshal(planningHex(t, test.Before)))
			require.NoError(t, after.Unmarshal(planningHex(t, test.After)))
			for _, value := range []struct {
				transfer sink.TransferState
				encoded  string
			}{
				{
					transfer: before,
					encoded:  test.Before,
				},
				{
					transfer: after,
					encoded:  test.After,
				},
			} {
				encoded, err := value.transfer.Marshal()
				require.NoError(t, err)
				require.Equal(t, value.encoded, hex.EncodeToString(encoded))
			}
			require.Equal(t, test.Reason, finalCheck(before, after))
			require.Equal(t, test.Unbacked, unbackedMint(after))
		})
	}
	for _, test := range fixture.Order {
		entries := make([]relayPlan, len(test.Entries))
		for i, entry := range test.Entries {
			id, err := native.ParseID(entry.ID)
			require.NoError(t, err)
			entries[i] = relayPlan{
				id:       id,
				retained: entry.Retained,
				transfer: sink.TransferState{
					Status: entry.Status,
					Settlement: sink.SettlementData{
						Hold: entry.Hold,
					},
				},
			}
			if entry.Authorized {
				entries[i].transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: 11,
					RequestedRound:  1,
				}
			}
		}
		before := append([]relayPlan(nil), entries...)
		got := relayOrder(entries, solana.MustPublicKeyFromBase58(test.Payer))
		ids := make([]string, len(got))
		for i, entry := range got {
			ids[i] = entry.id.String()
		}
		require.Equal(t, test.Result, ids)
		require.Equal(t, before, entries)
	}
	for _, test := range fixture.Refresh {
		require.Equal(t, test.Reason, headerRefresh(refreshInput{
			Latest:          test.Latest,
			Target:          test.Target,
			Required:        test.Required,
			AgeSeconds:      test.AgeSeconds,
			TrustingSeconds: test.TrustingPeriodSeconds,
			Fraction:        test.RefreshFraction,
		}))
	}
	for _, test := range fixture.Waits {
		integer := func(text string) uint64 {
			v, err := strconv.ParseUint(text, 10, 64)
			require.NoError(t, err)
			return v
		}
		now, err := strconv.ParseInt(test.Now, 10, 64)
		require.NoError(t, err)
		amount, ok := new(big.Int).SetString(test.Amount, 10)
		require.True(t, ok)
		reserved, ok := new(big.Int).SetString(test.Reserved, 10)
		require.True(t, ok)
		require.Equal(t, test.Reason, mintWait(bridge.MintView{
			Enabled:        test.Enabled,
			PerTransferMax: integer(test.PerTransferMax),
			DailyMintCap:   integer(test.DailyMintCap),
			MintDay:        integer(test.MintDay),
			MintedToday:    integer(test.MintedToday),
			NowSeconds:     now,
		}, amount, test.Mints, reserved))
		require.Equal(t, test.Reserved, reserved.String())
	}
}

func planningProof(t *testing.T, s *service, transfer sink.TransferState, height int64) *proofJob {
	t.Helper()
	key, value, err := native.ProofValue(transfer, height-1)
	require.NoError(t, err)
	proof := commitment.MerkleProof{
		Proofs: []*ics23.CommitmentProof{
			{
				Proof: &ics23.CommitmentProof_Exist{
					Exist: &ics23.ExistenceProof{
						Key:   key,
						Value: value,
					},
				},
			},
			{
				Proof: &ics23.CommitmentProof_Exist{
					Exist: &ics23.ExistenceProof{
						Key:   []byte("sink"),
						Value: []byte{1, 2, 3},
					},
				},
			},
		},
	}
	encoded, err := proof.Marshal()
	require.NoError(t, err)
	job, err := newProofJob(s.store.identity, transfer.ID, s.store.identity.ProgramID, height, key, value, encoded)
	require.NoError(t, err)
	require.NoError(t, s.store.saveProof(job))
	return job
}

func TestPlanHistoricalCandidates(t *testing.T) {
	for _, name := range []string{"fresh", "retained", "pruned", "authorized-retained", "authorized-latest", "authorized-target", "authorized-new", "candidate-error", "checkpoint-outage", "checkpoint-value-mismatch", "sdk38-not-pruning", "pruned-authorized"} {
		t.Run(name, func(t *testing.T) {
			s := planningService(t)
			current := planningTransfer(1)
			if bytes.Contains([]byte(name), []byte("authorized")) || name == "candidate-error" {
				current.Settlement.VoidAuthorization = &sink.VoidAuthorization{
					RequestedHeight: 12,
					RequestedRound:  1,
				}
				current.Status = sink.StatusRefundPending
				current.Settlement.VetoReason = "retain full historical evidence"
			}
			saved := name != "fresh" && name != "candidate-error"
			var job *proofJob
			if saved {
				job = planningProof(t, s, current, 20)
			}
			var queried []int64
			s.native = planningNative(t, func(path string, height int64, data []byte, prove bool) (abci.ResponseQuery, error) {
				require.False(t, prove)
				if path == "/store/sink/key" {
					require.Equal(t, int64(19), height)
					if name == "checkpoint-outage" {
						return abci.ResponseQuery{}, errors.New("offline")
					}
					if name == "sdk38-not-pruning" {
						return abci.ResponseQuery{
							Code:      38,
							Codespace: "sdk",
						}, nil
					}
					key, value, err := native.ProofValue(current, height)
					require.NoError(t, err)
					require.Equal(t, planningHex(t, job.Key), data)
					out := abci.ResponseQuery{
						Height: height,
						Key:    key,
						Value:  value,
					}
					if name == "pruned" || name == "pruned-authorized" {
						out.Log = "version does not exist"
						out.Value = nil
					}
					if name == "checkpoint-value-mismatch" {
						out.Value = []byte("changed")
					}
					return out, nil
				}
				require.Equal(t, "/akash.sink.v1.Query/Transfer", path)
				queried = append(queried, height)
				if name == "candidate-error" {
					return abci.ResponseQuery{}, errors.New("historical RPC unavailable")
				}
				snapshot := current
				if name == "authorized-new" || (name == "authorized-target" && height < 28) || (name == "authorized-latest" && height < 24) {
					snapshot.Settlement.VetoReason = "different complete settlement"
				}
				return planningReply(t, &sink.QueryTransferResponse{Transfer: snapshot}, height), nil
			})
			got, err := s.plan(context.Background(), current.ID, 30, 25, &current)
			if name == "candidate-error" || name == "checkpoint-outage" || name == "checkpoint-value-mismatch" || name == "sdk38-not-pruning" {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, current, got.transfer)
			require.Equal(t, saved, got.retained)
			wantHeight, wantResume := headerHeight(20), headerHeight(20)
			switch name {
			case "fresh":
				wantHeight, wantResume = 11, 0
			case "pruned":
				wantHeight, wantResume = 21, 0
			case "authorized-latest", "pruned-authorized":
				wantHeight, wantResume = 25, 0
			case "authorized-target":
				wantHeight, wantResume = 29, 0
			case "authorized-new":
				wantHeight, wantResume = 30, 0
			}
			require.Equal(t, wantHeight, got.height)
			require.Equal(t, wantResume, got.resumeHeight)
			if name == "authorized-target" || name == "authorized-new" {
				require.Equal(t, []int64{19, 24, 28}, queried)
			}
			if name == "pruned-authorized" {
				require.Equal(t, []int64{24}, queried)
			}
		})
	}
}

func TestPlanKnownQueryAndSafeHeight(t *testing.T) {
	s := planningService(t)
	transfer := planningTransfer(1)
	calls := 0
	s.native = planningNative(t, func(path string, height int64, data []byte, prove bool) (abci.ResponseQuery, error) {
		calls++
		require.Equal(t, "/akash.sink.v1.Query/Transfer", path)
		require.Equal(t, int64(100), height)
		return planningReply(t, &sink.QueryTransferResponse{Transfer: transfer}, height), nil
	})
	_, err := s.plan(context.Background(), transfer.ID, 100, 90, nil)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	transfer.ID.Height = maxSafeInteger
	_, err = s.plan(context.Background(), transfer.ID, 100, 90, &transfer)
	require.ErrorContains(t, err, "represent")
	require.Equal(t, 1, calls)
}

func TestMintWaitWideReservation(t *testing.T) {
	view := bridge.MintView{
		Enabled:        true,
		PerTransferMax: math.MaxUint64,
		DailyMintCap:   math.MaxUint64,
		MintedToday:    math.MaxUint64,
	}
	require.Equal(t, "daily-limit", mintWait(view, big.NewInt(1), true, new(big.Int)))
	require.Empty(t, mintWait(view, big.NewInt(1), false, new(big.Int)))
	view.Enabled = false
	require.Equal(t, "bridge-paused", mintWait(view, big.NewInt(1), false, new(big.Int)))
}

func TestPayableReservationsAndWaitTransitions(t *testing.T) {
	s := planningService(t)
	var events bytes.Buffer
	s.deps.streams.Out = &events
	var fixtures struct{ Accounts map[string]string }
	raw, err := os.ReadFile("bridge/testdata/wire.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &fixtures))
	program := solana.PublicKey(bytes.Repeat([]byte{1}, 32))
	deployment, err := bridge.Derive(program)
	require.NoError(t, err)
	launchData := planningHex(t, fixtures.Accounts["LaunchConfig"])
	launch, err := bridge.DecodeLaunch(launchData)
	require.NoError(t, err)
	account := func(owner solana.PublicKey, raw []byte) *rpc.Account {
		return &rpc.Account{
			RentEpoch: new(big.Int),
			Owner:     owner,
			Lamports:  1,
			Data:      rpc.DataBytesOrJSONFromBytes(raw),
		}
	}
	seed := sha256.Sum256([]byte("planning-test-only-recipient"))
	key := ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey)
	wallet := solana.PublicKey(key)
	plans := make([]relayPlan, 13)
	for i := range plans {
		transfer := planningTransfer(uint64(i + 1))
		transfer.Data.Dest = wallet.String()
		transfer.Data.Amount = sdk.NewInt64Coin("uakt", 1000)
		plans[i] = relayPlan{
			id:       transfer.ID,
			transfer: transfer,
		}
	}
	plans[0].transfer.Settlement.VoidAuthorization = &sink.VoidAuthorization{
		RequestedHeight: 12,
		RequestedRound:  1,
	}
	plans[0].transfer.Data.Amount = sdk.NewInt64Coin("uakt", 999999)
	plans[1].transfer.Data.Dest = solana.SystemProgramID.String()
	plans[1].transfer.Data.Amount = sdk.NewInt64Coin("uakt", 999999)
	now := int64(7 * 86_400)
	paused, failRecipients := false, false
	s.reader = planningSolana(t, func(keys []solana.PublicKey) ([]*rpc.Account, error) {
		if len(keys) == 3 && keys[0] == deployment.Config {
			state := launch
			state.BridgeEnabled = !paused
			body, err := bin.MarshalBorsh(state)
			require.NoError(t, err)
			if !paused {
				require.Equal(t, launchData[8:], body)
			}
			clock := make([]byte, 40)
			binary.LittleEndian.PutUint64(clock[32:], uint64(now))
			return []*rpc.Account{
				account(program, append(bytes.Clone(launchData[:8]), body...)),
				account(program, planningHex(t, fixtures.Accounts["BridgeConfig"])),
				account(solana.SystemProgramID, clock),
			}, nil
		}
		if failRecipients {
			return nil, errors.New("recipient RPC outage")
		}
		require.Len(t, keys, 24)
		return make([]*rpc.Account, len(keys)), nil
	})
	ctx := context.Background()
	ready, err := s.payable(ctx, plans, nil)
	require.NoError(t, err)
	require.Equal(t, plans[:11], ready)
	require.Len(t, s.waiting, 2)
	require.Equal(t, "daily-limit", s.waiting[plans[11].id])
	firstEvents := events.String()
	_, err = s.payable(ctx, plans, nil)
	require.NoError(t, err)
	require.Equal(t, firstEvents, events.String())
	now += 86_400
	ready, err = s.payable(ctx, plans, nil)
	require.NoError(t, err)
	require.Equal(t, plans[:12], ready)
	require.NotContains(t, s.waiting, plans[11].id)
	paused = true
	ready, err = s.payable(ctx, plans, nil)
	require.NoError(t, err)
	require.Equal(t, plans[:1], ready)
	require.Equal(t, "bridge-paused", s.waiting[plans[1].id])
	before := events.String()
	failRecipients = true
	ready, err = s.payable(ctx, plans, nil)
	require.ErrorContains(t, err, "recipient RPC outage")
	require.Nil(t, ready)
	require.Equal(t, before, events.String())
}

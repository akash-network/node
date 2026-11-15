package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/internal/relayer/bridge"
)

func interceptHeaderAdmission(t *testing.T, h *headerJobRPC, hook func(headerJobCall)) {
	t.Helper()
	client := rpc.NewWithCustomRPCClient(jsonrpc.NewClientWithOpts("http://header-admission-inert.invalid", &jsonrpc.RPCClientOpts{
		HTTPClient: &http.Client{
			Transport: r32Transport(func(request *http.Request) (*http.Response, error) {
				raw, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				request.Body = io.NopCloser(bytes.NewReader(raw))
				var call headerJobCall
				if err := json.Unmarshal(raw, &call); err != nil {
					return nil, err
				}
				hook(call)
				response := httptest.NewRecorder()
				h.serve(response, request)
				return response.Result(), nil
			}),
		},
	}))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	h.s.solana = client
	h.s.reader = bridge.NewReader(client, h.s.cfg.program, h.s.cfg.chainID, nil)
}

func headerAdmissionFiles(t *testing.T, h *headerJobRPC) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	require.NoError(t, filepath.WalkDir(h.s.cfg.stateDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(path, ".lock") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil {
			files[path] = data
		}
		return err
	}))
	return files
}

func changeHeaderAdmission(t *testing.T, h *headerJobRPC, change string) {
	t.Helper()
	switch change {
	case "manager":
		h.accounts[h.f.AppState] = resolutionAccountData(t, h.f.Program, "AppState", struct {
			Manager solana.PublicKey
			Pending bool
			Padding [256]byte
		}{
			Manager: solana.PublicKey{99},
		})
	case "program", "chain":
		state, err := bridge.DecodeBridge(h.accounts[h.s.deployment.Bridge].Data.GetBinary())
		require.NoError(t, err)
		if change == "program" {
			state.LightClientProgram = solana.PublicKey{98}
			changed, err := bridge.DeriveClient(state.LightClientProgram, h.f.AccessManager)
			require.NoError(t, err)
			h.accounts[changed.AppState] = headerJobAccount(changed.Program, h.accounts[h.f.AppState].Data.GetBinary())
		} else {
			state.ChainID = "another-chain-1"
		}
		h.accounts[h.s.deployment.Bridge] = resolutionAccountData(t, h.s.cfg.program, "BridgeConfig", state)
	case "revoked":
		h.access = bridge.AccessNone
		h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
	case "access-read":
		h.failAddress = h.f.AccessState
	case "follow-read":
		h.failAddress = h.s.deployment.Bridge
	default:
		t.Fatalf("unknown admission change %q", change)
	}
}

func TestHeaderAdmissionScheduledRejectsBeforePayment(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		for _, command := range []string{"update", "resume"} {
			for _, change := range []string{"manager", "program", "chain", "revoked", "access-read", "follow-read"} {
				t.Run(fmt.Sprintf("dirty=%v/%s/%s", dirty, command, change), func(t *testing.T) {
					h := newHeaderJobFixture(t)
					upkeepHeaderNative(t, h)
					u := startUpkeep(t, h.s)
					h.clock = 50000
					h.setClock()
					if command == "resume" {
						job := h.active(t)
						job.Target = 29
						require.NoError(t, h.s.store.saveActive(job))
					}
					id := planningTransfer(1).ID
					status, err := h.s.deployment.Status(id)
					require.NoError(t, err)
					if dirty {
						_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
							return true, ledger.attempt(id)
						})
						require.NoError(t, err)
						u.paymentsDirty = true
					}
					h.s.headerRetry = retryState{
						Failures: 2,
						RetryAt:  h.now.UnixMilli() - 1,
					}
					h.s.unsupported = retryState{
						Failures: 3,
						RetryAt:  h.now.UnixMilli() - 1,
					}
					retry, unsupported := h.s.headerRetry, h.s.unsupported
					before := headerAdmissionFiles(t, h)
					writes := 0
					write := h.s.store.writeJSON
					h.s.store.writeJSON = func(path string, data []byte) error {
						writes++
						return write(path, data)
					}
					follows, recovered, rights := 0, 0, 0
					admissionFollow := 2
					if dirty {
						admissionFollow = 3
					}
					interceptHeaderAdmission(t, h, func(call headerJobCall) {
						require.NotEqual(t, "getLatestBlockhash", call.Method, "no signing preparation on rejection")
						require.NotEqual(t, "sendTransaction", call.Method)
						if call.Method != "getAccountInfo" {
							return
						}
						var address solana.PublicKey
						require.NoError(t, json.Unmarshal(call.Params[0], &address))
						if address == status {
							recovered++
						}
						if address == h.f.AccessState {
							rights++
						}
						if address != h.s.deployment.Bridge {
							return
						}
						follows++
						if follows == admissionFollow {
							// Admission happens after recovery releases its payer lock.
							acquired, err := h.s.withPayer(context.Background(), func(*payerSession) error { return nil })
							require.NoError(t, err)
							require.True(t, acquired)
							changeHeaderAdmission(t, h, change)
						}
					})
					err = u.refresh(context.Background(), nil)
					var admission *headerAdmissionError
					require.ErrorAs(t, err, &admission)
					switch change {
					case "manager", "program":
						require.EqualError(t, err, "light client identity changed during tick")
					case "chain":
						require.EqualError(t, err, "bridge accepts another-chain-1, configured akash-scale-1")
						var mismatch *bridge.FollowMismatch
						require.ErrorAs(t, err, &mismatch)
					case "revoked":
						require.EqualError(t, err, "header update access revoked during tick")
					default:
						require.ErrorContains(t, err, "503")
					}
					require.Equal(t, admissionFollow, follows)
					wantRights := admissionFollow - 1
					if change == "revoked" || change == "access-read" {
						wantRights++
					}
					require.Equal(t, wantRights, rights)
					wantRecovered := 0
					if dirty {
						wantRecovered = 1
					}
					require.Equal(t, wantRecovered, recovered)
					require.False(t, u.paymentsDirty)
					require.Zero(t, writes)
					require.Equal(t, before, headerAdmissionFiles(t, h))
					require.Empty(t, h.broadcasts)
					require.Empty(t, h.transactions)
					if change == "program" {
						// follow's existing repin bookkeeping is independent of headerStep.
						require.Equal(t, retryState{}, h.s.headerRetry)
						require.Equal(t, retryState{}, h.s.unsupported)
						require.Contains(t, h.errOut.String(), "light-client-changed")
					} else {
						require.Equal(t, retry, h.s.headerRetry)
						require.Equal(t, unsupported, h.s.unsupported)
					}
					require.NotContains(t, h.errOut.String(), "header-retry")
					require.NotContains(t, h.errOut.String(), "header-unsupported")
				})
			}
		}
	}
}

func TestHeaderAdmissionRejectsRetainSignedIntent(t *testing.T) {
	for _, command := range []string{"update", "resume"} {
		for _, change := range []string{"manager", "program", "chain", "revoked", "access-read", "follow-read"} {
			t.Run(command+"/"+change, func(t *testing.T) {
				h := newHeaderJobFixture(t)
				expected, err := h.s.follow(context.Background())
				require.NoError(t, err)
				h.active(t)
				h.dropStage = "chunk"
				require.Error(t, h.s.resumeHeader(context.Background(), nil))
				h.dropStage = ""
				ids, err := h.s.store.sessionIDs()
				require.NoError(t, err)
				require.Len(t, ids, 1)
				path, err := h.s.store.sessionPendingPath(ids[0])
				require.NoError(t, err)
				intent, err := h.s.store.loadIntent(path)
				require.NoError(t, err)
				require.NotEmpty(t, intent.Raw)
				before := headerAdmissionFiles(t, h)
				broadcasts := len(h.broadcasts)
				calls := len(h.calls)
				changeHeaderAdmission(t, h, change)
				if command == "update" {
					err = h.s.update(context.Background(), 20, false, &expected)
				} else {
					err = h.s.resumeHeader(context.Background(), &expected)
				}
				var admission *headerAdmissionError
				require.ErrorAs(t, err, &admission)
				require.Equal(t, before, headerAdmissionFiles(t, h), "includes active job, signed bytes, session, transaction journal and fees")
				require.Len(t, h.broadcasts, broadcasts)
				for _, call := range h.calls[calls:] {
					require.NotEqual(t, "getLatestBlockhash", call.Method)
					require.NotEqual(t, "getSignatureStatuses", call.Method, "rejection must not replay the pending header intent")
				}
			})
		}
	}
}

func TestHeaderAdmissionErrorEscapesRetryUnchanged(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(fmt.Sprint(wrapped), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			h.s.headerRetry = retryState{
				Failures: 7,
				RetryAt:  h.now.UnixMilli() - 1,
			}
			h.s.unsupported = retryState{
				Failures: 8,
				RetryAt:  h.now.UnixMilli() - 2,
			}
			retry, unsupported := h.s.headerRetry, h.s.unsupported
			cause := errors.New("admission read failed")
			var failure error = &headerAdmissionError{cause: cause}
			if wrapped {
				failure = fmt.Errorf("header operation: %w", failure)
			}
			updated, err := h.s.headerStep(func() error { return failure }, 29)
			require.False(t, updated)
			require.Same(t, failure, err)
			require.ErrorIs(t, err, cause)
			var admission *headerAdmissionError
			require.ErrorAs(t, err, &admission)
			require.Equal(t, retry, h.s.headerRetry)
			require.Equal(t, unsupported, h.s.unsupported)
			require.Empty(t, h.errOut.String())
		})
	}
}

func TestHeaderAdmissionOriginalAccessAndDirtyRecoveryControls(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		for _, command := range []string{"update", "resume"} {
			for _, access := range []bridge.Access{bridge.AccessMember, bridge.AccessOpen, bridge.AccessNone} {
				t.Run(fmt.Sprintf("dirty=%v/%s/%s", dirty, command, access), func(t *testing.T) {
					h := newHeaderJobFixture(t)
					upkeepHeaderNative(t, h)
					u := startUpkeep(t, h.s)
					h.clock = 50000
					h.setClock()
					if command == "resume" {
						job := h.active(t)
						job.Target = 29
						require.NoError(t, h.s.store.saveActive(job))
					}
					h.access = access
					h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
					id := planningTransfer(1).ID
					status, err := h.s.deployment.Status(id)
					require.NoError(t, err)
					if dirty {
						_, err := h.s.store.updateLedger(context.Background(), func(ledger *spendLedger) (bool, error) {
							return true, ledger.attempt(id)
						})
						require.NoError(t, err)
						u.paymentsDirty = true
					}
					before := headerAdmissionFiles(t, h)
					recovered := 0
					interceptHeaderAdmission(t, h, func(call headerJobCall) {
						if call.Method == "getAccountInfo" {
							var address solana.PublicKey
							require.NoError(t, json.Unmarshal(call.Params[0], &address))
							if address == status {
								recovered++
							}
						}
						if call.Method == "sendTransaction" {
							wantRecovered := 0
							if dirty {
								wantRecovered = 1
							}
							require.Equal(t, wantRecovered, recovered, "full recovery precedes every new header send")
							require.False(t, u.paymentsDirty)
						}
					})
					require.NoError(t, u.refresh(context.Background(), nil))
					if access == bridge.AccessNone {
						require.Empty(t, h.broadcasts)
						require.Zero(t, recovered)
						require.Equal(t, dirty, u.paymentsDirty)
						require.Equal(t, before, headerAdmissionFiles(t, h))
					} else {
						require.Equal(t, uint64(29), h.latest)
						require.Equal(t, 1, countHeaderStage(h.stages, "assembly"))
						require.Contains(t, h.stages, "cleanup")
						require.NotContains(t, h.errOut.String(), errPayerBusy.Error(), "recovery and header payment cannot nest payer sessions")
					}
				})
			}
		}
	}
}

func TestHeaderAdmissionNoOpAndStandaloneControls(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(fmt.Sprint(bound), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			found, err := h.s.follow(context.Background())
			require.NoError(t, err)
			var expected *bridge.Followed
			if bound {
				expected = &found
			}
			// No job remains a no-op, even when following would fail.
			h.failAddress = h.s.deployment.Bridge
			calls := len(h.calls)
			require.NoError(t, h.s.resumeHeader(context.Background(), expected))
			require.Len(t, h.calls, calls)
			// Invalid arguments still fail before any network observation.
			h.s.cfg.local = false
			require.EqualError(t, h.s.update(context.Background(), 20, true, expected), "header tampering is only available to the local stack")
			require.EqualError(t, h.s.update(context.Background(), 0, false, expected), "invalid header target")
			require.Len(t, h.calls, calls)
			err = h.s.update(context.Background(), 10, false, expected)
			require.ErrorContains(t, err, "503")
			var admission *headerAdmissionError
			require.Equal(t, bound, errors.As(err, &admission))
			h.failAddress = solana.PublicKey{}
			// Same target does not generate a new job or header transaction.
			before := headerAdmissionFiles(t, h)
			require.NoError(t, h.s.update(context.Background(), 10, false, expected))
			require.Equal(t, before, headerAdmissionFiles(t, h))
			h.access = bridge.AccessNone
			h.accounts[h.f.AccessState] = headerJobAccount(h.f.AccessManager, h.accessData())
			err = h.s.update(context.Background(), 10, false, expected)
			if bound {
				require.EqualError(t, err, "header update access revoked during tick")
			} else {
				require.NoError(t, err, "standalone no-op keeps the original access behavior")
			}
			require.Equal(t, before, headerAdmissionFiles(t, h))
			require.Empty(t, h.broadcasts)
		})
	}
}

func TestHeaderAdmissionRequiredPayerContention(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		for _, command := range []string{"update", "resume"} {
			t.Run(fmt.Sprintf("dirty=%v/%s", dirty, command), func(t *testing.T) {
				h := newHeaderJobFixture(t)
				upkeepHeaderNative(t, h)
				u := startUpkeep(t, h.s)
				h.clock = 50000
				h.setClock()
				if command == "resume" {
					job := h.active(t)
					job.Target = 29
					require.NoError(t, h.s.store.saveActive(job))
				}
				u.paymentsDirty = dirty
				before := headerAdmissionFiles(t, h)
				acquired, err := h.s.withPayer(context.Background(), func(*payerSession) error {
					failure := u.refresh(context.Background(), nil)
					if dirty {
						require.ErrorIs(t, failure, errPayerBusy, "recovery failure escapes header retry handling")
						require.True(t, u.paymentsDirty)
						require.Zero(t, h.s.headerRetry.Failures)
					} else {
						require.NoError(t, failure)
						require.Equal(t, int64(1), h.s.headerRetry.Failures)
						require.Contains(t, h.errOut.String(), errPayerBusy.Error())
					}
					return nil
				})
				require.NoError(t, err)
				require.True(t, acquired)
				require.Empty(t, h.broadcasts)
				require.Equal(t, before, headerAdmissionFiles(t, h))
			})
		}
	}
}

func TestHeaderAdmissionUnknownHeaderHistoryStillBlocks(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(fmt.Sprint(dirty), func(t *testing.T) {
			h := newHeaderJobFixture(t)
			upkeepHeaderNative(t, h)
			u := startUpkeep(t, h.s)
			job := h.active(t)
			job.Target = 29
			require.NoError(t, h.s.store.saveActive(job))
			h.dropStage = "chunk"
			require.Error(t, h.s.resumeHeader(context.Background(), nil))
			h.dropStage = ""
			h.blockHeight = 1001
			ids, err := h.s.store.sessionIDs()
			require.NoError(t, err)
			require.Len(t, ids, 1)
			path, err := h.s.store.sessionPendingPath(ids[0])
			require.NoError(t, err)
			pending, err := os.ReadFile(path)
			require.NoError(t, err)
			active, err := os.ReadFile(h.s.store.activePath)
			require.NoError(t, err)
			broadcasts := len(h.broadcasts)
			u.paymentsDirty = dirty
			require.NoError(t, u.refresh(context.Background(), nil))
			require.Contains(t, h.errOut.String(), "expired transaction history unresolved")
			require.Equal(t, int64(1), h.s.headerRetry.Failures)
			require.Len(t, h.broadcasts, broadcasts)
			require.NotContains(t, h.stages, "cleanup")
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, pending, after)
			after, err = os.ReadFile(h.s.store.activePath)
			require.NoError(t, err)
			require.Equal(t, active, after)
		})
	}
}

func TestHeaderAdmissionLaterIdentityCanAbandonOldJob(t *testing.T) {
	h := newHeaderJobFixture(t)
	old, err := h.s.follow(context.Background())
	require.NoError(t, err)
	h.active(t)
	before, err := os.ReadFile(h.s.store.activePath)
	require.NoError(t, err)
	changeHeaderAdmission(t, h, "program")
	require.EqualError(t, h.s.resumeHeader(context.Background(), &old), "light client identity changed during tick")
	current, err := h.s.follow(context.Background())
	require.NoError(t, err)
	// A later tick binds the new identity and authorizes its original job handling.
	require.NoError(t, h.s.resumeHeader(context.Background(), &current))
	archives, err := filepath.Glob(filepath.Join(h.s.store.headerDirectory, "abandoned-*.json"))
	require.NoError(t, err)
	require.Len(t, archives, 1)
	after, err := os.ReadFile(archives[0])
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoFileExists(t, h.s.store.activePath)
	require.Empty(t, h.broadcasts)
}

package bridge

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func mutateAccountField(t *testing.T, value any, field string, mode string) any {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var account map[string]any
	require.NoError(t, json.Unmarshal(data, &account))
	account["rentEpoch"] = float64(0)
	switch mode {
	case "missing":
		delete(account, field)
	case "null":
		account[field] = nil
	case "json":
		account[field] = map[string]any{}
	case "null-payload":
		account[field] = []any{nil, "base64"}
	case "encoding":
		account[field] = []any{"", "bogus"}
	}
	return account
}

func TestRPCNonNullAccountSchema(t *testing.T) {
	cases := []struct {
		field string
		mode  string
	}{
		{
			field: "lamports",
			mode:  "missing",
		},
		{
			field: "lamports",
			mode:  "null",
		},
		{
			field: "owner",
			mode:  "missing",
		},
		{
			field: "owner",
			mode:  "null",
		},
		{
			field: "rentEpoch",
			mode:  "missing",
		},
		{
			field: "rentEpoch",
			mode:  "null",
		},
		{
			field: "data",
			mode:  "null-payload",
		},
		{
			field: "executable",
			mode:  "missing",
		},
		{
			field: "executable",
			mode:  "null",
		},
		{
			field: "data",
			mode:  "missing",
		},
		{
			field: "data",
			mode:  "null",
		},
		{
			field: "data",
			mode:  "json",
		},
		{
			field: "data",
			mode:  "encoding",
		},
	}
	for _, tc := range cases {
		for _, batch := range []bool{false, true} {
			name := tc.field + "-" + tc.mode
			if batch {
				name += "-batch"
			}
			t.Run(name, func(t *testing.T) {
				m, r, _, _ := newRPCFixture(t)
				malformed := mutateAccountField(t, accountFixture(solana.SystemProgramID, []byte{}), tc.field, tc.mode)
				m.change(func() {
					m.changeResult = func(_ rpcCall, result any) any {
						out := result.(map[string]any)
						if batch {
							out["value"] = []any{malformed}
						} else {
							out["value"] = malformed
						}
						return out
					}
				})
				var account *rpc.Account
				var err error
				if batch {
					var accounts []*rpc.Account
					accounts, _, err = r.Accounts(context.Background(), []solana.PublicKey{keyByte(4)}, rpc.CommitmentConfirmed)
					if len(accounts) != 0 {
						account = accounts[0]
					}
				} else {
					account, _, err = r.Account(context.Background(), keyByte(4), rpc.CommitmentConfirmed, nil)
				}
				if account != nil {
					t.Logf("accepted: lamports=%d owner=%s executable=%t dataNil=%t binaryLen=%d", account.Lamports, account.Owner, account.Executable, account.Data == nil, len(account.Data.GetBinary()))
				}
				require.Error(t, err, "the TS AccountInfoResult rejects this malformed account")
			})
		}
	}
}

func TestRPCMalformedStatusIsNotUnresolved(t *testing.T) {
	for _, field := range []string{"owner", "data"} {
		t.Run(field, func(t *testing.T) {
			m, r, d, _ := newRPCFixture(t)
			account := accountFixture(d.Program, unhex(t, fixtures(t).Accounts["TransferStatus"]))
			mode := "missing"
			if field == "data" {
				mode = "json"
			}
			malformed := mutateAccountField(t, account, field, mode)
			m.change(func() {
				m.changeResult = func(_ rpcCall, result any) any {
					result.(map[string]any)["value"] = []any{malformed}
					return result
				}
			})
			resolved, err := r.Resolved(context.Background(), []sink.ID{fixtureID(t)}, nil)
			t.Logf("Resolved=%v error=%v", resolved, err)
			require.Error(t, err, "malformed existing status must not be classified unresolved")
		})
	}
}

func TestRPCIncompleteATAStopsMintPrediction(t *testing.T) {
	m, r, d, _ := newRPCFixture(t)
	g := fixtures(t)
	m.set(d.Config, accountFixture(d.Program, unhex(t, g.Accounts["LaunchConfig"])))
	m.set(d.Bridge, accountFixture(d.Program, unhex(t, g.Accounts["BridgeConfig"])))
	m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(604800)))
	wallet := testWallet()
	ata, err := AssociatedToken(wallet, d.Mint)
	require.NoError(t, err)
	m.set(ata, accountFixture(solana.TokenProgramID, tokenData(d.Mint, wallet, 1)))
	records := []sink.RecordData{
		{Dest: wallet.String()},
	}
	view, err := r.MintView(context.Background(), records, nil)
	require.NoError(t, err)
	require.True(t, view.Recipients[0].Mints, "valid ATA control")
	m.change(func() {
		m.changeResult = func(call rpcCall, result any) any {
			var addresses []string
			require.NoError(t, json.Unmarshal(call.Params[0], &addresses))
			if len(addresses) == 2 {
				result.(map[string]any)["value"] = []any{
					nil,
					mutateAccountField(t, m.accounts[ata.String()], "data", "missing"),
				}
			}
			return result
		}
	})
	view, err = r.MintView(context.Background(), records, nil)
	t.Logf("missing ATA data: recipients=%+v error=%v", view.Recipients, err)
	require.Error(t, err, "malformed ATA must stop payable before the non-mint cap bypass")
}

func TestRPCAccountPreservesValidZeroAndLargeValues(t *testing.T) {
	for _, lamports := range []uint64{0, math.MaxUint64} {
		t.Run(new(big.Int).SetUint64(lamports).String(), func(t *testing.T) {
			m, r, _, _ := newRPCFixture(t)
			address := keyByte(4)
			account := accountFixture(solana.SystemProgramID, []byte{})
			account.Lamports = lamports
			account.RentEpoch = new(big.Int).SetUint64(math.MaxUint64)
			m.set(address, account)
			one, _, err := r.Account(context.Background(), address, rpc.CommitmentConfirmed, nil)
			require.NoError(t, err)
			require.Equal(t, account, one)
			batch, _, err := r.Accounts(context.Background(), []solana.PublicKey{address, keyByte(5)}, rpc.CommitmentConfirmed)
			require.NoError(t, err)
			require.Equal(t, []*rpc.Account{account, nil}, batch)
		})
	}
}

func TestRPCConsensusScanRejectsIncompleteAccounts(t *testing.T) {
	for _, field := range []string{"lamports", "rentEpoch", "executable", "pubkey"} {
		t.Run(field, func(t *testing.T) {
			m, r, _, f := newRPCFixture(t)
			address, err := f.ConsensusAt(20)
			require.NoError(t, err)
			account := accountFixture(f.Program, unhex(t, fixtures(t).Accounts["ConsensusStateStore"]))
			var value any = account
			if field != "pubkey" {
				value = mutateAccountField(t, account, field, "missing")
			}
			row := map[string]any{"pubkey": address, "account": value}
			if field == "pubkey" {
				delete(row, field)
			}
			m.change(func() {
				m.changeResult = func(_ rpcCall, _ any) any { return []any{row} }
			})
			_, err = r.ConsensusStates(context.Background(), f)
			require.Error(t, err, "malformed scan must not produce a prune candidate")
		})
	}
}

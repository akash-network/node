package bridge

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"

	sink "pkg.akt.dev/go/node/sink/v1"
)

func testWallet() solana.PublicKey {
	seed := sha256.Sum256([]byte("bridge tests deterministic wallet v1"))
	private := ed25519.NewKeyFromSeed(seed[:])
	return solana.PublicKeyFromBytes(private.Public().(ed25519.PublicKey))
}

func tokenData(mint, recipient solana.PublicKey, state uint8) []byte {
	e := encoder{}
	e.key(mint)
	e.key(recipient)
	e.u64(123)
	e.raw(make([]byte, 36))
	e.u8(state)
	e.raw(make([]byte, 12+8+36))
	return e
}

func TestRecipientAndATAParity(t *testing.T) {
	d, err := Derive(keyByte(1))
	require.NoError(t, err)

	wallet := testWallet()
	require.True(t, canonicalOnCurve(wallet))
	require.True(t, recipientMints(wallet, nil, nil, d, nil))

	initialized := accountFixture(solana.TokenProgramID, tokenData(d.Mint, wallet, 1))
	require.True(t, recipientMints(wallet, nil, initialized, d, nil))

	for _, info := range []*rpc.Account{
		accountFixture(solana.TokenProgramID, tokenData(d.Mint, wallet, 0)),
		accountFixture(solana.TokenProgramID, tokenData(d.Mint, wallet, 2)),
		accountFixture(solana.TokenProgramID, tokenData(keyByte(8), wallet, 1)),
		accountFixture(solana.TokenProgramID, tokenData(d.Mint, keyByte(8), 1)),
		accountFixture(solana.TokenProgramID, []byte{1}),
		accountFixture(solana.Token2022ProgramID, tokenData(d.Mint, wallet, 1)),
		accountFixture(solana.SystemProgramID, []byte{1}),
		accountFixture(keyByte(8), nil),
	} {
		require.False(t, recipientMints(wallet, nil, info, d, nil))
	}
	require.True(t, recipientMints(wallet, nil, accountFixture(solana.SystemProgramID, nil), d, nil))

	for _, owner := range []solana.PublicKey{solana.TokenProgramID, solana.Token2022ProgramID} {
		require.False(t, recipientMints(wallet, accountFixture(owner, nil), nil, d, []solana.PublicKey{wallet}))
	}
	executable := accountFixture(solana.SystemProgramID, nil)
	executable.Executable = true
	require.False(t, recipientMints(wallet, executable, nil, d, []solana.PublicKey{wallet}))
	require.False(t, recipientMints(solana.PublicKey{}, nil, nil, d, []solana.PublicKey{{}}))
	require.False(t, recipientMints(d.MintAuthority, nil, nil, d, []solana.PublicKey{d.MintAuthority}))

	vault, err := pda(d.Program, "test-vault")
	require.NoError(t, err)
	require.False(t, recipientMints(vault, nil, nil, d, nil))
	require.True(t, recipientMints(vault, accountFixture(d.Program, nil), nil, d, []solana.PublicKey{vault}))
	// The noncanonical encoding of the identity with x-sign set is accepted
	// by permissive point decoders but rejected by the baseline noble path.
	noncanonical := solana.PublicKey{1}
	noncanonical[31] = 0x80
	require.False(t, canonicalOnCurve(noncanonical))
}

func TestMintViewBatchesAndDoesNotReserve(t *testing.T) {
	m, r, d, _ := newRPCFixture(t)
	g := fixtures(t)
	m.set(d.Config, accountFixture(d.Program, unhex(t, g.Accounts["LaunchConfig"])))
	m.set(d.Bridge, accountFixture(d.Program, unhex(t, g.Accounts["BridgeConfig"])))
	m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(-1)))
	records := make([]sink.RecordData, 51)
	for i := range records {
		records[i].Dest = testWallet().String()
	}
	first, err := r.MintView(context.Background(), records, nil)
	require.NoError(t, err)
	require.True(t, first.Enabled)
	require.Equal(t, uint64(1000), first.PerTransferMax)
	require.Equal(t, uint64(10000), first.DailyMintCap)
	require.Equal(t, uint64(7), first.MintDay)
	require.Equal(t, uint64(900), first.MintedToday)
	require.Equal(t, int64(-1), first.NowSeconds)
	require.Len(t, first.Recipients, 51)

	for _, result := range first.Recipients {
		require.Equal(t, testWallet(), result.Recipient)
		require.True(t, result.Mints)
	}
	calls := m.observed()
	require.Len(t, calls, 3)

	for i, want := range []int{3, 100, 2} {
		var addresses []string
		require.NoError(t, json.Unmarshal(calls[i].Params[0], &addresses))
		require.Len(t, addresses, want)
	}
	first.Recipients[0].Mints = false
	second, err := r.MintView(context.Background(), records, nil)
	require.NoError(t, err)
	require.True(t, second.Recipients[0].Mints)
	require.Equal(t, uint64(900), second.MintedToday, "observation must not reserve capacity")

	m.change(func() { m.batchDelta = -1 })
	_, err = r.MintView(context.Background(), records, nil)
	require.ErrorContains(t, err, "batch length")

	m.change(func() {
		m.batchDelta = 0
		m.failure = true
	})
	_, err = r.MintView(context.Background(), records, nil)
	require.ErrorContains(t, err, "injected unavailable")
}

func TestMintViewMalformedRequiredAccounts(t *testing.T) {
	for _, bad := range []string{"owner", "discriminator", "clock", "missing", "recipient", "recipient-batch"} {
		t.Run(bad, func(t *testing.T) {
			m, r, d, _ := newRPCFixture(t)
			g := fixtures(t)
			m.set(d.Config, accountFixture(d.Program, unhex(t, g.Accounts["LaunchConfig"])))
			m.set(d.Bridge, accountFixture(d.Program, unhex(t, g.Accounts["BridgeConfig"])))
			m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, clockData(1)))
			record := sink.RecordData{Dest: testWallet().String()}
			switch bad {
			case "owner":
				m.set(d.Config, accountFixture(keyByte(77), unhex(t, g.Accounts["LaunchConfig"])))
			case "discriminator":
				m.set(d.Config, accountFixture(d.Program, bytes.Repeat([]byte{1}, 100)))
			case "clock":
				m.set(solana.SysVarClockPubkey, accountFixture(solana.SystemProgramID, make([]byte, 39)))
			case "missing":
				m.set(d.Config, nil)
			case "recipient":
				record.Dest = "invalid"
			case "recipient-batch":
				m.change(func() {
					m.changeResult = func(call rpcCall, value any) any {
						var addresses []string
						_ = json.Unmarshal(call.Params[0], &addresses)
						if len(addresses) == 2 {
							value.(map[string]any)["value"] = []*rpc.Account{nil}
						}
						return value
					}
				})
			}
			_, err := r.MintView(context.Background(), []sink.RecordData{record}, nil)
			require.Error(t, err)
		})
	}
}

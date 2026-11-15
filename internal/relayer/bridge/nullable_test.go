package bridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func TestUninitializedStatusAndClosingBufferAreAbsent(t *testing.T) {
	for _, kind := range []string{"status", "closing-buffer"} {
		for _, owner := range []solana.PublicKey{solana.SystemProgramID, keyByte(1), keyByte(99)} {
			t.Run(kind+"/"+owner.String(), func(t *testing.T) {
				m, reader, deployment, _ := newRPCFixture(t)
				id := fixtureID(t)
				address, err := deployment.Status(id)
				require.NoError(t, err)

				read := func() error {
					status, err := reader.Status(context.Background(), id)
					require.Nil(t, status)
					return err
				}
				if kind == "closing-buffer" {
					address, err = deployment.ProofBuffer(keyByte(4), unhex(t, fixtures(t).NativeKey))
					require.NoError(t, err)
					read = func() error {
						buffer, err := reader.BufferConfirmed(context.Background(), address)
						require.Nil(t, buffer)
						return err
					}
				}

				require.NoError(t, read(), "an absent account remains absent")
				m.set(address, accountFixture(owner, []byte{}))
				require.NoError(t, read(), "Anchor fetchNullable returns no value for zero data")
				var options map[string]any
				require.NoError(t, json.Unmarshal(m.observed()[1].Params[1], &options))
				require.Equal(t, "confirmed", options["commitment"])

				if kind == "closing-buffer" {
					_, err := reader.Buffer(context.Background(), address)
					require.Error(t, err, "upload's finalized buffer read remains strict")
				}

				m.set(address, accountFixture(owner, []byte{1}))
				require.Error(t, read(), "nonempty malformed or foreign accounts remain errors")
				account := accountFixture(owner, []byte{})
				account.Data = nil
				m.set(address, account)
				require.Error(t, read(), "missing data is a malformed RPC account, not absence")

				m.set(address, nil)
				m.change(func() { m.failure = true })
				require.ErrorContains(t, read(), "injected unavailable")
			})
		}
	}
}

func TestUninitializedBootstrapAndConsensusAccountsAreAbsent(t *testing.T) {
	m, reader, deployment, followed := newRPCFixture(t)
	ctx := context.Background()
	consensus, err := followed.ConsensusAt(20)
	require.NoError(t, err)
	cases := []struct {
		name       string
		address    solana.PublicKey
		read       func() (any, error)
		commitment string
	}{
		{
			name:       "launch",
			address:    deployment.Config,
			read:       func() (any, error) { return reader.Launch(ctx) },
			commitment: "confirmed",
		},
		{
			name:       "bridge",
			address:    deployment.Bridge,
			read:       func() (any, error) { return reader.Bridge(ctx) },
			commitment: "confirmed",
		},
		{
			name:       "access",
			address:    followed.AccessState,
			read:       func() (any, error) { return reader.AccessState(ctx, followed) },
			commitment: "confirmed",
		},
		{
			name:       "client",
			address:    followed.ClientState,
			read:       func() (any, error) { return reader.ClientNullable(ctx, followed) },
			commitment: "confirmed",
		},
		{
			name:       "consensus-finalized",
			address:    consensus,
			read:       func() (any, error) { return reader.Consensus(ctx, followed, 20) },
			commitment: "finalized",
		},
		{
			name:       "consensus-confirmed",
			address:    consensus,
			read:       func() (any, error) { return reader.ConsensusConfirmed(ctx, followed, 20) },
			commitment: "confirmed",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			m.set(test.address, accountFixture(solana.SystemProgramID, []byte{}))
			value, err := test.read()
			require.NoError(t, err)
			require.Nil(t, value)
			calls := m.observed()
			var options map[string]any
			require.NoError(t, json.Unmarshal(calls[len(calls)-1].Params[1], &options))
			require.Equal(t, test.commitment, options["commitment"])

			m.set(test.address, accountFixture(solana.SystemProgramID, []byte{1}))
			_, err = test.read()
			require.ErrorContains(t, err, "owner")

			account := accountFixture(solana.SystemProgramID, []byte{})
			account.Data = nil
			m.set(test.address, account)
			_, err = test.read()
			require.Error(t, err)
		})
	}

	m.set(followed.ClientState, accountFixture(solana.SystemProgramID, []byte{}))
	_, err = reader.Client(ctx, followed)
	require.ErrorContains(t, err, "client state unavailable")
	_, err = reader.ClientFinalized(ctx, followed)
	require.ErrorContains(t, err, "client state unavailable")
	m.set(followed.AccessState, accountFixture(solana.SystemProgramID, []byte{}))
	_, err = reader.Access(ctx, followed, keyByte(4))
	require.ErrorContains(t, err, "access manager state unavailable")

	encoded := encoder(discriminator("account", "ClientState"))
	encoded.client(fixtureClient())
	m.set(followed.ClientState, accountFixture(followed.Program, encoded))
	m.set(consensus, accountFixture(solana.SystemProgramID, []byte{}))
	require.ErrorContains(t, reader.Active(ctx, followed), "latest consensus unavailable")
}

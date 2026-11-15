package bridge

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/stretchr/testify/require"
)

// The followed client must track the configured chain.
func TestClientChainMustMatch(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	other := fixtureClient()
	other.ChainID = "another-chain-1"
	encoded := encoder(discriminator("account", "ClientState"))
	encoded.client(other)
	m.set(f.ClientState, accountFixture(f.Program, encoded))

	ctx := context.Background()
	_, err := r.Client(ctx, f)
	require.ErrorContains(t, err, "client chain ID mismatch")
	_, err = r.ClientFinalized(ctx, f)
	require.ErrorContains(t, err, "client chain ID mismatch")
	_, err = r.ClientNullable(ctx, f)
	require.ErrorContains(t, err, "client chain ID mismatch")
	require.ErrorContains(t, r.Active(ctx, f), "client chain ID mismatch")
}

// Only RELAYER_ROLE (1) grants update or prune
// rights; membership of another role, including the admin role 0, grants none.
func TestOnlyRelayerRoleCounts(t *testing.T) {
	m, r, _, f := newRPCFixture(t)
	payer := keyByte(4)
	manager := func(roles ...Role) []byte {
		e := encoder(discriminator("account", "AccessManager"))
		e.u32(uint32(len(roles)))
		for _, role := range roles {
			e.u64(role.ID)
			e.u32(uint32(len(role.Members)))
			for _, member := range role.Members {
				e.key(member)
			}
		}
		e.u32(0)
		e.u32(0)
		return e
	}
	for _, test := range []struct {
		name  string
		roles []Role
		want  Access
	}{
		{
			name: "admin-role-member",
			roles: []Role{
				{
					ID:      0,
					Members: []solana.PublicKey{payer},
				},
			},
			want: AccessNone,
		},
		{
			name: "other-role-open-marker",
			roles: []Role{
				{
					ID:      2,
					Members: []solana.PublicKey{f.Program, payer},
				},
			},
			want: AccessNone,
		},
		{
			name: "relayer-open-other-role-member",
			roles: []Role{
				{
					ID:      0,
					Members: []solana.PublicKey{payer},
				},
				{
					ID:      1,
					Members: []solana.PublicKey{f.Program},
				},
			},
			want: AccessOpen,
		},
		{
			name: "relayer-member-after-other-role",
			roles: []Role{
				{
					ID:      7,
					Members: []solana.PublicKey{f.Program},
				},
				{
					ID:      1,
					Members: []solana.PublicKey{payer},
				},
			},
			want: AccessMember,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			m.set(f.AccessState, accountFixture(f.AccessManager, manager(test.roles...)))
			access, err := r.Access(context.Background(), f, payer)
			require.NoError(t, err)
			require.Equal(t, test.want, access)
		})
	}
}

// count is the allocation boundary for decoded vectors. Inspect the count and
// error directly: even a regressed guard must never allocate from MaxUint32 in
// this test. The account decoders use the returned count for their allocations.
func TestDecodeVectorCountBounds(t *testing.T) {
	for _, minSize := range []int{1, 12, 32, 64} {
		for _, count := range []uint32{0, 1, 2, 3, math.MaxUint32} {
			t.Run(fmt.Sprintf("element-%d/count-%d", minSize, count), func(t *testing.T) {
				data := binary.LittleEndian.AppendUint32(nil, count)
				data = append(data, make([]byte, 2*minSize)...)
				d := &decoder{data: data}
				got := d.count(minSize)
				if count > 2 {
					require.ErrorIs(t, d.err, io.ErrUnexpectedEOF)
					require.Zero(t, got, "an invalid count must not reach an allocation")
					return
				}
				require.NoError(t, d.err)
				require.Equal(t, int(count), got)
				require.Len(t, d.data, 2*minSize, "only the count prefix is consumed")
			})
		}
	}
}

// The owner must be the canonical lowercase
// Akash encoding; the same bytes in another case or prefix are rejected.
func TestOwnerMustBeCanonicalAkash(t *testing.T) {
	d, err := Derive(keyByte(1))
	require.NoError(t, err)
	f, err := DeriveClient(keyByte(2), keyByte(3))
	require.NoError(t, err)
	id := fixtureID(t)
	_, raw, err := bech32.DecodeAndConvert(id.Owner)
	require.NoError(t, err)
	foreign, err := bech32.ConvertAndEncode("cosmos", raw)
	require.NoError(t, err)

	for name, owner := range map[string]string{
		"uppercase":      strings.ToUpper(id.Owner),
		"another-prefix": foreign,
	} {
		t.Run(name, func(t *testing.T) {
			changed := id
			changed.Owner = owner
			_, err := d.Status(changed)
			require.ErrorContains(t, err, "invalid transfer identity")
			_, err = d.MintTransfer(MintArgs{
				Payer:       keyByte(4),
				Client:      f,
				ID:          changed,
				Recipient:   keyByte(5),
				Amount:      1,
				ProofHeight: 20,
			})
			require.Error(t, err)
			_, err = d.VoidTransfer(VoidArgs{
				Payer:            keyByte(4),
				Client:           f,
				ID:               changed,
				SettlementStatus: 3,
				ProofHeight:      20,
			})
			require.Error(t, err)
		})
	}
}

// The bridge chain ID is 1 to 64 bytes.
func TestBridgeChainIDBounds(t *testing.T) {
	for _, length := range []int{0, 65} {
		e := encoder(discriminator("account", "BridgeConfig"))
		e.key(keyByte(2))
		e.str(strings.Repeat("a", length))
		e.policy(MintPolicy{
			PerTransferMax: 1,
			DailyMintCap:   1,
		})
		e.u64(0)
		e.u64(0)
		_, err := DecodeBridge(e)
		require.ErrorContains(t, err, "invalid bridge chain ID", "length %d", length)
	}
	e := encoder(discriminator("account", "BridgeConfig"))
	e.key(keyByte(2))
	e.str(strings.Repeat("a", 64))
	e.policy(MintPolicy{
		PerTransferMax: 1,
		DailyMintCap:   1,
	})
	e.u64(0)
	e.u64(0)
	_, err := DecodeBridge(e)
	require.NoError(t, err)
}

// A status whose stored identity is outside
// the program's bounds is malformed, not a resolved transfer.
func TestStatusIdentityBounds(t *testing.T) {
	status := func(owner []byte, height, sequence uint64) []byte {
		e := encoder(discriminator("account", "TransferStatus"))
		e.vector(owner)
		e.u64(height)
		e.u64(sequence)
		e.u64(height + 1)
		e.u64(42)
		e.u8(uint8(OutcomeVoided))
		e.u8(uint8(VoidGovernance))
		return e
	}
	owner := bytes.Repeat([]byte{7}, 20)
	_, err := DecodeStatus(status(owner, 9, 1))
	require.NoError(t, err)
	for name, data := range map[string][]byte{
		"empty-owner":   status(nil, 9, 1),
		"long-owner":    status(bytes.Repeat([]byte{7}, 256), 9, 1),
		"zero-height":   status(owner, 0, 1),
		"height-int64":  status(owner, math.MaxInt64+1, 1),
		"zero-sequence": status(owner, 9, 0),
	} {
		_, err := DecodeStatus(data)
		require.ErrorContains(t, err, "invalid transfer identity", name)
	}
}

// RPC responses are untrusted even for a sysvar whose on-chain capacity is 512.
func TestSlotHashesCountBounds(t *testing.T) {
	for _, count := range []int{0, 1, 512, 513} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			m, r, _, _ := newRPCFixture(t)
			data := binary.LittleEndian.AppendUint64(nil, uint64(count))
			want := make([]uint64, count)
			for i := range want {
				want[i] = uint64(1000 - i)
				data = binary.LittleEndian.AppendUint64(data, want[i])
				data = append(data, make([]byte, 32)...)
			}
			m.set(solana.SysVarSlotHashesPubkey, accountFixture(solana.SystemProgramID, data))

			got, err := r.SlotHashes(context.Background())
			if count == 0 || count > 512 {
				require.ErrorContains(t, err, "invalid slot hashes count")
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

// A server can return accounts that violate the requested dataSize filter.
// Padding is otherwise allowed by the decoder, so an oversized result must be
// rejected by the scan boundary itself.
func TestConsensusScanRejectsIncorrectAccountSize(t *testing.T) {
	for _, size := range []int{87, 88, 89} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m, r, _, f := newRPCFixture(t)
			data := unhex(t, fixtures(t).Accounts["ConsensusStateStore"])
			require.Len(t, data, 88)
			data = append(data, 0)[:size]
			address, err := f.ConsensusAt(20)
			require.NoError(t, err)
			m.change(func() {
				m.programAccounts = []*rpc.KeyedAccount{
					{
						Pubkey:  address,
						Account: accountFixture(f.Program, data),
					},
				}
			})

			states, err := r.ConsensusStates(context.Background(), f)
			if size != 88 {
				require.Error(t, err)
				if size > 88 {
					require.ErrorContains(t, err, "invalid consensus store size")
				}
				require.Nil(t, states)
				return
			}
			require.NoError(t, err)
			require.Len(t, states, 1)
			require.Equal(t, address, states[0].Address)
			require.Equal(t, uint64(20), states[0].Height)
		})
	}
}

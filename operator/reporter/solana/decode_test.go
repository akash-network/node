package solana_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
	aktsolana "pkg.akt.dev/node/v2/operator/reporter/solana"
)

const (
	launchConfigSize    = 84
	bridgeConfigSize    = 656
	transferStatusSize  = 340
	maxUint64           = math.MaxUint64
	maxInt64AsUint64    = math.MaxInt64
	outcomeTagMinted    = 0
	outcomeTagVoided    = 1
	nameTruncated       = "truncated"
	deployedInterfaceJS = "../../../contracts/solana/tests/fixtures/deployed-interface.json"
)

type outcomeCase struct {
	name    string
	encoded []byte
	want    aktsolana.Outcome
}

func key(fill byte) solana.PublicKey {
	var k solana.PublicKey
	for i := range k {
		k[i] = fill
	}

	return k
}

func u64(v uint64) []byte {
	return binary.LittleEndian.AppendUint64(nil, v)
}

func u32(v int) []byte {
	return binary.LittleEndian.AppendUint32(nil, uint32(v)) //nolint:gosec // test sizes are small
}

func vec(b []byte) []byte {
	return append(u32(len(b)), b...)
}

func outcomes() []outcomeCase {
	recipient := key(0x07)
	void := func(r aktsolana.VoidReason) *aktsolana.VoidReason { return &r }

	return []outcomeCase{
		{
			name:    "minted",
			encoded: bytes.Join([][]byte{{outcomeTagMinted}, u64(maxUint64), recipient[:]}, nil),
			want:    aktsolana.Outcome{Minted: &aktsolana.Minted{Amount: maxUint64, Recipient: recipient}},
		},
		{
			name:    "voided invalid recipient",
			encoded: []byte{outcomeTagVoided, 0},
			want:    aktsolana.Outcome{Voided: void(aktsolana.VoidInvalidRecipient)},
		},
		{
			name:    "voided governance",
			encoded: []byte{outcomeTagVoided, 1},
			want:    aktsolana.Outcome{Voided: void(aktsolana.VoidGovernance)},
		},
	}
}

// statusBody encodes the fields shared by the TransferStatus account and the TransferResolved event.
func statusBody(owner []byte, height, sequence uint64, outcome []byte) []byte {
	return bytes.Join([][]byte{
		vec(owner), u64(height), u64(sequence),
		u64(height + 1), u64(99),
		outcome,
	}, nil)
}

func wantStatus(owner []byte, o aktsolana.Outcome) aktsolana.TransferStatus {
	return aktsolana.TransferStatus{
		ID:             aktsolana.TransferID{Owner: owner, Height: maxInt64AsUint64, Sequence: maxUint64},
		ProofHeight:    maxInt64AsUint64 + 1,
		ResolvedAtSlot: 99,
		Outcome:        o,
	}
}

func withDisc(disc [8]byte, body []byte) []byte {
	return append(append([]byte{}, disc[:]...), body...)
}

func pad(b []byte, size int) []byte {
	return append(b, make([]byte, size-len(b))...)
}

func TestDiscriminatorsMatchDeployedInterface(t *testing.T) {
	raw, err := os.ReadFile(deployedInterfaceJS)
	require.NoError(t, err)

	var iface struct {
		Accounts []struct {
			Name          string `json:"name"`
			Discriminator []byte `json:"discriminator"`
		} `json:"accounts"`
		Events []struct {
			Name          string `json:"name"`
			Discriminator []byte `json:"discriminator"`
		} `json:"events"`
	}
	require.NoError(t, json.Unmarshal(raw, &iface))

	found := map[string][]byte{}
	for _, a := range iface.Accounts {
		found["account:"+a.Name] = a.Discriminator
	}

	for _, e := range iface.Events {
		found["event:"+e.Name] = e.Discriminator
	}

	for name, want := range map[string][8]byte{
		"account:LaunchConfig":   aktsolana.DiscLaunchConfig,
		"account:BridgeConfig":   aktsolana.DiscBridgeConfig,
		"account:TransferStatus": aktsolana.DiscTransferStatus,
		"event:TransferResolved": aktsolana.DiscTransferResolved,
	} {
		require.Equal(t, want[:], found[name], name)
	}
}

func TestTransferStatusRoundTrip(t *testing.T) {
	owner := bytes.Repeat([]byte{0x02}, 20)

	for _, tc := range outcomes() {
		t.Run(tc.name, func(t *testing.T) {
			data := withDisc(aktsolana.DiscTransferStatus, statusBody(owner, maxInt64AsUint64, maxUint64, tc.encoded))

			got, err := aktsolana.DecodeTransferStatus(data)
			require.NoError(t, err)
			require.Equal(t, wantStatus(owner, tc.want), got)

			padded := pad(data, transferStatusSize)
			got, err = aktsolana.DecodeTransferStatus(padded)
			require.NoError(t, err)
			require.Equal(t, wantStatus(owner, tc.want), got)

			event := withDisc(aktsolana.DiscTransferResolved, statusBody(owner, maxInt64AsUint64, maxUint64, tc.encoded))
			resolved, err := aktsolana.DecodeTransferResolved(event)
			require.NoError(t, err)
			require.Equal(t, aktsolana.TransferResolved(wantStatus(owner, tc.want)), resolved)
		})
	}
}

func TestTransferStatusRejects(t *testing.T) {
	owner := bytes.Repeat([]byte{0x02}, 20)
	minted := outcomes()[0].encoded
	body := func(owner []byte, outcome []byte) []byte {
		return withDisc(aktsolana.DiscTransferStatus, statusBody(owner, 1, 1, outcome))
	}

	valid := body(owner, minted)

	for name, data := range map[string][]byte{
		"wrong discriminator": withDisc(aktsolana.DiscLaunchConfig, statusBody(owner, 1, 1, minted)),
		"short":               valid[:len(valid)-1],
		"empty":               nil,
		"empty owner":         body(nil, minted),
		"owner over 255":      body(make([]byte, 256), minted),
		"outcome tag 2":       body(owner, []byte{2}),
		"void tag 2":          body(owner, []byte{outcomeTagVoided, 2}),
		"owner length lies":   append(append(append([]byte{}, aktsolana.DiscTransferStatus[:]...), u32(1<<31)...), 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aktsolana.DecodeTransferStatus(data)
			require.Error(t, err)
		})
	}

	_, err := aktsolana.DecodeTransferStatus(body(make([]byte, 255), minted))
	require.NoError(t, err)

	_, err = aktsolana.DecodeTransferStatus(body(make([]byte, 1), minted))
	require.NoError(t, err)
}

func TestTransferResolvedStrict(t *testing.T) {
	owner := bytes.Repeat([]byte{0x02}, 20)
	minted := outcomes()[0].encoded
	event := func(outcome []byte) []byte {
		return withDisc(aktsolana.DiscTransferResolved, statusBody(owner, 1, 1, outcome))
	}

	_, err := aktsolana.DecodeTransferResolved(event(minted))
	require.NoError(t, err)

	for name, data := range map[string][]byte{
		"trailing byte":       append(event(minted), 0),
		"outcome tag 2":       event([]byte{2}),
		"void tag 2":          event([]byte{outcomeTagVoided, 2}),
		nameTruncated:         event(minted)[:len(event(minted))-1],
		"account discr":       withDisc(aktsolana.DiscTransferStatus, statusBody(owner, 1, 1, minted)),
		"void trailing bytes": append(event([]byte{outcomeTagVoided, 0}), 0),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aktsolana.DecodeTransferResolved(data)
			require.Error(t, err)
		})
	}
}

func launchConfigBytes() []byte {
	return bytes.Join([][]byte{
		aktsolana.DiscLaunchConfig[:],
		{3},
		bytes.Repeat([]byte{0x0a}, 32),
		bytes.Repeat([]byte{0x0b}, 32),
		{1},
		u64(4242),
		{254, 253},
	}, nil)
}

func bridgeConfigBytes(chainID string, vaults int) []byte {
	vaultKeys := bytes.Repeat(bytes.Repeat([]byte{0x0d}, 32), vaults)

	return bytes.Join([][]byte{
		aktsolana.DiscBridgeConfig[:],
		bytes.Repeat([]byte{0x0c}, 32),
		vec([]byte(chainID)),
		u64(10), u64(20),
		u32(vaults), vaultKeys,
		u64(7), u64(8),
	}, nil)
}

func mintBytes(authority, freeze *solana.PublicKey, supply uint64, decimals, initialized byte) []byte {
	coption := func(k *solana.PublicKey) []byte {
		if k == nil {
			return append(u32(0), make([]byte, 32)...)
		}

		return append(u32(1), k[:]...)
	}

	return bytes.Join([][]byte{coption(authority), u64(supply), {decimals, initialized}, coption(freeze)}, nil)
}

func TestDecodeConfigAndMint(t *testing.T) {
	t.Run("launch config", func(t *testing.T) {
		want := aktsolana.LaunchConfig{
			Version:           3,
			Initializer:       key(0x0a),
			Mint:              key(0x0b),
			BridgeEnabled:     true,
			InitializedAtSlot: 4242,
			Bump:              254,
			MintAuthorityBump: 253,
		}

		data := launchConfigBytes()
		require.Len(t, data, launchConfigSize)

		got, err := aktsolana.DecodeLaunchConfig(data)
		require.NoError(t, err)
		require.Equal(t, want, got)

		got, err = aktsolana.DecodeLaunchConfig(append(data, 0, 0, 0))
		require.NoError(t, err)
		require.Equal(t, want, got)

		_, err = aktsolana.DecodeLaunchConfig(data[:launchConfigSize-1])
		require.Error(t, err)

		bad := launchConfigBytes()
		bad[8+1+32+32] = 2
		_, err = aktsolana.DecodeLaunchConfig(bad)
		require.Error(t, err)

		_, err = aktsolana.DecodeLaunchConfig(withDisc(aktsolana.DiscBridgeConfig, data[8:]))
		require.Error(t, err)
	})

	t.Run("bridge config", func(t *testing.T) {
		full := bridgeConfigBytes("akashnet-2", 16)
		// 16 vaults and a 64-byte chain ID fill the account exactly.
		require.Len(t, pad(bridgeConfigBytes(string(bytes.Repeat([]byte{'a'}, 64)), 16), bridgeConfigSize), bridgeConfigSize)
		require.Len(t, bridgeConfigBytes(string(bytes.Repeat([]byte{'a'}, 64)), 16), bridgeConfigSize)

		got, err := aktsolana.DecodeBridgeConfig(pad(full, bridgeConfigSize))
		require.NoError(t, err)
		require.Equal(t, aktsolana.BridgeConfig{LightClientProgram: key(0x0c), ChainID: "akashnet-2"}, got)

		got, err = aktsolana.DecodeBridgeConfig(bridgeConfigBytes("local", 0))
		require.NoError(t, err)
		require.Equal(t, "local", got.ChainID)

		_, err = aktsolana.DecodeBridgeConfig(full[:len(full)-1])
		require.Error(t, err)

		_, err = aktsolana.DecodeBridgeConfig(bridgeConfigBytes("local", 0)[:60])
		require.Error(t, err)

		_, err = aktsolana.DecodeBridgeConfig(withDisc(aktsolana.DiscLaunchConfig, full[8:]))
		require.Error(t, err)

		invalidUTF8 := bytes.Join([][]byte{
			aktsolana.DiscBridgeConfig[:], bytes.Repeat([]byte{0x0c}, 32), vec([]byte{0xff, 0xfe}),
			u64(0), u64(0), u32(0), u64(0), u64(0),
		}, nil)
		_, err = aktsolana.DecodeBridgeConfig(invalidUTF8)
		require.Error(t, err)
	})

	t.Run("mint", func(t *testing.T) {
		authority, freeze := key(0x31), key(0x32)

		data := mintBytes(&authority, &freeze, 1_000_000, 6, 1)
		require.Len(t, data, 82)

		got, err := aktsolana.DecodeMint(data)
		require.NoError(t, err)
		require.Equal(t, aktsolana.Mint{
			MintAuthority:   &authority,
			Supply:          1_000_000,
			Decimals:        6,
			Initialized:     true,
			FreezeAuthority: &freeze,
		}, got)

		got, err = aktsolana.DecodeMint(mintBytes(nil, nil, 0, 0, 0))
		require.NoError(t, err)
		require.Equal(t, aktsolana.Mint{}, got)

		_, err = aktsolana.DecodeMint(data[:81])
		require.Error(t, err)

		_, err = aktsolana.DecodeMint(append(append([]byte{}, data...), 0))
		require.Error(t, err)

		badAuthority := append([]byte{}, data...)
		badAuthority[0] = 2
		_, err = aktsolana.DecodeMint(badAuthority)
		require.Error(t, err)

		badFreeze := append([]byte{}, data...)
		badFreeze[46] = 2
		_, err = aktsolana.DecodeMint(badFreeze)
		require.Error(t, err)

		_, err = aktsolana.DecodeMint(mintBytes(&authority, &freeze, 1, 6, 2))
		require.Error(t, err)
	})
}

func TestStatusAddressSeeds(t *testing.T) {
	program := key(0x42)
	owner := bytes.Repeat([]byte{0x02}, 20)
	id := sinkid.ID{Owner: "unused", OwnerBytes: owner, Height: math.MaxInt64, Sequence: maxUint64}

	digest := sha256.Sum256(owner)
	want, _, err := solana.FindProgramAddress([][]byte{
		[]byte("transfer"), digest[:], u64(maxInt64AsUint64), u64(maxUint64),
	}, program)
	require.NoError(t, err)

	require.Equal(t, want, aktsolana.StatusAddress(program, id))
	require.Equal(t, want, aktsolana.PDA(program, []byte("transfer"), digest[:], u64(maxInt64AsUint64), u64(maxUint64)))

	other := id
	other.Sequence--
	require.NotEqual(t, want, aktsolana.StatusAddress(program, other))
}

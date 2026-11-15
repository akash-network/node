package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"testing"

	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"
)

func accountDecoders() map[string]func([]byte) error {
	return map[string]func([]byte) error{
		"LaunchConfig": func(b []byte) error {
			_, err := DecodeLaunch(b)
			return err
		},
		"BridgeConfig": func(b []byte) error {
			_, err := DecodeBridge(b)
			return err
		},
		"ProofBuffer": func(b []byte) error {
			_, err := DecodeProofBuffer(b)
			return err
		},
		"TransferStatus": func(b []byte) error {
			_, err := DecodeStatus(b)
			return err
		},
		"TransferStatusVoided": func(b []byte) error {
			_, err := DecodeStatus(b)
			return err
		},
		"ClientState": func(b []byte) error {
			_, err := DecodeClient(b)
			return err
		},
		"ConsensusStateStore": func(b []byte) error {
			_, err := DecodeConsensus(b)
			return err
		},
		"AppState": func(b []byte) error {
			_, err := DecodeApp(b)
			return err
		},
		"HeaderChunk": func(b []byte) error {
			_, err := DecodeChunk(b)
			return err
		},
		"SignatureBatch": func(b []byte) error {
			_, err := DecodeBatch(b)
			return err
		},
		"AccessManager": func(b []byte) error {
			_, err := DecodeAccess(b)
			return err
		},
	}
}

func TestAccountCodecsFieldValues(t *testing.T) {
	g := fixtures(t)
	data := func(name string) []byte { return unhex(t, g.Accounts[name]) }
	d, err := Derive(keyByte(1))
	require.NoError(t, err)

	launch, err := DecodeLaunch(data("LaunchConfig"))
	require.NoError(t, err)
	require.Equal(t, LaunchConfig{
		Version:           2,
		Initializer:       keyByte(4),
		Mint:              d.Mint,
		BridgeEnabled:     true,
		InitializedAtSlot: 42,
		Bump:              253,
		MintAuthorityBump: 252,
	}, launch)
	configured, err := DecodeBridge(data("BridgeConfig"))
	require.NoError(t, err)
	require.Equal(t, BridgeConfig{
		LightClientProgram: keyByte(2),
		ChainID:            "akash-local-1",
		Policy: MintPolicy{
			PerTransferMax: 1000,
			DailyMintCap:   10000,
			Vaults:         []solana.PublicKey{keyByte(5)},
		},
		MintDay:     7,
		MintedToday: 900,
	}, configured)
	proof, err := DecodeProofBuffer(data("ProofBuffer"))
	require.NoError(t, err)
	require.Equal(t, ProofBuffer{
		Submitter:   keyByte(4),
		Nonce:       sha256.Sum256(unhex(t, g.NativeKey)),
		ExpectedLen: 1234,
		Data:        []byte{1, 2, 3},
		Allocated:   83,
	}, proof)
	state, err := DecodeClient(data("ClientState"))
	require.NoError(t, err)
	require.Equal(t, fixtureClient(), state)

	consensus, err := DecodeConsensus(data("ConsensusStateStore"))
	require.NoError(t, err)
	require.Equal(t, Consensus{
		Height:             20,
		Timestamp:          math.MaxUint64,
		Root:               keyByte(7),
		NextValidatorsHash: keyByte(8),
	}, consensus)
	app, err := DecodeApp(data("AppState"))
	require.NoError(t, err)
	require.Equal(t, keyByte(3), app.AccessManager)
	require.Equal(t, keyByte(5), *app.PendingAccessManager)
	require.Equal(t, bytes.Repeat([]byte{17}, 256), app.Reserved[:])

	chunk, err := DecodeChunk(data("HeaderChunk"))
	require.NoError(t, err)
	require.Equal(t, HeaderChunk{
		Submitter: keyByte(4),
		Data:      []byte{1, 2, 3},
	}, chunk)
	batch, err := DecodeBatch(data("SignatureBatch"))
	require.NoError(t, err)
	require.Equal(t, SignatureBatch{
		Version:   1,
		Submitter: keyByte(4),
		Session:   keyByte(6),
		Bump:      254,
		Hashes:    [][32]byte{keyByte(9), keyByte(10)},
	}, batch)
	access, err := DecodeAccess(data("AccessManager"))
	require.NoError(t, err)
	require.Equal(t, AccessManager{
		Roles: []Role{
			{
				ID:      1,
				Members: []solana.PublicKey{keyByte(2), keyByte(4)},
			},
		},
		WhitelistedPrograms: []solana.PublicKey{keyByte(5)},
		PendingAuthorityTransfers: []AuthorityTransfer{
			{
				TargetProgram: keyByte(2),
				NewAuthority:  keyByte(5),
			},
		},
	}, access)
	for _, name := range []string{"TransferStatus", "TransferStatusVoided"} {
		status, err := DecodeStatus(data(name))
		require.NoError(t, err)
		require.Equal(t, fixtureID(t), status.ID)
		require.Equal(t, uint64(20), status.ProofHeight)
		require.Equal(t, uint64(42), status.ResolvedAtSlot)
		if name == "TransferStatus" {
			require.Equal(t, Outcome{
				Kind:      OutcomeMinted,
				Amount:    math.MaxUint64,
				Recipient: keyByte(5),
			}, status.Outcome)
		} else {
			require.Equal(t, Outcome{
				Kind:   OutcomeVoided,
				Reason: VoidGovernance,
			}, status.Outcome)
		}
	}
}

func TestAllAccountsRejectTruncationAndDiscriminator(t *testing.T) {
	g := fixtures(t)
	for name, decode := range accountDecoders() {
		t.Run(name, func(t *testing.T) {
			data := unhex(t, g.Accounts[name])
			require.NoError(t, decode(data))
			for n := 0; n < len(data); n++ {
				require.Error(t, decode(data[:n]), "truncation %d", n)
			}
			wrong := bytes.Clone(data)
			wrong[0] ^= 1
			require.Error(t, decode(wrong))
			require.NoError(t, decode(append(bytes.Clone(data), make([]byte, 200)...)), "rent padding")
		})
	}
}

func TestMalformedAccountFields(t *testing.T) {
	g := fixtures(t)
	launch := unhex(t, g.Accounts["LaunchConfig"])
	launch[8+1+32+32] = 2
	_, err := DecodeLaunch(launch)
	require.Error(t, err)

	app := unhex(t, g.Accounts["AppState"])
	app[8+32] = 2
	_, err = DecodeApp(app)
	require.Error(t, err)

	for _, name := range []string{"ClientState", "AccessManager", "TransferStatus"} {
		data := unhex(t, g.Accounts[name])
		binary.LittleEndian.PutUint32(data[8:], math.MaxUint32)
		require.Error(t, accountDecoders()[name](data), name)
	}
	proof := unhex(t, g.Accounts["ProofBuffer"])
	binary.LittleEndian.PutUint32(proof[8+32+32:], 2)
	_, err = DecodeProofBuffer(proof)
	require.Error(t, err)

	status := unhex(t, g.Accounts["TransferStatusVoided"])
	status[len(status)-1] = 2
	_, err = DecodeStatus(status)
	require.Error(t, err)
	status[len(status)-2] = 2
	_, err = DecodeStatus(status)
	require.Error(t, err)
}

func TestTrustArithmetic(t *testing.T) {
	require.False(t, Expired(math.MaxUint64, math.MaxUint64, math.MaxInt64))
	require.False(t, Expired(1, 1, math.MinInt64))
	require.False(t, Expired(1_000_000_001, 1, 2))
	require.True(t, Expired(1_000_000_000, 1, 2))
	require.True(t, Expired(0, 0, 0))
	require.True(t, Expired(math.MaxUint64, 0, math.MaxInt64))
}

func FuzzAccountDecoders(f *testing.F) {
	for _, raw := range fixtures(f).Accounts {
		f.Add(unhex(f, raw))
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, decode := range accountDecoders() {
			_ = decode(data)
		}
	})
}

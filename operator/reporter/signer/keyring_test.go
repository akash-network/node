package signer_test

import (
	"context"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

const testPrefix = "akash"

func newTestKeyring(t *testing.T) keyring.Keyring {
	t.Helper()

	reg := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(reg)

	kr, err := keyring.New("akt-reporter-test", keyring.BackendTest, t.TempDir(), nil, codec.NewProtoCodec(reg))
	require.NoError(t, err)

	return kr
}

func newKey(t *testing.T, kr keyring.Keyring, name string) (sdk.AccAddress, cryptotypes.PubKey) {
	t.Helper()

	rec, _, err := kr.NewMnemonic(name, keyring.English, sdk.FullFundraiserPath, "", hd.Secp256k1)
	require.NoError(t, err)

	addr, err := rec.GetAddress()
	require.NoError(t, err)

	pub, err := rec.GetPubKey()
	require.NoError(t, err)

	return addr, pub
}

func TestKeyringSigner(t *testing.T) {
	kr := newTestKeyring(t)
	addr, pub := newKey(t, kr, "r")
	otherAddr, _ := newKey(t, kr, "other")

	msg := []byte("sign bytes")

	t.Run("sign by name verifies", func(t *testing.T) {
		s, err := signer.NewKeyring(kr, "r", addr)
		require.NoError(t, err)

		require.Equal(t, addr, s.Address())
		require.True(t, pub.Equals(s.PubKey()))

		sig, err := s.Sign(context.Background(), msg)
		require.NoError(t, err)
		require.Len(t, sig, 64)
		require.True(t, s.PubKey().VerifySignature(msg, sig))
	})

	t.Run("declares no sign-bytes limit", func(t *testing.T) {
		s, err := signer.NewKeyring(kr, "r", addr)
		require.NoError(t, err)

		require.Equal(t, 0, signer.SignBytesLimit(s))
	})

	t.Run("resolves a bech32 address", func(t *testing.T) {
		bech, err := sdk.Bech32ifyAddressBytes(testPrefix, addr)
		require.NoError(t, err)

		s, err := signer.NewKeyring(kr, bech, addr)
		require.NoError(t, err)
		require.Equal(t, addr, s.Address())

		sig, err := s.Sign(context.Background(), msg)
		require.NoError(t, err)
		require.True(t, s.PubKey().VerifySignature(msg, sig))
	})

	t.Run("key that does not derive reporter", func(t *testing.T) {
		_, err := signer.NewKeyring(kr, "r", otherAddr)
		require.EqualError(t, err, "signer does not match reporter")
	})

	t.Run("unknown key", func(t *testing.T) {
		_, err := signer.NewKeyring(kr, "missing", addr)
		require.Error(t, err)
	})

	t.Run("unknown address", func(t *testing.T) {
		bech, err := sdk.Bech32ifyAddressBytes(testPrefix, make([]byte, 20))
		require.NoError(t, err)

		_, err = signer.NewKeyring(kr, bech, addr)
		require.Error(t, err)
	})

	t.Run("non-secp256k1 key", func(t *testing.T) {
		edPub := ed25519.GenPrivKey().PubKey()
		_, err := kr.SaveOfflineKey("ed", edPub)
		require.NoError(t, err)

		_, err = signer.NewKeyring(kr, "ed", sdk.AccAddress(edPub.Address()))
		require.Error(t, err)
		require.NotEqual(t, "signer does not match reporter", err.Error())
	})

	t.Run("cancelled context", func(t *testing.T) {
		s, err := signer.NewKeyring(kr, "r", addr)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = s.Sign(ctx, msg)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("offline key cannot sign", func(t *testing.T) {
		offline := newKeyringOffline(t, kr)
		s, err := signer.NewKeyring(kr, offline.name, offline.addr)
		require.NoError(t, err)

		_, err = s.Sign(context.Background(), msg)
		require.Error(t, err)
	})
}

type offlineKey struct {
	name string
	addr sdk.AccAddress
}

func newKeyringOffline(t *testing.T, kr keyring.Keyring) offlineKey {
	t.Helper()

	_, pub := newKey(t, kr, "tmp")
	require.NoError(t, kr.Delete("tmp"))

	_, err := kr.SaveOfflineKey("offline", pub)
	require.NoError(t, err)

	return offlineKey{name: "offline", addr: sdk.AccAddress(pub.Address())}
}

// rawSign signs msg with a fresh key and returns the pub key and the low-S r||s.
func rawSign(t *testing.T, msg []byte) (cryptotypes.PubKey, []byte) {
	t.Helper()

	kr := newTestKeyring(t)
	newKey(t, kr, "k")

	sig, pub, err := kr.Sign("k", msg, signing.SignMode_SIGN_MODE_DIRECT)
	require.NoError(t, err)
	require.Len(t, sig, 64)

	return pub, sig
}

// highS flips s to N-s, which is the malleable twin of a low-S signature.
func highS(t *testing.T, sig []byte) []byte {
	t.Helper()

	var s secp256k1.ModNScalar
	require.False(t, s.SetByteSlice(sig[32:]))
	s.Negate()

	out := append([]byte(nil), sig...)
	b := s.Bytes()
	copy(out[32:], b[:])

	return out
}

func TestNormalizeAndVerify(t *testing.T) {
	msg := []byte("sign bytes")
	pub, sig := rawSign(t, msg)

	t.Run("low-S passes through", func(t *testing.T) {
		got, err := signer.NormalizeAndVerify(pub, msg, sig)
		require.NoError(t, err)
		require.Equal(t, sig, got)
	})

	t.Run("high-S becomes low-S", func(t *testing.T) {
		high := highS(t, sig)
		require.NotEqual(t, sig, high)
		require.False(t, pub.VerifySignature(msg, high), "sdk must reject high-S")

		got, err := signer.NormalizeAndVerify(pub, msg, high)
		require.NoError(t, err)
		require.Equal(t, sig, got)
		require.True(t, pub.VerifySignature(msg, got))
	})

	t.Run("65-byte r||s||v drops v", func(t *testing.T) {
		for _, v := range []byte{0, 1, 27, 28} {
			got, err := signer.NormalizeAndVerify(pub, msg, append(append([]byte(nil), sig...), v))
			require.NoError(t, err)
			require.Equal(t, sig, got)
		}
	})

	t.Run("65-byte high-S", func(t *testing.T) {
		got, err := signer.NormalizeAndVerify(pub, msg, append(highS(t, sig), 1))
		require.NoError(t, err)
		require.Equal(t, sig, got)
	})

	t.Run("input is not modified", func(t *testing.T) {
		high := highS(t, sig)
		before := append([]byte(nil), high...)

		_, err := signer.NormalizeAndVerify(pub, msg, high)
		require.NoError(t, err)
		require.Equal(t, before, high)
	})

	t.Run("wrong key", func(t *testing.T) {
		otherPub, _ := rawSign(t, msg)

		_, err := signer.NormalizeAndVerify(otherPub, msg, sig)
		require.Error(t, err)
	})

	t.Run("wrong message", func(t *testing.T) {
		_, err := signer.NormalizeAndVerify(pub, []byte("other"), sig)
		require.Error(t, err)
	})

	t.Run("bit flip", func(t *testing.T) {
		for _, i := range []int{0, 31, 32, 63} {
			bad := append([]byte(nil), sig...)
			bad[i] ^= 0x01

			_, err := signer.NormalizeAndVerify(pub, msg, bad)
			require.Error(t, err, "byte %d", i)
		}
	})

	t.Run("bad lengths", func(t *testing.T) {
		for _, n := range []int{0, 1, 63, 66, 128} {
			_, err := signer.NormalizeAndVerify(pub, msg, make([]byte, n))
			require.Error(t, err, "len %d", n)
		}

		_, err := signer.NormalizeAndVerify(pub, msg, sig[:63])
		require.Error(t, err)
	})

	t.Run("zero or out-of-range r and s", func(t *testing.T) {
		n := secp256k1.S256().N.Bytes()
		ones := func() []byte {
			b := make([]byte, 32)
			for i := range b {
				b[i] = 0xff
			}
			return b
		}

		cases := map[string][]byte{
			"zero r":   append(make([]byte, 32), sig[32:]...),
			"zero s":   append(append([]byte(nil), sig[:32]...), make([]byte, 32)...),
			"r == N":   append(append([]byte(nil), n...), sig[32:]...),
			"s == N":   append(append([]byte(nil), sig[:32]...), n...),
			"r > N":    append(ones(), sig[32:]...),
			"s > N":    append(append([]byte(nil), sig[:32]...), ones()...),
			"all zero": make([]byte, 64),
		}
		for name, bad := range cases {
			_, err := signer.NormalizeAndVerify(pub, msg, bad)
			require.Error(t, err, name)
		}
	})

	t.Run("nil key", func(t *testing.T) {
		_, err := signer.NormalizeAndVerify(nil, msg, sig)
		require.Error(t, err)
	})

	t.Run("errors do not echo bytes", func(t *testing.T) {
		secret := []byte("super-secret-sign-bytes")
		bad := append([]byte(nil), sig...)
		bad[5] ^= 0xff

		_, err := signer.NormalizeAndVerify(pub, secret, bad)
		require.Error(t, err)
		require.NotContains(t, err.Error(), string(secret))
		require.NotContains(t, err.Error(), string(bad))
	})
}

package keeper

import (
	"math/big"
	"testing"

	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cosmos/cosmos-sdk/types/address"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	types "pkg.akt.dev/go/node/cert/v1"
)

func TestCertStateToPrefix(t *testing.T) {
	tests := []struct {
		name     string
		state    types.State
		expected []byte
	}{
		{
			name:     "valid certificate state",
			state:    types.CertificateValid,
			expected: CertStateValidPrefix,
		},
		{
			name:     "revoked certificate state",
			state:    types.CertificateRevoked,
			expected: CertStateRevokedPrefix,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := certStateToPrefix(tc.state)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestCertStateToPrefixPanics(t *testing.T) {
	require.Panics(t, func() {
		certStateToPrefix(types.CertificateStateInvalid)
	}, "should panic for invalid certificate state")
}

func TestBuildCertPrefix(t *testing.T) {
	tests := []struct {
		name     string
		state    types.State
		expected []byte
	}{
		{
			name:     "valid certificate state",
			state:    types.CertificateValid,
			expected: append(CertPrefix, CertStateValidPrefix...),
		},
		{
			name:     "revoked certificate state",
			state:    types.CertificateRevoked,
			expected: append(CertPrefix, CertStateRevokedPrefix...),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := buildCertPrefix(tc.state)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestBuildCertPrefixPanics(t *testing.T) {
	require.Panics(t, func() {
		buildCertPrefix(types.CertificateStateInvalid)
	}, "should panic for invalid certificate state")
}

func TestCertificateKey(t *testing.T) {
	owner := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	serial := big.NewInt(123)

	tests := []struct {
		name     string
		state    types.State
		certID   types.CertID
		expected []byte
		wantErr  bool
	}{
		{
			name:  "valid certificate",
			state: types.CertificateValid,
			certID: types.CertID{
				Owner:  owner,
				Serial: *serial,
			},
			wantErr: false,
		},
		{
			name:  "revoked certificate",
			state: types.CertificateRevoked,
			certID: types.CertID{
				Owner:  owner,
				Serial: *serial,
			},
			wantErr: false,
		},
		{
			name:  "empty owner",
			state: types.CertificateValid,
			certID: types.CertID{
				Owner:  sdk.AccAddress{},
				Serial: *serial,
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CertificateKey(tc.state, tc.certID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, result)

			// Verify the key structure
			state, parsedID, err := ParseCertKey(result)
			require.NoError(t, err)
			require.Equal(t, tc.state, state)
			require.Equal(t, tc.certID.Owner, parsedID.Owner)
			require.Equal(t, tc.certID.Serial.String(), parsedID.Serial.String())
		})
	}
}

func TestCertificateKeyRaw(t *testing.T) {
	owner := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	serial := big.NewInt(123)

	tests := []struct {
		name     string
		state    types.State
		certID   types.CertID
		expected []byte
		wantErr  bool
	}{
		{
			name:  "valid key",
			state: types.CertificateValid,
			certID: types.CertID{
				Owner:  owner,
				Serial: *serial,
			},
			expected: append(append(append(CertPrefix, CertStateValidPrefix...), address.MustLengthPrefix(owner.Bytes())...), mustSerialPrefix(serial.Bytes())...),
			wantErr:  false,
		},
		{
			name:  "valid key 2",
			state: types.CertificateRevoked,
			certID: types.CertID{
				Owner:  owner,
				Serial: *serial,
			},
			expected: append(append(append(CertPrefix, CertStateRevokedPrefix...), address.MustLengthPrefix(owner.Bytes())...), mustSerialPrefix(serial.Bytes())...),
			wantErr:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := CertificateKey(tc.state, tc.certID)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, result)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestMustCertificateKey(t *testing.T) {
	owner := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	serial := big.NewInt(123)
	validID := types.CertID{
		Owner:  owner,
		Serial: *serial,
	}

	t.Run("valid case", func(t *testing.T) {
		require.NotPanics(t, func() {
			key := MustCertificateKey(types.CertificateValid, validID)
			require.NotEmpty(t, key)
		})
	})

	t.Run("panic on invalid input", func(t *testing.T) {
		require.Panics(t, func() {
			MustCertificateKey(types.CertificateValid, types.CertID{})
		})
	})
}

func TestParseCertKey(t *testing.T) {
	owner := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	serial := big.NewInt(123)
	certID := types.CertID{
		Owner:  owner,
		Serial: *serial,
	}

	tests := []struct {
		name    string
		key     []byte
		state   types.State
		certID  types.CertID
		wantErr bool
	}{
		{
			name:    "valid key - valid state",
			key:     MustCertificateKey(types.CertificateValid, certID),
			state:   types.CertificateValid,
			certID:  certID,
			wantErr: false,
		},
		{
			name:    "valid key - revoked state",
			key:     MustCertificateKey(types.CertificateRevoked, certID),
			state:   types.CertificateRevoked,
			certID:  certID,
			wantErr: false,
		},
		{
			name:    "invalid key - too short",
			key:     []byte{0x11},
			wantErr: true,
		},
		{
			name:    "invalid key - wrong prefix",
			key:     []byte{0x12, 0x01, 0x20},
			wantErr: true,
		},
		{
			name:    "invalid key - invalid address length",
			key:     append(append(CertPrefix, CertStateValidPrefix...), 0xFF),
			wantErr: true,
		},
		{
			name:    "invalid key - malformed address",
			key:     append(append(CertPrefix, CertStateValidPrefix...), 0x01, 0x00),
			wantErr: true,
		},
		{
			name:    "invalid key - malformed address 2",
			key:     append(append(CertPrefix, CertStateValidPrefix...), 0x13, 0x00),
			wantErr: true,
		},
		{
			name:    "invalid key - malformed address 2",
			key:     append(append(CertPrefix, CertStateValidPrefix...), 0x04, 0x00, 0x01, 0x02, 0x03, 0x04, 0x00),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, parsedID, err := ParseCertKey(tc.key)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.state, state)
			require.Equal(t, tc.certID.Owner.String(), parsedID.Owner.String())
			require.Equal(t, tc.certID.Serial.String(), parsedID.Serial.String())
		})
	}
}

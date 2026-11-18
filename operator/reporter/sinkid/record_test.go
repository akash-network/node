package sinkid_test

import (
	"math"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gagliardetto/solana-go"
	"github.com/stretchr/testify/require"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"

	"pkg.akt.dev/node/v2/operator/reporter/sinkid"
)

const (
	denomUakt    = "uakt"
	errRecipient = "noncanonical recipient"
)

func transfer(t *testing.T, dest string, coin sdk.Coin) sinkv1.TransferState {
	t.Helper()

	addr, _ := ownerOf(t, 20, 0x01)

	return sinkv1.TransferState{
		ID:   sinkv1.ID{Owner: addr, Height: 10, Sequence: 2},
		Data: sinkv1.RecordData{Dest: dest, Amount: coin},
	}
}

func TestDecodeRecord(t *testing.T) {
	recipient := solana.NewWallet().PublicKey()

	t.Run("valid", func(t *testing.T) {
		state := transfer(t, recipient.String(), sdk.NewInt64Coin(denomUakt, 1500))

		rec, err := sinkid.DecodeRecord(state)
		require.NoError(t, err)

		id, err := sinkid.FromProto(state.ID)
		require.NoError(t, err)
		require.Equal(t, sinkid.Record{ID: id, Recipient: recipient, Amount: 1500}, rec)
	})

	t.Run("max uint64", func(t *testing.T) {
		coin := sdk.NewCoin(denomUakt, sdkmath.NewIntFromUint64(math.MaxUint64))

		rec, err := sinkid.DecodeRecord(transfer(t, recipient.String(), coin))
		require.NoError(t, err)
		require.Equal(t, uint64(math.MaxUint64), rec.Amount)
	})

	for name, tc := range map[string]struct {
		state func(t *testing.T) sinkv1.TransferState
		err   string
	}{
		"missing record data": {
			state: func(t *testing.T) sinkv1.TransferState {
				return transfer(t, "", sdk.Coin{})
			},
			err: "missing record data",
		},
		"noncanonical recipient": {
			state: func(t *testing.T) sinkv1.TransferState {
				return transfer(t, recipient.String()+"0", sdk.NewInt64Coin(denomUakt, 1))
			},
			err: errRecipient,
		},
		"short recipient": {
			state: func(t *testing.T) sinkv1.TransferState {
				return transfer(t, "abc", sdk.NewInt64Coin(denomUakt, 1))
			},
			err: errRecipient,
		},
		"transfer amount exceeds uint64": {
			state: func(t *testing.T) sinkv1.TransferState {
				over := sdkmath.NewIntFromUint64(math.MaxUint64).AddRaw(1)
				return transfer(t, recipient.String(), sdk.Coin{Denom: denomUakt, Amount: over})
			},
			err: "transfer amount exceeds uint64",
		},
		"invalid uakt Coin": {
			state: func(t *testing.T) sinkv1.TransferState {
				return transfer(t, recipient.String(), sdk.Coin{Denom: "uatom", Amount: sdkmath.NewInt(5)})
			},
			err: "invalid uakt Coin",
		},
		"amount must be positive": {
			state: func(t *testing.T) sinkv1.TransferState {
				return transfer(t, recipient.String(), sdk.NewInt64Coin(denomUakt, 0))
			},
			err: "amount must be positive",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := sinkid.DecodeRecord(tc.state(t))
			require.EqualError(t, err, tc.err)
		})
	}

	t.Run("invalid ID", func(t *testing.T) {
		state := transfer(t, recipient.String(), sdk.NewInt64Coin(denomUakt, 1))
		state.ID.Height = 0

		_, err := sinkid.DecodeRecord(state)
		require.EqualError(t, err, errCanonical)
	})
}

func TestCoinAmount(t *testing.T) {
	t.Run("zero allowed unless positive", func(t *testing.T) {
		amount, err := sinkid.CoinAmount(sdk.NewInt64Coin(denomUakt, 0), false)
		require.NoError(t, err)
		require.True(t, amount.IsZero())

		_, err = sinkid.CoinAmount(sdk.NewInt64Coin(denomUakt, 0), true)
		require.EqualError(t, err, "amount must be positive")
	})

	t.Run("uninitialized amount", func(t *testing.T) {
		_, err := sinkid.CoinAmount(sdk.Coin{Denom: denomUakt}, false)
		require.EqualError(t, err, "invalid uakt Coin")
	})

	t.Run("negative amount", func(t *testing.T) {
		_, err := sinkid.CoinAmount(sdk.Coin{Denom: denomUakt, Amount: sdkmath.NewInt(-1)}, false)
		require.EqualError(t, err, "invalid uakt Coin")
	})

	t.Run("wrong denom", func(t *testing.T) {
		_, err := sinkid.CoinAmount(sdk.Coin{Denom: "akt", Amount: sdkmath.NewInt(1)}, false)
		require.EqualError(t, err, "invalid uakt Coin")
	})

	t.Run("beyond uint64 is not an error", func(t *testing.T) {
		big := sdkmath.NewIntFromUint64(math.MaxUint64).MulRaw(4)
		amount, err := sinkid.CoinAmount(sdk.Coin{Denom: denomUakt, Amount: big}, true)
		require.NoError(t, err)
		require.True(t, amount.Equal(big))
	})
}

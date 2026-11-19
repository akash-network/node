package sinkid

import (
	"errors"
	"math"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gagliardetto/solana-go"

	sinkv1 "pkg.akt.dev/go/node/sink/v1"
	"pkg.akt.dev/go/sdkutil"
)

var (
	errMissingData  = errors.New("missing record data")
	errRecipient    = errors.New("noncanonical recipient")
	errAmountTooBig = errors.New("transfer amount exceeds uint64")
	errInvalidCoin  = errors.New("invalid uakt Coin")
	errNotPositive  = errors.New("amount must be positive")
	maxUint64Amount = sdkmath.NewIntFromUint64(math.MaxUint64)
)

// Record is a decoded sink transfer: who gets paid and how much.
type Record struct {
	ID        ID
	Recipient solana.PublicKey
	Amount    uint64
}

// DecodeRecord validates a chain transfer and extracts its Solana recipient and uakt amount.
func DecodeRecord(transfer sinkv1.TransferState) (Record, error) {
	id, err := FromProto(transfer.ID)
	if err != nil {
		return Record{}, err
	}

	data := transfer.Data
	if data.Dest == "" && data.Amount.Denom == "" && data.Amount.Amount.IsNil() {
		return Record{}, errMissingData
	}

	recipient, err := solana.PublicKeyFromBase58(data.Dest)
	if err != nil {
		return Record{}, errRecipient
	}

	amount, err := CoinAmount(data.Amount, true)
	if err != nil {
		return Record{}, err
	}

	if recipient.String() != data.Dest {
		return Record{}, errRecipient
	}

	if amount.GT(maxUint64Amount) {
		return Record{}, errAmountTooBig
	}

	return Record{ID: id, Recipient: recipient, Amount: amount.Uint64()}, nil
}

// CoinAmount returns the amount of an initialized nonnegative uakt Coin, optionally requiring it to be positive.
func CoinAmount(coin sdk.Coin, positive bool) (sdkmath.Int, error) {
	if coin.Denom != sdkutil.DenomUakt || coin.Amount.IsNil() || coin.Amount.IsNegative() {
		return sdkmath.Int{}, errInvalidCoin
	}

	if positive && !coin.Amount.IsPositive() {
		return sdkmath.Int{}, errNotPositive
	}

	return coin.Amount, nil
}

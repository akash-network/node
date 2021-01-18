package sdl

import (
	"errors"
	"fmt"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gopkg.in/yaml.v3"
	"math/big"
)

// v2Coin is an alias sdk.Coin to allow our custom UnmarshalYAML
// for now it supports PoC when actual pricing is specified as two fields
// aka amount and denom. we let UnmarshalYAML to deal with that and put result
// into Value field.
// discussion https://github.com/ovrclk/akash/issues/771
type v2Coin struct {
	Value sdk.Coin `yaml:"-"`
}

var errInvalidCoinAmount = errors.New("invalid coin amount")

func (sdl *v2Coin) UnmarshalYAML(node *yaml.Node) error {
	parsedCoin := struct {
		Amount string `yaml:"amount"`
		Denom  string `yaml:"denom"`
	}{}

	if err := node.Decode(&parsedCoin); err != nil {
		return err
	}

	asFloat, _, err := big.ParseFloat(parsedCoin.Amount, 0, 54, big.AwayFromZero)
	if err != nil {
		return err
	}
	if !asFloat.IsInt() {
		return fmt.Errorf("%w: %q is not an integer", errInvalidCoinAmount, parsedCoin.Amount)
	}

	coin, err := sdk.ParseCoinNormalized(parsedCoin.Amount + parsedCoin.Denom)
	if err != nil {
		return err
	}

	*sdl = v2Coin{
		Value: coin,
	}

	return nil
}

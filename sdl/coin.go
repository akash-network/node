package sdl

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gopkg.in/yaml.v3"
)

// v2Coin is an alias sdk.Coin to allow our custom UnmarshalYAML
// for now it supports PoC when actual pricing is specified as two fields
// aka amount and denom. we let UnmarshalYAML to deal with that and put result
// into Value field.
// discussion https://github.com/ovrclk/akash/issues/771
type v2Coin struct {
	Value sdk.Coin `yaml:"-"`
}

func (sdl *v2Coin) UnmarshalYAML(node *yaml.Node) error {
	parsedCoin := struct {
		Amount string `yaml:"amount"`
		Denom  string `yaml:"denom"`
	}{}

	if err := node.Decode(&parsedCoin); err != nil {
		return err
	}

	coin, err := sdk.ParseCoin(parsedCoin.Amount + parsedCoin.Denom)
	if err != nil {
		return err
	}

	*sdl = v2Coin{
		Value: coin,
	}

	return nil
}

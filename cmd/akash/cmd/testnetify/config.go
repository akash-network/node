package testnetify

import (
	"fmt"
	"strings"
	"time"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

type AccAddress struct {
	sdk.AccAddress
}

type ValAddress struct {
	sdk.ValAddress
}

type ConsAddress struct {
	sdk.ConsAddress
}

type JSONCoin struct {
	sdk.Coin
}

type JSONCoins []JSONCoin

type PubKey struct {
	cryptotypes.PubKey
}

type VotingPeriod struct {
	time.Duration
}

type AccountConfig struct {
	Address AccAddress `json:"address"`
	PubKey  PubKey     `json:"pubkey"`
	Coins   JSONCoins  `json:"coins,omitempty"`
}

type Delegator struct {
	Address AccAddress `json:"address"`
	Coins   JSONCoins  `json:"coins"`
}

type ValidatorConfig struct {
	PubKey     PubKey                       `json:"pubkey"`
	Name       string                       `json:"name"`
	Bonded     bool                         `json:"bonded"`
	Delegators []Delegator                  `json:"delegators,omitempty"`
	Rates      stakingtypes.CommissionRates `json:"rates"`
}

type AccountsConfig struct {
	Add []AccountConfig `json:"add,omitempty"`
	Del []AccAddress    `json:"del,omitempty"`
}

type GovConfig struct {
	VotingParams *struct {
		VotingPeriod VotingPeriod `json:"voting_period,omitempty"`
	} `json:"voting_params,omitempty"`
}

type ValidatorsConfig struct {
	Add []ValidatorConfig `json:"add,omitempty"`
	Del []AccAddress      `json:"del,omitempty"`
}

type IBCConfig struct {
	Prune bool `json:"prune"`
}

type EscrowConfig struct {
	PatchDanglingPayments bool `json:"patch_dangling_payments"`
}

type config struct {
	ChainID    *string           `json:"chain_id"`
	Accounts   *AccountsConfig   `json:"accounts,omitempty"`
	Validators *ValidatorsConfig `json:"validators"`
	IBC        *IBCConfig        `json:"ibc"`
	Escrow     *EscrowConfig     `json:"escrow"`
	Gov        *GovConfig        `json:"gov,omitempty"`
}

func (t *VotingPeriod) UnmarshalJSON(data []byte) error {
	val := TrimQuotes(string(data))

	if !strings.HasSuffix(val, "s") {
		return fmt.Errorf("invalid format of voting period. must contain time unit. Valid time units are ns|us(µs)|ms|s|m|h") // nolint: goerr113
	}

	var err error
	t.Duration, err = time.ParseDuration(val)
	if err != nil {
		return err
	}

	return nil
}

func (k *PubKey) UnmarshalJSON(data []byte) error {
	if err := cdc.UnmarshalInterfaceJSON(data, &k.PubKey); err != nil {
		return err
	}

	return nil
}

func (s *ValAddress) UnmarshalJSON(data []byte) error {
	var err error
	if s.ValAddress, err = sdk.ValAddressFromBech32(TrimQuotes(string(data))); err != nil {
		return err
	}

	return nil
}

func (s *AccAddress) UnmarshalJSON(data []byte) error {
	var err error
	if s.AccAddress, err = sdk.AccAddressFromBech32(TrimQuotes(string(data))); err != nil {
		return err
	}

	return nil
}

func (s *ConsAddress) UnmarshalJSON(data []byte) error {
	var err error
	if s.ConsAddress, err = sdk.ConsAddressFromBech32(TrimQuotes(string(data))); err != nil {
		return err
	}

	return nil
}

func (k *JSONCoin) UnmarshalJSON(data []byte) error {
	coin, err := sdk.ParseCoinNormalized(TrimQuotes(string(data)))
	if err != nil {
		return err
	}

	k.Coin = coin

	return nil
}

func (k JSONCoins) ToSDK() sdk.Coins {
	coins := make(sdk.Coins, 0, len(k))

	for _, coin := range k {
		coins = append(coins, coin.Coin)
	}

	return coins
}

func TrimQuotes(data string) string {
	data = strings.TrimPrefix(data, "\"")
	return strings.TrimSuffix(data, "\"")
}

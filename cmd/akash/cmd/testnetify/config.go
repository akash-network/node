package testnetify

import (
	"strings"

	"github.com/cometbft/cometbft/crypto"
	cmtjson "github.com/cometbft/cometbft/libs/json"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/types"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	akash "pkg.akt.dev/node/app"
)

type PrivValidatorKey struct {
	Address types.Address  `json:"address"`
	PubKey  crypto.PubKey  `json:"pub_key"`
	PrivKey crypto.PrivKey `json:"priv_key"`
}

type NodeKey struct {
	PrivKey crypto.PrivKey `json:"priv_key"`
}

type Keys struct {
	Priv PrivValidatorKey `json:"priv"`
	Node NodeKey          `json:"node"`
}
type AccAddress struct {
	sdk.AccAddress
}

type ValAddress struct {
	sdk.ValAddress
}

type ConsAddress struct {
	sdk.ConsAddress
}

type TestnetValidator struct {
	Moniker           string                    `json:"moniker"`
	Operator          AccAddress                `json:"operator"`
	Status            stakingtypes.BondStatus   `json:"status"`
	Commission        stakingtypes.Commission   `json:"commission"`
	MinSelfDelegation sdkmath.Int               `json:"min_self_delegation"`
	Home              string                    `json:"home"`
	Delegations       []akash.TestnetDelegation `json:"delegations"`

	privValidator    *pvm.FilePV
	pubKey           crypto.PubKey
	validatorAddress crypto.Address
	consAddress      sdk.ConsAddress
}

type TestnetValidators []TestnetValidator

type TestnetConfig struct {
	ChainID    string                 `json:"chain_id"`
	Validators TestnetValidators      `json:"validators"`
	Accounts   []akash.TestnetAccount `json:"accounts"`
	Gov        akash.TestnetGov       `json:"gov"`
	upgrade    akash.TestnetUpgrade
}

func TrimQuotes(data string) string {
	data = strings.TrimPrefix(data, "\"")
	return strings.TrimSuffix(data, "\"")
}

func (k *PrivValidatorKey) UnmarshalJSON(data []byte) error {
	err := cmtjson.Unmarshal(data, k)
	if err != nil {
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

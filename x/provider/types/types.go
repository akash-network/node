package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Attributes []sdk.Attribute

// Provider stores owner and host details
type Provider struct {
	Owner      sdk.AccAddress `json:"owner"`
	HostURI    string         `json:"host-uri"`
	Attributes Attributes     `json:"attributes"`
}

func (attr Attributes) Validate() error {
	store := make(map[string]bool)

	for i := range attr {
		if _, ok := store[attr[i].Key]; ok {
			return ErrDuplicateAttributes
		}

		store[attr[i].Key] = true
	}

	return nil
}

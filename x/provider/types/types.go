package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Attributes []sdk.Attribute

// Provider stores owner and host details
type Provider struct {
	Owner         sdk.AccAddress `json:"owner"`
	HostURI       string         `json:"host-uri"`
	Attributes    Attributes     `json:"attributes"`
	ReqAttributes Attributes     `json:"req-attributes" yaml:"req-attributes"`
}

// MatchReqAttributes method compares provided attributes with specific provider require attributes
func (p Provider) MatchReqAttributes(attrs []sdk.Attribute) bool {
loop:
	for _, req := range p.ReqAttributes {
		for _, attr := range attrs {
			if req.Key == attr.Key && req.Value == attr.Value {
				continue loop
			}
		}
		return false
	}
	return true
}

// GetAllAttributes appends normal and required attributes of provider
func (p Provider) GetAllAttributes() []sdk.Attribute {
	return append(p.Attributes, p.ReqAttributes...)
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

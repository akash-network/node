package query

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/provider/types"
)

type (
	// Provider type
	Provider types.Provider
	// Providers - Slice of Provider Struct
	Providers []Provider
)

func (p Provider) String() string {
	return fmt.Sprintf(`Deployment
	Owner:   %s
	HostURI: %s
	Attributes: %v
	ReqAttributes:%v
	`, p.Owner, p.HostURI, p.Attributes, p.ReqAttributes)
}

func (obj Providers) String() string {
	var buf bytes.Buffer

	const sep = "\n\n"

	for _, p := range obj {
		buf.WriteString(p.String())
		buf.WriteString(sep)
	}

	if len(obj) > 0 {
		buf.Truncate(buf.Len() - len(sep))
	}

	return buf.String()
}

// Address implements provider and returns owner of provider
func (p *Provider) Address() sdk.AccAddress {
	return p.Owner
}

// MatchReqAttributes method compares provided attributes with specific provider require attributes
func (p Provider) MatchReqAttributes(attrs []sdk.Attribute) bool {
	return types.Provider(p).MatchReqAttributes(attrs)
}

// GetAllAttributes appends normal and required attributes of provider
func (p Provider) GetAllAttributes() []sdk.Attribute {
	return types.Provider(p).GetAllAttributes()
}

package types

import (
	"bytes"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/types"
)

type Attributes []types.Attribute

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

// String implements the Stringer interface for a Provider object.
func (p Provider) String() string {
	return fmt.Sprintf(`Deployment
	Owner:   %s
	HostURI: %s
	Attributes: %v
	`, p.Owner, p.HostURI, p.Attributes)
}

// Providers is the collection of Provider
type Providers []Provider

// String implements the Stringer interface for a Providers object.
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

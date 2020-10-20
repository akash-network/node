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
	`, p.Owner, p.HostURI, p.Attributes)
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
	owner, err := sdk.AccAddressFromBech32(p.Owner)
	if err != nil {
		panic(err)
	}

	return owner
}

package v1beta1

import (
	"bytes"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ProviderID struct {
	Owner   sdk.Address
	Auditor sdk.Address
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

package query

import (
	"bytes"
	"fmt"

	"github.com/ovrclk/akash/x/deployment/types"
)

// Deployment stores deployment and groups details
type Deployment struct {
	types.Deployment `json:"deployment"`
	Groups           []types.Group `json:"groups"`
}

func (d Deployment) String() string {
	return fmt.Sprintf(`Deployment
	Owner:   %s
	DSeq:    %d
	State:   %v
	Version: %s
	Num Groups: %d
	`, d.Owner, d.DSeq, d.State, d.Version, len(d.Groups))
}

// Deployments - Slice of deployment struct
type Deployments []Deployment

func (ds Deployments) String() string {
	var buf bytes.Buffer

	const sep = "\n\n"

	for _, d := range ds {
		buf.WriteString(d.String())
		buf.WriteString(sep)
	}

	if len(ds) > 0 {
		buf.Truncate(buf.Len() - len(sep))
	}

	return buf.String()
}

// Group stores group ID, state and other specifications
type Group types.Group

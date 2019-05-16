package deployment

import (
	"github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/dsky"
)

func AppendLease(lease *types.Lease, ld dsky.SectionData) {
	ld.Add("Lease ID", lease.LeaseID).
		Add("Price", humanize.Comma(int64(lease.Price)))

	switch lease.State {
	case types.Lease_ACTIVE:
		ld.Add("State", dsky.Color.Hi.Sprint(lease.State.String()))
	}
}

// MakeLease creates a new SectionData for the Lease
func MakeLease(lease *types.Lease) dsky.SectionData {
	ld := dsky.NewSectionData("")
	AppendLease(lease, ld)
	return ld
}

func AppendProvider(p *types.Provider, data dsky.SectionData) {
	data.
		Add("Address", X(p.Address)).
		Add("Owner", X(p.Owner)).
		Add("Host URI", p.HostURI)
	attrs := make(map[string]string)
	for _, a := range p.Attributes {
		attrs[a.Name] = a.Value
	}
	data.Add("Attributes", attrs)
}

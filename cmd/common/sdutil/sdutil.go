package sdutil

import (
	"fmt"

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

func AppendLeaseStatus(status *types.LeaseStatusResponse, sd dsky.SectionData) {
	for _, service := range status.Services {
		name, avail, total := " ", "0", "0"
		if len(service.Name) > 0 {
			name = service.Name
		}
		if service.Available > 0 {
			avail = fmt.Sprintf("%d", service.Available)
		}

		if service.Total > 0 {
			total = fmt.Sprintf("%d", service.Total)
		}
		for _, uri := range service.URIs {
			sd.Add("Name", name)
			sd.Add("Hosts", uri).WithLabel("Hosts", "Host(s) / IP(s)")
			sd.Add("Available", avail)
			sd.Add("Total", total)
		}
	}
}

func AppendTxCreateFulfilment(ff []*types.TxCreateFulfillment, data dsky.SectionData) {
	for _, tx := range ff {
		data.Add("Group", tx.Group).Add("Price", tx.Price).Add("Provider", tx.Provider.String())
	}
}

func AppendGroupSpec(groups []*types.GroupSpec, data dsky.SectionData) {
	for _, g := range groups {
		data.Add("Group", g.Name)
		reqs := make(map[string]string)
		for _, a := range g.Requirements {
			reqs[a.Name] = a.Value
		}
		data.Add("Requirements", reqs)
		rd := dsky.NewSectionData(" ")
		AppendResourceGroup(g.Resources, rd)
		data.Add("Resources", rd)
	}
}

func AppendResourceGroup(rg []types.ResourceGroup, data dsky.SectionData) {
	for _, r := range rg {
		data.Add("Count", r.Count)
		data.Add("Price", r.Price)
		data.Add("CPU", r.Unit.CPU)
		data.Add("Memory", r.Unit.Memory)
		data.Add("Disk", r.Unit.Disk)
	}
}

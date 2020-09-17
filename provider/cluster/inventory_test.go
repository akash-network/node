package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

func newResourceUnits() types.ResourceUnits {
	return types.ResourceUnits{
		CPU:    &types.CPU{Units: types.NewResourceValue(1000)},
		Memory: &types.Memory{Quantity: types.NewResourceValue(10 * unit.Gi)},
	}
}

func TestInventory_reservationAllocateable(t *testing.T) {
	mkrg := func(cpu uint64, memory uint64, count uint32) dtypes.Resource {
		return dtypes.Resource{
			Resources: types.ResourceUnits{
				CPU: &types.CPU{
					Units: types.NewResourceValue(cpu),
				},
				Memory: &types.Memory{
					Quantity: types.NewResourceValue(memory),
				},
			},
			Count: count,
		}
	}

	mkres := func(allocated bool, res ...dtypes.Resource) *reservation {
		return &reservation{
			allocated: allocated,
			resources: &dtypes.GroupSpec{Resources: res},
		}
	}

	inventory := []Node{
		NewNode("a", newResourceUnits()),
		NewNode("b", newResourceUnits()),
	}

	reservations := []*reservation{
		mkres(false, mkrg(750, 3*unit.Gi, 1)),
		mkres(false, mkrg(100, 4*unit.Gi, 2)),
		mkres(true, mkrg(2000, 3*unit.Gi, 2)),
		mkres(true, mkrg(250, 12*unit.Gi, 2)),
	}

	tests := []struct {
		res *reservation
		ok  bool
	}{
		{mkres(false, mkrg(100, 1*unit.G, 2)), true},
		{mkres(false, mkrg(100, 4*unit.G, 1)), true},
		{mkres(false, mkrg(250, 1*unit.G, 1)), false},
		{mkres(false, mkrg(1000, 1*unit.G, 1)), false},
		{mkres(false, mkrg(100, 9*unit.G, 1)), false},
	}

	assert.Equal(t, tests[0].ok, reservationAllocateable(inventory, reservations, tests[0].res))
	reservations[0].allocated = true
	reservations[1].allocated = true

	for _, test := range tests[1:] {
		assert.Equal(t, test.ok, reservationAllocateable(inventory, reservations, test.res))
	}
}

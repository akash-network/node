package cluster

import (
	"testing"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/unit"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	"github.com/stretchr/testify/assert"
)

const (
	randCPU    uint32 = 1000
	randMemory uint64 = 10 * unit.Gi
)

func TestInventory_reservationAllocateable(t *testing.T) {

	mkrg := func(cpu uint32, memory uint64, count uint32) dtypes.Resource {
		return dtypes.Resource{
			Unit: types.Unit{
				CPU:    cpu,
				Memory: memory,
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
		NewNode("a", types.Unit{CPU: randCPU, Memory: randMemory}),
		NewNode("b", types.Unit{CPU: randCPU, Memory: randMemory}),
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
		{mkres(false, mkrg(250, 1*unit.G, 1)), true},
		{mkres(false, mkrg(1000, 1*unit.G, 1)), false},
		{mkres(false, mkrg(100, 9*unit.G, 1)), false},
	}

	for _, test := range tests {
		assert.Equal(t, test.ok, reservationAllocateable(inventory, reservations, test.res))
	}

}

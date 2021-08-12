package kube

import (
	"math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"k8s.io/apimachinery/pkg/api/resource"

	akashv1 "github.com/ovrclk/akash/pkg/apis/akash.network/v1"
	"github.com/ovrclk/akash/types"
)

type resourcePair struct {
	allocatable resource.Quantity
	allocated   resource.Quantity
}

type clusterStorage map[string]*resourcePair

func (cs clusterStorage) dup() clusterStorage {
	res := make(clusterStorage, len(cs))
	for class, resources := range cs {
		res[class] = resources.dup()
	}

	return res
}

func rpNewFromAkash(res akashv1.ResourcePair) *resourcePair {
	return &resourcePair{
		allocatable: *resource.NewQuantity(int64(res.Allocatable), resource.DecimalSI),
		allocated:   *resource.NewQuantity(int64(res.Allocated), resource.DecimalSI),
	}
}

func (rp *resourcePair) dup() *resourcePair {
	return &resourcePair{
		allocatable: rp.allocatable.DeepCopy(),
		allocated:   rp.allocated.DeepCopy(),
	}
}

func (rp *resourcePair) subMilliNLZ(val types.ResourceValue) bool {
	avail := rp.available()

	res := sdk.NewInt(avail.MilliValue())
	res = res.Sub(val.Val)
	if res.IsNegative() {
		return false
	}

	allocated := rp.allocated.DeepCopy()
	allocated.Add(*resource.NewMilliQuantity(int64(val.Value()), resource.DecimalSI))
	*rp = resourcePair{
		allocatable: rp.allocatable.DeepCopy(),
		allocated:   allocated,
	}

	return true
}

func (rp *resourcePair) subNLZ(val types.ResourceValue) bool {
	avail := rp.available()

	res := sdk.NewInt(avail.Value())
	res = res.Sub(val.Val)

	if res.IsNegative() {
		return false
	}

	allocated := rp.allocated.DeepCopy()
	allocated.Add(*resource.NewQuantity(int64(val.Value()), resource.DecimalSI))

	*rp = resourcePair{
		allocatable: rp.allocatable.DeepCopy(),
		allocated:   allocated,
	}

	return true
}

func (rp resourcePair) available() resource.Quantity {
	result := rp.allocatable.DeepCopy()

	if result.Value() == -1 {
		result = *resource.NewQuantity(math.MaxInt64, resource.DecimalSI)
	}

	// Modifies the value in place
	(&result).Sub(rp.allocated)
	return result
}

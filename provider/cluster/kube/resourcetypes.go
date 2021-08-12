package kube

import (
	"math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/ovrclk/akash/types"
)

type resourcePair struct {
	allocatable resource.Quantity
	allocated   resource.Quantity
}

type storageClassState struct {
	resourcePair
	isActive  bool
	isDefault bool
}

type clusterStorage map[string]*storageClassState

func (cs clusterStorage) dup() clusterStorage {
	res := make(clusterStorage)
	for k, v := range cs {
		res[k] = &storageClassState{
			resourcePair: v.resourcePair.dup(),
			isActive:     v.isActive,
			isDefault:    v.isDefault,
		}
	}

	return res
}

func (rp *resourcePair) dup() resourcePair {
	return resourcePair{
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

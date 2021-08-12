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

func (rp *resourcePair) subMilliNLZ(val types.ResourceValue) (resourcePair, bool) {
	avail := rp.available()

	res := sdk.NewInt(avail.MilliValue()).Sub(val.Val)
	if res.IsNegative() {
		return resourcePair{}, false
	}

	return resourcePair{
		allocatable: rp.allocatable.DeepCopy(),
		allocated:   *resource.NewMilliQuantity(res.Int64(), resource.DecimalSI),
	}, true
}

func (rp *resourcePair) subNLZ(val types.ResourceValue) (resourcePair, bool) {
	avail := rp.available()

	res := sdk.NewInt(avail.Value()).Sub(val.Val)
	if res.IsNegative() {
		return resourcePair{}, false
	}

	return resourcePair{
		allocatable: rp.allocatable.DeepCopy(),
		allocated:   *resource.NewQuantity(res.Int64(), resource.DecimalSI),
	}, true
}

func (rp resourcePair) available() resource.Quantity {
	result := rp.allocatable.DeepCopy()

	//
	if result.Value() == -1 {
		result = *resource.NewQuantity(math.MaxInt64, resource.DecimalSI)
	}

	// Modifies the value in place
	(&result).Sub(rp.allocated)
	return result
}

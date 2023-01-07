package migrate

import (
	"github.com/akash-network/node/types/v1beta1"
	"github.com/akash-network/node/types/v1beta2"
)

func ResourceValueFromV1Beta1(from v1beta1.ResourceValue) v1beta2.ResourceValue {
	return v1beta2.NewResourceValue(from.Value())
}

func AttributesFromV1Beta1(from v1beta1.Attributes) v1beta2.Attributes {
	res := make(v1beta2.Attributes, 0, len(from))

	for _, attr := range from {
		res = append(res, v1beta2.Attribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}

	return res
}

func SignedByFromV1Beta1(from v1beta1.SignedBy) v1beta2.SignedBy {
	return v1beta2.SignedBy{
		AllOf: from.AllOf,
		AnyOf: from.AnyOf,
	}
}

func PlacementRequirementsFromV1Beta1(from v1beta1.PlacementRequirements) v1beta2.PlacementRequirements {
	res := v1beta2.PlacementRequirements{
		SignedBy:   SignedByFromV1Beta1(from.SignedBy),
		Attributes: AttributesFromV1Beta1(from.Attributes),
	}

	return res
}

func CPUFromV1Beta1(from *v1beta1.CPU) *v1beta2.CPU {
	if from == nil {
		return nil
	}

	return &v1beta2.CPU{
		Units:      ResourceValueFromV1Beta1(from.Units),
		Attributes: AttributesFromV1Beta1(from.Attributes),
	}
}

func MemoryFromV1Beta1(from *v1beta1.Memory) *v1beta2.Memory {
	if from == nil {
		return nil
	}

	return &v1beta2.Memory{
		Quantity:   ResourceValueFromV1Beta1(from.Quantity),
		Attributes: AttributesFromV1Beta1(from.Attributes),
	}
}

func VolumesFromV1Beta1(from *v1beta1.Storage) v1beta2.Volumes {
	var res v1beta2.Volumes
	if from != nil {
		res = append(res, v1beta2.Storage{
			Name:       "default",
			Quantity:   ResourceValueFromV1Beta1(from.Quantity),
			Attributes: AttributesFromV1Beta1(from.Attributes),
		})
	}

	return res
}

func EndpointsFromV1Beta1(from []v1beta1.Endpoint) []v1beta2.Endpoint {
	res := make([]v1beta2.Endpoint, 0, len(from))

	for _, endpoint := range from {
		res = append(res, v1beta2.Endpoint{
			Kind:           v1beta2.Endpoint_Kind(endpoint.Kind),
			SequenceNumber: 0, // All previous data does not have a use for sequence number
		})
	}

	return res
}

func ResourceUnitsFromV1Beta1(from v1beta1.ResourceUnits) v1beta2.ResourceUnits {
	return v1beta2.ResourceUnits{
		CPU:       CPUFromV1Beta1(from.CPU),
		Memory:    MemoryFromV1Beta1(from.Memory),
		Storage:   VolumesFromV1Beta1(from.Storage),
		Endpoints: EndpointsFromV1Beta1(from.Endpoints),
	}
}

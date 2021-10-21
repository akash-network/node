package migrate

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	amigrate "github.com/ovrclk/akash/types/v1beta2/migrate"
	"github.com/ovrclk/akash/x/deployment/types/v1beta1"
	"github.com/ovrclk/akash/x/deployment/types/v1beta2"
)

func ResourceFromV1Beta1(from v1beta1.Resource) v1beta2.Resource {
	return v1beta2.Resource{
		Resources: amigrate.ResourceUnitsFromV1Beta1(from.Resources),
		Count:     from.Count,
		Price:     sdk.NewDecCoinFromCoin(from.Price),
	}
}

func ResourcesFromV1Beta1(from []v1beta1.Resource) []v1beta2.Resource {
	res := make([]v1beta2.Resource, 0, len(from))

	for _, oval := range from {
		res = append(res, ResourceFromV1Beta1(oval))
	}

	return res
}

func GroupIDFromV1Beta1(from v1beta1.GroupID) v1beta2.GroupID {
	return v1beta2.GroupID{
		Owner: from.Owner,
		DSeq:  from.DSeq,
		GSeq:  from.GSeq,
	}
}

func GroupSpecFromV1Beta1(from v1beta1.GroupSpec) v1beta2.GroupSpec {
	return v1beta2.GroupSpec{
		Name:         from.Name,
		Requirements: amigrate.PlacementRequirementsFromV1Beta1(from.Requirements),
		Resources:    ResourcesFromV1Beta1(from.Resources),
	}
}

func GroupFromV1Beta1(from v1beta1.Group) v1beta2.Group {
	return v1beta2.Group{
		GroupID:   GroupIDFromV1Beta1(from.GroupID),
		State:     v1beta2.Group_State(from.State),
		GroupSpec: GroupSpecFromV1Beta1(from.GroupSpec),
		CreatedAt: from.CreatedAt,
	}
}

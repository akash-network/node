package types

import (
	"github.com/ovrclk/akash/types/base"
)

func (id DeploymentGroupID) DeploymentID() base.Bytes {
	return id.Deployment
}

func (id OrderID) GroupID() DeploymentGroupID {
	return DeploymentGroupID{
		Deployment: id.Deployment,
		Seq:        id.Group,
	}
}

func (id OrderID) DeploymentID() base.Bytes {
	return id.Deployment
}

func (id FulfillmentID) LeaseID() LeaseID {
	return LeaseID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Order:      id.Order,
		Provider:   id.Provider,
	}
}

func (id FulfillmentID) OrderID() OrderID {
	return OrderID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Seq:        id.Order,
	}
}

func (id FulfillmentID) GroupID() DeploymentGroupID {
	return id.OrderID().GroupID()
}

func (id FulfillmentID) DeploymentID() base.Bytes {
	return id.Deployment
}

func (id LeaseID) FulfillmentID() FulfillmentID {
	return FulfillmentID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Order:      id.Order,
		Provider:   id.Provider,
	}
}

func (id LeaseID) OrderID() OrderID {
	return OrderID{
		Deployment: id.Deployment,
		Group:      id.Group,
		Seq:        id.Order,
	}
}

func (id LeaseID) GroupID() DeploymentGroupID {
	return id.OrderID().GroupID()
}

func (id LeaseID) DeploymentID() base.Bytes {
	return id.Deployment
}

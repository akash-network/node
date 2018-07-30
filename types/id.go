package types

import (
	"bytes"
	"strconv"

	"github.com/ovrclk/akash/types/base"
)

func (id DeploymentGroupID) String() string {
	return id.Deployment.String() + "/" + strconv.FormatUint(id.Seq, 10)
}

func (id DeploymentGroupID) Path() string {
	return id.String()
}

func (id DeploymentGroupID) Compare(that interface{}) int {
	switch that := that.(type) {
	case DeploymentGroupID:
		if cmp := bytes.Compare(id.Deployment, that.Deployment); cmp != 0 {
			return cmp
		}
		return int(id.Seq - that.Seq)
	case *DeploymentGroupID:
		if cmp := bytes.Compare(id.Deployment, that.Deployment); cmp != 0 {
			return cmp
		}
		return int(id.Seq - that.Seq)
	default:
		return 1
	}
}

func (id DeploymentGroupID) DeploymentID() base.Bytes {
	return id.Deployment
}

func (id OrderID) Compare(that interface{}) int {
	switch that := that.(type) {
	case OrderID:
		if cmp := id.GroupID().Compare(that.GroupID()); cmp != 0 {
			return cmp
		}
		return int(id.Seq - that.Seq)
	case *OrderID:
		if cmp := id.GroupID().Compare(that.GroupID()); cmp != 0 {
			return cmp
		}
		return int(id.Seq - that.Seq)
	default:
		return 1
	}
}

func (id OrderID) String() string {
	return id.Deployment.String() + "/" +
		strconv.FormatUint(id.Group, 10) + "/" +
		strconv.FormatUint(id.Seq, 10)
}

func (id OrderID) Path() string {
	return id.String()
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

func (id FulfillmentID) String() string {
	return id.Deployment.String() + "/" +
		strconv.FormatUint(id.Group, 10) + "/" +
		strconv.FormatUint(id.Order, 10) + "/" +
		id.Provider.String()
}

func (id FulfillmentID) Path() string {
	return id.String()
}

func (id FulfillmentID) Compare(that interface{}) int {
	switch that := that.(type) {
	case FulfillmentID:
		if cmp := id.OrderID().Compare(that.OrderID()); cmp != 0 {
			return cmp
		}
		return bytes.Compare(id.Provider, that.Provider)
	case *FulfillmentID:
		if cmp := id.OrderID().Compare(that.OrderID()); cmp != 0 {
			return cmp
		}
		return bytes.Compare(id.Provider, that.Provider)
	default:
		return 1
	}
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

func (id LeaseID) String() string {
	return id.FulfillmentID().String()
}

func (id LeaseID) Path() string {
	return id.String()
}

func (id LeaseID) Compare(that interface{}) int {
	switch that := that.(type) {
	case LeaseID:
		return id.FulfillmentID().Compare(that.FulfillmentID())
	case *LeaseID:
		return id.FulfillmentID().Compare(that.FulfillmentID())
	default:
		return 1
	}
}

func (id LeaseID) Equal(that interface{}) bool {
	return id.Compare(that) == 0
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

func (r ResourceUnit) Equal(that interface{}) bool {
	return (&r).Compare(that) == 0
}

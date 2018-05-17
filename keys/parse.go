package keys

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/types/base"
)

// XXX: interim hack (iteration!)

func ParseDeploymentPath(buf string) (Deployment, error) {
	obj, err := base.DecodeString(buf)
	return Deployment(obj), err
}

func ParseGroupPath(buf string) (DeploymentGroup, error) {
	var err error
	obj := DeploymentGroup{}
	parts := strings.Split(buf, "/")

	if len(parts) != 2 {
		return obj, fmt.Errorf("invalid group path '%v': truncated", buf)
	}

	obj.Deployment, err = base.DecodeString(parts[0])
	if err != nil {
		return obj, err
	}

	obj.Seq, err = strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return obj, err
	}

	return obj, nil
}

func ParseOrderPath(buf string) (Order, error) {
	obj := Order{}
	parts := strings.Split(buf, "/")

	if len(parts) != 3 {
		return obj, fmt.Errorf("invalid order path '%v': truncated", buf)
	}

	group, err := ParseGroupPath(strings.Join(parts[0:2], "/"))
	if err != nil {
		return obj, fmt.Errorf("invalid order path %v: %v", buf, err)
	}

	obj.Deployment = group.Deployment
	obj.Group = group.Seq

	obj.Seq, err = strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return obj, fmt.Errorf("invalid order path '%v': bad sequence %v", buf, parts[2])
	}

	return obj, nil
}

func ParseFulfillmentPath(buf string) (Fulfillment, error) {
	obj := Fulfillment{}
	parts := strings.Split(buf, "/")

	if len(parts) != 4 {
		return obj, fmt.Errorf("invalid fulfillment path '%v': truncated", buf)
	}

	order, err := ParseOrderPath(strings.Join(parts[0:3], "/"))
	if err != nil {
		return obj, fmt.Errorf("invalid fulfillment path '%v': %v", buf, err)
	}

	obj.Deployment = order.Deployment
	obj.Group = order.Group
	obj.Order = order.Seq

	obj.Provider, err = base.DecodeString(parts[3])

	return obj, err
}

func ParseLeasePath(buf string) (Lease, error) {
	obj, err := ParseFulfillmentPath(buf)
	return LeaseID(types.LeaseID{
		Deployment: obj.Deployment,
		Group:      obj.Group,
		Order:      obj.Order,
		Provider:   obj.Provider,
	}), err
}

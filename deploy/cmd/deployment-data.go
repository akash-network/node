package cmd

import (
	"sync"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/sdl"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	mtypes "github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/pflag"
)

// DeploymentData contains the various IDs involved in a deployment
type DeploymentData struct {
	SDL          sdl.SDL
	Manifest     manifest.Manifest
	Groups       []*dtypes.GroupSpec
	DeploymentID dtypes.DeploymentID
	OrderID      []mtypes.OrderID
	LeaseID      []mtypes.LeaseID
	Version      []byte

	sync.RWMutex
}

// MsgCreate constructor for MsgCreateDeployment
func (dd *DeploymentData) MsgCreate() *dtypes.MsgCreateDeployment {
	// Create the deployment message
	msg := &dtypes.MsgCreateDeployment{
		ID:      dd.DeploymentID,
		Groups:  make([]dtypes.GroupSpec, 0, len(dd.Groups)),
		Version: dd.Version,
	}

	// Append the groups to the message
	for _, group := range dd.Groups {
		msg.Groups = append(msg.Groups, *group)
	}

	return msg
}

// ExpectedLeases returns true if all the leases are in state
func (dd *DeploymentData) ExpectedLeases() bool {
	dd.RLock()
	defer dd.RUnlock()
	return len(dd.Groups) == len(dd.LeaseID)
}

// ExpectedOrders returns true if all the orders are in state
func (dd *DeploymentData) ExpectedOrders() bool {
	dd.RLock()
	defer dd.RUnlock()
	return len(dd.Groups) == len(dd.OrderID)
}

// AddOrder adds an order for tracking
func (dd *DeploymentData) AddOrder(order mtypes.OrderID) {
	dd.Lock()
	defer dd.Unlock()
	// TODO: Check that order isn't already tracked
	dd.OrderID = append(dd.OrderID, order)
}

// RemoveOrder adds an order for tracking
func (dd *DeploymentData) RemoveOrder(order mtypes.OrderID) {
	dd.Lock()
	defer dd.Unlock()
	var out []mtypes.OrderID
	for _, o := range dd.OrderID {
		if !order.Equals(o) {
			out = append(out, o)
		}
	}
	dd.OrderID = out
}

// Leases returns a copy of the LeaseIDs tracked
func (dd *DeploymentData) Leases() []mtypes.LeaseID {
	dd.RLock()
	defer dd.RUnlock()
	out := dd.LeaseID
	return out
}

// AddLease adds a lease for tracking
func (dd *DeploymentData) AddLease(lease mtypes.LeaseID) {
	dd.Lock()
	defer dd.Unlock()
	// TODO: Check that lease isn't already tracked
	dd.LeaseID = append(dd.LeaseID, lease)
}

// NewDeploymentData returns a DeploymentData struct initialized from a file and flags
func NewDeploymentData(filename string, flags *pflag.FlagSet, clientCtx client.Context) (*DeploymentData, error) {
	sdlSpec, err := sdl.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	groups, err := sdlSpec.DeploymentGroups()
	if err != nil {
		return nil, err
	}
	mani, err := sdlSpec.Manifest()
	if err != nil {
		return nil, err
	}
	ver, err := sdl.ManifestVersion(mani)
	if err != nil {
		return nil, err
	}
	id, err := dcli.DeploymentIDFromFlagsForOwner(flags, clientCtx.GetFromAddress())

	if err != nil {
		return nil, err
	}
	if id.DSeq == 0 {
		if id.DSeq, err = dcli.CurrentBlockHeight(clientCtx); err != nil {
			return nil, err
		}
	}
	return &DeploymentData{
		SDL:          sdlSpec,
		Manifest:     mani,
		Groups:       groups,
		DeploymentID: id,
		OrderID:      make([]mtypes.OrderID, 0),
		LeaseID:      make([]mtypes.LeaseID, 0),
		Version:      ver,
	}, nil
}

package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

// OrderID stores owner and all other seq numbers
type OrderID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
	GSeq  uint32         `json:"gseq"`
	OSeq  uint32         `json:"oseq"`
}

// MakeOrderID returns OrderID instance with provided groupID details and oseq
func MakeOrderID(id dtypes.GroupID, oseq uint32) OrderID {
	return OrderID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
		OSeq:  oseq,
	}
}

// GroupID method returns groupID details for specific order
func (id OrderID) GroupID() dtypes.GroupID {
	return dtypes.GroupID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
	}
}

// Equals method compares specific order with provided order
func (id OrderID) Equals(other OrderID) bool {
	return id.GroupID().Equals(other.GroupID()) && id.OSeq == other.OSeq
}

// Validate method for OrderID and returns nil
func (id OrderID) Validate() error {
	if id.Owner.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "owner cannot be empty")
	}
	if id.DSeq == 0 && id.GSeq == 0 && id.OSeq == 0 {
		return sdkerrors.Wrap(ErrInvalidOrder, "at least one id must be > 0")
	}
	return nil
}

// BidID stores owner, provider and all other seq numbers
type BidID struct {
	Owner    sdk.AccAddress `json:"owner"`
	DSeq     uint64         `json:"dseq"`
	GSeq     uint32         `json:"gseq"`
	OSeq     uint32         `json:"oseq"`
	Provider sdk.AccAddress `json:"provider"`
}

// MakeBidID returns BidID instance with provided order details and provider
func MakeBidID(id OrderID, provider sdk.AccAddress) BidID {
	return BidID{
		Owner:    id.Owner,
		DSeq:     id.DSeq,
		GSeq:     id.GSeq,
		OSeq:     id.OSeq,
		Provider: provider,
	}
}

// Equals method compares specific bid with provided bid
func (id BidID) Equals(other BidID) bool {
	return id.OrderID().Equals(other.OrderID()) &&
		id.Provider.Equals(other.Provider)
}

// LeaseID method returns lease details of bid
func (id BidID) LeaseID() LeaseID {
	return LeaseID(id)
}

// OrderID method returns OrderID details with specific bid details
func (id BidID) OrderID() OrderID {
	return OrderID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
		OSeq:  id.OSeq,
	}
}

// GroupID method returns GroupID details with specific bid details
func (id BidID) GroupID() dtypes.GroupID {
	return id.OrderID().GroupID()
}

// DeploymentID method returns deployment details with specific bid details
func (id BidID) DeploymentID() dtypes.DeploymentID {
	return id.GroupID().DeploymentID()
}

// Validate validates bid instance and returns nil
func (id BidID) Validate() error {
	if id.Provider.Empty() {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "provider cannot be empty")
	}
	return id.OrderID().Validate()
}

// LeaseID stores bid details of lease
type LeaseID BidID

// Equals method compares specific lease with provided lease
func (id LeaseID) Equals(other LeaseID) bool {
	return id.BidID().Equals(other.BidID())
}

// BidID method returns BidID details with specific LeaseID
func (id LeaseID) BidID() BidID {
	return BidID(id)
}

// OrderID method returns OrderID details with specific lease details
func (id LeaseID) OrderID() OrderID {
	return id.BidID().OrderID()
}

// GroupID method returns GroupID details with specific lease details
func (id LeaseID) GroupID() dtypes.GroupID {
	return id.OrderID().GroupID()
}

// DeploymentID method returns deployment details with specific lease details
func (id LeaseID) DeploymentID() dtypes.DeploymentID {
	return id.GroupID().DeploymentID()
}

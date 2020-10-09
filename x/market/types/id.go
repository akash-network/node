package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

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
	if err := id.GroupID().Validate(); err != nil {
		return sdkerrors.Wrap(err, "OrderID: Invalid GroupID")
	}
	if id.OSeq == 0 {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidSequence, "OrderID: Invalid Order Sequence")
	}
	return nil
}

// String provides stringer interface to save reflected formatting.
func (id OrderID) String() string {
	return fmt.Sprintf("%s/%v", id.GroupID(), id.OSeq)
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

// String method for consistent output.
func (id BidID) String() string {
	return fmt.Sprintf("%s/%v", id.OrderID(), id.Provider)
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
	if err := id.OrderID().Validate(); err != nil {
		return sdkerrors.Wrap(err, "BidID: Invalid OrderID")
	}
	if err := sdk.VerifyAddressFormat(id.Provider); err != nil {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "BidID: Invalid Provider Address")
	}
	return nil
}

// MakeLeaseID returns LeaseID instance with provided bid details
func MakeLeaseID(id BidID) LeaseID {
	return LeaseID(id)
}

// Equals method compares specific lease with provided lease
func (id LeaseID) Equals(other LeaseID) bool {
	return id.BidID().Equals(other.BidID())
}

// Validate calls the BidID's validator and returns any error.
func (id LeaseID) Validate() error {
	if err := id.BidID().Validate(); err != nil {
		return sdkerrors.Wrap(err, "LeaseID: Invalid BidID")
	}
	return nil
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

// String method provides human readable representation of LeaseID.
func (id LeaseID) String() string {
	return id.BidID().String()
}

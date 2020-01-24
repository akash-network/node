package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

type OrderID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
	GSeq  uint32         `json:"gseq"`
	OSeq  uint32         `json:"oseq"`
}

func MakeOrderID(id dtypes.GroupID, oseq uint32) OrderID {
	return OrderID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
		OSeq:  oseq,
	}
}

func (id OrderID) GroupID() dtypes.GroupID {
	return dtypes.GroupID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
	}
}

func (id OrderID) Equals(other OrderID) bool {
	return id.GroupID().Equals(other.GroupID()) && id.OSeq == other.OSeq
}

func (id OrderID) Validate() error {
	return nil
}

type BidID struct {
	Owner    sdk.AccAddress `json:"owner"`
	DSeq     uint64         `json:"dseq"`
	GSeq     uint32         `json:"gseq"`
	OSeq     uint32         `json:"oseq"`
	Provider sdk.AccAddress `json:"provider"`
}

func MakeBidID(id OrderID, provider sdk.AccAddress) BidID {
	return BidID{
		Owner:    id.Owner,
		DSeq:     id.DSeq,
		GSeq:     id.GSeq,
		OSeq:     id.OSeq,
		Provider: provider,
	}
}

func (id BidID) Equals(other BidID) bool {
	return id.OrderID().Equals(other.OrderID()) &&
		id.Provider.Equals(other.Provider)
}

func (id BidID) LeaseID() LeaseID {
	return LeaseID(id)
}

func (id BidID) OrderID() OrderID {
	return OrderID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
		OSeq:  id.OSeq,
	}
}

func (id BidID) GroupID() dtypes.GroupID {
	return id.OrderID().GroupID()
}

func (id BidID) DeploymentID() dtypes.DeploymentID {
	return id.GroupID().DeploymentID()
}

func (id BidID) Validate() error {
	return nil
}

type LeaseID BidID

func (id LeaseID) Equals(other LeaseID) bool {
	return id.BidID().Equals(other.BidID())
}

func (id LeaseID) BidID() BidID {
	return BidID(id)
}

func (id LeaseID) OrderID() OrderID {
	return id.BidID().OrderID()
}

func (id LeaseID) GroupID() dtypes.GroupID {
	return id.OrderID().GroupID()
}

func (id LeaseID) DeploymentID() dtypes.DeploymentID {
	return id.GroupID().DeploymentID()
}

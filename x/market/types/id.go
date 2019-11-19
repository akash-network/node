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

func (id BidID) OrderID() OrderID {
	return OrderID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
		OSeq:  id.OSeq,
	}
}
func (id BidID) Validate() error {
	return nil
}

type LeaseID BidID

func (id LeaseID) GroupID() dtypes.GroupID {
	return dtypes.GroupID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  id.GSeq,
	}
}

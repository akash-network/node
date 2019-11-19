package types

import sdk "github.com/cosmos/cosmos-sdk/types"

type DeploymentID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
}

func (id DeploymentID) Equals(other DeploymentID) bool {
	return id.Owner.Equals(other.Owner) && id.DSeq == other.DSeq
}

func (id DeploymentID) Validate() error {
	return nil
}

type GroupID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
	GSeq  uint32         `json:"gseq"`
}

func MakeGroupID(id DeploymentID, gseq uint32) GroupID {
	return GroupID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  gseq,
	}
}

func (id GroupID) DeploymentID() DeploymentID {
	return DeploymentID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
	}
}

func (id GroupID) Equals(other GroupID) bool {
	return id.DeploymentID().Equals(other.DeploymentID()) && id.GSeq == other.GSeq
}

func (id GroupID) Validate() error {
	return nil
}

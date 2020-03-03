package types

import sdk "github.com/cosmos/cosmos-sdk/types"

// DeploymentID stores owner and sequence number
type DeploymentID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
}

// Equals method compares specific deployment with provided deployment
func (id DeploymentID) Equals(other DeploymentID) bool {
	return id.Owner.Equals(other.Owner) && id.DSeq == other.DSeq
}

// Validate method for DeploymentID and returns nil
func (id DeploymentID) Validate() error {
	return nil
}

// GroupID stores owner, deployment sequence number and group sequence number
type GroupID struct {
	Owner sdk.AccAddress `json:"owner"`
	DSeq  uint64         `json:"dseq"`
	GSeq  uint32         `json:"gseq"`
}

// MakeGroupID returns GroupID instance with provided deployment details
// and group sequence number.
func MakeGroupID(id DeploymentID, gseq uint32) GroupID {
	return GroupID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
		GSeq:  gseq,
	}
}

// DeploymentID method returns DeploymentID details with specific group details
func (id GroupID) DeploymentID() DeploymentID {
	return DeploymentID{
		Owner: id.Owner,
		DSeq:  id.DSeq,
	}
}

// Equals method compares specific group with provided group
func (id GroupID) Equals(other GroupID) bool {
	return id.DeploymentID().Equals(other.DeploymentID()) && id.GSeq == other.GSeq
}

// Validate method for GroupID and returns nil
func (id GroupID) Validate() error {
	return nil
}

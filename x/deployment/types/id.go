package types

import (
	fmt "fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// Equals method compares specific deployment with provided deployment
func (id DeploymentID) Equals(other DeploymentID) bool {
	return id.Owner.Equals(other.Owner) && id.DSeq == other.DSeq
}

// Validate method for DeploymentID and returns nil
func (id DeploymentID) Validate() error {
	err := sdk.VerifyAddressFormat(id.Owner)
	switch {
	case err != nil:
		return sdkerrors.Wrap(sdkerrors.ErrInvalidAddress, "DeploymentID: Invalid Owner Address")
	case id.DSeq == 0:
		return sdkerrors.Wrap(sdkerrors.ErrInvalidSequence, "DeploymentID: Invalid Deployment Sequence")
	}
	return nil
}

// String method for deployment IDs
func (id DeploymentID) String() string {
	return fmt.Sprintf("%s/%d", id.Owner, id.DSeq)
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
	if err := id.DeploymentID().Validate(); err != nil {
		return sdkerrors.Wrap(err, "GroupID: Invalid DeploymentID")
	}
	if id.GSeq == 0 {
		return sdkerrors.Wrap(sdkerrors.ErrInvalidSequence, "GroupID: Invalid Group Sequence")
	}
	return nil
}

// String method provides human readable representation of GroupID.
func (id GroupID) String() string {
	return fmt.Sprintf("%s/%d", id.DeploymentID(), id.GSeq)
}

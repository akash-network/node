package v1beta1

import (
	fmt "fmt"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// Equals method compares specific deployment with provided deployment
func (id DeploymentID) Equals(other DeploymentID) bool {
	return id.Owner == other.Owner && id.DSeq == other.DSeq
}

// Validate method for DeploymentID and returns nil
func (id DeploymentID) Validate() error {
	_, err := sdk.AccAddressFromBech32(id.Owner)
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

func (id DeploymentID) GetOwnerAddress() (sdk.Address, error) {
	return sdk.AccAddressFromBech32(id.Owner)
}

func ParseDeploymentID(val string) (DeploymentID, error) {
	parts := strings.Split(val, "/")
	return ParseDeploymentPath(parts)
}

// ParseDeploymentPath returns DeploymentID details with provided queries, and return
// error if occurred due to wrong query
func ParseDeploymentPath(parts []string) (DeploymentID, error) {
	if len(parts) != 2 {
		return DeploymentID{}, ErrInvalidIDPath
	}

	owner, err := sdk.AccAddressFromBech32(parts[0])
	if err != nil {
		return DeploymentID{}, err
	}

	dseq, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return DeploymentID{}, err
	}

	return DeploymentID{
		Owner: owner.String(),
		DSeq:  dseq,
	}, nil
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

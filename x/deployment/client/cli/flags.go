package cli

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AddDeploymentIDFlags add flags for deployment
func AddDeploymentIDFlags(cmd *cobra.Command) {
	cmd.Flags().String("owner", "", "Deployment Owner")
	// owner flag required for command
	cmd.MarkFlagRequired("owner")
	cmd.Flags().Uint64("dseq", 0, "Deployment Sequence")
	// dseq flag required for command
	cmd.MarkFlagRequired("dseq")
}

// DeploymentIDFromFlags returns DeploymentID with given flags, owner and error if occured
func DeploymentIDFromFlags(flags *pflag.FlagSet, defaultOwner string) (types.DeploymentID, error) {
	var id types.DeploymentID
	owner, err := flags.GetString("owner")
	if err != nil {
		return id, err
	}
	if owner == "" {
		owner = defaultOwner
	}
	id.Owner, err = sdk.AccAddressFromBech32(owner)
	if err != nil {
		return id, err
	}
	if id.DSeq, err = flags.GetUint64("dseq"); err != nil {
		return id, err
	}
	return id, nil
}

// AddGroupIDFlags add flags for Group
func AddGroupIDFlags(cmd *cobra.Command) {
	AddDeploymentIDFlags(cmd)
	cmd.Flags().Uint32("gseq", 0, "Group Sequence")
}

// GroupIDFromFlags returns GroupID with given flags and error if occured
func GroupIDFromFlags(flags *pflag.FlagSet) (types.GroupID, error) {
	var id types.GroupID
	prev, err := DeploymentIDFromFlags(flags, "")
	if err != nil {
		return id, err
	}

	gseq, err := flags.GetUint32("gseq")
	if err != nil {
		return id, err
	}
	return types.MakeGroupID(prev, gseq), nil
}

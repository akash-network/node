package cli

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ovrclk/akash/x/deployment/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	ErrOwnerValue = errors.New("query: invalid owner value")
	ErrStateValue = errors.New("query: invalid state value")
)

// AddDeploymentIDFlags add flags for deployment
func AddDeploymentIDFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "Deployment Owner")
	flags.Uint64("dseq", 0, "Deployment Sequence")
}

// MarkReqDeploymentIDFlags marks flags required for deployment
func MarkReqDeploymentIDFlags(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.MarkFlagRequired("dseq")
}

// DeploymentIDFromFlags returns DeploymentID with given flags, owner and error if occurred
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
func AddGroupIDFlags(flags *pflag.FlagSet) {
	AddDeploymentIDFlags(flags)
	flags.Uint32("gseq", 0, "Group Sequence")
}

// MarkReqGroupIDFlags marks flags required for group
func MarkReqGroupIDFlags(cmd *cobra.Command) {
	MarkReqDeploymentIDFlags(cmd)
	_ = cmd.MarkFlagRequired("gseq")
}

// GroupIDFromFlags returns GroupID with given flags and error if occurred
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

// AddDeploymentFilterFlags add flags to filter for deployment list
func AddDeploymentFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "deployment owner address to filter")
	flags.String("state", "", "deployment state to filter (active,closed)")
	flags.Uint64("dseq", 0, "deployment sequence to filter")
}

// DepFiltersFromFlags returns DeploymentFilters with given flags and error if occurred
func DepFiltersFromFlags(flags *pflag.FlagSet) (types.DeploymentFilters, string, error) {
	var dfilters types.DeploymentFilters
	owner, err := flags.GetString("owner")
	if err != nil {
		return dfilters, "", err
	}

	if owner != "" {
		dfilters.Owner, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return dfilters, "", err
		}
	} else {
		dfilters.Owner = sdk.AccAddress{}
	}

	if !dfilters.Owner.Empty() && sdk.VerifyAddressFormat(dfilters.Owner) != nil {
		return dfilters, "", ErrOwnerValue
	}

	state, err := flags.GetString("state")
	if err != nil {
		return dfilters, "", err
	}

	if dfilters.DSeq, err = flags.GetUint64("dseq"); err != nil {
		return dfilters, state, err
	}

	return dfilters, state, nil
}

// // AddGroupFilterFlags add flags to filter for group list
// func AddGroupFilterFlags(flags *pflag.FlagSet) {
// 	flags.String("owner", "", "group owner address to filter")
// 	flags.String("state", "", "group state to filter (open,ordered,matched,insufficient_funds,closed)")
// 	flags.Uint64("dseq", 0, "deployment sequence to filter")
// 	flags.Uint32("gseq", 0, "group sequence to filter")
// }

// // GroupFiltersFromFlags returns GroupFilters with given flags and error if occurred
// func GroupFiltersFromFlags(flags *pflag.FlagSet) (query.GroupFilters, error) {
// 	dfilters, err := DepFiltersFromFlags(flags)
// 	if err != nil {
// 		return query.GroupFilters{}, err
// 	}
// 	gfilters := query.GroupFilters{
// 		Owner:        dfilters.Owner,
// 		StateFlagVal: dfilters.StateFlagVal,
// 	}
// 	return gfilters, nil
// }

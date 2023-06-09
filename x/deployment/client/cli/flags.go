package cli

import (
	"errors"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	types "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
)

const (
	FlagDepositorAccount = "depositor-account"
	FlagExpiration       = "expiration"
)

var (
	ErrStateValue     = errors.New("query: invalid state value")
	DefaultDeposit, _ = types.DefaultParams().MinDepositFor("uakt")
)

type DeploymentIDOptions struct {
	NoOwner bool
}

type DeploymentIDOption func(*DeploymentIDOptions)

// DeploymentIDOptionNoOwner do not add mark as required owner flag
func DeploymentIDOptionNoOwner(val bool) DeploymentIDOption {
	return func(opt *DeploymentIDOptions) {
		opt.NoOwner = val
	}
}

type MarketOptions struct {
	Owner    sdk.AccAddress
	Provider sdk.AccAddress
}

type MarketOption func(*MarketOptions)

func WithOwner(val sdk.AccAddress) MarketOption {
	return func(opt *MarketOptions) {
		opt.Owner = val
	}
}

func WithProvider(val sdk.AccAddress) MarketOption {
	return func(opt *MarketOptions) {
		opt.Provider = val
	}
}

// AddDeploymentIDFlags add flags for deployment except for Owner when NoOwner is set
func AddDeploymentIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	opt := &DeploymentIDOptions{}

	for _, o := range opts {
		o(opt)
	}

	if !opt.NoOwner {
		flags.String("owner", "", "Deployment Owner")
	}

	flags.Uint64("dseq", 0, "Deployment Sequence")
}

// MarkReqDeploymentIDFlags marks flags required except for Owner when NoOwner is set
func MarkReqDeploymentIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	opt := &DeploymentIDOptions{}

	for _, o := range opts {
		o(opt)
	}

	if !opt.NoOwner {
		_ = cmd.MarkFlagRequired("owner")
	}

	_ = cmd.MarkFlagRequired("dseq")
}

// DeploymentIDFromFlags returns DeploymentID with given flags, owner and error if occurred
func DeploymentIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (types.DeploymentID, error) {
	var id types.DeploymentID
	opt := &MarketOptions{}

	for _, o := range opts {
		o(opt)
	}

	var owner string
	if flag := flags.Lookup("owner"); flag != nil {
		owner = flag.Value.String()
	}

	// if --owner flag was explicitly provided, use that.
	var err error
	if owner != "" {
		opt.Owner, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return id, err
		}
	}

	id.Owner = opt.Owner.String()

	if id.DSeq, err = flags.GetUint64("dseq"); err != nil {
		return id, err
	}
	return id, nil
}

// DeploymentIDFromFlagsForOwner returns DeploymentID with given flags, owner and error if occurred
func DeploymentIDFromFlagsForOwner(flags *pflag.FlagSet, owner sdk.Address) (types.DeploymentID, error) {
	id := types.DeploymentID{
		Owner: owner.String(),
	}

	var err error
	if id.DSeq, err = flags.GetUint64("dseq"); err != nil {
		return id, err
	}

	return id, nil
}

// AddGroupIDFlags add flags for Group
func AddGroupIDFlags(flags *pflag.FlagSet, opts ...DeploymentIDOption) {
	AddDeploymentIDFlags(flags, opts...)
	flags.Uint32("gseq", 1, "Group Sequence")
}

// MarkReqGroupIDFlags marks flags required for group
func MarkReqGroupIDFlags(cmd *cobra.Command, opts ...DeploymentIDOption) {
	MarkReqDeploymentIDFlags(cmd, opts...)
}

// GroupIDFromFlags returns GroupID with given flags and error if occurred
func GroupIDFromFlags(flags *pflag.FlagSet, opts ...MarketOption) (types.GroupID, error) {
	var id types.GroupID
	prev, err := DeploymentIDFromFlags(flags, opts...)
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
func DepFiltersFromFlags(flags *pflag.FlagSet) (types.DeploymentFilters, error) {
	var dfilters types.DeploymentFilters
	owner, err := flags.GetString("owner")
	if err != nil {
		return dfilters, err
	}

	if owner != "" {
		_, err = sdk.AccAddressFromBech32(owner)
		if err != nil {
			return dfilters, err
		}
	}

	dfilters.Owner = owner

	if dfilters.State, err = flags.GetString("state"); err != nil {
		return dfilters, err
	}

	if dfilters.DSeq, err = flags.GetUint64("dseq"); err != nil {
		return dfilters, err
	}

	return dfilters, nil
}

// AddDepositorFlag adds the `--depositor-account` flag
func AddDepositorFlag(flags *pflag.FlagSet) {
	flags.String(FlagDepositorAccount, "", "Depositor account pays for the deposit instead of deducting from the owner")
}

// DepositorFromFlags returns the depositor account if one was specified in flags,
// otherwise it returns the owner's account.
func DepositorFromFlags(flags *pflag.FlagSet, owner string) (string, error) {
	depositorAcc, err := flags.GetString(FlagDepositorAccount)
	if err != nil {
		return "", err
	}

	// if no depositor is specified, owner is the default depositor
	if strings.TrimSpace(depositorAcc) == "" {
		return owner, nil
	}

	_, err = sdk.AccAddressFromBech32(depositorAcc)
	return depositorAcc, err
}

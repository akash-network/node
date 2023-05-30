package cli

import (
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	types "github.com/akash-network/akash-api/go/node/market/v1beta3"

	dcli "github.com/akash-network/node/x/deployment/client/cli"
)

var (
	ErrStateValue  = errors.New("query: invalid state value")
	DefaultDeposit = types.DefaultBidMinDeposit
)

// AddOrderIDFlags add flags for order
func AddOrderIDFlags(flags *pflag.FlagSet, opts ...dcli.DeploymentIDOption) {
	dcli.AddGroupIDFlags(flags, opts...)
	flags.Uint32("oseq", 1, "Order Sequence")
}

// MarkReqOrderIDFlags marks flags required for order
func MarkReqOrderIDFlags(cmd *cobra.Command, opts ...dcli.DeploymentIDOption) {
	dcli.MarkReqGroupIDFlags(cmd, opts...)
}

// AddProviderFlag add provider flag to command flags set
func AddProviderFlag(flags *pflag.FlagSet) {
	flags.String("provider", "", "Provider")
}

// MarkReqProviderFlag marks provider flag as required
func MarkReqProviderFlag(cmd *cobra.Command) {
	_ = cmd.MarkFlagRequired("provider")
}

// OrderIDFromFlags returns OrderID with given flags and error if occurred
func OrderIDFromFlags(flags *pflag.FlagSet, opts ...dcli.MarketOption) (types.OrderID, error) {
	prev, err := dcli.GroupIDFromFlags(flags, opts...)
	if err != nil {
		return types.OrderID{}, err
	}
	val, err := flags.GetUint32("oseq")
	if err != nil {
		return types.OrderID{}, err
	}
	return types.MakeOrderID(prev, val), nil
}

// AddBidIDFlags add flags for bid
func AddBidIDFlags(flags *pflag.FlagSet, opts ...dcli.DeploymentIDOption) {
	AddOrderIDFlags(flags, opts...)
	AddProviderFlag(flags)
}

// AddQueryBidIDFlags add flags for bid in query commands
func AddQueryBidIDFlags(flags *pflag.FlagSet) {
	AddBidIDFlags(flags)
}

// MarkReqBidIDFlags marks flags required for bid
// Used in get bid query command
func MarkReqBidIDFlags(cmd *cobra.Command, opts ...dcli.DeploymentIDOption) {
	MarkReqOrderIDFlags(cmd, opts...)
	MarkReqProviderFlag(cmd)
}

// BidIDFromFlags returns BidID with given flags and error if occurred
// Here provider value is taken from flags
func BidIDFromFlags(flags *pflag.FlagSet, opts ...dcli.MarketOption) (types.BidID, error) {
	prev, err := OrderIDFromFlags(flags, opts...)
	if err != nil {
		return types.BidID{}, err
	}

	opt := &dcli.MarketOptions{}

	for _, o := range opts {
		o(opt)
	}

	if opt.Provider.Empty() {
		provider, err := flags.GetString("provider")
		if err != nil {
			return types.BidID{}, err
		}

		if opt.Provider, err = sdk.AccAddressFromBech32(provider); err != nil {
			return types.BidID{}, err
		}
	}

	return types.MakeBidID(prev, opt.Provider), nil
}

func AddLeaseIDFlags(flags *pflag.FlagSet, opts ...dcli.DeploymentIDOption) {
	AddBidIDFlags(flags, opts...)
}

// MarkReqLeaseIDFlags marks flags required for bid
// Used in get bid query command
func MarkReqLeaseIDFlags(cmd *cobra.Command, opts ...dcli.DeploymentIDOption) {
	MarkReqBidIDFlags(cmd, opts...)
}

// LeaseIDFromFlags returns LeaseID with given flags and error if occurred
// Here provider value is taken from flags
func LeaseIDFromFlags(flags *pflag.FlagSet, opts ...dcli.MarketOption) (types.LeaseID, error) {
	bid, err := BidIDFromFlags(flags, opts...)
	if err != nil {
		return types.LeaseID{}, err
	}

	return bid.LeaseID(), nil
}

// AddOrderFilterFlags add flags to filter for order list
func AddOrderFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "order owner address to filter")
	flags.String("state", "", "order state to filter (open,matched,closed)")
	flags.Uint64("dseq", 0, "deployment sequence to filter")
	flags.Uint32("gseq", 1, "group sequence to filter")
	flags.Uint32("oseq", 1, "order sequence to filter")
}

// OrderFiltersFromFlags returns OrderFilters with given flags and error if occurred
func OrderFiltersFromFlags(flags *pflag.FlagSet) (types.OrderFilters, error) {
	dfilters, err := dcli.DepFiltersFromFlags(flags)
	if err != nil {
		return types.OrderFilters{}, err
	}
	ofilters := types.OrderFilters{
		Owner: dfilters.Owner,
		DSeq:  dfilters.DSeq,
		State: dfilters.State,
	}

	if ofilters.GSeq, err = flags.GetUint32("gseq"); err != nil {
		return ofilters, err
	}

	if ofilters.OSeq, err = flags.GetUint32("oseq"); err != nil {
		return ofilters, err
	}

	return ofilters, nil
}

// AddBidFilterFlags add flags to filter for bid list
func AddBidFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "bid owner address to filter")
	flags.String("state", "", "bid state to filter (open,matched,lost,closed)")
	flags.Uint64("dseq", 0, "deployment sequence to filter")
	flags.Uint32("gseq", 1, "group sequence to filter")
	flags.Uint32("oseq", 1, "order sequence to filter")
	flags.String("provider", "", "bid provider address to filter")
}

// BidFiltersFromFlags returns BidFilters with given flags and error if occurred
func BidFiltersFromFlags(flags *pflag.FlagSet) (types.BidFilters, error) {
	ofilters, err := OrderFiltersFromFlags(flags)
	if err != nil {
		return types.BidFilters{}, err
	}
	bfilters := types.BidFilters{
		Owner: ofilters.Owner,
		DSeq:  ofilters.DSeq,
		GSeq:  ofilters.OSeq,
		OSeq:  ofilters.OSeq,
		State: ofilters.State,
	}

	provider, err := flags.GetString("provider")
	if err != nil {
		return bfilters, err
	}

	if provider != "" {
		_, err = sdk.AccAddressFromBech32(provider)
		if err != nil {
			return bfilters, err
		}
	}
	bfilters.Provider = provider

	return bfilters, nil
}

// AddLeaseFilterFlags add flags to filter for lease list
func AddLeaseFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "lease owner address to filter")
	flags.String("state", "", "lease state to filter (active,insufficient_funds,closed)")
	flags.Uint64("dseq", 0, "deployment sequence to filter")
	flags.Uint32("gseq", 1, "group sequence to filter")
	flags.Uint32("oseq", 1, "order sequence to filter")
	flags.String("provider", "", "bid provider address to filter")
}

// LeaseFiltersFromFlags returns LeaseFilters with given flags and error if occurred
func LeaseFiltersFromFlags(flags *pflag.FlagSet) (types.LeaseFilters, error) {
	bfilters, err := BidFiltersFromFlags(flags)
	if err != nil {
		return types.LeaseFilters{}, err
	}
	return types.LeaseFilters(bfilters), nil
}

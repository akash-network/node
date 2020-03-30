package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AddOrderIDFlags add flags for order
func AddOrderIDFlags(flags *pflag.FlagSet) {
	dcli.AddGroupIDFlags(flags)
	flags.Uint32("oseq", 0, "Order Sequence")
}

// MarkReqOrderIDFlags marks flags required for order
func MarkReqOrderIDFlags(cmd *cobra.Command) {
	dcli.MarkReqGroupIDFlags(cmd)
	cmd.MarkFlagRequired("oseq")
}

// OrderIDFromFlags returns OrderID with given flags and error if occured
func OrderIDFromFlags(flags *pflag.FlagSet) (types.OrderID, error) {
	prev, err := dcli.GroupIDFromFlags(flags)
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
func AddBidIDFlags(flags *pflag.FlagSet) {
	AddOrderIDFlags(flags)
}

// AddQueryBidIDFlags add flags for bid in query commands
func AddQueryBidIDFlags(flags *pflag.FlagSet) {
	AddOrderIDFlags(flags)
	flags.String("provider", "", "Bid Provider")
}

// MarkReqBidIDFlags marks flags required for bid
// Used in get bid query command
func MarkReqBidIDFlags(cmd *cobra.Command) {
	MarkReqOrderIDFlags(cmd)
	cmd.MarkFlagRequired("provider")
}

// BidIDFromFlags returns BidID with given flags and error if occured
func BidIDFromFlags(ctx context.CLIContext, flags *pflag.FlagSet) (types.BidID, error) {
	prev, err := OrderIDFromFlags(flags)
	if err != nil {
		return types.BidID{}, err
	}
	return types.MakeBidID(prev, ctx.GetFromAddress()), nil
}

// BidIDFromFlagsWithoutCtx returns BidID with given flags and error if occured
// Here provider value is taken from flags
func BidIDFromFlagsWithoutCtx(flags *pflag.FlagSet) (types.BidID, error) {
	prev, err := OrderIDFromFlags(flags)
	if err != nil {
		return types.BidID{}, err
	}
	provider, err := flags.GetString("provider")
	if err != nil {
		return types.BidID{}, err
	}
	addr, err := sdk.AccAddressFromBech32(provider)
	if err != nil {
		return types.BidID{}, err
	}
	return types.MakeBidID(prev, addr), nil
}

// AddOrderFilterFlags add flags to filter for order list
func AddOrderFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "order owner address to filter")
	flags.Uint8("state", 100, "order state to filter (0-2)")
}

// OrderFiltersFromFlags returns OrderFilters with given flags and error if occured
func OrderFiltersFromFlags(flags *pflag.FlagSet) (types.OrderFilters, error) {
	prev, err := dcli.GroupFiltersFromFlags(flags)
	if err != nil {
		return types.OrderFilters{}, err
	}
	id := types.OrderFilters{
		Owner: prev.Owner,
		State: types.OrderState(prev.State),
	}
	return id, nil
}

// AddBidFilterFlags add flags to filter for bid list
func AddBidFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "bid owner address to filter")
	flags.Uint8("state", 100, "bid state to filter (0-3)")
}

// BidFiltersFromFlags returns BidFilters with given flags and error if occured
func BidFiltersFromFlags(flags *pflag.FlagSet) (types.BidFilters, error) {
	prev, err := OrderFiltersFromFlags(flags)
	if err != nil {
		return types.BidFilters{}, err
	}
	id := types.BidFilters{
		Owner: prev.Owner,
		State: types.BidState(prev.State),
	}
	return id, nil
}

// AddLeaseFilterFlags add flags to filter for lease list
func AddLeaseFilterFlags(flags *pflag.FlagSet) {
	flags.String("owner", "", "lease owner address to filter")
	flags.Uint8("state", 100, "lease state to filter (0-2)")
}

// LeaseFiltersFromFlags returns LeaseFilters with given flags and error if occured
func LeaseFiltersFromFlags(flags *pflag.FlagSet) (types.LeaseFilters, error) {
	prev, err := OrderFiltersFromFlags(flags)
	if err != nil {
		return types.LeaseFilters{}, err
	}
	id := types.LeaseFilters{
		Owner: prev.Owner,
		State: types.LeaseState(prev.State),
	}
	return id, nil
}

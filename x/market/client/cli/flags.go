package cli

import (
	"github.com/cosmos/cosmos-sdk/client/context"
	dcli "github.com/ovrclk/akash/x/deployment/client/cli"
	"github.com/ovrclk/akash/x/market/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AddOrderIDFlags add flags for order
func AddOrderIDFlags(cmd *cobra.Command) {
	dcli.AddGroupIDFlags(cmd)
	cmd.Flags().Uint32("oseq", 0, "Order Sequence")
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
func AddBidIDFlags(cmd *cobra.Command) {
	AddOrderIDFlags(cmd)
}

// BidIDFromFlags returns BidID with given flags and error if occured
func BidIDFromFlags(ctx context.CLIContext, flags *pflag.FlagSet) (types.BidID, error) {
	prev, err := OrderIDFromFlags(flags)
	if err != nil {
		return types.BidID{}, err
	}
	return types.MakeBidID(prev, ctx.GetFromAddress()), nil
}

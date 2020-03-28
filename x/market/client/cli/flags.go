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

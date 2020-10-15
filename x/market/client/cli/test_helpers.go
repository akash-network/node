package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"
	"github.com/ovrclk/akash/x/market/types"
)

const key string = types.StoreKey

// TxCreateBidExec is used for testing create bid tx
func TxCreateBidExec(clientCtx client.Context, orderID types.OrderID, price, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%s", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
		fmt.Sprintf("--price=%s", price.String()),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCreateBid(key), args)
}

// TxCloseBidExec is used for testing close bid tx
func TxCloseBidExec(clientCtx client.Context, orderID types.OrderID, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%s", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCloseBid(key), args)
}

// TxCloseOrderExec is used for testing close order tx
func TxCloseOrderExec(clientCtx client.Context, orderID types.OrderID, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%s", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdCloseOrder(key), args)
}

// QueryOrdersExec is used for testing orders query
func QueryOrdersExec(clientCtx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetOrders(), args)
}

// QueryOrderExec is used for testing order query
func QueryOrderExec(clientCtx client.Context, orderID types.OrderID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetOrder(), args)
}

// QueryBidsExec is used for testing bids query
func QueryBidsExec(clientCtx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetBids(), args)
}

// QueryBidExec is used for testing bid query
func QueryBidExec(clientCtx client.Context, bidID types.BidID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", bidID.Owner),
		fmt.Sprintf("--dseq=%v", bidID.DSeq),
		fmt.Sprintf("--gseq=%v", bidID.GSeq),
		fmt.Sprintf("--oseq=%v", bidID.OSeq),
		fmt.Sprintf("--provider=%v", bidID.Provider),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetBid(), args)
}

// QueryLeasesExec is used for testing leases query
func QueryLeasesExec(clientCtx client.Context, args ...string) (sdktest.BufferWriter, error) {
	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetLeases(), args)
}

// QueryLeaseExec is used for testing lease query
func QueryLeaseExec(clientCtx client.Context, leaseID types.LeaseID, extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--owner=%s", leaseID.Owner),
		fmt.Sprintf("--dseq=%v", leaseID.DSeq),
		fmt.Sprintf("--gseq=%v", leaseID.GSeq),
		fmt.Sprintf("--oseq=%v", leaseID.OSeq),
		fmt.Sprintf("--provider=%v", leaseID.Provider),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdGetLease(), args)
}

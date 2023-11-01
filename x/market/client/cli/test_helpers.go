package cli

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	sdktest "github.com/cosmos/cosmos-sdk/testutil"
	clitestutil "github.com/cosmos/cosmos-sdk/testutil/cli"

	types "github.com/akash-network/akash-api/go/node/market/v1beta4"
)

// XXX: WHY TF DON'T THESE RETURN OBJECTS

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

	return clitestutil.ExecTestCLICmd(clientCtx, cmdBidCreate(key), args)
}

// TxCloseBidExec is used for testing close bid tx
func TxCloseBidExec(clientCtx client.Context, orderID types.OrderID, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%v", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdBidClose(key), args)
}

// TxCreateLeaseExec is used for creating a lease
func TxCreateLeaseExec(clientCtx client.Context, bid types.BidID, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--dseq=%v", bid.DSeq),
		fmt.Sprintf("--gseq=%v", bid.GSeq),
		fmt.Sprintf("--oseq=%v", bid.OSeq),
		fmt.Sprintf("--provider=%s", bid.Provider),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdLeaseCreate(key), args)
}

// TxCloseLeaseExec is used for testing close order tx
func TxCloseLeaseExec(clientCtx client.Context, orderID types.OrderID, from fmt.Stringer,
	extraArgs ...string) (sdktest.BufferWriter, error) {
	args := []string{
		fmt.Sprintf("--from=%s", from.String()),
		fmt.Sprintf("--owner=%s", orderID.Owner),
		fmt.Sprintf("--dseq=%v", orderID.DSeq),
		fmt.Sprintf("--gseq=%v", orderID.GSeq),
		fmt.Sprintf("--oseq=%v", orderID.OSeq),
	}

	args = append(args, extraArgs...)

	return clitestutil.ExecTestCLICmd(clientCtx, cmdLeaseClose(key), args)
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

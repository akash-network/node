package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	akashclient "github.com/ovrclk/akash/client"
	cmdcommon "github.com/ovrclk/akash/cmd/common"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	cutils "github.com/ovrclk/akash/x/cert/utils"
)

const(
	FlagStartTime = "start-time"
	FlagEndTime = "end-time"
	FlagForward = "forward"
	FlagLimit = "limit"
	FlagRunIndex = "run-index"
)

func leaseLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "lease-logs",
		Short:        "get lease logs",
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLeaseLogsV2(cmd, args)
		},
	}

	addServiceFlags(cmd)

	cmd.Flags().BoolP(flagFollow, "f", false, "Specify if the logs should be streamed. Defaults to false")
	cmd.Flags().Int64P(flagTail, "t", -1, "The number of lines from the end of the logs to show. Defaults to -1")
	cmd.Flags().StringP(flagOutput, "o", outputText, "Output format text|json. Defaults to text")
	cmd.Flags().Bool(FlagForward, true, "")
	cmd.Flags().String(FlagEndTime, "", "")
	cmd.Flags().String(FlagStartTime, "", "")
	cmd.Flags().Uint(FlagLimit, 100, "")
	cmd.Flags().Int(FlagRunIndex, -1, "")

	return cmd
}

func leaseLogStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "lease-log-status",
		Short:        "get lease log status",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLeaseLogsStatus(cmd)
		},
	}

	addServiceFlags(cmd)

	return cmd
}

func withLeasesForDeployment(ctx context.Context, cctx sdkclient.Context, deploymentID dtypes.DeploymentID, provider string,gseq, oseq uint32, fn func(leaseID mtypes.LeaseID) error) error {
	filter := mtypes.LeaseFilters{
		Owner: deploymentID.Owner,
		DSeq:  deploymentID.DSeq,
		State: mtypes.Lease_State_name[int32(mtypes.LeaseActive)],
	}

	if len(provider) != 0 {
		filter.Provider = provider
	}

	if gseq > 0 {
		filter.GSeq = gseq
	}

	if oseq > 0 {
		filter.OSeq = oseq
	}

	cclient := akashclient.NewQueryClientFromCtx(cctx)
	resp, err := cclient.Leases(ctx, &mtypes.QueryLeasesRequest{
		Filters: filter,
	})
	if err != nil {
		return err
	}

	if len(resp.Leases) == 0 {
		msg := &bytes.Buffer{}
		_, _ = fmt.Fprintf(msg, "dseq=%v", filter.DSeq)
		if len(provider) > 0 {
			_, _ = fmt.Fprintf(msg, " provider=%v", filter.Provider)
		}
		if filter.GSeq > 0 {
			_, _ = fmt.Fprintf(msg, " gseq=%v", filter.GSeq)
		}
		if filter.OSeq > 0 {
			_, _ = fmt.Fprintf(msg, " oseq=%v", filter.OSeq)
		}
		return fmt.Errorf("%w: %s",errNoActiveLease, msg.String())
	}

	for _, lease := range resp.Leases {
		err = fn(lease.Lease.LeaseID)
		if err != nil {
			return err
		}
	}
	return nil
}

func doLeaseLogsStatus(cmd *cobra.Command) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	dseq, err := dseqFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	deploymentID := dtypes.DeploymentID{
		Owner: cctx.GetFromAddress().String(),
		DSeq:  dseq,
	}

	provider, err := cmd.Flags().GetString(FlagProvider)
	if err != nil {
		return err
	}
	gseq, err := cmd.Flags().GetUint32(FlagGSeq)
	if err != nil {
		return err
	}
	oseq, err := cmd.Flags().GetUint32(FlagOSeq)
	if err != nil {
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return markRPCServerError(err)
	}

	return withLeasesForDeployment(cmd.Context(),
		cctx,
		deploymentID,
		provider,
		gseq,
		oseq,
		func(leaseID mtypes.LeaseID) error {
			// TODO - use client directory object here?
			prov, _ := sdk.AccAddressFromBech32(leaseID.GetProvider())
			gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
			if err != nil {
				return err
			}
			status, err := gclient.LeaseLogsStatus(cmd.Context(), leaseID)
			if err != nil {
				 return err
			}

			_ = status // TODO - print status
			buf := &bytes.Buffer{}
			encoder := json.NewEncoder(buf)
			err = encoder.Encode(status)
			if err != nil {
				return err
			}
			return cctx.PrintBytes(buf.Bytes())
		})
}

func  streamLeaseLogs(ctx context.Context,
	cert tls.Certificate,
cctx sdkclient.Context,
deploymentID dtypes.DeploymentID,
provider string,
gseq uint32,
oseq uint32,
serviceName string,
startTime time.Time,
replicaIndex int,
runIndex int) error {
	/** TODO - since service name is specified does this need to actually search for all the providers?
	it should only be running on 1 provider with our architecture
	 */
	return withLeasesForDeployment(ctx,
		cctx,
		deploymentID,
		provider,
		gseq,
		oseq,
		func(leaseID mtypes.LeaseID) error {
			prov, _ := sdk.AccAddressFromBech32(leaseID.GetProvider())
			gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
			if err != nil {
				return err
			}

			return gclient.LeaseLogsV2Follow(ctx,
				leaseID,
				serviceName,
				uint(replicaIndex),
				runIndex,
				startTime, func(at time.Time, line string) error {
					return cctx.PrintString(fmt.Sprintf("%v %s\n", at, line))
				})
		})
}


func doLeaseLogsV2(cmd *cobra.Command, args []string) error {
	serviceName := args[0]
	replicaIndexStr := args[1]

	replicaIndex, err := strconv.ParseUint(replicaIndexStr, 10, 31)
	if err != nil {
		// TODO - better error here
		return err
	}

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	dseq, err := dseqFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	deploymentID := dtypes.DeploymentID{
		Owner: cctx.GetFromAddress().String(),
		DSeq:  dseq,
	}

	provider, err := cmd.Flags().GetString(FlagProvider)
	if err != nil {
		return err
	}
	gseq, err := cmd.Flags().GetUint32(FlagGSeq)
	if err != nil {
		return err
	}
	oseq, err := cmd.Flags().GetUint32(FlagOSeq)
	if err != nil {
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return markRPCServerError(err)
	}

	limit, err := cmd.Flags().GetUint(FlagLimit)
	if err != nil {
		return err
	}
	startTimeStr, err := cmd.Flags().GetString(FlagStartTime)
	if err != nil {
		return err
	}
	endTimeStr,err := cmd.Flags().GetString(FlagEndTime)
	if err != nil {
		return err
	}
	forward, err := cmd.Flags().GetBool(FlagForward)
	if err != nil {
		return err
	}
	runIndex, err := cmd.Flags().GetInt(FlagRunIndex)

	startTime := time.Time{}
	if len(startTimeStr) != 0 {
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return err
		}
	}
	endTime := time.Time{}
	if len(endTimeStr) != 0 {
		endTime, err = time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return err
		}
	}

	follow, err := cmd.Flags().GetBool(flagFollow)
	if err != nil {
		return err
	}
	if follow {
		return streamLeaseLogs(cmd.Context(),
			cert,
			cctx,
			deploymentID,
			provider,
			gseq,
			oseq,
			serviceName,
			startTime,
			int(replicaIndex),
			runIndex)
	}

	/** TODO - since service name is specified does this need to actually search for all the providers?
	it should only be running on 1 provider with our architecture
	*/
	return withLeasesForDeployment(cmd.Context(),
		cctx,
		deploymentID,
		provider,
		gseq,
		oseq,
		func(leaseID mtypes.LeaseID) error {
			// TODO - use client directory object here?
			prov, _ := sdk.AccAddressFromBech32(leaseID.GetProvider())
			gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
			if err != nil {
				return err
			}

			result, err := gclient.LeaseLogsV2(cmd.Context(),
				leaseID,
				serviceName,
				uint(replicaIndex),
				runIndex,
				startTime,
				endTime,
				forward,
				limit)

			if err != nil {
				clientError, ok := err.(gwrest.ClientResponseError)
				if ok{
					_ = cctx.PrintString(clientError.ClientError())
				}
				return err
			}

			// TODO - check output flag for format
			buf := &bytes.Buffer{}
			encoder := json.NewEncoder(buf)
			err = encoder.Encode(result)
			if err != nil {
				return err
			}

			return cctx.PrintBytes(buf.Bytes())
		})
}

func doLeaseLogs(cmd *cobra.Command) error {
	// TODO - clean up most of this into some sort of simple "withEachLease" function that calls a
	// function for each lease found
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return markRPCServerError(err)
	}

	dseq, err := dseqFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	leases, err := leasesForDeployment(cmd.Context(), cctx, cmd.Flags(), dtypes.DeploymentID{
		Owner: cctx.GetFromAddress().String(),
		DSeq:  dseq,
	})
	if err != nil {
		return markRPCServerError(err)
	}

	svcs, err := cmd.Flags().GetString(FlagService)
	if err != nil {
		return err
	}

	outputFormat, err := cmd.Flags().GetString(flagOutput)
	if err != nil {
		return err
	}

	if outputFormat != outputText && outputFormat != outputJSON {
		return errors.Errorf("invalid output format %s. expected text|json", outputFormat)
	}

	follow, err := cmd.Flags().GetBool(flagFollow)
	if err != nil {
		return err
	}

	tailLines, err := cmd.Flags().GetInt64(flagTail)
	if err != nil {
		return err
	}

	if tailLines < -1 {
		return errors.Errorf("tail flag supplied with invalid value. must be >= -1")
	}

	type result struct {
		lid    mtypes.LeaseID
		error  error
		stream *gwrest.ServiceLogs
	}

	streams := make([]result, 0, len(leases))

	ctx := cmd.Context()

	for _, lid := range leases {
		stream := result{lid: lid}
		prov, _ := sdk.AccAddressFromBech32(lid.Provider)
		gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
		if err == nil {
			stream.stream, stream.error = gclient.LeaseLogs(ctx, lid, svcs, follow, tailLines)
		} else {
			stream.error = err
		}

		streams = append(streams, stream)
	}

	var wgStreams sync.WaitGroup

	type logEntry struct {
		gwrest.ServiceLogMessage `json:",inline"`
		Lid                      mtypes.LeaseID `json:"lease_id"`
	}

	outch := make(chan logEntry)

	printFn := func(evt logEntry) {
		fmt.Printf("[%s][%s] %s\n", evt.Lid, evt.Name, evt.Message)
	}

	if outputFormat == "json" {
		printFn = func(evt logEntry) {
			_ = cmdcommon.PrintJSON(cctx, evt)
		}
	}

	go func() {
		for evt := range outch {
			printFn(evt)
		}
	}()

	for _, stream := range streams {
		if stream.error != nil {
			continue
		}

		wgStreams.Add(1)
		go func(stream result) {
			defer wgStreams.Done()

			for res := range stream.stream.Stream {
				outch <- logEntry{
					ServiceLogMessage: res,
					Lid:               stream.lid,
				}
			}
		}(stream)
	}

	wgStreams.Wait()
	close(outch)

	return nil
}

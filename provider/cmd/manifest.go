package cmd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	dtypes "github.com/ovrclk/akash/x/deployment/types"

	akashclient "github.com/ovrclk/akash/client"
	gwrest "github.com/ovrclk/akash/provider/gateway/rest"
	"github.com/ovrclk/akash/sdl"
	cutils "github.com/ovrclk/akash/x/cert/utils"
)

var (
	errSubmitManifestFailed = errors.New("submit manifest to some providers has been failed")
)

// SendManifestCmd looks up the Providers blockchain information,
// and POSTs the SDL file to the Gateway address.
func SendManifestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "send-manifest <sdl-path>",
		Args:         cobra.ExactArgs(1),
		Short:        "Submit manifest to provider(s)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doSendManifest(cmd, args[0])
		},
	}

	addManifestFlags(cmd)

	cmd.Flags().StringP(flagOutput, "o", outputText, "output format text|json|yaml. default text")

	return cmd
}

func doSendManifest(cmd *cobra.Command, sdlpath string) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	sdl, err := sdl.ReadFile(sdlpath)
	if err != nil {
		return err
	}

	mani, err := sdl.Manifest()
	if err != nil {
		return err
	}
	// TODO - dump mani

	cert, err := cutils.LoadAndQueryCertificateForAccount(cmd.Context(), cctx, cctx.Keyring)
	if err != nil {
		return err
	}

	dseq, err := dseqFromFlags(cmd.Flags())
	if err != nil {
		return err
	}

	// owner address in FlagFrom has already been validated thus save to just pull its value as string
	leases, err := leasesForDeployment(cmd.Context(), cctx, cmd.Flags(), dtypes.DeploymentID{
		Owner: cctx.GetFromAddress().String(),
		DSeq:  dseq,
	})
	if err != nil {
		return err
	}

	type result struct {
		Provider sdk.Address `json:"provider" yaml:"provider"`
		Status   string      `json:"status" yaml:"status"`
		Error    error       `json:"error,omitempty" yaml:"error,omitempty"`
	}

	results := make([]result, len(leases))

	submitFailed := false

	for i, lid := range leases {
		prov, _ := sdk.AccAddressFromBech32(lid.Provider)
		gclient, err := gwrest.NewClient(akashclient.NewQueryClientFromCtx(cctx), prov, []tls.Certificate{cert})
		if err != nil {
			return err
		}

		err = gclient.SubmitManifest(context.Background(), dseq, mani)
		res := result{
			Provider: prov,
			Status:   "PASS",
			Error:    err,
		}

		if err != nil {
			res.Status = "FAIL"
			submitFailed = true
		}

		results[i] = res
	}

	buf := &bytes.Buffer{}

	switch cmd.Flag(flagOutput).Value.String() {
	case outputText:
		for _, res := range results {
			_, _ = fmt.Fprintf(buf, "provider: %s\n\tstatus: %s\n", res.Provider, res.Status)
			if res.Error != nil {
				_, _ = fmt.Fprintf(buf, "\terror: %v\n", res.Error)
			}
		}
	case outputJSON:
		err = json.NewEncoder(buf).Encode(results)
	case outputYAML:
		err = yaml.NewEncoder(buf).Encode(results)
	}

	if err != nil {
		return err
	}

	_, err = fmt.Fprint(cmd.OutOrStdout(), buf.String())

	if err != nil {
		return err
	}

	if submitFailed {
		return errSubmitManifestFailed
	}

	return nil
}

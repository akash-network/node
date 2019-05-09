package query

import (
	"fmt"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/util/uiutil"
	"github.com/spf13/cobra"
)

func queryDeploymentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment <deployment>...",
		Short: "query deployment",
		RunE:  session.WithSession(session.RequireNode(doQueryDeploymentCommand)),
	}
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	return cmd
}

func doQueryDeploymentCommand(s session.Session, cmd *cobra.Command, args []string) error {
	var hasSigner, hasDepIDs bool
	var depID string
	var deployments []*types.Deployment
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

	hasDepIDs = len(args) > 0
	_, info, err := s.Signer()
	if err == nil {
		hasSigner = true
	}

	switch {
	case hasSigner == false && hasDepIDs == false:
		depID = s.Mode().Ask().StringVar(depID, "Deployment ID (required): ", true)
		if len(depID) == 0 {
			return fmt.Errorf("required argument missing: id")
		}
		args = []string{depID}
		hasDepIDs = true
		fallthrough
	case hasDepIDs:
		for _, arg := range args {
			key, err := keys.ParseDeploymentPath(arg)
			if err != nil {
				return err
			}
			dep, err := s.QueryClient().Deployment(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			deployments = append(deployments, dep)
		}
	case hasSigner:
		tdeps, err := s.QueryClient().TenantDeployments(s.Ctx(), info.GetPubKey().Address().Bytes())
		if err != nil {
			return err
		}
		for _, dep := range tdeps.Items {
			deployments = append(deployments, &dep)
		}
	}

	for _, dep := range deployments {
		dat := map[string]string{
			"deployment": X(dep.Address),
			"tenant":     X(dep.Tenant),
			"state":      dep.State.String(),
			"version":    X(dep.Version),
		}
		printerDat.AddResultList(dat)
		rawDat = append(rawDat, dep)
	}
	printerDat.Raw = rawDat
	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			table := uitable.New().
				AddRow(
					uiutil.NewTitle("State").String(),
					uiutil.NewTitle("Deployment ID").String(),
					uiutil.NewTitle("Tenant ID").String(),
					uiutil.NewTitle("Version").String(),
				)
			table.MaxColWidth = 100
			table.Wrap = true
			for _, dat := range printerDat.Result {
				table.AddRow(dat["state"], dat["deployment"], dat["tenant"], dat["version"])
			}
			return session.NewIPrinter(nil).AddText("").Add(table).Flush()
		}).When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

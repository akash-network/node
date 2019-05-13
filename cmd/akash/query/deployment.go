package query

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/util/uiutil"
	"github.com/ovrclk/akash/util/ulog"
	"github.com/ovrclk/dsky"
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
	deployments := make([]types.Deployment, 0, 0)
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

	hasDepIDs = len(args) > 0
	_, info, err := s.Signer()
	if err == nil {
		hasSigner = true
	}

	printer := s.Mode().Printer()
	data := printer.NewSection("Deployment Query").NewData()

	switch {
	case hasSigner == false && hasDepIDs == false:
		if err != nil && s.Mode().IsInteractive() {
			var warn string
			switch err.(type) {
			case *session.TooManyKeysForDefaultError:
				warn = fmt.Sprintf("%v", err)
			case session.NoKeysForDefaultError:
				warn = fmt.Sprintf("%v", err)
			}
			warn = warn + "\n\nEither re-run the command by providing a key using '-k <key>' or a deployment ID as attribute. Alternatively, you can also provide the below info to continue."
			fmt.Printf("%s\n", ulog.Warn(warn))
			depID = s.Mode().Ask().StringVar(depID, "Deployment ID (required): ", true)
			args = []string{depID}
			hasDepIDs = true
		}
		fallthrough
	case hasDepIDs:
		if len(args) == 0 {
			return errors.NewArgumentError("deployment_id")
		}
		for _, arg := range args {
			key, err := keys.ParseDeploymentPath(arg)
			if err != nil {
				return err
			}
			dep, err := s.QueryClient().Deployment(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			deployments = append(deployments, *dep)
		}
	case hasSigner:
		tdeps, err := s.QueryClient().TenantDeployments(s.Ctx(), info.GetPubKey().Address().Bytes())
		if err != nil {
			return err
		}
		for _, dep := range tdeps.Items {
			deployments = append(deployments, dep)
		}
	}

	for _, dep := range deployments {
		data.Add("Deployment ID", X(dep.Address))
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
		When(dsky.ModeTypeInteractive, func() error {
			p := session.NewIPrinter(nil).AddText("")
			lt := uiutil.NewListTable().AddHeader("State", "Deployment ID", "Version")

			// Display tenant ID only when signer is not present to avoid redudency
			if hasDepIDs {
				p.AddTitle("Deployments")
				lt.AddHeader("Tenant ID")
			} else {
				p.AddTitle(fmt.Sprintf("Deployments for %s (%s)", X(info.GetPubKey().Address()), s.KeyName()))
			}

			for _, dat := range printerDat.Result {
				row := []interface{}{dat["state"], dat["deployment"], dat["version"]}
				if hasDepIDs {
					row = append(row, dat["tenant"])
				}
				lt.AddRow(row...)
			}

			t := lt.UITable()
			t.MaxColWidth = 100
			t.Wrap = true

			return p.Add(t).Flush()
		}).When(dsky.ModeTypeShell, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(dsky.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

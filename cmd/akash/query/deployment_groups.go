package query

import (
	"fmt"
	"strings"

	humanize "github.com/dustin/go-humanize"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/util/uiutil"
	"github.com/spf13/cobra"
)

func queryDeploymentGroupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deployment-group [deployment-group ...]",
		Short: "query deployment groups",
		RunE:  session.WithSession(session.RequireNode(doQueryDeploymentGroupCommand)),
	}

	return cmd
}

func doQueryDeploymentGroupCommand(s session.Session, cmd *cobra.Command, args []string) error {
	hasIDs := len(args) > 0
	groups := make([]*types.DeploymentGroup, 0)
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

	if hasIDs {
		for _, arg := range args {
			key, err := keys.ParseGroupPath(arg)
			if err != nil {
				return err
			}
			group, err := s.QueryClient().DeploymentGroup(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			groups = append(groups, group)
		}
	} else {
		depgroups, err := s.QueryClient().DeploymentGroups(s.Ctx())
		if err != nil {
			return err
		}
		groups = depgroups.Items
	}

	for _, group := range groups {
		printerDat.AddResultList(makePrinterResultDeploymentGroup(group))
		rawDat = append(rawDat, group)
	}
	printerDat.Raw = rawDat

	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			return session.NewIPrinter(nil).AddText("").Add(makeUITableDeploymentGroups(groups)).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

func makeUITableDeploymentGroups(groups []*types.DeploymentGroup) *uitable.Table {
	t := uitable.New().
		AddRow(
			uiutil.NewTitle("Group (Deployment/Sequence)").String(),
			uiutil.NewTitle("Name").String(),
			uiutil.NewTitle("State").String(),
			uiutil.NewTitle("Requirements").String(),
			uiutil.NewTitle("Resources").String(),
		)
	t.Wrap = true
	for _, group := range groups {
		res := makePrinterResultDeploymentGroup(group)
		t.AddRow(res["group"], res["name"], res["state"], res["requirements"], res["resources"])
	}
	return t
}

func makePrinterResultDeploymentGroup(group *types.DeploymentGroup) session.PrinterResult {
	var reqs []string
	for _, r := range group.Requirements {
		reqs = append(reqs, fmt.Sprintf("%s:%s", r.Name, r.Value))
	}
	var resources []string
	for _, r := range group.Resources {
		count := humanize.Comma(int64(r.Count))
		price := humanize.Comma(int64(r.Price))
		cpu := humanize.Comma(int64(r.Unit.CPU))
		mem := humanize.Bytes(r.Unit.Memory)
		disk := humanize.Bytes(r.Unit.Disk)
		rg := fmt.Sprintf("Count: %s, Price %s, CPU: %s, Memory: %s, Disk: %s", count, price, cpu, mem, disk)
		resources = append(resources, rg)
	}

	return map[string]string{
		"group":        group.DeploymentGroupID.String(),
		"name":         group.Name,
		"state":        group.State.String(),
		"requirements": strings.Join(reqs, "\n"),
		"resources":    strings.Join(resources, "\n"),
	}
}

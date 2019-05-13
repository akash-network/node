package query

import (
	"strconv"

	humanize "github.com/dustin/go-humanize"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/dsky"
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
	groups := make([]*types.DeploymentGroup, 0)
	printer := s.Mode().Printer()
	if len(args) > 0 {
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
	data := printer.NewSection("Deployment Group(s)").NewData().WithTag("raw", groups)
	if len(groups) > 1 {
		data.AsList()
	}
	for _, group := range groups {
		data.
			Add("Group ID", group.DeploymentGroupID.String()).
			WithLabel("Group ID", "Group ID (Deployment/Sequence)").
			Add("Name", group.Name)
		if group.State == types.DeploymentGroup_OPEN {
			data.Add("State", dsky.Color.Hi.Sprint(group.State.String()))
		} else {
			data.Add("State", group.State.String())
		}
		data.Add("Order TTL (Blocks)", strconv.FormatInt(group.OrderTTL, 10))

		req := make(map[string]string)
		for _, r := range group.Requirements {
			req[r.Name] = r.Value
		}
		data.Add("Requirements", req)

		rd := dsky.NewSectionData("").AsList()
		for _, r := range group.Resources {
			rd.Add("count", humanize.Comma(int64(r.Count))).
				Add("price", humanize.Comma(int64(r.Price))).
				Add("cpu", humanize.Comma(int64(r.Unit.CPU))).
				Add("mem", humanize.Bytes(r.Unit.Memory)).
				Add("disk", humanize.Bytes(r.Unit.Disk))
		}
		data.Add("Resources", rd)

	}
	return printer.Flush()
}

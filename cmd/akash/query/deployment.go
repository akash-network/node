package query

import (
	"fmt"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/errors"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
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
	hasDepIDs = len(args) > 0
	_, info, err := s.Signer()
	if err == nil {
		hasSigner = true
	}
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
			s.Mode().Printer().Log().Warn(warn)
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

	data := s.Mode().Printer().NewSection("Deployment").WithLabel("Deployment(s)").NewData().WithTag("raw", deployments)
	if len(deployments) > 1 {
		data.AsList()
	}
	for _, dep := range deployments {
		data.
			Add("Deployment ID", X(dep.Address)).
			Add("Tenant ID", X(dep.Tenant))
		if dep.State == types.Deployment_ACTIVE {
			data.Add("State", dsky.Color.Hi.Sprint(dep.State.String()))
		} else {
			data.Add("State", dep.State.String())
		}
		data.Add("Version", X(dep.Version))
	}
	return s.Mode().Printer().Flush()
}

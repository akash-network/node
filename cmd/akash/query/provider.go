package query

import (
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/spf13/cobra"
)

func queryProviderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider [provider ...]",
		Short: "query provider",
		RunE:  session.WithSession(session.RequireNode(doQueryProviderCommand)),
	}
	return cmd
}

func doQueryProviderCommand(s session.Session, cmd *cobra.Command, args []string) error {
	providers := make([]*types.Provider, 0)

	if len(args) == 0 {
		res, err := s.QueryClient().Providers(s.Ctx())
		if err != nil {
			return err
		}
		providers = res.Providers
	} else {
		for _, arg := range args {
			key, err := keys.ParseProviderPath(arg)
			if err != nil {
				return err
			}
			provider, err := s.QueryClient().Provider(s.Ctx(), key.ID())
			if err != nil {
				return err
			}
			providers = append(providers, provider)
		}
	}

	data := s.Mode().Printer().NewSection("Provider(s)").NewData().WithTag("raw", providers)
	if len(providers) > 1 {
		data.AsList()
	}
	for _, p := range providers {
		data.
			Add("Address", X(p.Address)).
			Add("Owner", X(p.Owner)).
			Add("Host URI", p.HostURI)
		attrs := make(map[string]string)
		for _, a := range p.Attributes {
			attrs[a.Name] = a.Value
		}
		data.Add("Attributes", attrs)
	}

	return s.Mode().Printer().Flush()
}

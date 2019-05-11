package query

import (
	"fmt"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/util/uiutil"
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
	printerDat := session.NewPrinterDataList()
	rawDat := make([]interface{}, 0, 0)

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

	for _, p := range providers {
		printerDat.AddResultList(makePrinterResultProvider(p))
		rawDat = append(rawDat, p)
	}
	printerDat.Raw = rawDat
	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			t := uitable.New().
				AddRow(
					uiutil.NewTitle("Address").String(),
					uiutil.NewTitle("Owner").String(),
					uiutil.NewTitle("Host URI").String(),
					uiutil.NewTitle("Attributes").String(),
				)
			t.Wrap = true
			for _, p := range providers {
				res := makePrinterResultProvider(p)
				t.AddRow(res["address"], res["owner"], res["host_uri"], res["attributes"])
			}
			return session.NewIPrinter(nil).AddText("").Add(t).Flush()
		}).
		When(session.ModeTypeText, func() error { return session.NewTextPrinter(printerDat, nil).Flush() }).
		When(session.ModeTypeJSON, func() error { return session.NewJSONPrinter(printerDat, nil).Flush() }).
		Run()
}

func makePrinterResultProvider(provider *types.Provider) session.PrinterResult {
	var attrs []string
	for _, a := range provider.Attributes {
		attrs = append(attrs, fmt.Sprintf("%s:%s", a.Name, a.Value))
	}
	return session.PrinterResult{
		"address":    X(provider.Address),
		"owner":      X(provider.Owner),
		"host_uri":   provider.HostURI,
		"attributes": strings.Join(attrs, ", "),
	}
}

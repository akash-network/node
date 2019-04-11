package query

import (
	"strconv"

	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
	"github.com/ovrclk/akash/util/uiutil"
	"github.com/spf13/cobra"
)

func queryAccountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "account <account>...",
		Short:   "query account balance",
		Example: queryAccountExample,
		RunE:    session.WithSession(session.RequireNode(doQueryAccountCommand)),
	}
	session.AddFlagKeyOptional(cmd, cmd.Flags())
	return cmd
}

func doQueryAccountCommand(s session.Session, cmd *cobra.Command, args []string) error {
	var account *types.Account

	// When a signer key is used instead of a public key(s) in the args
	if len(args) == 0 {
		_, info, err := s.Signer()
		if err != nil {
			return err
		}
		account, err = s.QueryClient().Account(s.Ctx(), info.GetPubKey().Address().Bytes())
		if err != nil {
			return err
		}

		addr, balance := X(account.Address), strconv.FormatUint(account.Balance, 10)
		nonce := strconv.FormatUint(account.Nonce, 10)

		pdata := session.NewPrinterDataKV().
			AddResultKV("public_key_address", addr).
			AddResultKV("balance", balance).
			AddResultKV("nonce", nonce)
		pdata.Raw = account

		return s.Mode().
			When(session.ModeTypeInteractive, func() error {
				return session.NewIPrinter(nil).
					AddTitle("Account Details").
					Add(uitable.New().
						AddRow("Public Key (Address):", addr).
						AddRow("Balance (mAKASH):", balance)).
					AddText("").
					AddText("Please note, the token balance is denominated in microAKASH (AKASH * 10^-6)").
					Flush()
			}).
			When(session.ModeTypeText, func() error {
				return session.NewTextPrinter(pdata, nil).Flush()
			}).
			When(session.ModeTypeJSON, func() error {
				return session.NewJSONPrinter(pdata, nil).Flush()
			}).
			Run()
	}

	// When public keys are provided as args
	pdata := session.NewPrinterDataList()
	raws := make([]interface{}, 0, 0)

	for _, arg := range args {
		key, err := keys.ParseAccountPath(arg)
		if err != nil {
			return err
		}

		account, err = s.QueryClient().Account(s.Ctx(), key.ID())
		if err != nil {
			return err
		}

		dat := map[string]string{
			"public_key_address": X(account.Address),
			"balance":            strconv.FormatUint(account.Balance, 10),
			"nonce":              strconv.FormatUint(account.Nonce, 10),
		}
		pdata.AddResultList(dat)
		raws = append(raws, account)
	}
	pdata.Raw = raws
	return s.Mode().
		When(session.ModeTypeInteractive, func() error {
			table := uitable.New().
				AddRow(uiutil.NewTitle("Public Key (Address)").String(), uiutil.NewTitle("Balance (mAKASH)"))
			table.MaxColWidth = 100
			table.Wrap = true
			for _, dat := range pdata.Result {
				table.AddRow(dat["public_key_address"], dat["balance"])
			}

			return session.NewIPrinter(nil).
				Add(table).
				AddText("").
				AddText("Please note, the token balance is denominated in microAKASH (AKASH * 10^-6)").
				Flush()
		}).
		When(session.ModeTypeText, func() error {
			return session.NewTextPrinter(pdata, nil).Flush()
		}).
		When(session.ModeTypeJSON, func() error {
			return session.NewJSONPrinter(pdata, nil).Flush()
		}).
		Run()
}

var (
	queryAccountExample = `
- Query an account with a signer (local) key:

  $ akash query account -k master 

  Account Details
  ===============

  Public Key (Address): 8e1c5ffce48bf2c5c6193129a8e0977073a7f30f
  Balance (mAKASH):     999999899247072

  Please note, the token balance is denominated in microAKASH (AKASH * 10^-6)
  
- Query for multiple accounts using the public key:

  $ akash query account 8e1c5ffce48bf2c5c6193129a8e0977073a7f30f 192a4aa8bce49bdd6e259310e7ff538bde916e8d

  Public Key (Address)                      Balance (mAKASH)
  ====================                      ================

  8e1c5ffce48bf2c5c6193129a8e0977073a7f30f  999999899244499
  192a4aa8bce49bdd6e259310e7ff538bde916e8d  100000000

  Please note, the token balance is denominated in microAKASH (AKASH * 10^-6)
	`
)

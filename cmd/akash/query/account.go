package query

import (
	"errors"
	"fmt"
	"strconv"

	ckeys "github.com/cosmos/cosmos-sdk/crypto/keys"
	humanize "github.com/dustin/go-humanize"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/keys"
	"github.com/ovrclk/akash/types"
	. "github.com/ovrclk/akash/util"
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

func doQueryAccountCommand(s session.Session, cmd *cobra.Command, args []string) (err error) {
	var hasSigner, hasPubKeys bool
	var account *types.Account
	var signerInfo ckeys.Info
	var pubKey string
	hasPubKeys = len(args) > 0
	if _, signerInfo, err = s.Signer(); err == nil {
		hasSigner = true
	}
	mode := s.Mode()
	printer := mode.Printer()
	data := printer.NewSection("Account Query").NewData()

	switch {
	case hasSigner == false && hasPubKeys == false:
		pubKey = mode.Ask().StringVar(pubKey, "Public Key (required): ", true)
		if len(pubKey) == 0 {
			return fmt.Errorf("required argument missing: name")
		}
		args = []string{pubKey}
		fallthrough
	// When public keys are provided as args
	case hasPubKeys:
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
			bal := humanize.Comma(int64(account.Balance))
			data.AsList().
				Add("Public Key Address", X(account.Address)).
				Add("Balance", bal).
				Add("Nonce", strconv.FormatUint(account.Nonce, 10))
			raws = append(raws, account)
		}
		data.WithTag("raw", raws)
		printer.Log().Warn("please note, the token balance is denominated in uAKT (AKT * 10^-6)")
		return printer.Flush()
	// When a signer key is used instead of a public key(s) in the args
	case hasSigner:
		account, err = s.QueryClient().Account(s.Ctx(), signerInfo.GetPubKey().Address().Bytes())
		if err != nil {
			return err
		}
		addr, balance := X(account.Address), strconv.FormatUint(account.Balance, 10)
		nonce := strconv.FormatUint(account.Nonce, 10)
		bal, _ := strconv.ParseInt(balance, 10, 64)
		data.WithTag("raw", account).
			Add("Public Key Address", addr).
			Add("Balance", humanize.Comma(bal)).
			Add("Nonce", nonce)
		printer.Log().Warn("please note, the token balance is denominated in microAKASH (AKASH * 10^-6)")
		return printer.Flush()
	case hasSigner && hasPubKeys:
		return errors.New("Sign key and public keys cannot be present")
	}
	return nil
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

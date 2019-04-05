package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/crypto/keys"
	"github.com/cosmos/cosmos-sdk/crypto/keys/hd"
	"github.com/gosuri/uitable"
	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/ovrclk/akash/cmd/common"
	"github.com/spf13/cobra"

	. "github.com/ovrclk/akash/util"
)

func keyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Key commands",
	}
	cmd.AddCommand(keyCreateCommand())
	cmd.AddCommand(keyListCommand())
	cmd.AddCommand(keyShowCommand())
	cmd.AddCommand(keyRecoverCommand())
	return cmd
}
func keyCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create new key",
		RunE:  session.WithSession(session.RequireRootDir(doKeyCreateCommand)),
	}
	session.AddFlagKeyType(cmd, cmd.Flags())
	return cmd
}

func doKeyCreateCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("name argument required")
	}

	kmgr, err := session.KeyManager()
	if err != nil {
		return err
	}

	ktype, err := session.KeyType()
	if err != nil {
		return err
	}

	password, err := session.Password()
	if err != nil {
		return err
	}

	info, seed, err := kmgr.CreateMnemonic(args[0], common.DefaultCodec, password, ktype)
	if err != nil {
		return err
	}

	table := uitable.New()
	table.AddRow("Public Key:", X(info.GetPubKey().Address()))
	table.AddRow("Recovery Codes:", seed)
	fmt.Println(table)

	return nil
}

func keyListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list keys",
		RunE:  session.WithSession(session.RequireKeyManager(doKeyListCommand)),
	}
}

func keyRecoverCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "recover <name> <recovery-codes>...",
		Short:   "Recover key from recovery codes",
		Example: "akash key recover my-key today napkin arch picnic fox case thrive table journey ill any enforce awesome desert chapter regret narrow capable advice skull pipe giraffe clown outside",
		Args:    cobra.ExactArgs(25),
		RunE:    session.WithSession(session.RequireKeyManager(doKeyRecoverCommand)),
	}
}

func doKeyRecoverCommand(session session.Session, cmd *cobra.Command, args []string) error {
	// the first arg is the key name and the rest are mnemonic codes
	name, args := args[0], args[1:]
	seed := strings.Join(args, " ")

	password, err := session.Password()
	if err != nil {
		return err
	}

	kmgr, _ := session.KeyManager()

	params := *hd.NewFundraiserParams(0, 0)
	info, err := kmgr.Derive(name, seed, keys.DefaultBIP39Passphrase, password, params)
	if err != nil {
		return err
	}
	fmt.Println("import successful", X(info.GetPubKey().Address()))
	return nil

}

func doKeyListCommand(session session.Session, cmd *cobra.Command, args []string) error {
	kmgr, _ := session.KeyManager()
	infos, err := kmgr.List()
	if err != nil {
		return err
	}
	table := uitable.New()
	table.MaxColWidth = 80
	table.Wrap = true
	for _, info := range infos {
		table.AddRow(info.GetName(), X(info.GetPubKey().Address()))
	}
	fmt.Println(table)
	return nil
}

func keyShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [name]",
		Short: "display a key",
		Args:  cobra.ExactArgs(1),
		RunE:  session.WithSession(session.RequireRootDir(doKeyShowCommand)),
	}
	session.AddFlagKeyType(cmd, cmd.Flags())
	return cmd
}

func doKeyShowCommand(session session.Session, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("name argument required")
	}

	kmgr, err := session.KeyManager()
	if err != nil {
		return err
	}

	name := args[0]

	info, err := kmgr.Get(name)
	if err != nil {
		return err
	}

	if len(info.GetPubKey().Address()) == 0 {
		return fmt.Errorf("key not found %s", name)
	}

	fmt.Println(X(info.GetPubKey().Address()))
	return nil
}

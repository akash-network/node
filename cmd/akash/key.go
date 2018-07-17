package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ovrclk/akash/cmd/akash/session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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

	info, _, err := kmgr.Create(args[0], viper.GetString("PASSWORD"), ktype)
	if err != nil {
		return err
	}

	fmt.Println(X(info.Address()))

	return nil
}

func keyListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list keys",
		RunE:  session.WithSession(session.RequireKeyManager(doKeyListCommand)),
	}
}

func doKeyListCommand(session session.Session, cmd *cobra.Command, args []string) error {
	kmgr, _ := session.KeyManager()

	infos, err := kmgr.List()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 0, '\t', 0)
	for _, info := range infos {
		fmt.Fprintf(tw, "%v\t%v\n", info.Name, X(info.Address()))
	}
	tw.Flush()
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

	if len(info.Address()) == 0 {
		return fmt.Errorf("key not found %s", name)
	}

	fmt.Println(X(info.Address()))
	return nil
}

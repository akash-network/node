package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tendermint/go-wire/data"
)

const (
	flagKeyType = "type"

	// todo: interactive.
	password = "0123456789"
)

func keyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "key",
		Short: "Key commands",
	}
	cmd.AddCommand(keyCreateCommand())
	cmd.AddCommand(keyListCommand())
	return cmd
}

func keyCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create new key",
		RunE:  withContext(requireRootDir(doKeyCreateCommand)),
	}
	cmd.Flags().StringP(flagKeyType, "t", "ed25519", "Type of key (ed25519|secp256k1|ledger)")
	return cmd
}

func doKeyCreateCommand(ctx Context, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("name argument required")
	}

	kmgr, err := ctx.KeyManager()
	if err != nil {
		return err
	}

	ktype, err := cmd.Flags().GetString(flagKeyType)
	if err != nil {
		return err
	}

	info, _, err := kmgr.Create(args[0], password, ktype)
	if err != nil {
		return err
	}

	addr, err := data.ToText(info.Address)
	if err != nil {
		return err
	}

	fmt.Println(addr)

	return nil
}

func keyListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list keys",
		RunE:  withContext(requireKeyManager(doKeyListCommand)),
	}
}

func doKeyListCommand(ctx Context, cmd *cobra.Command, args []string) error {
	kmgr, _ := ctx.KeyManager()

	infos, err := kmgr.List()
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 0, '\t', 0)
	for _, info := range infos {
		addr, err := data.ToText(info.Address)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%v\t%v\n", info.Name, addr)
	}
	tw.Flush()
	return nil
}

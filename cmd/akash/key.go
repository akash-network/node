package main

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/ovrclk/akash/cmd/akash/constants"
	"github.com/ovrclk/akash/cmd/akash/context"
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
	return cmd
}

func keyCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create new key",
		RunE:  context.WithContext(context.RequireRootDir(doKeyCreateCommand)),
	}
	cmd.Flags().StringP(constants.FlagKeyType, "t", "ed25519", "Type of key (ed25519|secp256k1|ledger)")
	return cmd
}

func doKeyCreateCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.New("name argument required")
	}

	kmgr, err := ctx.KeyManager()
	if err != nil {
		return err
	}

	ktype, err := ctx.KeyType()
	if err != nil {
		return err
	}

	info, _, err := kmgr.Create(args[0], constants.Password, ktype)
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
		RunE:  context.WithContext(context.RequireKeyManager(doKeyListCommand)),
	}
}

func doKeyListCommand(ctx context.Context, cmd *cobra.Command, args []string) error {
	kmgr, _ := ctx.KeyManager()

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

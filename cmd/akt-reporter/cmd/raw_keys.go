package cmd

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/spf13/cobra"

	"pkg.akt.dev/node/v2/operator/reporter/signer"
)

func newRawKeysCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "create --out /absolute/key-file",
		Short:             "Create a raw reporter key without overwriting an existing file",
		Args:              cobra.NoArgs,
		PersistentPreRunE: prepareRawKeys,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("out")
			if !filepath.IsAbs(path) {
				return errors.New("key path must be absolute")
			}

			key, err := secp256k1.GeneratePrivateKey()
			if err != nil {
				return errors.New("cannot generate reporter key")
			}

			defer key.Zero()
			private := key.Serialize()
			defer clear(private)
			contents := make([]byte, hex.EncodedLen(len(private))+1)
			defer clear(contents)
			hex.Encode(contents, private)
			contents[len(contents)-1] = '\n'

			pub := &sdksecp.PubKey{Key: key.PubKey().SerializeCompressed()}
			if err := writeRawKey(path, contents); err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), sdk.AccAddress(pub.Address()).String())

			return err
		},
	}

	cmd.Flags().String("out", "", "new raw key file (absolute path)")
	_ = cmd.MarkFlagRequired("out")

	return cmd
}

func newRawKeysAddressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "address --key-file /absolute/key-file",
		Short:             "Print the address of an owner-only raw reporter key file",
		Args:              cobra.NoArgs,
		PersistentPreRunE: prepareRawKeys,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("key-file")
			if !filepath.IsAbs(path) {
				return errors.New("key path must be absolute")
			}

			address, err := signer.RawFileAddress(path)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), address.String())

			return err
		},
	}

	cmd.Flags().String("key-file", "", "raw key file (absolute path)")
	_ = cmd.MarkFlagRequired("key-file")

	return cmd
}

// prepareRawKeys overrides the keys parent's persistent hook: raw file commands
// use the root logger but must not load config or open a Cosmos keyring.
func prepareRawKeys(cmd *cobra.Command, _ []string) error {
	if err := setupLogger(cmd); err != nil {
		return err
	}

	broadcast, _ := cmd.Flags().GetBool("broadcast")
	if broadcast {
		return errors.New("unexpected service flags for keys")
	}

	network, _ := cmd.Flags().GetString("network")
	if network != "mainnet" && network != "testnet" {
		return errors.New("network must be mainnet or testnet")
	}

	return nil
}

// writeRawKey matches packaged exclusive creation, sync and close. A failed
// write or sync leaves the created file in place, just as the TS command does.
func writeRawKey(path string, contents []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create raw key: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	if _, err = file.Write(contents); err != nil {
		return err
	}

	return file.Sync()
}

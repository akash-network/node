package cmd

import (
	"fmt"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"strings"

	"encoding/base64"
	"encoding/hex"
	"github.com/cosmos/cosmos-sdk/crypto"
)

const PlainTextHeader = "PT"

var errInputMissingHeader = errors.New("invalid input: missing header")
var errInputTruncated = errors.New("invalid input: truncated")
var errInputInvalidBase64 = errors.New("invalid input: base64 decode failed")
var errKeyringEmpty = errors.New("keyring is empty, no keys to export")
var errKeyExists = errors.New("at least one key already exists, overwrite must be enabled ")
var errCannotExportKey = errors.New("cannot export key")

const (
	flagOverwrite = "overwrite"
)

func importPacked(cmd *cobra.Command, args []string) error {
	overwriteOk, err := cmd.Flags().GetBool(flagOverwrite)
	if err != nil {
		return err
	}
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	input := cmd.InOrStdin()
	if len(args) == 1 {
		input = strings.NewReader(args[0])
	}

	inputBytes, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	inputStr := string(inputBytes)
	inputParts := strings.Split(inputStr, ",")
	if len(inputParts) < 3 {
		return errInputTruncated
	}
	// The first component should always be the header
	if inputParts[0] != PlainTextHeader {
		return errInputMissingHeader
	}

	// Remove the header
	inputParts = inputParts[1:] // Remove the header
	// Check the remaining length is even, since it is pairs
	if len(inputParts)%2 != 0 {
		return errInputTruncated
	}

	// Decode each key
	packedKeys := make(map[string][]byte)
	for i := 0; i != len(inputParts); i += 2 {
		data, err := base64.RawURLEncoding.DecodeString(inputParts[i+1])
		if err != nil {
			return errInputInvalidBase64
		}
		packedKeys[inputParts[i]] = data
	}

	unsafeKeyring := keyring.NewUnsafe(clientCtx.Keyring)
	existingKeys, err := unsafeKeyring.List()
	if err != nil {
		return err
	}
	existingKeyNames := make(map[string]struct{})
	for _, existingKey := range existingKeys {
		existingKeyNames[existingKey.GetName()] = struct{}{}
	}
	for keyName := range packedKeys {
		// Check if each key being imported already exists
		_, exists := existingKeyNames[keyName]
		if exists {
			if overwriteOk {
				// Delete the key if configured to overwrite
				err = unsafeKeyring.Delete(keyName)
				if err != nil {
					return err
				}
			} else {
				// Return an error since overwriting is not allowed
				return errKeyExists
			}
		}
	}

	const notPassword = "secret"
	for keyName, keyBytes := range packedKeys {
		// Create a key, armor it, then import it
		k := &secp256k1.PrivKey{
			Key: keyBytes,
		}
		armored := crypto.EncryptArmorPrivKey(k, notPassword, string(hd.Secp256k1Type))
		err = unsafeKeyring.ImportPrivKey(keyName, armored, notPassword)
		if err != nil {
			return err
		}
	}
	return nil
}

func exportPacked(cmd *cobra.Command, args []string) error {
	clientCtx, err := client.GetClientQueryContext(cmd)
	if err != nil {
		return err
	}

	keyNames := []string{}
	if len(args) == 0 {
		allKeys, err := clientCtx.Keyring.List()
		if err != nil {
			return err
		}
		for _, key := range allKeys {
			keyNames = append(keyNames, key.GetName())
		}
		if len(keyNames) == 0 {
			return errKeyringEmpty
		}
	} else {
		keyNames = args
	}

	unsafeKeyring := keyring.NewUnsafe(clientCtx.Keyring)
	cmd.Printf("%s,", PlainTextHeader)

	for i, keyName := range keyNames {
		hexPrivKey, err := unsafeKeyring.UnsafeExportPrivKeyHex(keyName)
		if err != nil {
			if errors.Is(err, sdkerrors.ErrKeyNotFound) {
				return fmt.Errorf("%w: no key with name %q", errCannotExportKey, keyName)
			}
			return err
		}
		privKey, err := hex.DecodeString(hexPrivKey)
		if err != nil {
			return err
		}
		privKeyB64 := base64.RawURLEncoding.EncodeToString(privKey)
		cmd.Printf("%s,%s", keyName, privKeyB64)
		if i != len(keyNames)-1 {
			cmd.Print(",")
		}
	}

	return nil
}

func exportPackedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "export-packed",
		Short:         "export-packed",
		Args:          cobra.MinimumNArgs(0),
		RunE:          exportPacked,
		Hidden:        true,
		SilenceErrors: true,
	}

	return cmd
}

func importPackedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                "import-packed",
		Short:              "import-packed",
		Args:               cobra.MaximumNArgs(1),
		RunE:               importPacked,
		FParseErrWhitelist: cobra.FParseErrWhitelist{},
		CompletionOptions:  cobra.CompletionOptions{},
		Hidden:             true,
		SilenceErrors:      true,
	}

	cmd.Flags().Bool(flagOverwrite, false, "overwrite existing keys")

	return cmd
}

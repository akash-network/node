package cmd

import (
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"strings"

	"encoding/base64"
	"encoding/hex"
	"github.com/cosmos/cosmos-sdk/crypto"
)

const PLAIN_TEXT_HEADER = "PT"

var errInputMissingHeader = errors.New("invalid input: missing header")
var errInputTruncated = errors.New("invalid input: truncated")
var errInputInvalidBase64 = errors.New("invalid input: base64 decode failed")
var errKeyringEmpty = errors.New("keyring is empty, no keys to export")
var errKeyExists = errors.New("at least one key already exists, overwrite must be enabled ")

const (
	flagOverwrite = "overwrite"
)

func importPacked(cmd *cobra.Command, args []string) error {
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
	if inputParts[0] != PLAIN_TEXT_HEADER {
		return errInputMissingHeader
	}

	inputParts = inputParts[1:]
	if len(inputParts)%2 != 0 {
		return errInputTruncated
	}
	packedKeys := make(map[string][]byte)
	for i := 0; i != len(inputParts); i += 2 {
		data, err := base64.RawURLEncoding.DecodeString(inputParts[i+1])
		if err != nil {
			return errInputInvalidBase64
		}
		packedKeys[inputParts[i]] = data
	}

	unsafeKeyring := keyring.NewUnsafe(clientCtx.Keyring)
	overwriteOk, err := cmd.Flags().GetBool(flagOverwrite)
	if err != nil {
		return err
	}

	existingKeys, err := unsafeKeyring.List()
	if err != nil {
		return err
	}

	existingKeyNames := make(map[string]struct{})
	for _, existingKey := range existingKeys {
		existingKeyNames[existingKey.GetName()] = struct{}{}
	}
	for keyName := range packedKeys {
		_, exists := existingKeyNames[keyName]
		if exists {
			if overwriteOk {
				err = unsafeKeyring.Delete(keyName)
				if err != nil {
					return err
				}
			} else {
				return errKeyExists
			}
		}
	}

	const notPassword = "secret"
	for keyName, keyBytes := range packedKeys {
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
	cmd.Printf("%s,", PLAIN_TEXT_HEADER)

	for i, keyName := range keyNames {
		hexPrivKey, err := unsafeKeyring.UnsafeExportPrivKeyHex(keyName)
		if err != nil {
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
		Use:                        "export-packed",
		Aliases:                    nil,
		SuggestFor:                 nil,
		Short:                      "export-packed",
		Long:                       "",
		Example:                    "",
		ValidArgs:                  nil,
		ValidArgsFunction:          nil,
		Args:                       nil,
		ArgAliases:                 nil,
		BashCompletionFunction:     "",
		Deprecated:                 "",
		Annotations:                nil,
		Version:                    "",
		PersistentPreRun:           nil,
		PersistentPreRunE:          nil,
		PreRun:                     nil,
		PreRunE:                    nil,
		Run:                        nil,
		RunE:                       exportPacked,
		PostRun:                    nil,
		PostRunE:                   nil,
		PersistentPostRun:          nil,
		PersistentPostRunE:         nil,
		FParseErrWhitelist:         cobra.FParseErrWhitelist{},
		CompletionOptions:          cobra.CompletionOptions{},
		TraverseChildren:           false,
		Hidden:                     false,
		SilenceErrors:              false,
		SilenceUsage:               false,
		DisableFlagParsing:         false,
		DisableAutoGenTag:          false,
		DisableFlagsInUseLine:      false,
		DisableSuggestions:         false,
		SuggestionsMinimumDistance: 0,
	}

	return cmd
}

func importPackedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "import-packed",
		Aliases:                    nil,
		SuggestFor:                 nil,
		Short:                      "import-packed",
		Long:                       "",
		Example:                    "",
		ValidArgs:                  nil,
		ValidArgsFunction:          nil,
		Args:                       nil,
		ArgAliases:                 nil,
		BashCompletionFunction:     "",
		Deprecated:                 "",
		Annotations:                nil,
		Version:                    "",
		PersistentPreRun:           nil,
		PersistentPreRunE:          nil,
		PreRun:                     nil,
		PreRunE:                    nil,
		Run:                        nil,
		RunE:                       importPacked,
		PostRun:                    nil,
		PostRunE:                   nil,
		PersistentPostRun:          nil,
		PersistentPostRunE:         nil,
		FParseErrWhitelist:         cobra.FParseErrWhitelist{},
		CompletionOptions:          cobra.CompletionOptions{},
		TraverseChildren:           false,
		Hidden:                     false,
		SilenceErrors:              false,
		SilenceUsage:               false,
		DisableFlagParsing:         false,
		DisableAutoGenTag:          false,
		DisableFlagsInUseLine:      false,
		DisableSuggestions:         false,
		SuggestionsMinimumDistance: 0,
	}

	cmd.Flags().Bool(flagOverwrite, false, "overwrite existing keys")

	return cmd
}

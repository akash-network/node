package cli

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	cinpuit "github.com/cosmos/cosmos-sdk/client/input"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/x/genutil"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ovrclk/akash/x/cert/types"
	cutils "github.com/ovrclk/akash/x/cert/utils"
)

const (
	flagNbf    = "nbf"
	flagNaf    = "naf"
	flagRie    = "rie"
	flagSerial = "serial"
)

var AuthVersionOID = asn1.ObjectIdentifier{2, 23, 133, 2, 6}

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Certificates transaction subcommands",
		SuggestionsMinimumDistance: 2,
		DisableFlagParsing:         true,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		cmdCreate(),
		cmdRevoke(),
	)

	return cmd
}

func cmdCreate() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        "create",
		Short:                      "create/update api certificates",
		SuggestionsMinimumDistance: 2,
		DisableFlagParsing:         true,
		RunE:                       sdkclient.ValidateCmd,
	}

	cmd.AddCommand(
		cmdCreateClient(),
		cmdCreateServer(),
	)

	return cmd
}

func cmdRevoke() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "revoke",
		Short:        "revoke api certificate",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cctx, err := sdkclient.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			serial := cmd.Flag(flagSerial).Value.String()

			if serial != "" {
				if _, valid := new(big.Int).SetString(serial, 10); !valid {
					return errInvalidSerialFlag
				}
			} else {
				cpem, err := cutils.LoadPEMForAccount(cctx, cctx.Keyring)
				if err != nil {
					return err
				}

				blk, _ := pem.Decode(cpem.Cert)
				cert, err := x509.ParseCertificate(blk.Bytes)
				if err != nil {
					return err
				}

				serial = cert.SerialNumber.String()
			}

			return doRevoke(cctx, cmd.Flags(), serial)
		},
	}

	cmd.Flags().String(flagSerial, "", "revoke certificate by serial number")
	flags.AddTxFlagsToCmd(cmd)
	if err := cmd.MarkFlagRequired(flags.FlagFrom); err != nil {
		panic(err.Error())
	}

	return cmd
}

func cmdCreateClient() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "client",
		Short:        "create client api certificate",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doCreateCmd(cmd, nil)
		},
	}

	setCreateFlags(cmd)

	return cmd
}

func cmdCreateServer() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "server [space separated dns names and/or IP addresses]",
		Short:        "create server api certificate",
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE:         doCreateCmd,
	}

	setCreateFlags(cmd)

	return cmd
}

func handleCreate(cctx sdkclient.Context, cmd *cobra.Command, pemFile string, domains []string) error {
	toGenesis, _ := cmd.Flags().GetBool("to-genesis")

	msg, err := createAuthPem(cmd, pemFile, domains)
	if err != nil {
		return err
	}

	if !toGenesis {
		return tx.GenerateOrBroadcastTxCLI(cctx, cmd.Flags(), msg)
	}

	return addCertToGenesis(cmd, types.GenesisCertificate{
		Owner: msg.Owner,
		Certificate: types.Certificate{
			State:  types.CertificateValid,
			Cert:   msg.Cert,
			Pubkey: msg.Pubkey,
		},
	})
}

func doCreateCmd(cmd *cobra.Command, domains []string) error {
	revokeIfExists, err := cmd.Flags().GetBool(flagRie)
	if err != nil {
		return err
	}

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	fromAddress := cctx.GetFromAddress()

	pemFile := cctx.HomeDir + "/" + fromAddress.String() + ".pem"

	if _, err = os.Stat(pemFile); os.IsNotExist(err) {
		_ = cctx.PrintString(fmt.Sprintf("no certificate found for address %s. generating new...\n", fromAddress))

		return handleCreate(cctx, cmd, pemFile, domains)
	}

	cpem, err := cutils.LoadPEMForAccount(cctx, cctx.Keyring)
	if err != nil {
		return err
	}

	blk, _ := pem.Decode(cpem.Cert)
	x509cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return err
	}

	// if revoke-if-exists flag is true query is performed automatically
	// then certificate is being revoked (if valid) and file is removed without any prompts
	yes := revokeIfExists
	if !yes {
		yes, err = getConfirmation(cmd, fmt.Sprintf("certificate file for address %q already exist. check it on-chain status?", fromAddress))
		if err != nil {
			return err
		}
	}

	if yes {
		params := &types.QueryCertificatesRequest{
			Filter: types.CertificateFilter{
				Owner:  cctx.FromAddress.String(),
				Serial: x509cert.SerialNumber.String(),
			},
		}

		res, err := types.NewQueryClient(cctx).Certificates(cmd.Context(), params)
		if err != nil {
			return err
		}

		removeFile := revokeIfExists

		if len(res.Certificates) == 0 {
			if !revokeIfExists {
				yes, err = getConfirmation(cmd, "this certificate has not been found on chain. would you like to commit it?")
				if err != nil {
					return err
				}

				if !yes {
					yes, err = getConfirmation(cmd, "would you like to remove the file?")
					if err != nil {
						return err
					}

					removeFile = yes
				} else {
					cpem, err := cutils.LoadPEMForAccount(cctx, cctx.Keyring)
					if err != nil {
						return err
					}

					msg := &types.MsgCreateCertificate{
						Owner:  fromAddress.String(),
						Cert:   cpem.Cert,
						Pubkey: cpem.Pub,
					}

					if err = msg.ValidateBasic(); err != nil {
						return err
					}

					return tx.GenerateOrBroadcastTxCLI(cctx, cmd.Flags(), msg)
				}
			}
		} else {
			if res.Certificates[0].Certificate.IsState(types.CertificateValid) {
				err = doRevoke(cctx, cmd.Flags(), x509cert.SerialNumber.String())
				if err == nil {
					removeFile = true
				}
			}
		}

		if removeFile {
			if err = os.Remove(pemFile); err != nil {
				return err
			}

			_ = cctx.PrintString("generating new...\n")
			return handleCreate(cctx, cmd, pemFile, domains)
		}
	}

	return nil
}

func setCreateFlags(cmd *cobra.Command) {
	flags.AddTxFlagsToCmd(cmd)
	if err := cmd.MarkFlagRequired(flags.FlagFrom); err != nil {
		panic(err.Error())
	}

	cmd.Flags().String(flagNbf, "", "certificate is not valid before this date. default current timestamp. RFC3339")
	cmd.Flags().String(flagNaf, "", "certificate is not valid after this date. default 365d. days or RFC3339")
	cmd.Flags().Bool(flagRie, false, "revoke current certificate if it already present on chain")

	// fixme shall we use gentx instead? 🤔
	cmd.Flags().Bool("to-genesis", false, "export certificate to genesis")
}

func addCertToGenesis(cmd *cobra.Command, cert types.GenesisCertificate) error {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}

	cdc := cctx.JSONMarshaler.(codec.Marshaler)

	serverCtx := server.GetServerContextFromCmd(cmd)
	config := serverCtx.Config

	config.SetRoot(cctx.HomeDir)

	if err := cert.Validate(); err != nil {
		return errors.Errorf("failed to validate new genesis certificate: %v", err)
	}

	genFile := config.GenesisFile()
	appState, genDoc, err := genutiltypes.GenesisStateFromGenFile(genFile)
	if err != nil {
		return errors.Errorf("failed to unmarshal genesis state: %v", err)
	}

	certsGenState := types.GetGenesisStateFromAppState(cdc, appState)

	if certsGenState.Certificates.Contains(cert) {
		return errors.Errorf("cannot add already existing certificate")
	}
	certsGenState.Certificates = append(certsGenState.Certificates, cert)

	certsGenStateBz, err := cdc.MarshalJSON(certsGenState)
	if err != nil {
		return errors.Errorf("failed to marshal auth genesis state: %v", err)
	}

	appState[types.ModuleName] = certsGenStateBz

	appStateJSON, err := json.Marshal(appState)
	if err != nil {
		return errors.Errorf("failed to marshal application genesis state: %v", err)
	}

	genDoc.AppState = appStateJSON
	return genutil.ExportGenesisFile(genDoc, genFile)
}

func createAuthPem(cmd *cobra.Command, pemFile string, domains []string) (*types.MsgCreateCertificate, error) {
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return nil, err
	}

	fromAddress := cctx.GetFromAddress()
	// note operation below needs more digging to ensure security. current implementation is more like example
	//      private key we generate has to be password protected
	//      from user prospective remembering/handling yet another password
	//      would be a subject of obliviousness. instead we utilize account's key
	//      to generate signature of it's address and use it as a password to encrypt
	//      private key.
	//      from security prospective this signature must never be exposed to prevent certificate leak.
	//      from other hand user must never obtain signature of it's own addresses in shell
	//      so yet again - to be discussed
	sig, _, err := cctx.Keyring.SignByAddress(fromAddress, fromAddress.Bytes())
	if err != nil {
		return nil, err
	}

	var priv *ecdsa.PrivateKey

	if priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		return nil, err
	}

	nbf := time.Now()
	naf := nbf.Add(time.Hour * 24 * 365)

	if val := cmd.Flag(flagNbf).Value.String(); val != "" {
		nbf, err = time.Parse(time.RFC3339, val)
		if err != nil {
			return nil, err
		}
	}

	if val := cmd.Flag(flagNaf).Value.String(); val != "" {
		if strings.HasSuffix(val, "d") {
			days, err := strconv.ParseUint(strings.TrimSuffix(val, "d"), 10, 32)
			if err != nil {
				return nil, err
			}

			naf = nbf.Add(time.Hour * 24 * time.Duration(days))
		} else {
			naf, err = time.Parse(time.RFC3339, val)
			if err != nil {
				return nil, err
			}
		}
	}

	serialNumber := new(big.Int).SetInt64(time.Now().UTC().UnixNano())

	extKeyUsage := []x509.ExtKeyUsage{
		x509.ExtKeyUsageClientAuth,
	}

	if len(domains) > 0 {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: fromAddress.String(),
			ExtraNames: []pkix.AttributeTypeAndValue{
				{
					Type:  AuthVersionOID,
					Value: "v0.0.1",
				},
			},
		},
		Issuer: pkix.Name{
			CommonName: fromAddress.String(),
		},
		NotBefore:             nbf,
		NotAfter:              naf,
		KeyUsage:              x509.KeyUsageDataEncipherment | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           extKeyUsage,
		BasicConstraintsValid: true,
	}

	var ips []net.IP

	for i := len(domains) - 1; i >= 0; i-- {
		if ip := net.ParseIP(domains[i]); ip != nil {
			ips = append(ips, ip)
			domains = append(domains[:i], domains[i+1:]...)
		}
	}

	if len(domains) != 0 || len(ips) != 0 {
		template.PermittedDNSDomainsCritical = true
		template.PermittedDNSDomains = domains
		template.DNSNames = domains
		template.IPAddresses = ips
	}

	var certDer []byte
	if certDer, err = x509.CreateCertificate(rand.Reader, &template, &template, priv.Public(), priv); err != nil {
		_ = cctx.PrintString(fmt.Sprintf("Failed to create certificate: %v\n", err))
		return nil, err
	}

	var keyDer []byte
	if keyDer, err = x509.MarshalPKCS8PrivateKey(priv); err != nil {
		return nil, err
	}

	var pubKeyDer []byte
	if pubKeyDer, err = x509.MarshalPKIXPublicKey(priv.Public()); err != nil {
		return nil, err
	}

	var blk *pem.Block
	blk, err = x509.EncryptPEMBlock(rand.Reader, types.PemBlkTypeECPrivateKey, keyDer, sig, x509.PEMCipherAES256)
	if err != nil {
		_ = cctx.PrintString(fmt.Sprintf("failed to encrypt key file: %v\n", err))
		return nil, err
	}

	var pemOut *os.File
	if pemOut, err = os.OpenFile(pemFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600); err != nil {
		return nil, err
	}

	defer func() {
		if err = pemOut.Close(); err != nil {
			_ = cctx.PrintString(fmt.Sprintf("failed to close key file: %v\n", err))
		} else {
			_ = os.Chmod(pemFile, 0400)
		}
	}()

	if err = pem.Encode(pemOut, &pem.Block{Type: types.PemBlkTypeCertificate, Bytes: certDer}); err != nil {
		_ = cctx.PrintString(fmt.Sprintf("failed to write certificate to pem file: %v\n", err))
		return nil, err
	}

	if err = pem.Encode(pemOut, blk); err != nil {
		_ = cctx.PrintString(fmt.Sprintf("failed to write key to pem file: %v\n", err))
		return nil, err
	}

	msg := &types.MsgCreateCertificate{
		Owner: fromAddress.String(),
		Cert: pem.EncodeToMemory(&pem.Block{
			Type:  types.PemBlkTypeCertificate,
			Bytes: certDer,
		}),
		Pubkey: pem.EncodeToMemory(&pem.Block{
			Type:  types.PemBlkTypeECPublicKey,
			Bytes: pubKeyDer,
		}),
	}

	if err = msg.ValidateBasic(); err != nil {
		return nil, err
	}

	return msg, nil
}

func doRevoke(cctx sdkclient.Context, flags *pflag.FlagSet, serial string) error {
	msg := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
			Owner:  cctx.FromAddress.String(),
			Serial: serial,
		},
	}

	return tx.GenerateOrBroadcastTxCLI(cctx, flags, msg)
}

func getConfirmation(cmd *cobra.Command, prompt string) (bool, error) {
	return cinpuit.GetConfirmation(prompt, bufio.NewReader(cmd.InOrStdin()), cmd.ErrOrStderr())
}

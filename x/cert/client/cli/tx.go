package cli

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	types "github.com/akash-network/akash-api/go/node/cert/v1beta3"

	"github.com/akash-network/node/client/broadcaster"
	certerrors "github.com/akash-network/node/x/cert/errors"
	"github.com/akash-network/node/x/cert/utils"
)

const (
	flagSerial    = "serial"
	flagOverwrite = "overwrite"
	flagValidTime = "valid-duration"
	flagStart     = "start-time"
	flagToGenesis = "to-genesis"
)

var (
	errCertificateDoesNotExist    = fmt.Errorf("%w: does not exist", certerrors.ErrCertificate)
	errCannotOverwriteCertificate = fmt.Errorf("%w: cannot overwrite certificate", certerrors.ErrCertificate)
)

func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Certificates transaction subcommands",
		SuggestionsMinimumDistance: 2,
		RunE:                       sdkclient.ValidateCmd,
	}

	/**
	Commands
	1. Generate - create public / private key pair
	2. Publish - publish a key pair to the blockchain
	3. Revoke - revoke a key pair on the blockchain

	*/

	cmd.AddCommand(
		cmdGenerate(),
		cmdPublish(),
		cmdRevoke(),
	)

	return cmd
}

func doGenerateCmd(cmd *cobra.Command, domains []string) error {
	allowOverwrite := viper.GetBool(flagOverwrite)

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}
	fromAddress := cctx.GetFromAddress()

	kpm, err := utils.NewKeyPairManager(cctx, fromAddress)
	if err != nil {
		return err
	}

	exists, err := kpm.KeyExists()
	if err != nil {
		return err
	}
	if !allowOverwrite && exists {
		return errCannotOverwriteCertificate
	}

	var startTime time.Time
	startTimeStr := viper.GetString(flagStart)
	if len(startTimeStr) == 0 {
		startTime = time.Now().Truncate(time.Second)
	} else {
		startTime, err = time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return err
		}
	}
	validDuration := viper.GetDuration(flagValidTime)

	return kpm.Generate(startTime, startTime.Add(validDuration), domains)
}

func doPublishCmd(cmd *cobra.Command) error {
	toGenesis := viper.GetBool(flagToGenesis)

	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}
	fromAddress := cctx.GetFromAddress()

	kpm, err := utils.NewKeyPairManager(cctx, fromAddress)
	if err != nil {
		return err
	}

	exists, err := kpm.KeyExists()
	if err != nil {
		return err
	}
	if !exists {
		return errCertificateDoesNotExist
	}

	cert, _, pubKey, err := kpm.Read()
	if err != nil {
		return err
	}

	msg := &types.MsgCreateCertificate{
		Owner: fromAddress.String(),
		Cert: pem.EncodeToMemory(&pem.Block{
			Type:  types.PemBlkTypeCertificate,
			Bytes: cert,
		}),
		Pubkey: pem.EncodeToMemory(&pem.Block{
			Type:  types.PemBlkTypeECPublicKey,
			Bytes: pubKey,
		}),
	}

	if err = msg.ValidateBasic(); err != nil {
		return err
	}

	if toGenesis {
		return addCertToGenesis(cmd, types.GenesisCertificate{
			Owner: msg.Owner,
			Certificate: types.Certificate{
				State:  types.CertificateValid,
				Cert:   msg.Cert,
				Pubkey: msg.Pubkey,
			},
		})

	}

	return broadcaster.BroadcastTX(cmd.Context(), cctx, cmd.Flags(), msg)
}

func doRevokeCmd(cmd *cobra.Command) error {
	serial := viper.GetString(flagSerial)
	cctx, err := sdkclient.GetClientTxContext(cmd)
	if err != nil {
		return err
	}
	fromAddress := cctx.GetFromAddress()

	if len(serial) != 0 {
		if _, valid := new(big.Int).SetString(serial, 10); !valid {
			return errInvalidSerialFlag
		}
	} else {
		kpm, err := utils.NewKeyPairManager(cctx, fromAddress)
		if err != nil {
			return err
		}

		cert, _, _, err := kpm.Read()
		if err != nil {
			return err
		}

		parsedCert, err := x509.ParseCertificate(cert)
		if err != nil {
			return err
		}

		serial = parsedCert.SerialNumber.String()
	}

	params := &types.QueryCertificatesRequest{
		Filter: types.CertificateFilter{
			Owner:  fromAddress.String(),
			Serial: serial,
			State:  stateValid,
		},
	}

	queryClient := types.NewQueryClient(cctx)

	res, err := queryClient.Certificates(cmd.Context(), params)
	if err != nil {
		return err
	}

	exists := len(res.Certificates) != 0
	if !exists {
		return fmt.Errorf("%w: certificate with serial %v does not exist on chain and cannot be revoked", certerrors.ErrCertificate, serial)
	}

	msg := &types.MsgRevokeCertificate{
		ID: types.CertificateID{
			Owner:  cctx.FromAddress.String(),
			Serial: serial,
		},
	}

	return broadcaster.BroadcastTX(cmd.Context(), cctx, cmd.Flags(), msg)
}

package utils

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	ctypes "github.com/ovrclk/akash/x/cert/types"
)

func NewServerTLSConfig(ctx context.Context, certs []tls.Certificate, cquery ctypes.QueryClient) (*tls.Config, error) {
	// InsecureSkipVerify is set to true due to inability to use normal TLS verification
	// certificate validation and authentication performed in VerifyPeerCertificate
	cfg := &tls.Config{
		Certificates:       certs,
		ClientAuth:         tls.RequestClientCert,
		InsecureSkipVerify: true, // nolint: gosec
		MinVersion:         tls.VersionTLS13,
		VerifyPeerCertificate: func(certificates [][]byte, _ [][]*x509.Certificate) error {
			if len(certificates) > 0 {
				if len(certificates) != 1 {
					return errors.Errorf("invalid certificate chain")
				}

				cert, err := x509.ParseCertificate(certificates[0])
				if err != nil {
					return errors.Wrap(err, "failed to parse certificate")
				}

				// validation
				// 1. CommonName in issuer and Subject must match and be as Bech32 format
				if cert.Subject.CommonName != cert.Issuer.CommonName {
					return errors.Wrap(err, "invalid certificate")
				}

				var owner sdk.Address
				if owner, err = sdk.AccAddressFromBech32(cert.Subject.CommonName); err != nil {
					return errors.Wrap(err, "invalid certificate")
				}

				// 2. serial number must be in
				if cert.SerialNumber == nil {
					return errors.Wrap(err, "invalid certificate")
				}

				// 3. look up certificate on chain
				var resp *ctypes.QueryCertificatesResponse
				resp, err = cquery.Certificates(
					ctx,
					&ctypes.QueryCertificatesRequest{
						Filter: ctypes.CertificateFilter{
							Owner:  owner.String(),
							Serial: cert.SerialNumber.String(),
							State:  "valid",
						},
					},
				)
				if err != nil {
					return err
				}

				clientCertPool := x509.NewCertPool()

				if !clientCertPool.AppendCertsFromPEM(resp.Certificates[0].Cert) {
					return errors.Wrap(err, "invalid certificate")
				}

				opts := x509.VerifyOptions{
					Roots:                     clientCertPool,
					CurrentTime:               time.Now(),
					KeyUsages:                 []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
					MaxConstraintComparisions: 0,
				}

				if _, err = cert.Verify(opts); err != nil {
					return errors.Wrap(err, "invalid certificate")
				}
			}
			return nil
		},
	}

	return cfg, nil
}

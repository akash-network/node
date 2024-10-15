package utils

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"time"

	certerrors "pkg.akt.dev/node/x/cert/errors"

	"github.com/cosmos/cosmos-sdk/client"

	ctypes "pkg.akt.dev/go/node/cert/v1"
)

// LoadAndQueryCertificateForAccount wraps LoadAndQueryPEMForAccount and tls.X509KeyPair
func LoadAndQueryCertificateForAccount(ctx context.Context, cctx client.Context, fin io.Reader) (tls.Certificate, error) {
	kpm, err := NewKeyPairManager(cctx, cctx.FromAddress)
	if err != nil {
		return tls.Certificate{}, err
	}

	x509cert, tlsCert, err := kpm.ReadX509KeyPair(fin)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Check if valid according to time
	if x509cert.NotBefore.After(time.Now().UTC()) {
		return tls.Certificate{}, fmt.Errorf("%w: certificate is not yet active, start ts %s", certerrors.ErrCertificate, x509cert.NotBefore)
	}

	if time.Now().UTC().After(x509cert.NotAfter) {
		return tls.Certificate{}, fmt.Errorf("%w: certificate has been expired since %s", certerrors.ErrCertificate, x509cert.NotAfter)
	}

	params := &ctypes.QueryCertificatesRequest{
		Filter: ctypes.CertificateFilter{
			Owner:  x509cert.Subject.CommonName,
			Serial: x509cert.SerialNumber.String(),
		},
	}

	certs, err := ctypes.NewQueryClient(cctx).Certificates(ctx, params)
	if err != nil {
		return tls.Certificate{}, err
	}

	if len(certs.Certificates) == 0 {
		return tls.Certificate{}, fmt.Errorf("%w: certificate has not been committed to blockchain", certerrors.ErrCertificate)
	}

	foundCert := certs.Certificates[0]
	if foundCert.GetCertificate().State != ctypes.CertificateValid {
		return tls.Certificate{}, fmt.Errorf("%w: certificate is not valid", certerrors.ErrCertificate)
	}

	return tlsCert, nil
}

//nolint: revive

package utils

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"go.step.sm/crypto/pemutil"

	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"

	types "pkg.akt.dev/go/node/cert/v1"

	certerrors "pkg.akt.dev/node/x/cert/errors"
)

var (
	errCertificateNotFoundInPEM = fmt.Errorf("%w: certificate not found in PEM", certerrors.ErrCertificate)
	errPrivateKeyNotFoundInPEM  = fmt.Errorf("%w: private key not found in PEM", certerrors.ErrCertificate)
	errPublicKeyNotFoundInPEM   = fmt.Errorf("%w: public key not found in PEM", certerrors.ErrCertificate)
	errUnsupportedEncryptedPEM  = errors.New("unsupported encrypted PEM")
)

type KeyPairManager interface {
	KeyExists() (bool, error)
	Generate(notBefore, notAfter time.Time, domains []string) error

	// Read the PEM blocks, containing the cert, private key, & public key
	Read(fin ...io.Reader) ([]byte, []byte, []byte, error)

	ReadX509KeyPair(fin ...io.Reader) (*x509.Certificate, tls.Certificate, error)
}

type keyPairManager struct {
	addr           sdk.AccAddress
	passwordBytes  []byte
	passwordLegacy []byte
	homeDir        string
}

func NewKeyPairManager(cctx sdkclient.Context, fromAddress sdk.AccAddress) (KeyPairManager, error) {
	sig, _, err := cctx.Keyring.SignByAddress(fromAddress, []byte(fromAddress.String()), signing.SignMode_SIGN_MODE_DIRECT)
	if err != nil {
		return nil, err
	}

	// ignore error if ledger device is being used
	// due to its jsonparser not liking bech address sent as data in binary format
	// if test or file keyring used it will allow to decode old private keys for the mTLS cert
	sigLegacy, _, _ := cctx.Keyring.SignByAddress(fromAddress, fromAddress.Bytes(), signing.SignMode_SIGN_MODE_DIRECT)

	return &keyPairManager{
		addr:           fromAddress,
		passwordBytes:  sig,
		passwordLegacy: sigLegacy,
		homeDir:        cctx.HomeDir,
	}, nil
}

func (kpm *keyPairManager) getKeyPath() string {
	return kpm.homeDir + "/" + kpm.addr.String() + ".pem"
}

func (kpm *keyPairManager) ReadX509KeyPair(fin ...io.Reader) (*x509.Certificate, tls.Certificate, error) {
	certData, privKeyData, _, err := kpm.Read(fin...)
	if err != nil {
		return nil, tls.Certificate{}, err
	}

	x509cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("could not parse x509 cert: %w", err)
	}

	result := tls.Certificate{
		Certificate: [][]byte{certData},
	}

	result.PrivateKey, err = x509.ParsePKCS8PrivateKey(privKeyData)
	if err != nil {
		return nil, tls.Certificate{}, fmt.Errorf("%w: failed parsing private key data", err)
	}

	return x509cert, result, err
}

func (kpm *keyPairManager) KeyExists() (bool, error) {
	_, err := os.Stat(kpm.getKeyPath())
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func (kpm *keyPairManager) Generate(notBefore, notAfter time.Time, domains []string) error {
	var err error
	var pemOut *os.File
	if pemOut, err = os.OpenFile(kpm.getKeyPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600); err != nil {
		return err
	}

	err = kpm.generateImpl(notBefore, notAfter, domains, pemOut)

	closeErr := pemOut.Close()
	if closeErr != nil {
		return closeErr
	}

	return err
}

func (kpm *keyPairManager) generateImpl(notBefore, notAfter time.Time, domains []string, fout io.Writer) error {
	var err error
	// Generate the private key
	var priv *ecdsa.PrivateKey
	if priv, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader); err != nil {
		return fmt.Errorf("could not generate key: %w", err)
	}

	serialNumber := new(big.Int).SetInt64(time.Now().UTC().UnixNano())

	extKeyUsage := []x509.ExtKeyUsage{
		x509.ExtKeyUsageClientAuth,
	}

	if len(domains) != 0 {
		extKeyUsage = append(extKeyUsage, x509.ExtKeyUsageServerAuth)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: kpm.addr.String(),
			ExtraNames: []pkix.AttributeTypeAndValue{
				{
					Type:  AuthVersionOID,
					Value: "v0.0.1",
				},
			},
		},
		Issuer: pkix.Name{
			CommonName: kpm.addr.String(),
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
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
		return fmt.Errorf("could not create certificate: %w", err)
	}

	var keyDer []byte
	if keyDer, err = x509.MarshalPKCS8PrivateKey(priv); err != nil {
		return fmt.Errorf("could not create private key: %w", err)
	}

	var blk *pem.Block
	blk, err = pemutil.EncryptPKCS8PrivateKey(rand.Reader, keyDer, kpm.passwordBytes, x509.PEMCipherAES256)
	if err != nil {
		return fmt.Errorf("could not encrypt private key as PEM: %w", err)
	}

	// Write the certificate
	if err = pem.Encode(fout, &pem.Block{Type: types.PemBlkTypeCertificate, Bytes: certDer}); err != nil {
		return fmt.Errorf("could not encode certificate as PEM: %w", err)
	}

	// Write the encrypted private key
	if err = pem.Encode(fout, blk); err != nil {
		return fmt.Errorf("could not encode private key as PEM: %w", err)
	}

	return nil
}

func (kpm *keyPairManager) Read(fin ...io.Reader) ([]byte, []byte, []byte, error) {
	var pemIn io.Reader
	var closeMe io.ReadCloser

	if len(fin) != 0 {
		if len(fin) != 1 {
			return nil, nil, nil, fmt.Errorf("%w: Read() takes exactly 1 or 0 arguments, not %d", certerrors.ErrCertificate, len(fin))
		}
		pemIn = fin[0]
	}

	if pemIn == nil {
		fopen, err := os.OpenFile(kpm.getKeyPath(), os.O_RDONLY, 0x0)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("could not open certificate PEM file: %w", err)
		}
		closeMe = fopen
		pemIn = fopen
	}

	cert, privKey, pubKey, err := kpm.readImpl(pemIn)

	if closeMe != nil {
		closeErr := closeMe.Close()
		if closeErr != nil {
			return nil, nil, nil, fmt.Errorf("could not close PEM file: %w", closeErr)
		}
	}

	return cert, privKey, pubKey, err
}

func (kpm *keyPairManager) readImpl(fin io.Reader) ([]byte, []byte, []byte, error) {
	buf := &bytes.Buffer{}
	_, err := io.Copy(buf, fin)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed reading certificate PEM file: %w", err)
	}
	data := buf.Bytes()

	// Read certificate
	block, remaining := pem.Decode(data)
	if block == nil {
		return nil, nil, nil, errCertificateNotFoundInPEM
	}
	cert := block.Bytes

	// Read private key
	block, _ = pem.Decode(remaining)
	if block == nil {
		return nil, nil, nil, errPrivateKeyNotFoundInPEM
	}

	var privKeyPlaintext []byte
	var privKeyI interface{}

	// PKCS#8 header defined in RFC7468 section 11
	// nolint: gocritic
	if block.Type == "ENCRYPTED PRIVATE KEY" {
		privKeyPlaintext, err = pemutil.DecryptPKCS8PrivateKey(block.Bytes, kpm.passwordBytes)
	} else if block.Headers["Proc-Type"] == "4,ENCRYPTED" {
		// nolint: staticcheck
		privKeyPlaintext, _ = x509.DecryptPEMBlock(block, kpm.passwordBytes)

		// DecryptPEMBlock may not return IncorrectPasswordError.
		// Try parse private key instead and if it fails give another try with legacy password
		privKeyI, err = x509.ParsePKCS8PrivateKey(privKeyPlaintext)
		if err != nil {
			// nolint: staticcheck
			privKeyPlaintext, err = x509.DecryptPEMBlock(block, kpm.passwordLegacy)
		}
	} else {
		return nil, nil, nil, errUnsupportedEncryptedPEM
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: failed decrypting x509 block with private key", err)
	}

	if privKeyI == nil {
		if privKeyI, err = x509.ParsePKCS8PrivateKey(privKeyPlaintext); err != nil {
			return nil, nil, nil, fmt.Errorf("%w: failed parsing private key data", err)
		}
	}

	eckey, valid := privKeyI.(*ecdsa.PrivateKey)
	if !valid {
		return nil, nil, nil, fmt.Errorf("%w: unexpected private key type, expected %T but got %T",
			errPublicKeyNotFoundInPEM,
			&ecdsa.PrivateKey{},
			privKeyI)
	}

	var pubKey []byte
	if pubKey, err = x509.MarshalPKIXPublicKey(eckey.Public()); err != nil {
		return nil, nil, nil, fmt.Errorf("%w: failed extracting public key", err)
	}

	return cert, privKeyPlaintext, pubKey, nil
}

package cli

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	sdkclient "github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	types "github.com/ovrclk/akash/x/cert/types/v1beta2"
	"io"
	"math/big"
	"net"
	"os"
	"time"
)

type keyPairManager struct {
	addr sdk.AccAddress
	passwordBytes []byte
	homeDir string
}

func newKeyPairManager(cctx sdkclient.Context, fromAddress sdk.AccAddress) (*keyPairManager, error) {
	sig, _, err := cctx.Keyring.SignByAddress(fromAddress, fromAddress.Bytes())
	if err != nil {
		return nil, err
	}

	return &keyPairManager{
		addr:          fromAddress,
		passwordBytes: sig,
		homeDir: cctx.HomeDir,
	}, nil
}

func (kpm *keyPairManager) getKeyPath() string {
	return kpm.homeDir + "/" + kpm.addr.String() + ".pem"
}

func (kpm *keyPairManager) keyExists() (bool, error) {
	_, err := os.Stat(kpm.getKeyPath())
	if err == nil {
		return true, nil
	}

	if os.IsNotExist(err) {
		return false, nil
	}

	return false, err
}

func (kpm *keyPairManager) generate(notBefore, notAfter time.Time, domains []string) error {
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
		return err
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
		return err
	}

	var keyDer []byte
	if keyDer, err = x509.MarshalPKCS8PrivateKey(priv); err != nil {
		return err
	}

	var pubKeyDer []byte
	if pubKeyDer, err = x509.MarshalPKIXPublicKey(priv.Public()); err != nil {
		return err
	}

	var blk *pem.Block
	// fixme #1182
	blk, err = x509.EncryptPEMBlock(rand.Reader, types.PemBlkTypeECPrivateKey, keyDer, kpm.passwordBytes, x509.PEMCipherAES256) // nolint: staticcheck
	if err != nil {
		return err
	}

	// Write the certificate
	if err = pem.Encode(fout, &pem.Block{Type: types.PemBlkTypeCertificate, Bytes: certDer}); err != nil {
		return err
	}

	// Write the encrypted private key
	if err = pem.Encode(fout, blk); err != nil {
		return err
	}

	// Write the public key
	if err = pem.Encode(fout, &pem.Block{
		Type:    types.PemBlkTypeECPublicKey,
		Bytes:   pubKeyDer,
	}); err != nil {
		return err
	}

	return nil
}

func (kpm *keyPairManager) read() ([]byte, []byte, []byte, error) {
	pemIn, err := os.OpenFile(kpm.getKeyPath(), os.O_RDONLY, 0x0)
	if err != nil {
		return nil,nil,nil, err
	}

	cert, privKey, pubKey, err := kpm.readImpl(pemIn)

	closeErr := pemIn.Close()
	if closeErr != nil {
		return nil, nil, nil, closeErr
	}

	return cert, privKey, pubKey, err
}


func (kpm *keyPairManager) readImpl(fin io.Reader) ([]byte, []byte, []byte, error) {
	buf := &bytes.Buffer{}
	_, err := io.Copy(buf, fin)
	if err != nil {
		return nil,nil,nil, err
	}
	data := buf.Bytes()

	// Read certificate
	block, remaining := pem.Decode(data)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("certificate key not found in PEM")
	}
	cert := block.Bytes

	// Read private key
	block, remaining = pem.Decode(remaining)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("private key not found in PEM")
	}
	privKey := block.Bytes

	// Read public key
	block, remaining = pem.Decode(remaining)
	if block == nil {
		return nil, nil, nil, fmt.Errorf("public key not found in PEM")
	}
	pubKey := block.Bytes

	return cert, privKey, pubKey, nil
}

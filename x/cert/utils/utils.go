package utils

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/pkg/errors"

	ctypes "github.com/ovrclk/akash/x/cert/types"
)

type PEMBlocks struct {
	Cert []byte
	Priv []byte
	Pub  []byte
}

type loaderOption struct {
	rd io.Reader
}

type LoaderOption func(*loaderOption)

func PEMFromReader(rd io.Reader) LoaderOption {
	return func(opt *loaderOption) {
		opt.rd = rd
	}
}

// LoadPEMForAccount load certificate/private key from file named as
// account name supplied in FlagFrom
// file must contain two PEM blocks, certificate followed by a private key
func LoadPEMForAccount(cctx client.Context, keyring keyring.Keyring, opts ...LoaderOption) (PEMBlocks, error) {
	sig, _, err := keyring.SignByAddress(cctx.FromAddress, cctx.FromAddress.Bytes())
	if err != nil {
		return PEMBlocks{}, err
	}

	opt := &loaderOption{}

	for _, o := range opts {
		o(opt)
	}

	var pdata []byte

	if opt.rd == nil {
		pdata, err = ioutil.ReadFile(cctx.HomeDir + "/" + cctx.FromAddress.String() + ".pem")
		if os.IsNotExist(err) {
			pdata, err = ioutil.ReadFile(cctx.HomeDir + "/" + cctx.FromName + ".pem")
		}
	} else {
		pdata, err = ioutil.ReadAll(opt.rd)
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return PEMBlocks{}, err
	}

	var bcrt *pem.Block
	var bkey *pem.Block

	var kdata []byte
	bcrt, kdata = pem.Decode(pdata)
	bkey, _ = pem.Decode(kdata)

	if bcrt == nil {
		return PEMBlocks{}, errors.Errorf("no certificate found")
	}

	if bkey == nil {
		return PEMBlocks{}, errors.Errorf("no private key found")
	}

	pdata = pdata[:len(pdata)-len(kdata)-1]

	var pkey []byte
	if pkey, err = x509.DecryptPEMBlock(bkey, sig); err != nil {
		return PEMBlocks{}, err
	}

	var priv interface{}
	if priv, err = x509.ParsePKCS8PrivateKey(pkey); err != nil {
		return PEMBlocks{}, errors.Wrapf(err, "coudn't parse private key")
	}

	eckey, valid := priv.(*ecdsa.PrivateKey)
	if !valid {
		return PEMBlocks{}, errors.Errorf("unknown key type. expected %s, desired %s",
			reflect.TypeOf(&ecdsa.PrivateKey{}), reflect.TypeOf(eckey))
	}

	var pubKey []byte
	if pubKey, err = x509.MarshalPKIXPublicKey(eckey.Public()); err != nil {
		return PEMBlocks{}, err
	}

	return PEMBlocks{
		Cert: pdata,
		Priv: pem.EncodeToMemory(&pem.Block{Type: ctypes.PemBlkTypeECPrivateKey, Bytes: pkey}),
		Pub:  pubKey,
	}, nil
}

// LoadCertificateForAccount wraps LoadPEMForAccount and tls.X509KeyPair
func LoadCertificateForAccount(cctx client.Context, keyring keyring.Keyring, opts ...LoaderOption) (tls.Certificate, error) {
	pblk, err := LoadPEMForAccount(cctx, keyring, opts...)
	if err != nil {
		return tls.Certificate{}, err
	}

	cert, err := tls.X509KeyPair(pblk.Cert, pblk.Priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return cert, nil
}

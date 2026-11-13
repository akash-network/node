package signer

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/node/v2/operator/reporter/signer/kmspb"
)

// RemoteOptions configures the Cosmos-KMS SignerService connection. The
// connection always uses TLS.
type RemoteOptions struct {
	// Address is the SignerService host:port.
	Address string
	// KeyID names the key on the KMS.
	KeyID string
	// CAFile is a PEM bundle of trusted CAs; the system roots are used when
	// it is empty.
	CAFile string
	// ServerName overrides the name verified in the server certificate.
	ServerName string
	// CertFile and KeyFile are an optional client certificate, both or neither.
	CertFile string
	KeyFile  string
	// Timeout bounds each call.
	Timeout time.Duration
}

type remoteSigner struct {
	conn    *grpc.ClientConn
	client  kmspb.SignerServiceClient
	keyID   string
	scheme  kmspb.SignatureScheme
	timeout time.Duration
	addr    sdk.AccAddress
	pub     cryptotypes.PubKey
}

// rawMessageLimit is the most sign bytes an ECDSA_SECP256K1 key can sign:
// Cosmos-KMS signs them as AWS KMS raw messages, which upstream caps at 4096
// bytes (kmsRawMessageLimit in signing/awskms/signer.go).
const rawMessageLimit = 4096

var (
	_ Signer           = (*remoteSigner)(nil)
	_ SignBytesLimiter = (*remoteSigner)(nil)
	_ io.Closer        = (*remoteSigner)(nil)
)

// NewRemote connects to the SignerService at opts.Address and checks, once,
// that key opts.KeyID is a secp256k1 key that derives reporter. The returned
// Signer also implements io.Closer, which releases the connection.
func NewRemote(ctx context.Context, opts RemoteOptions, reporter sdk.AccAddress) (Signer, error) {
	if opts.Address == "" {
		return nil, errors.New("remote signer address is empty")
	}

	if opts.KeyID == "" {
		return nil, errors.New("remote signer key-id is empty")
	}

	if opts.Timeout <= 0 {
		return nil, errors.New("remote signer timeout must be positive")
	}

	creds, err := remoteCredentials(opts)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(opts.Address, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("remote signer connection: %w", err)
	}

	s := &remoteSigner{
		conn:    conn,
		client:  kmspb.NewSignerServiceClient(conn),
		keyID:   opts.KeyID,
		timeout: opts.Timeout,
	}

	if err := s.loadKey(ctx, reporter); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return s, nil
}

func remoteCredentials(opts RemoteOptions) (credentials.TransportCredentials, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: opts.ServerName}

	if opts.CAFile != "" {
		pem, err := os.ReadFile(opts.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read ca-file: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("ca-file holds no PEM certificate")
		}

		cfg.RootCAs = pool
	}

	if (opts.CertFile == "") != (opts.KeyFile == "") {
		return nil, errors.New("remote signer cert-file and key-file must be set together")
	}

	if opts.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}

		cfg.Certificates = []tls.Certificate{cert}
	}

	return credentials.NewTLS(cfg), nil
}

func (s *remoteSigner) loadKey(ctx context.Context, reporter sdk.AccAddress) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.client.GetKey(ctx, &kmspb.GetKeyRequest{Id: s.keyID})
	if err != nil {
		return fmt.Errorf("remote signer get key %q: %w", s.keyID, err)
	}

	key := resp.GetKey()

	pub, err := parseRemoteKey(key.GetPubkey())
	if err != nil {
		return err
	}

	switch scheme := key.GetScheme(); scheme {
	case kmspb.SignatureScheme_ECDSA_SECP256K1, kmspb.SignatureScheme_ECDSA_SECP256K1ETH:
		s.scheme = scheme
	default:
		return fmt.Errorf("remote signer key %q has unsupported signature scheme %s", s.keyID, scheme)
	}

	addr := sdk.AccAddress(pub.Address())
	if !addr.Equals(reporter) {
		return errMismatch
	}

	s.addr = addr
	s.pub = pub

	return nil
}

// parseRemoteKey accepts only a 33-byte compressed secp256k1 point on the
// curve. The errors carry no key bytes.
func parseRemoteKey(b []byte) (*sdksecp.PubKey, error) {
	if len(b) != secp256k1.PubKeyBytesLenCompressed ||
		(b[0] != secp256k1.PubKeyFormatCompressedEven && b[0] != secp256k1.PubKeyFormatCompressedOdd) {
		return nil, errors.New("remote signer public key must be a 33-byte compressed secp256k1 key")
	}

	if _, err := secp256k1.ParsePubKey(b); err != nil {
		return nil, errors.New("remote signer returned an invalid secp256k1 public key")
	}

	return &sdksecp.PubKey{Key: append([]byte(nil), b...)}, nil
}

func (s *remoteSigner) Address() sdk.AccAddress { return s.addr }

func (s *remoteSigner) PubKey() cryptotypes.PubKey { return s.pub }

// SignBytesLimit is 4096 for ECDSA_SECP256K1 and none (0) for
// ECDSA_SECP256K1ETH, whose payload is always a 32-byte digest.
func (s *remoteSigner) SignBytesLimit() int {
	if s.scheme == kmspb.SignatureScheme_ECDSA_SECP256K1 {
		return rawMessageLimit
	}

	return 0
}

// Sign sends the payload for the key's scheme and returns the verified low-S
// r||s. Errors never carry the payload, the sign bytes or the signature; a
// failed call reports only its gRPC status code, because the server's
// description is not under our control.
func (s *remoteSigner) Sign(ctx context.Context, signBytes []byte) ([]byte, error) {
	if limit := s.SignBytesLimit(); limit > 0 && len(signBytes) > limit {
		return nil, fmt.Errorf("sign bytes exceed the remote signer limit of %d bytes", limit)
	}

	payload := signBytes
	if s.scheme == kmspb.SignatureScheme_ECDSA_SECP256K1ETH {
		digest := sha256.Sum256(signBytes)
		payload = digest[:]
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.client.Sign(ctx, &kmspb.SignRequest{
		KeyId:   s.keyID,
		Payload: &kmspb.Payload{Kind: &kmspb.Payload_Generic{Generic: payload}},
	})
	if err != nil {
		return nil, fmt.Errorf("remote signer sign failed: %s", status.Code(err))
	}

	sig := resp.GetSignature()
	if len(sig) != sigLen && len(sig) != sigLen+1 {
		return nil, fmt.Errorf("remote signature must be 64 or 65 bytes, got %d", len(sig))
	}

	out, err := NormalizeAndVerify(s.pub, signBytes, sig)
	if err != nil {
		return nil, fmt.Errorf("remote signature: %w", err)
	}

	return out, nil
}

// Close releases the connection.
func (s *remoteSigner) Close() error {
	return s.conn.Close()
}

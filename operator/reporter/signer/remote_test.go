package signer

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	decredecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	sdksecp "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.akt.dev/node/v2/operator/reporter/signer/kmspb"
)

const (
	testKeyID      = "reporter"
	testServerName = "kms.test"
	errGetKey      = "get key"
)

// fakeKMS is an in-process SignerService. sign answers a Sign call with the
// raw signature bytes for the payload it received.
type fakeKMS struct {
	kmspb.UnimplementedSignerServiceServer

	key  *kmspb.Key
	sign func(ctx context.Context, payload []byte) ([]byte, error)

	mu       sync.Mutex
	payloads [][]byte
	answers  [][]byte
}

func (f *fakeKMS) seen() (payloads, answers [][]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.payloads, f.answers
}

func (f *fakeKMS) GetKey(_ context.Context, req *kmspb.GetKeyRequest) (*kmspb.GetKeyResponse, error) {
	if req.GetId() != f.key.GetId() {
		return nil, io.ErrUnexpectedEOF
	}

	return &kmspb.GetKeyResponse{Key: f.key}, nil
}

func (f *fakeKMS) Sign(ctx context.Context, req *kmspb.SignRequest) (*kmspb.SignResponse, error) {
	payload := req.GetPayload().GetGeneric()

	sig, err := f.sign(ctx, payload)

	f.mu.Lock()
	f.payloads = append(f.payloads, payload)
	f.answers = append(f.answers, sig)
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}

	return &kmspb.SignResponse{Signature: sig}, nil
}

// testPKI is a CA with a server certificate for 127.0.0.1 and kms.test, and a
// client certificate, all written as PEM files under dir.
type testPKI struct {
	dir        string
	caFile     string
	certFile   string
	keyFile    string
	serverCert tls.Certificate
	pool       *x509.CertPool
}

func newTestPKI(t *testing.T) *testPKI {
	t.Helper()

	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	issue := func(serial int64, usage x509.ExtKeyUsage) ([]byte, *ecdsa.PrivateKey) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: testServerName},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			DNSNames:     []string{testServerName},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		require.NoError(t, err)

		return der, key
	}

	writePEM := func(name, typ string, der []byte) string {
		path := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600))

		return path
	}

	srvDER, srvKey := issue(2, x509.ExtKeyUsageServerAuth)
	cliDER, cliKey := issue(3, x509.ExtKeyUsageClientAuth)
	cliKeyDER, err := x509.MarshalECPrivateKey(cliKey)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	return &testPKI{
		dir:        dir,
		caFile:     writePEM("ca.pem", "CERTIFICATE", caDER),
		certFile:   writePEM("client.pem", "CERTIFICATE", cliDER),
		keyFile:    writePEM("client-key.pem", "EC PRIVATE KEY", cliKeyDER),
		serverCert: tls.Certificate{Certificate: [][]byte{srvDER}, PrivateKey: srvKey},
		pool:       pool,
	}
}

// serve starts f behind TLS and returns its address. requireClient demands a
// client certificate issued by the test CA.
func serve(t *testing.T, pki *testPKI, f *fakeKMS, requireClient bool) string {
	t.Helper()

	cfg := &tls.Config{Certificates: []tls.Certificate{pki.serverCert}, MinVersion: tls.VersionTLS12}
	if requireClient {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = pki.pool
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(cfg)))
	kmspb.RegisterSignerServiceServer(srv, f)

	go func() { _ = srv.Serve(lis) }()

	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

// rsSigner signs SHA-256(payload) and answers r||s, as ECDSA_SECP256K1 does.
func rsSigner(priv *secp256k1.PrivateKey) func(context.Context, []byte) ([]byte, error) {
	return func(_ context.Context, payload []byte) ([]byte, error) {
		h := sha256.Sum256(payload)
		compact := decredecdsa.SignCompact(priv, h[:], true)

		return compact[1:], nil
	}
}

// rsvHighSSigner signs the 32-byte digest it receives and answers r||s||v
// with s in the high half, as ECDSA_SECP256K1ETH may.
func rsvHighSSigner(priv *secp256k1.PrivateKey) func(context.Context, []byte) ([]byte, error) {
	return func(_ context.Context, digest []byte) ([]byte, error) {
		compact := decredecdsa.SignCompact(priv, digest, true)

		var s secp256k1.ModNScalar
		s.SetByteSlice(compact[33:65])
		s.Negate()

		out := make([]byte, 65)
		copy(out[:32], compact[1:33])
		s.PutBytesUnchecked(out[32:64])
		out[64] = compact[0] - 27 - 4

		return out, nil
	}
}

func newPriv(t *testing.T) *secp256k1.PrivateKey {
	t.Helper()

	priv, err := secp256k1.GeneratePrivateKey()
	require.NoError(t, err)

	return priv
}

func reporterOf(priv *secp256k1.PrivateKey) sdk.AccAddress {
	pub := &sdksecp.PubKey{Key: priv.PubKey().SerializeCompressed()}

	return sdk.AccAddress(pub.Address())
}

func remoteOpts(pki *testPKI, addr string) RemoteOptions {
	return RemoteOptions{Address: addr, KeyID: testKeyID, CAFile: pki.caFile, Timeout: 5 * time.Second}
}

func closeSigner(t *testing.T, s Signer) {
	t.Helper()

	c, ok := s.(io.Closer)
	require.True(t, ok, "remote signer implements io.Closer")
	require.NoError(t, c.Close())
}

func TestRemoteSchemes(t *testing.T) {
	signBytes := []byte("a sign doc that is not 32 bytes long")
	digest := sha256.Sum256(signBytes)

	cases := []struct {
		name    string
		scheme  kmspb.SignatureScheme
		signer  func(*secp256k1.PrivateKey) func(context.Context, []byte) ([]byte, error)
		payload []byte
	}{
		{"ECDSA_SECP256K1 signs the sign bytes and answers r||s", kmspb.SignatureScheme_ECDSA_SECP256K1, rsSigner, signBytes},
		{"ECDSA_SECP256K1ETH signs their SHA-256 and answers r||s||v", kmspb.SignatureScheme_ECDSA_SECP256K1ETH, rsvHighSSigner, digest[:]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pki := newTestPKI(t)
			priv := newPriv(t)
			f := &fakeKMS{
				key:  &kmspb.Key{Id: testKeyID, Pubkey: priv.PubKey().SerializeCompressed(), Scheme: tc.scheme},
				sign: tc.signer(priv),
			}
			addr := serve(t, pki, f, false)
			reporter := reporterOf(priv)

			s, err := NewRemote(context.Background(), remoteOpts(pki, addr), reporter)
			require.NoError(t, err)
			defer closeSigner(t, s)

			require.True(t, s.Address().Equals(reporter))
			require.Equal(t, priv.PubKey().SerializeCompressed(), s.PubKey().Bytes())

			sig, err := s.Sign(context.Background(), signBytes)
			require.NoError(t, err)
			require.Len(t, sig, 64)
			require.True(t, s.PubKey().VerifySignature(signBytes, sig))

			var sc secp256k1.ModNScalar
			sc.SetByteSlice(sig[32:])
			require.False(t, sc.IsOverHalfOrder(), "signature is low-S")

			payloads, _ := f.seen()
			require.Equal(t, [][]byte{tc.payload}, payloads)
		})
	}

	t.Run("server name override and client certificate", func(t *testing.T) {
		pki := newTestPKI(t)
		priv := newPriv(t)
		f := &fakeKMS{
			key:  &kmspb.Key{Id: testKeyID, Pubkey: priv.PubKey().SerializeCompressed(), Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
			sign: rsSigner(priv),
		}
		addr := serve(t, pki, f, true)

		opts := remoteOpts(pki, addr)
		opts.ServerName = testServerName
		opts.CertFile = pki.certFile
		opts.KeyFile = pki.keyFile

		s, err := NewRemote(context.Background(), opts, reporterOf(priv))
		require.NoError(t, err)
		defer closeSigner(t, s)

		sig, err := s.Sign(context.Background(), signBytes)
		require.NoError(t, err)
		require.Len(t, sig, 64)
	})
}

func TestRemoteRejects(t *testing.T) {
	signBytes := []byte("sign doc")

	t.Run("startup", func(t *testing.T) {
		priv := newPriv(t)
		other := newPriv(t)
		compressed := priv.PubKey().SerializeCompressed()

		cases := []struct {
			name    string
			key     *kmspb.Key
			opts    func(*RemoteOptions, *testPKI)
			mtls    bool
			wantErr string
		}{
			{
				name:    "a key that does not derive reporter",
				key:     &kmspb.Key{Id: testKeyID, Pubkey: other.PubKey().SerializeCompressed(), Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				wantErr: "signer does not match reporter",
			},
			{
				name:    "scheme ED25519",
				key:     &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ED25519},
				wantErr: "unsupported signature scheme",
			},
			{
				name:    "an uncompressed key",
				key:     &kmspb.Key{Id: testKeyID, Pubkey: priv.PubKey().SerializeUncompressed(), Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				wantErr: "33-byte compressed",
			},
			{
				name:    "a key off the curve",
				key:     &kmspb.Key{Id: testKeyID, Pubkey: append([]byte{0x02}, make([]byte, 32)...), Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				wantErr: "invalid secp256k1 public key",
			},
			{
				name:    "a missing key",
				key:     &kmspb.Key{Id: testKeyID},
				wantErr: "33-byte compressed",
			},
			{
				name: "a server with an untrusted certificate",
				key:  &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				opts: func(o *RemoteOptions, _ *testPKI) {
					o.CAFile = newTestPKI(t).caFile
				},
				wantErr: errGetKey,
			},
			{
				name: "a server name the certificate does not cover",
				key:  &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				opts: func(o *RemoteOptions, _ *testPKI) {
					o.ServerName = "other.test"
				},
				wantErr: errGetKey,
			},
			{
				name: "a cert file without a key file",
				key:  &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				opts: func(o *RemoteOptions, p *testPKI) {
					o.CertFile = p.certFile
				},
				wantErr: "cert-file and key-file must be set together",
			},
			{
				name:    "a server that requires a client certificate",
				key:     &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				mtls:    true,
				wantErr: errGetKey,
			},
			{
				name: "a missing CA file",
				key:  &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				opts: func(o *RemoteOptions, p *testPKI) {
					o.CAFile = filepath.Join(p.dir, "missing.pem")
				},
				wantErr: "read ca-file",
			},
			{
				name: "no timeout",
				key:  &kmspb.Key{Id: testKeyID, Pubkey: compressed, Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
				opts: func(o *RemoteOptions, _ *testPKI) {
					o.Timeout = 0
				},
				wantErr: "timeout must be positive",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				pki := newTestPKI(t)
				addr := serve(t, pki, &fakeKMS{key: tc.key, sign: rsSigner(priv)}, tc.mtls)

				opts := remoteOpts(pki, addr)
				if tc.opts != nil {
					tc.opts(&opts, pki)
				}

				s, err := NewRemote(context.Background(), opts, reporterOf(priv))
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, s)
			})
		}
	})

	t.Run("sign", func(t *testing.T) {
		priv := newPriv(t)
		other := newPriv(t)

		long := func(context.Context, []byte) ([]byte, error) {
			return append(make([]byte, 65), 0xab), nil
		}
		short := func(context.Context, []byte) ([]byte, error) {
			return make([]byte, 63), nil
		}
		blocked := func(ctx context.Context, _ []byte) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}

		cases := []struct {
			name    string
			sign    func(context.Context, []byte) ([]byte, error)
			wantErr string
		}{
			{"a bad signature", rsSigner(other), "does not verify"},
			{"a response over 65 bytes", long, "64 or 65 bytes"},
			{"a response under 64 bytes", short, "64 or 65 bytes"},
			{"a zero signature", func(context.Context, []byte) ([]byte, error) { return make([]byte, 64), nil }, "out of range"},
			{"a server past the timeout", blocked, "DeadlineExceeded"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				pki := newTestPKI(t)
				f := &fakeKMS{
					key:  &kmspb.Key{Id: testKeyID, Pubkey: priv.PubKey().SerializeCompressed(), Scheme: kmspb.SignatureScheme_ECDSA_SECP256K1},
					sign: tc.sign,
				}
				addr := serve(t, pki, f, false)

				opts := remoteOpts(pki, addr)
				opts.Timeout = time.Second

				s, err := NewRemote(context.Background(), opts, reporterOf(priv))
				require.NoError(t, err)
				defer closeSigner(t, s)

				sig, err := s.Sign(context.Background(), signBytes)
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, sig)

				msg := strings.ToLower(err.Error())
				require.NotContains(t, msg, hex.EncodeToString(signBytes))
				require.NotContains(t, msg, string(signBytes))

				_, answers := f.seen()
				for _, a := range answers {
					if len(a) > 0 {
						require.NotContains(t, msg, hex.EncodeToString(a))
					}
				}
			})
		}
	})
}

// TestRemoteAgainstKMS is the release gate against a real kms binary (Task
// 27). It needs REPORTER_KMS_ADDR, REPORTER_KMS_CA and REPORTER_KMS_KEYS.
func TestRemoteAgainstKMS(t *testing.T) {
	addr := os.Getenv("REPORTER_KMS_ADDR")
	if addr == "" {
		t.Skip("REPORTER_KMS_ADDR is not set")
	}

	keys := strings.Split(os.Getenv("REPORTER_KMS_KEYS"), ",")
	require.NotEmpty(t, strings.TrimSpace(keys[0]), "REPORTER_KMS_KEYS lists no key IDs")

	creds, err := remoteCredentials(RemoteOptions{CAFile: os.Getenv("REPORTER_KMS_CA")})
	require.NoError(t, err)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()

	client := kmspb.NewSignerServiceClient(conn)
	signBytes := []byte("akt-reporter release gate sign doc")

	for _, id := range keys {
		id = strings.TrimSpace(id)

		t.Run(id, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.GetKey(ctx, &kmspb.GetKeyRequest{Id: id})
			require.NoError(t, err)

			pub, err := parseRemoteKey(resp.GetKey().GetPubkey())
			require.NoError(t, err)
			reporter := sdk.AccAddress(pub.Address())

			s, err := NewRemote(ctx, RemoteOptions{
				Address: addr,
				KeyID:   id,
				CAFile:  os.Getenv("REPORTER_KMS_CA"),
				Timeout: 30 * time.Second,
			}, reporter)
			require.NoError(t, err)
			defer closeSigner(t, s)

			require.True(t, s.Address().Equals(reporter))

			sig, err := s.Sign(ctx, signBytes)
			require.NoError(t, err)

			out, err := NormalizeAndVerify(s.PubKey(), signBytes, sig)
			require.NoError(t, err)
			require.Equal(t, sig, out)
		})
	}
}

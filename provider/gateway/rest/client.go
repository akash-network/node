package rest

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"k8s.io/client-go/tools/remotecommand"

	cutils "github.com/ovrclk/akash/x/cert/utils"
	dtypes "github.com/ovrclk/akash/x/deployment/types/v1beta2"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"

	akashclient "github.com/ovrclk/akash/client"
	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/provider"
	cltypes "github.com/ovrclk/akash/provider/cluster/types"
	ctypes "github.com/ovrclk/akash/x/cert/types/v1beta2"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

const (
	schemeWSS   = "wss"
	schemeHTTPS = "https"
)

// Client defines the methods available for connecting to the gateway server.
type Client interface {
	Status(ctx context.Context) (*provider.Status, error)
	Validate(ctx context.Context, gspec dtypes.GroupSpec) (provider.ValidateGroupSpecResult, error)
	SubmitManifest(ctx context.Context, dseq uint64, mani manifest.Manifest) error
	LeaseStatus(ctx context.Context, id mtypes.LeaseID) (*cltypes.LeaseStatus, error)
	LeaseEvents(ctx context.Context, id mtypes.LeaseID, services string, follow bool) (*LeaseKubeEvents, error)
	LeaseLogs(ctx context.Context, id mtypes.LeaseID, services string, follow bool, tailLines int64) (*ServiceLogs, error)
	ServiceStatus(ctx context.Context, id mtypes.LeaseID, service string) (*cltypes.ServiceStatus, error)
	LeaseShell(ctx context.Context, id mtypes.LeaseID, service string, podIndex uint, cmd []string,
		stdin io.ReadCloser,
		stdout io.Writer,
		stderr io.Writer,
		tty bool,
		tsq <-chan remotecommand.TerminalSize) error
	MigrateHostnames(ctx context.Context, hostnames []string, dseq uint64, gseq uint32) error
}

type JwtClient interface {
	GetJWT(ctx context.Context) (*jwt.Token, error)
}

type LeaseKubeEvent struct {
	Action  string `json:"action"`
	Message string `json:"message"`
}

type ServiceLogMessage struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type LeaseKubeEvents struct {
	Stream  <-chan cltypes.LeaseEvent
	OnClose <-chan string
}

type ServiceLogs struct {
	Stream  <-chan ServiceLogMessage
	OnClose <-chan string
}

func NewJwtClient(qclient akashclient.QueryClient, addr sdk.Address, certs []tls.Certificate) (JwtClient, error) {
	res, err := qclient.Provider(context.Background(), &ptypes.QueryProviderRequest{Owner: addr.String()})
	if err != nil {
		return nil, err
	}

	uri, err := url.Parse(res.Provider.JWTHostURI)
	if err != nil {
		return nil, err
	}

	return newClient(qclient, addr, certs, uri), nil
}

// NewClient returns a new Client
func NewClient(qclient akashclient.QueryClient, addr sdk.Address, certs []tls.Certificate) (Client, error) {
	res, err := qclient.Provider(context.Background(), &ptypes.QueryProviderRequest{Owner: addr.String()})
	if err != nil {
		return nil, err
	}

	uri, err := url.Parse(res.Provider.HostURI)
	if err != nil {
		return nil, err
	}

	return newClient(qclient, addr, certs, uri), nil
}

func newClient(qclient akashclient.QueryClient, addr sdk.Address, certs []tls.Certificate, uri *url.URL) *client {
	cl := &client{
		host:    uri,
		addr:    addr,
		cclient: qclient,
	}

	tlsConfig := &tls.Config{
		// must use Hostname rather then Host field as certificate is issued for host without port
		ServerName:            uri.Hostname(),
		Certificates:          certs,
		InsecureSkipVerify:    true, // nolint: gosec
		VerifyPeerCertificate: cl.verifyPeerCertificate,
		MinVersion:            tls.VersionTLS13,
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		// Never  follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar:     nil,
		Timeout: 0,
	}

	cl.hclient = httpClient

	cl.wsclient = &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		TLSClientConfig:  tlsConfig,
	}

	return cl
}

type ClientDirectory struct {
	cosmosContext cosmosclient.Context
	clients       map[string]Client
	clientCert    tls.Certificate

	lock sync.Mutex
}

func (cd *ClientDirectory) GetClientFromBech32(providerAddrBech32 string) (Client, error) {
	id, err := sdk.AccAddressFromBech32(providerAddrBech32)
	if err != nil {
		return nil, err
	}
	return cd.GetClient(id)
}

func (cd *ClientDirectory) GetClient(providerAddr sdk.Address) (Client, error) {
	cd.lock.Lock()
	defer cd.lock.Unlock()

	client, clientExists := cd.clients[providerAddr.String()]
	if clientExists {
		return client, nil
	}

	client, err := NewClient(akashclient.NewQueryClientFromCtx(cd.cosmosContext), providerAddr, []tls.Certificate{cd.clientCert})
	if err != nil {
		return nil, err
	}

	cd.clients[providerAddr.String()] = client // Store the client

	return client, nil
}

func NewClientDirectory(ctx context.Context, cctx cosmosclient.Context) (*ClientDirectory, error) {
	cert, err := cutils.LoadAndQueryCertificateForAccount(ctx, cctx, cctx.Keyring)
	if err != nil {
		return nil, err
	}

	return &ClientDirectory{
		cosmosContext: cctx,
		clientCert:    cert,
		clients:       make(map[string]Client),
	}, nil
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type client struct {
	host     *url.URL
	hclient  httpClient
	wsclient *websocket.Dialer
	addr     sdk.Address
	cclient  ctypes.QueryClient
}

type ClientCustomClaims struct {
	AkashNamespace *AkashNamespace `json:"https://akash.network/"`
	jwt.RegisteredClaims
}

type AkashNamespace struct {
	V1 *ClaimsV1 `json:"v1"`
}

type ClaimsV1 struct {
	CertSerialNumber string `json:"cert_serial_number"`
}

var errRequiredCertSerialNum = errors.New("cert_serial_number must be present in claims")

func (c *ClientCustomClaims) Valid() error {
	_, err := sdk.AccAddressFromBech32(c.Subject)
	if err != nil {
		return err
	}
	_, err = sdk.AccAddressFromBech32(c.Issuer)
	if err != nil {
		return err
	}
	if c.AkashNamespace == nil || c.AkashNamespace.V1 == nil || c.AkashNamespace.V1.CertSerialNumber == "" {
		return errRequiredCertSerialNum
	}
	return nil
}

func (c *client) GetJWT(ctx context.Context) (*jwt.Token, error) {
	uri, err := makeURI(c.host, "jwt")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.hclient.Do(req)
	if err != nil {
		return nil, err
	}

	responseBuf := &bytes.Buffer{}
	_, err = io.Copy(responseBuf, resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return nil, err
	}

	err = createClientResponseErrorIfNotOK(resp, responseBuf)
	if err != nil {
		return nil, err
	}

	token, err := jwt.ParseWithClaims(responseBuf.String(), &ClientCustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		// return the public key to be used for JWT verification
		return resp.TLS.PeerCertificates[0].PublicKey.(*ecdsa.PublicKey), nil
	})
	if err != nil {
		return nil, err
	}

	return token, nil
}

type ClientResponseError struct {
	Status  int
	Message string
}

func (err ClientResponseError) Error() string {
	return fmt.Sprintf("remote server returned %d", err.Status)
}

func (err ClientResponseError) ClientError() string {
	return fmt.Sprintf("Remote Server returned %d\n%s", err.Status, err.Message)
}

func (c *client) verifyPeerCertificate(certificates [][]byte, _ [][]*x509.Certificate) error {
	if len(certificates) != 1 {
		return errors.Errorf("tls: invalid certificate chain")
	}

	cert, err := x509.ParseCertificate(certificates[0])
	if err != nil {
		return errors.Wrap(err, "tls: failed to parse certificate")
	}

	// validation
	var prov sdk.Address
	if prov, err = sdk.AccAddressFromBech32(cert.Subject.CommonName); err != nil {
		return errors.Wrap(err, "tls: invalid certificate's subject common name")
	}

	// 1. CommonName in issuer and Subject must be the same
	if cert.Subject.CommonName != cert.Issuer.CommonName {
		return errors.Wrap(err, "tls: invalid certificate's issuer common name")
	}

	if !c.addr.Equals(prov) {
		return errors.Errorf("tls: hijacked certificate")
	}

	// 2. serial number must be in
	if cert.SerialNumber == nil {
		return errors.Wrap(err, "tls: invalid certificate serial number")
	}

	// 3. look up certificate on chain. it must not be revoked
	var resp *ctypes.QueryCertificatesResponse
	resp, err = c.cclient.Certificates(
		context.Background(),
		&ctypes.QueryCertificatesRequest{
			Filter: ctypes.CertificateFilter{
				Owner:  prov.String(),
				Serial: cert.SerialNumber.String(),
				State:  "valid",
			},
		},
	)
	if err != nil {
		return errors.Wrap(err, "tls: unable to fetch certificate from chain")
	}
	if (len(resp.Certificates) != 1) || !resp.Certificates[0].Certificate.IsState(ctypes.CertificateValid) {
		return errors.New("tls: attempt to use non-existing or revoked certificate")
	}

	certPool := x509.NewCertPool()
	certPool.AddCert(cert)

	opts := x509.VerifyOptions{
		DNSName:                   c.host.Hostname(),
		Roots:                     certPool,
		CurrentTime:               time.Now(),
		KeyUsages:                 []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		MaxConstraintComparisions: 0,
	}

	if _, err = cert.Verify(opts); err != nil {
		return errors.Wrap(err, "tls: unable to verify certificate")
	}

	return nil
}

func (c *client) Status(ctx context.Context) (*provider.Status, error) {
	uri, err := makeURI(c.host, statusPath())
	if err != nil {
		return nil, err
	}
	var obj provider.Status

	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

func (c *client) Validate(ctx context.Context, gspec dtypes.GroupSpec) (provider.ValidateGroupSpecResult, error) {
	uri, err := makeURI(c.host, validatePath())
	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	if err = gspec.ValidateBasic(); err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	bgspec, err := json.Marshal(gspec)
	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", uri, bytes.NewReader(bgspec))
	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}
	req.Header.Set("Content-Type", contentTypeJSON)

	resp, err := c.hclient.Do(req)
	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	err = createClientResponseErrorIfNotOK(resp, buf)
	if err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	var obj provider.ValidateGroupSpecResult
	if err = json.NewDecoder(buf).Decode(&obj); err != nil {
		return provider.ValidateGroupSpecResult{}, err
	}

	return obj, nil
}

func (c *client) SubmitManifest(ctx context.Context, dseq uint64, mani manifest.Manifest) error {
	uri, err := makeURI(c.host, submitManifestPath(dseq))
	if err != nil {
		return err
	}

	buf, err := json.Marshal(mani)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", uri, bytes.NewBuffer(buf))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := c.hclient.Do(req)
	if err != nil {
		return err
	}
	responseBuf := &bytes.Buffer{}
	_, err = io.Copy(responseBuf, resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return err
	}

	return createClientResponseErrorIfNotOK(resp, responseBuf)
}

func (c *client) MigrateHostnames(ctx context.Context, hostnames []string, dseq uint64, gseq uint32) error {
	uri, err := makeURI(c.host, "hostname/migrate")
	if err != nil {
		return err
	}

	body := migrateRequestBody{
		HostnamesToMigrate: hostnames,
		DestinationDSeq:    dseq,
		DestinationGSeq:    gseq,
	}

	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)

	resp, err := c.hclient.Do(req)
	if err != nil {
		return err
	}
	responseBuf := &bytes.Buffer{}
	_, err = io.Copy(responseBuf, resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return err
	}

	return createClientResponseErrorIfNotOK(resp, responseBuf)
}

func (c *client) LeaseStatus(ctx context.Context, id mtypes.LeaseID) (*cltypes.LeaseStatus, error) {
	uri, err := makeURI(c.host, leaseStatusPath(id))
	if err != nil {
		return nil, err
	}

	var obj cltypes.LeaseStatus
	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

func (c *client) LeaseEvents(ctx context.Context, id mtypes.LeaseID, _ string, follow bool) (*LeaseKubeEvents, error) {
	endpoint, err := url.Parse(c.host.String() + "/" + leaseEventsPath(id))
	if err != nil {
		return nil, err
	}

	switch endpoint.Scheme {
	case schemeWSS, schemeHTTPS:
		endpoint.Scheme = schemeWSS
	default:
		return nil, errors.Errorf("invalid uri scheme %q", endpoint.Scheme)
	}

	query := url.Values{}
	query.Set("follow", strconv.FormatBool(follow))

	endpoint.RawQuery = query.Encode()
	conn, response, err := c.wsclient.DialContext(ctx, endpoint.String(), nil)
	if err != nil {
		if errors.Is(err, websocket.ErrBadHandshake) {
			buf := &bytes.Buffer{}
			_, _ = io.Copy(buf, response.Body)

			return nil, ClientResponseError{
				Status:  response.StatusCode,
				Message: buf.String(),
			}
		}

		return nil, err
	}

	streamch := make(chan cltypes.LeaseEvent)
	onclose := make(chan string, 1)
	logs := &LeaseKubeEvents{
		Stream:  streamch,
		OnClose: onclose,
	}

	processOnCloseErr := func(err error) {
		if err != nil {
			if _, ok := err.(*websocket.CloseError); ok { // nolint: gosimple
				onclose <- parseCloseMessage(err.Error())
			} else {
				onclose <- err.Error()
			}
		}
	}

	if err = conn.SetReadDeadline(time.Now().Add(pingWait)); err != nil {
		return nil, err
	}

	conn.SetPingHandler(func(string) error {
		err := conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
		if err != nil {
			return err
		}

		return conn.SetReadDeadline(time.Now().Add(pingWait))
	})

	go func(conn *websocket.Conn) {
		defer func() {
			close(streamch)
			close(onclose)
			_ = conn.Close()
		}()

		for {
			mType, msg, e := conn.ReadMessage()
			if e != nil {
				processOnCloseErr(e)
				return
			}

			switch mType {
			case websocket.TextMessage:
				var evt cltypes.LeaseEvent
				if e = json.Unmarshal(msg, &evt); e != nil {
					onclose <- e.Error()
					return
				}

				streamch <- evt
			case websocket.CloseMessage:
				onclose <- parseCloseMessage(string(msg))
				return
			default:
			}
		}
	}(conn)

	return logs, nil
}

func (c *client) ServiceStatus(ctx context.Context, id mtypes.LeaseID, service string) (*cltypes.ServiceStatus, error) {
	uri, err := makeURI(c.host, serviceStatusPath(id, service))
	if err != nil {
		return nil, err
	}

	var obj cltypes.ServiceStatus
	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

func (c *client) getStatus(ctx context.Context, uri string, obj interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)

	resp, err := c.hclient.Do(req)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	_, err = io.Copy(buf, resp.Body)
	defer func() {
		_ = resp.Body.Close()
	}()

	if err != nil {
		return err
	}

	err = createClientResponseErrorIfNotOK(resp, buf)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(buf)
	return dec.Decode(obj)
}

func createClientResponseErrorIfNotOK(resp *http.Response, responseBuf *bytes.Buffer) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return ClientResponseError{
		Status:  resp.StatusCode,
		Message: responseBuf.String(),
	}
}

// makeURI
// for client queries path must not include owner id
func makeURI(uri *url.URL, path string) (string, error) {
	endpoint, err := url.Parse(uri.String() + "/" + path)
	if err != nil {
		return "", err
	}

	return endpoint.String(), nil
}

func (c *client) LeaseLogs(ctx context.Context,
	id mtypes.LeaseID,
	services string,
	follow bool,
	tailLines int64) (*ServiceLogs, error) {

	endpoint, err := url.Parse(c.host.String() + "/" + serviceLogsPath(id))
	if err != nil {
		return nil, err
	}

	switch endpoint.Scheme {
	case schemeWSS, schemeHTTPS:
		endpoint.Scheme = schemeWSS
	default:
		return nil, errors.Errorf("invalid uri scheme \"%s\"", endpoint.Scheme)
	}

	query := url.Values{}

	query.Set("follow", strconv.FormatBool(follow))

	if services != "" {
		query.Set("services", services)
	}

	endpoint.RawQuery = query.Encode()

	conn, response, err := c.wsclient.DialContext(ctx, endpoint.String(), nil)
	if err != nil {
		if errors.Is(err, websocket.ErrBadHandshake) {
			buf := &bytes.Buffer{}
			_, _ = io.Copy(buf, response.Body)

			return nil, ClientResponseError{
				Status:  response.StatusCode,
				Message: buf.String(),
			}
		}

		return nil, err
	}

	streamch := make(chan ServiceLogMessage)
	onclose := make(chan string, 1)
	logs := &ServiceLogs{
		Stream:  streamch,
		OnClose: onclose,
	}

	if err = conn.SetReadDeadline(time.Now().Add(pingWait)); err != nil {
		return nil, err
	}

	conn.SetPingHandler(func(string) error {
		err := conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
		if err != nil {
			return err
		}

		return conn.SetReadDeadline(time.Now().Add(pingWait))
	})

	go func(conn *websocket.Conn) {
		defer func() {
			close(streamch)
			close(onclose)
			_ = conn.Close()
		}()

		for {
			mType, msg, e := conn.ReadMessage()
			if e != nil {
				onclose <- parseCloseMessage(e.Error())
				return
			}

			switch mType {
			case websocket.TextMessage:
				var logLine ServiceLogMessage
				if e = json.Unmarshal(msg, &logLine); e != nil {
					return
				}

				streamch <- logLine
			case websocket.CloseMessage:
				onclose <- parseCloseMessage(string(msg))
				return
			default:
			}
		}
	}(conn)

	return logs, nil
}

// parseCloseMessage extract close reason from websocket close message
// "websocket: [error code]: [client reason]"
func parseCloseMessage(msg string) string {
	errmsg := strings.SplitN(msg, ": ", 3)
	if len(errmsg) == 3 {
		return errmsg[2]
	}

	return ""
}

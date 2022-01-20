package operatorclients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	ipoptypes "github.com/ovrclk/akash/provider/operator/ipoperator/types"
	mtypes "github.com/ovrclk/akash/x/market/types/v1beta2"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
)

var (
	errNotImplemented   = errors.New("not implemented")
	errIPOperatorRemote = errors.New("ip operator remote error")
)

type IPOperatorClient interface {
	Check(ctx context.Context) error
	GetIPAddressUsage(ctx context.Context) (ipoptypes.IPAddressUsage, error)

	GetIPAddressStatus(ctx context.Context, orderID mtypes.OrderID) ([]ipoptypes.LeaseIPStatus, error)
	Stop()
	String() string
}

/* A null client for use in tests and other scenarios */
type ipOperatorNullClient struct{}

func NullIPOperatorClient() IPOperatorClient {
	return ipOperatorNullClient{}
}

func (v ipOperatorNullClient) String() string {
	return fmt.Sprintf("<%T>", v)
}

func (ipOperatorNullClient) Check(_ context.Context) error {
	return errNotImplemented
}

func (ipOperatorNullClient) GetIPAddressUsage(_ context.Context) (ipoptypes.IPAddressUsage, error) {
	return ipoptypes.IPAddressUsage{}, errNotImplemented
}

func (ipOperatorNullClient) Stop() {}

func (ipOperatorNullClient) GetIPAddressStatus(context.Context, mtypes.OrderID) ([]ipoptypes.LeaseIPStatus, error) {
	return nil, errNotImplemented
}

func NewIPOperatorClient(logger log.Logger, kubeConfig *rest.Config, endpoint *net.SRV) (IPOperatorClient, error) {
	sda, err := clusterutil.NewServiceDiscoveryAgent(logger, kubeConfig, "api", "akash-ip-operator", "akash-services", endpoint)
	if err != nil {
		return nil, err
	}

	return &ipOperatorClient{
		sda: sda,
		log: logger.With("operator", "ip"),
	}, nil
}

func (ipoc *ipOperatorClient) String() string {
	return fmt.Sprintf("<%T %p>", ipoc, ipoc)
}

func (ipoc *ipOperatorClient) Stop() {
	ipoc.sda.Stop()
}

const (
	ipOperatorHealthPath = "/health"
)

/* A client to talk to the Akash implementation of the IP Operator via HTTP */
type ipOperatorClient struct {
	sda    clusterutil.ServiceDiscoveryAgent
	client clusterutil.ServiceClient
	log    log.Logger
}

var errNotAlive = errors.New("ip operator is not yet alive")

func (ipoc *ipOperatorClient) Check(ctx context.Context) error {
	req, err := ipoc.newRequest(ctx, http.MethodGet, ipOperatorHealthPath, nil)
	if err != nil {
		return err
	}

	response, err := ipoc.client.DoRequest(req)
	if err != nil {
		return err
	}
	ipoc.log.Info("check result", "status", response.StatusCode)

	if response.StatusCode != http.StatusOK {
		return errNotAlive
	}

	return nil
}

func (ipoc *ipOperatorClient) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	if ipoc.client == nil {
		var err error
		ipoc.client, err = ipoc.sda.GetClient(ctx, false, false)
		if err != nil {
			return nil, err
		}
	}
	return ipoc.client.CreateRequest(ctx, method, path, body)
}

func (ipoc *ipOperatorClient) GetIPAddressStatus(ctx context.Context, orderID mtypes.OrderID) ([]ipoptypes.LeaseIPStatus, error) {
	path := fmt.Sprintf("/ip-lease-status/%s/%d/%d/%d", orderID.GetOwner(), orderID.GetDSeq(), orderID.GetGSeq(), orderID.GetOSeq())
	req, err := ipoc.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	ipoc.log.Debug("asking for IP address status", "method", req.Method, "url", req.URL)
	response, err := ipoc.client.DoRequest(req)
	if err != nil {
		return nil, err
	}
	ipoc.log.Debug("ip address status request result", "status", response.StatusCode)

	if response.StatusCode == http.StatusNoContent {
		return nil, nil // No data for this lease
	}

	if response.StatusCode != http.StatusOK {
		return nil, extractRemoteError(response)
	}

	var result []ipoptypes.LeaseIPStatus

	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (ipoc *ipOperatorClient) GetIPAddressUsage(ctx context.Context) (ipoptypes.IPAddressUsage, error) {
	req, err := ipoc.newRequest(ctx, http.MethodGet, "/usage", nil)
	if err != nil {
		return ipoptypes.IPAddressUsage{}, err
	}

	response, err := ipoc.client.DoRequest(req)
	if err != nil {
		return ipoptypes.IPAddressUsage{}, err
	}
	ipoc.log.Info("usage result", "status", response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return ipoptypes.IPAddressUsage{}, extractRemoteError(response)
	}

	decoder := json.NewDecoder(response.Body)
	result := ipoptypes.IPAddressUsage{}
	err = decoder.Decode(&result)
	if err != nil {
		return ipoptypes.IPAddressUsage{}, err
	}

	return result, nil
}

func extractRemoteError(response *http.Response) error {
	body := ipoptypes.IPOperatorErrorResponse{}
	decoder := json.NewDecoder(response.Body)
	err := decoder.Decode(&body)
	if err != nil {
		return err
	}

	if 0 == len(body.Error) {
		return io.EOF
	}

	if body.Code > 0 {
		return ipoptypes.LookupError(body.Code)
	}

	return fmt.Errorf("%w: http status %d - %s", errIPOperatorRemote, response.StatusCode, body.Error)
}

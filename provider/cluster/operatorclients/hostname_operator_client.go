package operatorclients

import (
	"context"
	"fmt"
	clusterutil "github.com/ovrclk/akash/provider/cluster/util"
	"github.com/tendermint/tendermint/libs/log"
	"io"
	"k8s.io/client-go/rest"
	"net"
	"net/http"
)

const (
	hostnameOperatorHealthPath = "/health"
)

type HostnameOperatorClient interface {
	Check(ctx context.Context) error
	String() string

	Stop()
}

type hostnameOperatorClient struct {
	sda    clusterutil.ServiceDiscoveryAgent
	client clusterutil.ServiceClient
	log    log.Logger
}

func NewHostnameOperatorClient(logger log.Logger, kubeConfig *rest.Config, endpoint *net.SRV) (HostnameOperatorClient, error) {
	sda, err := clusterutil.NewServiceDiscoveryAgent(logger, kubeConfig, "status", "akash-hostname-operator", "akash-services", endpoint)
	if err != nil {
		return nil, err
	}

	return &hostnameOperatorClient{
		log: logger.With("operator", "hostname"),
		sda: sda,
	}, nil

}

func (hopc *hostnameOperatorClient) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	if nil == hopc.client {
		var err error
		hopc.client, err = hopc.sda.GetClient(ctx, false, false)
		if err != nil {
			return nil, err
		}
	}

	return hopc.client.CreateRequest(ctx, method, path, body)
}

func (hopc *hostnameOperatorClient) Check(ctx context.Context) error {
	req, err := hopc.newRequest(ctx, http.MethodGet, hostnameOperatorHealthPath, nil)
	if err != nil {
		return err
	}

	response, err := hopc.client.DoRequest(req)
	if err != nil {
		return err
	}
	hopc.log.Info("check result", "status", response.StatusCode)

	if response.StatusCode != http.StatusOK {
		return errNotAlive
	}

	return nil
}

func (hopc *hostnameOperatorClient) String() string {
	return fmt.Sprintf("<%T %p>", hopc, hopc)
}
func (hopc *hostnameOperatorClient) Stop() {
	hopc.sda.Stop()
}

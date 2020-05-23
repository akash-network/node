package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/ovrclk/akash/provider"
	"github.com/ovrclk/akash/provider/cluster"
	"github.com/ovrclk/akash/provider/manifest"
	mtypes "github.com/ovrclk/akash/x/market/types"
)

type Client interface {
	Status(ctx context.Context, host string) (*provider.Status, error)
	SubmitManifest(ctx context.Context, host string, req *manifest.SubmitRequest) error
	LeaseStatus(ctx context.Context, host string, id mtypes.LeaseID) (*cluster.LeaseStatus, error)
	ServiceStatus(ctx context.Context, host string, id mtypes.LeaseID, service string) (*cluster.ServiceStatus, error)
}

func NewClient() Client {
	return &client{
		hclient: http.DefaultClient,
	}
}

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

type client struct {
	hclient httpClient
}

func (c *client) Status(ctx context.Context, host string) (*provider.Status, error) {
	uri, err := makeURI(host, statusPath())
	if err != nil {
		return nil, err
	}
	var obj provider.Status

	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

func (c *client) SubmitManifest(ctx context.Context, host string, mreq *manifest.SubmitRequest) error {
	uri, err := makeURI(host, submitManifestPath(mreq.Deployment))
	if err != nil {
		return err
	}

	buf, err := json.Marshal(mreq)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", uri, bytes.NewBuffer(buf))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := c.hclient.Do(req)
	if resp.StatusCode != http.StatusOK {
		io.Copy(ioutil.Discard, resp.Body)
		return errors.New(resp.Status)
	}

	return err
}

func (c *client) LeaseStatus(ctx context.Context, host string, id mtypes.LeaseID) (*cluster.LeaseStatus, error) {
	uri, err := makeURI(host, leaseStatusPath(id))
	if err != nil {
		return nil, err
	}

	var obj cluster.LeaseStatus
	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}
func (c *client) ServiceStatus(ctx context.Context, host string, id mtypes.LeaseID, service string) (*cluster.ServiceStatus, error) {
	uri, err := makeURI(host, serviceStatusPath(id, service))
	if err != nil {
		return nil, err
	}

	var obj cluster.ServiceStatus
	if err := c.getStatus(ctx, uri, &obj); err != nil {
		return nil, err
	}

	return &obj, nil
}

func (c *client) getStatus(ctx context.Context, uri string, obj interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	req.Header.Set("Content-Type", contentTypeJSON)
	if err != nil {
		return err
	}

	resp, err := c.hclient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(ioutil.Discard, resp.Body)
		return errors.New(resp.Status)
	}

	dec := json.NewDecoder(resp.Body)
	return dec.Decode(obj)
}

func makeURI(host string, path string) (string, error) {
	endpoint, err := url.Parse(host + "/" + path)
	if err != nil {
		return "", err
	}
	return endpoint.String(), nil
}

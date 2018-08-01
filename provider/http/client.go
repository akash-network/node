package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	nhttp "net/http"

	mutil "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
)

func LeaseStatus(
	ctx context.Context,
	provider *types.Provider,
	leaseID types.LeaseID) (*types.LeaseStatusResponse, error) {
	resp, err := get(ctx, provider.GetHostURI()+"/lease/"+leaseID.String())
	if err != nil {
		return nil, err
	}
	status := &types.LeaseStatusResponse{}
	err = json.Unmarshal(resp, status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func SendManifest(
	ctx context.Context,
	manifest *types.Manifest,
	signer txutil.Signer,
	provider *types.Provider,
	deployment []byte) error {
	_, buf, err := mutil.SignManifest(manifest, signer, deployment)
	if err != nil {
		return err
	}
	return post(ctx, provider.GetHostURI()+"/manifest", buf)
}

func Status(ctx context.Context, provider *types.Provider) (*types.ServerStatusParseable, error) {
	resp, err := get(ctx, provider.GetHostURI()+"/status")
	if err != nil {
		return nil, err
	}
	status := &types.ServerStatusParseable{}
	err = json.Unmarshal(resp, status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// XXX assumes url is http/https
func post(ctx context.Context, url string, data []byte) error {
	req, err := nhttp.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("X-Custom-Header", "Akash")
	req.Header.Set("Content-Type", "application/json")
	client := &nhttp.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("response not ok: " + resp.Status)
	}

	return nil
}

func get(ctx context.Context, url string) ([]byte, error) {
	req, err := nhttp.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("X-Custom-Header", "Akash")
	req.Header.Set("Content-Type", "application/json")
	client := &nhttp.Client{
		Timeout: 0,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, errors.New("response not ok: " + resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

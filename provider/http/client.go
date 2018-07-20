package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	nhttp "net/http"

	mutil "github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
)

func LeaseStatus(provider *types.Provider, leaseID types.LeaseID) (*types.LeaseStatusResponse, error) {
	resp, err := get(provider.GetHostURI() + "/lease/" + leaseID.String())
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

func SendManifest(manifest *types.Manifest, signer txutil.Signer, provider *types.Provider, deployment []byte) error {
	_, buf, err := mutil.SignManifest(manifest, signer, deployment)
	if err != nil {
		return err
	}
	return post(provider.GetHostURI()+"/manifest", buf)
}

// XXX assumes url is http/https
func post(url string, data []byte) error {
	req, err := nhttp.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
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

func get(url string) ([]byte, error) {
	req, err := nhttp.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
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

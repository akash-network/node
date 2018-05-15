package manifest

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/ovrclk/akash/txutil"
	"github.com/ovrclk/akash/types"
)

func Send(manifest *types.Manifest, signer txutil.Signer, provider *types.Provider, deployment []byte) error {
	mr := &types.ManifestRequest{
		Deployment: deployment,
		Manifest:   manifest,
	}

	buf, err := marshalRequest(mr)
	if err != nil {
		return err
	}
	return post(provider.GetHostURI(), buf)
}

// XXX assumes url is http/https
func post(url string, data []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("X-Custom-Header", "Akash")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
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

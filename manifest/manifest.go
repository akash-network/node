package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/ovrclk/akash/txutil"
	"gopkg.in/yaml.v2"
)

type Manifest struct {
	Image string
}

type Package struct {
	Manifest Manifest
	Lease    []byte
}

type Body struct {
	Package
	Signature []byte
}

func (mani *Manifest) Parse(file string) error {
	contents, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal([]byte(contents), mani)
	if err != nil {
		return err
	}

	return nil
}

// encrypt manifest with provider key, sign with tenant key, send to dest
func (mani *Manifest) Send(signer txutil.Signer, lease, provider []byte, dest string) error {
	pack := &Package{
		Manifest: *mani,
		Lease:    lease,
	}

	// encode package
	encPack, err := json.Marshal(mani)
	if err != nil {
		fmt.Println("ERROR: marshal manifest to json")
		return err
	}

	// sign the encoded manifest with tenant key
	sig, _, err := signer.SignBytes(encPack)
	if err != nil {
		fmt.Println("ERROR: sign manifest")
		return err
	}

	body := &Body{
		Package:   *pack,
		Signature: sig.Bytes(),
	}

	// encode body
	encodBod, err := json.Marshal(body)
	if err != nil {
		fmt.Println("ERROR: marshal package to json")
		return err
	}

	// XXX encrypt package with providers RSA key
	encryBod := encodBod

	// post secured package to provider
	return post(encryBod, dest)
}

// XXX assumes url is http/https
func post(data []byte, url string) error {
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

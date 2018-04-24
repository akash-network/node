package manifest

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/ovrclk/akash/txutil"
	"gopkg.in/yaml.v2"
	// crypto "github.com/tendermint/go-crypto"
)

type Manifest struct {
	image string
}

type Package struct {
	data      []byte
	signature []byte
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
func (mani *Manifest) Send(signer txutil.Signer, provider []byte, dest string) error {
	// encode manifest
	encMani, err := json.Marshal(mani)
	if err != nil {
		return err
	}

	// sign the encoded manifest with tenant key
	sig, _, err := signer.SignBytes(encMani)
	if err != nil {
		return err
	}

	pack := &Package{
		data:      encMani,
		signature: sig.Bytes(),
	}

	// encode package
	encPack, err := json.Marshal(pack)
	if err != nil {
		return err
	}

	fmt.Println("encPack %v", encPack)
	// encrypt package
	return nil
}

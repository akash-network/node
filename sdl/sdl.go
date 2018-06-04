package sdl

import (
	"io/ioutil"

	"github.com/ovrclk/akash/types"
	yaml "gopkg.in/yaml.v2"
)

type SDL interface {
	Validate() error
	DeploymentGroups() ([]*types.GroupSpec, error)
	Manifest() (*types.Manifest, error)
}

func ReadFile(path string) (SDL, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(buf)
}

func Read(buf []byte) (SDL, error) {
	// TODO: handle versions
	obj := &v1{}
	return obj, yaml.Unmarshal(buf, obj)
}
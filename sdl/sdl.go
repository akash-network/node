package sdl

import (
	"io/ioutil"

	"github.com/ovrclk/akash/types"
	"github.com/ovrclk/akash/validation"
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
	if err := yaml.Unmarshal(buf, obj); err != nil {
		return nil, err
	}

	if err := obj.Validate(); err != nil {
		return nil, err
	}

	dgroups, err := obj.DeploymentGroups()
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateGroupSpecs(dgroups); err != nil {
		return nil, err
	}

	m, err := obj.Manifest()
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateManifest(m); err != nil {
		return nil, err
	}
	if err := validation.ValidateManifestWithGroupSpecs(m, dgroups); err != nil {
		return nil, err
	}

	return obj, nil
}

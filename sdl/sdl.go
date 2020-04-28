package sdl

import (
	"io/ioutil"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
	yaml "gopkg.in/yaml.v2"
)

// SDL is the interface which wraps Validate, Deployment and Manifest methods
type SDL interface {
	Validate() error
	DeploymentGroups() ([]*dtypes.GroupSpec, error)
	Manifest() (manifest.Manifest, error)
}

// ReadFile read from given path and returns SDL instance
func ReadFile(path string) (SDL, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Read(buf)
}

// Read reads buffer data and returns SDL instance
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

	vgroups := make([]dtypes.GroupSpec, 0, len(dgroups))
	for _, dgroup := range dgroups {
		vgroups = append(vgroups, *dgroup)
	}

	if err := validation.ValidateDeploymentGroups(vgroups); err != nil {
		return nil, err
	}

	m, err := obj.Manifest()
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateManifest(m); err != nil {
		return nil, err
	}
	// if err := validation.ValidateManifestWithGroupSpecs(m, dgroups); err != nil {
	// 	return nil, err
	// }

	return obj, nil
}

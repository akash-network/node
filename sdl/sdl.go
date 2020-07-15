package sdl

import (
	"crypto/sha256"
	"encoding/json"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
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
	// TODO: handle versions; read 'version' field and switch
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

	// TODO: Determine if worth repairing ValidateManifest; functionality is commented out
	if err := validation.ValidateManifest(m); err != nil {
		return nil, err
	}
	// if err := validation.ValidateManifestWithGroupSpecs(m, dgroups); err != nil {
	// 	return nil, err
	// }

	return obj, nil
}

// Version creates the deterministic Deployment Version hash from the SDL.
// Sha256 returns 32 byte  sum of the SDL.
func Version(s SDL) ([]byte, error) {
	manifest, err := s.Manifest()
	if err != nil {
		return nil, err
	}

	m, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	sortedBytes, err := sdk.SortJSON(m)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(sortedBytes)
	return []byte(sum[:]), nil
}

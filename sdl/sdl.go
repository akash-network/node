package sdl

import (
	"crypto/sha256"
	"encoding/json"
	"io/ioutil"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/ovrclk/akash/manifest"
	"github.com/ovrclk/akash/validation"
	dtypes "github.com/ovrclk/akash/x/deployment/types"
)

var (
	errUninitializedConfig = errors.New("uninitialized config")
)

// SDL is the interface which wraps Validate, Deployment and Manifest methods
type SDL interface {
	DeploymentGroups() ([]*dtypes.GroupSpec, error)
	Manifest() (manifest.Manifest, error)
}

var _ SDL = (*sdl)(nil)

type sdl struct {
	Version semver.Version `yaml:"-"`
	data    SDL            `yaml:"-"`
}

func (s *sdl) UnmarshalYAML(node *yaml.Node) error {
	var result sdl

	for idx := range node.Content {
		if node.Content[idx].Value == "version" {
			var err error
			if result.Version, err = semver.ParseTolerant(node.Content[idx+1].Value); err != nil {
				return err
			}

			break
		}
	}

	// nolint: gocritic
	if result.Version.GE(semver.MustParse("2.0.0")) && result.Version.LT(semver.MustParse("3.0.0")) {
		var decoded v2
		if err := node.Decode(&decoded); err != nil {
			return err
		}

		result.data = &decoded
	} else {
		return errors.Errorf("config: unsupported version")
	}

	*s = result

	return nil
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
	obj := &sdl{}
	if err := yaml.Unmarshal(buf, obj); err != nil {
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
func Version(s SDL) ([]byte, error) {
	manifest, err := s.Manifest()
	if err != nil {
		return nil, err
	}
	return ManifestVersion(manifest)
}

// ManifestVersion calculates the identifying deterministic hash for an SDL.
// Sha256 returns 32 byte sum of the SDL.
func ManifestVersion(manifest manifest.Manifest) ([]byte, error) {
	m, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	sortedBytes, err := sdk.SortJSON(m)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(sortedBytes)

	return sum[:], nil
}

func (s *sdl) DeploymentGroups() ([]*dtypes.GroupSpec, error) {
	if s.data == nil {
		return []*dtypes.GroupSpec{}, errUninitializedConfig
	}

	return s.data.DeploymentGroups()
}

func (s *sdl) Manifest() (manifest.Manifest, error) {
	if s.data == nil {
		return manifest.Manifest{}, errUninitializedConfig
	}

	return s.data.Manifest()
}

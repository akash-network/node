package sdl

import (
	"errors"
	"fmt"
	"os"

	"github.com/blang/semver/v4"
	"gopkg.in/yaml.v3"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
	dtypes "github.com/akash-network/akash-api/go/node/deployment/v1beta3"
)

const (
	sdlVersionField = "version"
)

var (
	errUninitializedConfig = errors.New("sdl: uninitialized")
	errSDLInvalidNoVersion = fmt.Errorf("%w: no version found", errSDLInvalid)
)

// SDL is the interface which wraps Validate, Deployment and Manifest methods
type SDL interface {
	DeploymentGroups() (dtypes.GroupSpecs, error)
	Manifest() (manifest.Manifest, error)
	Version() ([]byte, error)
	validate() error
}

var _ SDL = (*sdl)(nil)

type sdl struct {
	Ver  semver.Version `yaml:"version,-"`
	data SDL            `yaml:"-"`
}

func (s *sdl) UnmarshalYAML(node *yaml.Node) error {
	var result sdl

	foundVersion := false
	for idx := range node.Content {
		if node.Content[idx].Value == sdlVersionField {
			var err error
			if result.Ver, err = semver.ParseTolerant(node.Content[idx+1].Value); err != nil {
				return err
			}
			foundVersion = true
			break
		}
	}

	if !foundVersion {
		return errSDLInvalidNoVersion
	}

	// nolint: gocritic
	if result.Ver.EQ(semver.MustParse("2.0.0")) {
		var decoded v2
		if err := node.Decode(&decoded); err != nil {
			return err
		}

		result.data = &decoded
	} else if result.Ver.GE(semver.MustParse("2.1.0")) {
		var decoded v2_1
		if err := node.Decode(&decoded); err != nil {
			return err
		}

		result.data = &decoded
	} else {
		return fmt.Errorf("%w: config: unsupported version %q", errSDLInvalid, result.Ver)
	}

	*s = result

	return nil
}

// ReadFile read from given path and returns SDL instance
func ReadFile(path string) (SDL, error) {
	buf, err := os.ReadFile(path)
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

	if err := obj.validate(); err != nil {
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

	if err := dtypes.ValidateDeploymentGroups(vgroups); err != nil {
		return nil, err
	}

	m, err := obj.Manifest()
	if err != nil {
		return nil, err
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	return obj, nil
}

// Version creates the deterministic Deployment Version hash from the SDL.
func (s *sdl) Version() ([]byte, error) {
	if s.data == nil {
		return nil, errUninitializedConfig
	}

	return s.data.Version()
}

func (s *sdl) DeploymentGroups() (dtypes.GroupSpecs, error) {
	if s.data == nil {
		return dtypes.GroupSpecs{}, errUninitializedConfig
	}

	return s.data.DeploymentGroups()
}

func (s *sdl) Manifest() (manifest.Manifest, error) {
	if s.data == nil {
		return manifest.Manifest{}, errUninitializedConfig
	}

	return s.data.Manifest()
}

func (s *sdl) validate() error {
	if s.data == nil {
		return errUninitializedConfig
	}

	return s.data.validate()
}

package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	ptypes "pkg.akt.dev/go/node/provider/v1beta4"
	tattr "pkg.akt.dev/go/node/types/attributes/v1"
)

var (
	ErrDuplicatedAttribute = errors.New("provider: duplicated attribute")
)

// Config is the struct that stores provider config
type Config struct {
	Host       string           `json:"host" yaml:"host"`
	Info       ptypes.Info      `json:"info" yaml:"info"`
	Attributes tattr.Attributes `json:"attributes" yaml:"attributes"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() tattr.Attributes {
	return c.Attributes
}

// ReadConfigPath reads and parses file
func ReadConfigPath(path string) (Config, error) {
	buf, err := os.ReadFile(path) //nolint: gosec
	if err != nil {
		return Config{}, err
	}
	var val Config
	if err := yaml.Unmarshal(buf, &val); err != nil {
		return Config{}, err
	}

	dups := make(map[string]string)
	for _, attr := range val.Attributes {
		if _, exists := dups[attr.Key]; exists {
			return Config{}, fmt.Errorf("%w: %s", ErrDuplicatedAttribute, attr.Key)
		}

		dups[attr.Key] = attr.Value
	}

	return val, err
}

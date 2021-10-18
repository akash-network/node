package config

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	types "github.com/ovrclk/akash/types/v1beta2"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

var (
	ErrDuplicatedAttribute = errors.New("provider: duplicated attribute")
)

// Config is the struct that stores provider config
type Config struct {
	Host       string              `json:"host" yaml:"host"`
	Info       ptypes.ProviderInfo `json:"info" yaml:"info"`
	Attributes types.Attributes    `json:"attributes" yaml:"attributes"`
	JWTHost    string              `json:"jwt-host" yaml:"jwt-host"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() types.Attributes {
	return c.Attributes
}

// ReadConfigPath reads and parses file
func ReadConfigPath(path string) (Config, error) {
	buf, err := ioutil.ReadFile(path)
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
			return Config{}, errors.Wrapf(ErrDuplicatedAttribute, attr.Key)
		}

		dups[attr.Key] = attr.Value
	}

	return val, err
}

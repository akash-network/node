package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v3"

	types "github.com/ovrclk/akash/types/v1beta2"
	ptypes "github.com/ovrclk/akash/x/provider/types/v1beta2"
)

// Config is the struct that stores provider config
type Config struct {
	Host       string              `json:"host" yaml:"host"`
	Info       ptypes.ProviderInfo `json:"info" yaml:"info"`
	Attributes []types.Attribute   `json:"attributes" yaml:"attributes"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() []types.Attribute {
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
	return val, err
}

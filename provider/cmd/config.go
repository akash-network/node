package cmd

import (
	"io/ioutil"

	ptypes "github.com/ovrclk/akash/x/provider/types"

	"gopkg.in/yaml.v2"
)

// Config is the struct that stores provider daemon config
type Config struct {
	Attributes ptypes.Attributes `json:"attributes" yaml:"attributes"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() ptypes.Attributes {
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

	err = val.Attributes.Validate()
	if err := yaml.Unmarshal(buf, &val); err != nil {
		return Config{}, err
	}

	return val, err
}

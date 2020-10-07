package config

import (
	"io/ioutil"

	"gopkg.in/yaml.v3"

	"github.com/ovrclk/akash/types"
)

// Config is the struct that stores provider config
type Config struct {
	Host       string            `json:"host"`
	Attributes []types.Attribute `json:"attributes"`
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

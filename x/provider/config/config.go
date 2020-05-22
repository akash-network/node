package config

import (
	"io/ioutil"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"gopkg.in/yaml.v2"
)

// Config is the struct that stores provider config
type Config struct {
	Host       string          `json:"host"`
	Attributes []sdk.Attribute `json:"attributes"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() []sdk.Attribute {
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

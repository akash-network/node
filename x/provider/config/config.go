package config

import (
	"io/ioutil"

	tmkv "github.com/tendermint/tendermint/libs/kv"
	"gopkg.in/yaml.v2"
)

// Config is the struct that stores provider config
type Config struct {
	Host       string `json:"host"`
	Attributes []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"attributes"`
}

// GetAttributes returns config attributes into key value pairs
func (c Config) GetAttributes() []tmkv.Pair {
	pairs := make([]tmkv.Pair, 0, len(c.Attributes))
	for _, attr := range c.Attributes {
		pairs = append(pairs, tmkv.Pair{
			Key:   []byte(attr.Key),
			Value: []byte(attr.Value),
		})
	}
	return pairs
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

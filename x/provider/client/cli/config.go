package cli

import (
	"io/ioutil"

	tmkv "github.com/tendermint/tendermint/libs/kv"
	"gopkg.in/yaml.v2"
)

type config struct {
	Host       string `json:"host"`
	Attributes []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"attributes"`
}

func (c config) getAttributes() []tmkv.Pair {
	pairs := make([]tmkv.Pair, 0, len(c.Attributes))
	for _, attr := range c.Attributes {
		pairs = append(pairs, tmkv.Pair{
			Key:   []byte(attr.Key),
			Value: []byte(attr.Value),
		})
	}
	return pairs
}

func readConfigPath(path string) (config, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var val config
	if err := yaml.Unmarshal(buf, &val); err != nil {
		return config{}, err
	}
	return val, err
}

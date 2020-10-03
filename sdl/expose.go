package sdl

import (
	"net/url"

	"gopkg.in/yaml.v3"
)

type v2Accept struct {
	Items []string `yaml:"items,omitempty"`
}

func (p *v2Accept) UnmarshalYAML(node *yaml.Node) error {
	var accept []string
	if err := node.Decode(&accept); err != nil {
		return err
	}

	for _, item := range accept {
		if _, err := url.ParseRequestURI("http://" + item); err != nil {
			return err
		}
	}

	p.Items = accept
	return nil
}

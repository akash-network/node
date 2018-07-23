package sdl

import "net/url"

func (p *v1Accept) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var accept []string
	if err := unmarshal(&accept); err != nil {
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

package sdl

import "github.com/ovrclk/akash/denom"

func (p *v1PricingProfile) UnmarshalYAML(unmarshal func(interface{}) error) error {

	var sval string
	if err := unmarshal(&sval); err != nil {
		return err
	}
	val, err := denom.ToBase(sval)
	if err != nil {
		return err
	}
	p.Value = val
	return nil
}

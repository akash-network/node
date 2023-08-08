package sdl

import (
	"net/url"
	"sort"

	manifest "github.com/akash-network/akash-api/go/manifest/v2beta2"
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

func (sdl v2Exposes) toManifestExpose(endpointNames map[string]uint32) (manifest.ServiceExposes, error) {
	exposeCount := 0
	for _, expose := range sdl {
		if len(expose.To) > 0 {
			exposeCount += len(expose.To)
		} else {
			exposeCount++
		}
	}

	res := make(manifest.ServiceExposes, 0, exposeCount)

	for _, expose := range sdl {
		exp, err := expose.toManifestExposes(endpointNames)
		if err != nil {
			return nil, err
		}

		res = append(res, exp...)
	}

	sort.Sort(res)

	return res, nil
}

func (sdl v2Expose) toManifestExposes(endpointNames map[string]uint32) (manifest.ServiceExposes, error) {
	exposeCount := len(sdl.To)
	if exposeCount == 0 {
		exposeCount = 1
	}

	res := make(manifest.ServiceExposes, 0, exposeCount)

	proto, err := manifest.ParseServiceProtocol(sdl.Proto)
	if err != nil {
		return nil, err
	}

	httpOptions, err := sdl.HTTPOptions.asManifest()
	if err != nil {
		return nil, err
	}

	if len(sdl.To) > 0 {
		for _, to := range sdl.To {
			// This value is created just so it can be passed to the utility function
			expose := manifest.ServiceExpose{
				Service:      to.Service,
				Port:         sdl.Port,
				ExternalPort: sdl.As,
				Proto:        proto,
				Global:       to.Global,
				Hosts:        sdl.Accept.Items,
				HTTPOptions:  httpOptions,
				IP:           to.IP,
			}

			// Check to see if an IP endpoint is also specified
			if expose.Global && len(expose.IP) != 0 {
				seqNo := endpointNames[expose.IP]
				expose.EndpointSequenceNumber = seqNo
			}

			res = append(res, expose)
		}
	} else {
		expose := manifest.ServiceExpose{
			Service:      "",
			Port:         sdl.Port,
			ExternalPort: sdl.As,
			Proto:        proto,
			Global:       false,
			Hosts:        sdl.Accept.Items,
			HTTPOptions:  httpOptions,
			IP:           "",
		}

		res = append(res, expose)
	}

	return res, nil
}

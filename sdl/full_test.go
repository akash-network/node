package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestFull(t *testing.T) {
	stream := `
version: "2.0"
services:
  web:
    image: quay.io/ovrclk/demo-app
    expose:
    - port: 80
      as: 80
      accept:
        - hello.localhost
      to:
        - global: true
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.1
          attributes:
            arch: amd64
        memory:
          size: 16Mi
        storage:
          size: 128Mi
          attributes:
            storage-class: ssd
  placement:
    westcoast:
      attributes:
        region: us-west
      pricing:
        web:
          amount: 1
          denom: akt
deployment:
  web:
    westcoast:
      profile: web
      count: 1
`

	var sdl v2

	err := yaml.Unmarshal([]byte(stream), &sdl)
	require.NoError(t, err)
}

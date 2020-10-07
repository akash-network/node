package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSdlVersion(t *testing.T) {
	stream := `
version: v2.0
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
`

	var sdl sdl

	err := yaml.Unmarshal([]byte(stream), &sdl)
	require.NoError(t, err)
}

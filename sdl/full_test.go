package sdl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFull(t *testing.T) {
	stream := `
version: "2.0"
services:
  web:
    image: ghcr.io/akash-network/demo-app
    expose:
    - port: 80
      as: 80
      accept:
        - hello.localhost
      to:
        - global: true
    params:
      storage:
        data:
          mount: "/var/lib/demo-app/data"
profiles:
  compute:
    web:
      resources:
        cpu:
          units: 0.1
          attributes:
            arch: amd64
        gpu:
          units: 1
          attributes:
            vendor:
              nvidia:
                - model: a100
        memory:
          size: 16Mi
        storage:
          - size: 128Mi
          - name: data
            size: 1Gi
            attributes:
              persistent: true
              class: default
  placement:
    westcoast:
      attributes:
        region: us-west
      pricing:
        web:
          amount: 1
          denom: uakt
deployment:
  web:
    westcoast:
      profile: web
      count: 1
`

	_, err := Read([]byte(stream))
	require.NoError(t, err)
}

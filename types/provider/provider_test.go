package provider_test

import (
	"testing"

	"github.com/ovrclk/akash/types/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	prov := provider.Provider{}
	ppath := "../../_docs/provider.yml"

	require.NoError(t, prov.Parse(ppath))

	assert.Equal(t, "http://localhost:3001", prov.HostURI)

	assert.Len(t, prov.Attributes, 2)
}

package bidengine

import (
	"testing"

	"github.com/ovrclk/akash/types"
	"github.com/stretchr/testify/assert"
)

func TestMatchProviderAttributes(t *testing.T) {
	tests := []struct {
		message string
		pattrs  []types.ProviderAttribute
		reqs    []types.ProviderAttribute
		match   bool
	}{

		{
			"both empty set",
			[]types.ProviderAttribute{},
			[]types.ProviderAttribute{},
			true,
		},

		{
			"requirements empty",
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			[]types.ProviderAttribute{},
			true,
		},

		{
			"provider empty",
			[]types.ProviderAttribute{},
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			false,
		},

		{
			"non-empty equal",
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			true,
		},

		{
			"non-empty non-equal",
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			[]types.ProviderAttribute{{Name: "a", Value: "a"}},
			false,
		},

		{
			"non-overlap",
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			[]types.ProviderAttribute{{Name: "b", Value: "B"}},
			false,
		},

		{
			"provider extra",
			[]types.ProviderAttribute{{Name: "a", Value: "A"}, {Name: "b", Value: "B"}},
			[]types.ProviderAttribute{{Name: "a", Value: "A"}},
			true,
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.match, matchProviderAttributes(test.pattrs, test.reqs), test.message)
	}

}

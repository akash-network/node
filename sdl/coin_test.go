package sdl

import (
	"github.com/stretchr/testify/require"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestPricing(t *testing.T) {
	lessThanOne, err := sdk.NewDecFromStr("0.7")
	require.NoError(t, err)
	tests := []struct {
		text  string
		value sdk.DecCoin
		err   bool
	}{
		{"amount: 1\ndenom: uakt", sdk.NewDecCoin("uakt", sdk.NewInt(1)), false},
		{"amount: -1\ndenom: uakt", sdk.NewDecCoin("uakt", sdk.NewInt(1)), true},
		{"amount: 0.7\ndenom: uakt", sdk.NewDecCoinFromDec("uakt", lessThanOne), false},
		{"amount: -0.7\ndenom: uakt", sdk.NewDecCoin("uakt", sdk.NewInt(0)), true},
	}

	for idx, test := range tests {
		buf := []byte(test.text)
		obj := &v2Coin{}

		err := yaml.Unmarshal(buf, obj)

		if test.err {
			assert.Error(t, err, "idx:%v text:`%v`", idx, test.text)
			continue
		}

		if !assert.NoError(t, err, "idx:%v text:`%v`", idx, test.text) {
			continue
		}

		assert.Equal(t, test.value, obj.Value, "idx:%v text:`%v`", idx, test.text)
	}
}

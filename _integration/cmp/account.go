package cmp

import (
	"fmt"

	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
	"github.com/ovrclk/gestalt/exec/js"
)

func accountBalance(key key, amount int64) gestalt.Component {
	parse := js.Do(js.Int(amount, "balance"))

	return akash("account-balance",
		"query", "account", key.addr.Var()).
		FN(parse).
		WithMeta(g.Require(key.addr.Name()))
}

func accountSendTo(from key, to key, amount int64) gestalt.Component {
	value := fmt.Sprintf("%0.06f", float64(amount)/float64(1000000))
	return akash("send-to",
		"send", value, to.addr.Var(), "-k", from.name.Name()).
		WithMeta(g.Require(to.addr.Name()))
}

func groupAccountSend(key key) gestalt.Component {
	start := int64(1000000000000000)
	amount := int64(100)
	other := newKey("other")
	return g.Group("account-send").
		Run(groupKey(other)).
		Run(accountBalance(key, start)).
		Run(accountSendTo(key, other, amount)).
		Run(g.Retry(5).
			Run(accountBalance(key, start-amount))).
		Run(accountBalance(other, amount))
}

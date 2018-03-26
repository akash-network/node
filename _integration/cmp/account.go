package cmp

import (
	"strconv"

	"github.com/ovrclk/akash/_integration/js"
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func AccountBalance(key key, amount int) gestalt.Component {
	return Akash("query", "account", key.addr.Var()).
		FN(js.PathEQInt(amount, "balance"))
}

func AccountSendTo(from key, to key, amount int) gestalt.Component {
	return Akash("send", strconv.Itoa(amount), to.addr.Var(), "-k", from.name.Name())
}

func GroupAccountSend(key key) gestalt.Component {
	start := 1000000000
	amount := 100
	other := newKey("other")
	return g.Group("account-send").
		Run(GroupKeyCreate(other)).
		Run(AccountBalance(key, start)).
		Run(AccountSendTo(key, other, amount)).
		Run(g.Retry(5).
			Run(AccountBalance(key, start-amount))).
		Run(AccountBalance(other, amount))
}

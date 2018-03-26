package cmp

import (
	"github.com/ovrclk/gestalt"
	g "github.com/ovrclk/gestalt/builder"
)

func AccountBalance(key key, amount int) gestalt.Component {
	// check account balance
	return g.Group("account-balance")
}

func AccountSendTo(from key, to key, amount int) gestalt.Component {
	// send `amount` from `from` to `to`
	return g.Group("account-send")
}

func GroupAccountSend(key key) gestalt.Component {
	other := newKey("other")
	return g.Group("account-send").
		Run(GroupKeyCreate(other)).
		Run(AccountBalance(key, 10000)).
		Run(AccountSendTo(key, other, 100)).
		Run(AccountBalance(key, 10000-100)).
		Run(AccountBalance(other, 100))
}

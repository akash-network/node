//go:build e2e.upgrade

package upgrade

import (
	_ "github.com/akash-network/node/tests/upgrade/v0.26.0"
	_ "github.com/akash-network/node/tests/upgrade/v0.32.0"
	_ "github.com/akash-network/node/tests/upgrade/v0.34.0"
	_ "github.com/akash-network/node/tests/upgrade/v0.36.0"
)

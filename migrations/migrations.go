package migrations

import (
	// ensure init functions called for all migrations
	// nolint: revive
	_ "github.com/akash-network/node/migrations/v0.15.0"
	// nolint: revive
	_ "github.com/akash-network/node/migrations/v0.24.0"
)

//go:build e2e.upgrade

package upgrade

import (
	"context"
	"testing"

	uttypes "pkg.akt.dev/node/tests/upgrade/types"
)

func init() {
	uttypes.RegisterPostUpgradeWorker("v1.1.0", &postUpgrade{})
}

type postUpgrade struct{}

var _ uttypes.TestWorker = (*postUpgrade)(nil)

func (pu *postUpgrade) Run(ctx context.Context, t *testing.T, params uttypes.TestParams) {

}

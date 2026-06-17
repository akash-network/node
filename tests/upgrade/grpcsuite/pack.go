package grpcsuite

// Pack is a per-module group of ordered cases. Packs run in slice order so that
// earlier packs (cert, provider, deployment) can set up the on-chain state that
// later packs (market, escrow, audit) consume, via the shared World.
type Pack interface {
	// Name is the pack's display name and subtest name.
	Name() string
	// Available reports whether the pack's module is served by the running binary.
	// Packs for modules absent on the branch under test (e.g. verification on main)
	// return false and are skipped.
	Available(d *Discovery) bool
	// Run executes the pack's cases against s.
	Run(s *Suite)
}

// packs is the ordered list of module packs the suite runs. Order encodes data
// dependencies. New packs are appended here as they are authored.
var packs = []Pack{
	deploymentPack{},
	providerPack{},
	govParamsPack{},
}

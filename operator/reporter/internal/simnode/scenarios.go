package simnode

// Scenarios returns the named scenarios that goldens and benchmarks serve, each built fresh by its function.
func Scenarios() map[string]func() Scenario {
	return map[string]func() Scenario{}
}

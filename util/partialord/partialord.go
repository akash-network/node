package partialord

import (
	"sort"

	"pkg.akt.dev/node/util/partialord/internal/dag"
)

type PartialOrdering struct {
	// underlying dag, the partial ordering is stored via a dag
	// https://en.wikipedia.org/wiki/Topological_sorting#Relation_to_partial_orders
	dag dag.DAG
	// bools for sealing, to prevent repeated invocation of first or last methods.
	firstSealed bool
	lastSealed  bool
}

// NewPartialOrdering creates a new partial ordering over the set of provided elements.
func NewPartialOrdering(elements []string) PartialOrdering {
	elementsCopy := make([]string, len(elements))
	copy(elementsCopy, elements)
	sort.Strings(elementsCopy)
	return PartialOrdering{
		dag:         dag.NewDAG(elementsCopy),
		firstSealed: false,
		lastSealed:  false,
	}
}

func handleDAGErr(err error) {
	// all dag errors are logic errors that the intended users of this package should not make.
	if err != nil {
		panic(err)
	}
}

// After marks that A should come after B
func (ord *PartialOrdering) After(a string, b string) {
	// Set that A depends on B / an edge from B -> A
	err := ord.dag.AddEdge(b, a)
	handleDAGErr(err)
}

// Before marks that A should come before B
func (ord *PartialOrdering) Before(a string, b string) {
	// Set that B depends on A / an edge from A -> B
	err := ord.dag.AddEdge(a, b)
	handleDAGErr(err)
}

// FirstElements Sets elems to be the first elements in the ordering.
// So if were making an ordering over {A, B, C, D, E}, and elems provided is {D, B, A}
// then we are guaranteed that the total ordering will begin with {D, B, A}
func (ord *PartialOrdering) FirstElements(elems ...string) {
	if ord.firstSealed {
		panic("FirstElements has already been called")
	}
	// We make every node in the dag have a dependency on elems[-1]
	// then we change elems[-1] to depend on elems[-2], and so forth.
	err := ord.dag.AddFirstElements(elems...)
	handleDAGErr(err)
	ord.firstSealed = true
}

// LastElements Sets elems to be the last elements in the ordering.
// So if were making an ordering over {A, B, C, D, E}, and elems provided is {D, B, A}
// then we are guaranteed that the total ordering will end with {D, B, A}
func (ord *PartialOrdering) LastElements(elems ...string) {
	if ord.lastSealed {
		panic("FirstElements has already been called")
	}
	// We make every node in the dag have a dependency on elems[0]
	// then we make elems[1] depend on elems[0], and so forth.
	err := ord.dag.AddLastElements(elems...)
	handleDAGErr(err)
	ord.lastSealed = true
}

// Sequence sets a sequence of ordering constraints.
// So if were making an ordering over {A, B, C, D, E}, and elems provided is {D, B, A}
// then we are guaranteed that the total ordering will have D comes before B comes before A.
// (They're may be elements interspersed, e.g. {D, C, E, B, A} is a valid ordering)
func (ord *PartialOrdering) Sequence(seq ...string) {
	// We make every node in the sequence have a prior node
	for i := 0; i < (len(seq) - 1); i++ {
		err := ord.dag.AddEdge(seq[i], seq[i+1])
		handleDAGErr(err)
	}
}

// TotalOrdering returns a deterministically chosen total ordering that satisfies all specified
// partial ordering constraints.
//
// Panics if no total ordering exists.
func (ord *PartialOrdering) TotalOrdering() []string {
	return ord.dag.TopologicalSort()
}

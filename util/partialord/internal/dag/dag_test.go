package dag_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"pkg.akt.dev/node/util/partialord/internal/dag"
)

type edge struct {
	start, end string
}

func TestTopologicalSort(t *testing.T) {
	// Tests that topological sort works for various inputs.
	// We hardcode the satisfying solution in the tests, even though it suffices
	// to check that the partial ordering is sufficient. (and that's the only guarantee given externally)
	// This is to ensure we catch differences in order between changes, and across machines.
	simpleNodes := []string{"dog", "cat", "banana", "apple"}
	simpleNodesRev := []string{"apple", "banana", "cat", "dog"}
	tests := []struct {
		nodes                    []string
		edges                    []edge
		expectedTopologicalOrder []string
	}{
		{
			// alphabetical ordering of simple nodes
			nodes:                    simpleNodes,
			edges:                    []edge{{"banana", "apple"}, {"cat", "banana"}, {"dog", "cat"}},
			expectedTopologicalOrder: simpleNodes,
		},
		{
			// apple > dog
			nodes:                    simpleNodes,
			edges:                    []edge{{"apple", "dog"}},
			expectedTopologicalOrder: []string{"cat", "banana", "apple", "dog"},
		},
		{
			// apple > everything
			nodes:                    simpleNodes,
			edges:                    []edge{{"apple", "banana"}, {"apple", "cat"}, {"apple", "dog"}},
			expectedTopologicalOrder: []string{"apple", "dog", "cat", "banana"},
		},
		{
			// apple > everything, on list with reversed initial order
			nodes:                    simpleNodesRev,
			edges:                    []edge{{"apple", "banana"}, {"apple", "cat"}, {"apple", "dog"}},
			expectedTopologicalOrder: []string{"apple", "banana", "cat", "dog"},
		},
	}
	for _, tc := range tests {
		dag := dag.NewDAG(tc.nodes)
		for _, edge := range tc.edges {
			err := dag.AddEdge(edge.start, edge.end)
			require.NoError(t, err)
		}
		order := dag.TopologicalSort()
		require.Equal(t, tc.expectedTopologicalOrder, order)
	}
}

func TestAddFirst(t *testing.T) {
	simpleNodes := []string{"frog", "elephant", "dog", "cat", "banana", "apple"}
	tests := []struct {
		nodes                    []string
		first                    []string
		replaceEdges             []edge
		expectedTopologicalOrder []string
	}{
		{
			nodes:                    simpleNodes,
			first:                    []string{"frog"},
			replaceEdges:             []edge{{"banana", "apple"}, {"cat", "banana"}, {"dog", "cat"}},
			expectedTopologicalOrder: simpleNodes,
		},
		{
			nodes:                    simpleNodes,
			first:                    []string{"elephant"},
			replaceEdges:             []edge{{"banana", "apple"}, {"apple", "frog"}, {"dog", "cat"}},
			expectedTopologicalOrder: []string{"elephant", "dog", "banana", "cat", "apple", "frog"},
		},
		{
			nodes:                    simpleNodes,
			first:                    []string{"elephant", "frog"},
			replaceEdges:             []edge{},
			expectedTopologicalOrder: []string{"elephant", "frog", "dog", "cat", "banana", "apple"},
		},
		{
			// add three items in first, if implemented incorrectly could cause a cycle
			nodes:                    simpleNodes,
			first:                    []string{"dog", "elephant", "frog"},
			replaceEdges:             []edge{},
			expectedTopologicalOrder: []string{"dog", "elephant", "frog", "cat", "banana", "apple"},
		},
	}
	for _, tc := range tests {
		dag := dag.NewDAG(tc.nodes)
		err := dag.AddFirstElements(tc.first...)
		require.NoError(t, err)

		for _, edge := range tc.replaceEdges {
			err := dag.ReplaceEdge(edge.start, edge.end)
			require.NoError(t, err)
		}
		order := dag.TopologicalSort()
		require.Equal(t, tc.expectedTopologicalOrder, order)
	}
}

func TestAddLast(t *testing.T) {
	simpleNodes := []string{"frog", "elephant", "dog", "cat", "banana", "apple"}
	tests := []struct {
		nodes                    []string
		last                     []string
		replaceEdges             []edge
		expectedTopologicalOrder []string
	}{
		{
			// causes no order change
			nodes:                    simpleNodes,
			last:                     []string{"apple"},
			replaceEdges:             []edge{{"banana", "apple"}, {"cat", "banana"}, {"dog", "cat"}},
			expectedTopologicalOrder: simpleNodes,
		},
		{
			nodes:                    simpleNodes,
			last:                     []string{"elephant"},
			replaceEdges:             []edge{{"banana", "apple"}, {"apple", "frog"}, {"dog", "cat"}},
			expectedTopologicalOrder: []string{"dog", "banana", "cat", "apple", "frog", "elephant"},
		},
		{
			nodes:                    simpleNodes,
			last:                     []string{"elephant", "frog"},
			replaceEdges:             []edge{},
			expectedTopologicalOrder: []string{"dog", "cat", "banana", "apple", "elephant", "frog"},
		},
		{
			// add three items in last, if implemented incorrectly could cause a cycle
			nodes:                    simpleNodes,
			last:                     []string{"dog", "elephant", "frog"},
			replaceEdges:             []edge{},
			expectedTopologicalOrder: []string{"cat", "banana", "apple", "dog", "elephant", "frog"},
		},
	}
	for _, tc := range tests {
		dag := dag.NewDAG(tc.nodes)
		err := dag.AddLastElements(tc.last...)
		require.NoError(t, err)

		for _, edge := range tc.replaceEdges {
			err := dag.ReplaceEdge(edge.start, edge.end)
			require.NoError(t, err)
		}
		order := dag.TopologicalSort()
		require.Equal(t, tc.expectedTopologicalOrder, order)
	}
}

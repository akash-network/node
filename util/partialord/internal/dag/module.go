// Package dag implements a simple Directed Acyclical Graph (DAG) for deterministic topological sorts
//
// It should not be externally exposed, and is intended to be a very simple dag implementation
// utilizing adjacency lists to store edges.
//
// This package is intended to be used for small scales, where performance of the algorithms is not critical.
// (e.g. sub 10k entries)
// Thus none of the algorithms in here are benchmarked, and just have correctness checks.
package dag

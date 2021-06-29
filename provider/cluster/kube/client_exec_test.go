package kube

import (
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"testing"
)

func TestSortablePodsSorting(t *testing.T) {
	v := sortablePods{
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "z"}},
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "a"}},
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "b"}},
	}

	sort.Sort(v)

	require.Equal(t, "a", v[0].Name)
	require.Equal(t, "b", v[1].Name)
	require.Equal(t, "z", v[2].Name)
}

func TestSortablePodsAlreadySorted(t *testing.T) {
	v := sortablePods{
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "a"}},
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "b"}},
		corev1.Pod{ObjectMeta: v1.ObjectMeta{Name: "c"}},
	}

	sort.Sort(v)

	require.Equal(t, "a", v[0].Name)
	require.Equal(t, "b", v[1].Name)
	require.Equal(t, "c", v[2].Name)
}

func TestExecResultImpl(t *testing.T) {
	v := execResult{exitCode: 133}

	require.Equal(t, 133, v.ExitCode())
}

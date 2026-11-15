package relayer

import (
	"os"
	"path/filepath"
	"testing"
)

func testRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("repository root not found")
		}
		root = next
	}
}

func durableTestDirectory(t *testing.T) string {
	t.Helper()
	root := os.Getenv("RELAYER_TEST_CACHE")
	if root != "" && !filepath.IsAbs(root) {
		t.Fatal("RELAYER_TEST_CACHE must be an absolute test-only scratch path")
	}
	if root == "" {
		root = filepath.Join(testRepositoryRoot(t), ".cache", "relayer-go-tests")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(root, "case-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Error(err)
		}
	})
	return path
}

func TestTestPathsStayUnderRepositoryOrExplicitCache(t *testing.T) {
	fixture := durableTestDirectory(t)
	if err := os.WriteFile(filepath.Join(fixture, "go.mod"), []byte("module test.fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(fixture, "internal", "relayer")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv("RELAYER_TEST_CACHE", "")
	if got := testRepositoryRoot(t); got != fixture {
		t.Fatalf("repository root: got %q, want %q", got, fixture)
	}
	path := durableTestDirectory(t)
	expected := filepath.Join(fixture, ".cache", "relayer-go-tests")
	if !filepath.IsAbs(path) || filepath.Dir(path) != expected {
		t.Fatalf("default scratch outside repository cache: %q", path)
	}

	override := filepath.Join(fixture, "explicit-cache")
	t.Setenv("RELAYER_TEST_CACHE", override)
	path = durableTestDirectory(t)
	if !filepath.IsAbs(path) || filepath.Dir(path) != override {
		t.Fatalf("explicit scratch override not honored: %q", path)
	}
}

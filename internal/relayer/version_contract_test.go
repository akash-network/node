package relayer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalledVersionMatchesPackagedBuildInfo(t *testing.T) {
	root := durableTestDirectory(t)
	want := "service akt-relayer\nversion 0.2.0\ncommit packaged-commit\nplatform linux_amd64\nabc123  bin/akt-relayer\n"
	if err := os.WriteFile(filepath.Join(root, "service-build-info.txt"), []byte(want), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	handled, err := runOffline(command{name: "version"}, BuildInfo{
		Version:  "0.2.0",
		Commit:   "packaged-commit",
		Platform: "linux_amd64",
	}, Streams{Out: &output}, root)
	if err != nil || !handled || output.String() != want {
		t.Fatalf("installed version differs from packaged build info: handled=%v err=%v output=%q", handled, err, output.String())
	}
}

func TestSourceVersionWithoutPackageMetadata(t *testing.T) {
	var output bytes.Buffer
	handled, err := runOffline(command{name: "version"}, BuildInfo{
		Version:  "dev",
		Commit:   "source-commit",
		Platform: "darwin_arm64",
	}, Streams{Out: &output}, durableTestDirectory(t))
	want := "service akt-relayer\nversion dev\ncommit source-commit\nplatform darwin_arm64\n"
	if err != nil || !handled || output.String() != want {
		t.Fatalf("source version: handled=%v err=%v output=%q", handled, err, output.String())
	}
}

func TestInstalledVersionReadFailureIsNotHidden(t *testing.T) {
	root := durableTestDirectory(t)
	if err := os.Mkdir(filepath.Join(root, "service-build-info.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	handled, err := runOffline(command{name: "version"}, BuildInfo{}, Streams{Out: &output}, root)
	if err == nil || !handled || output.Len() != 0 {
		t.Fatalf("unreadable package metadata accepted: handled=%v err=%v output=%q", handled, err, output.String())
	}
}

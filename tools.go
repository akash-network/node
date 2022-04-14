//go:build tools
// +build tools

package tools

// nolint
import (
	_ "github.com/regen-network/cosmos-proto/protoc-gen-gocosmos"
	_ "k8s.io/code-generator"
	_ "sigs.k8s.io/kind"
)

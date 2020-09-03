// +build tools

package tools

//nolint
import (
	_ "github.com/vektra/mockery"
	_ "golang.org/x/tools/cmd/stringer"
	_ "k8s.io/code-generator"
	_ "sigs.k8s.io/kind"
	_ "github.com/regen-network/cosmos-proto/protoc-gen-gocosmos"
)

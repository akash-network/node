// +build tools

package tools

//nolint
import (
	_ "github.com/gogo/protobuf/protoc-gen-gogo"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway"
	_ "github.com/grpc-ecosystem/grpc-gateway/protoc-gen-swagger"
	_ "github.com/vektra/mockery"
)

// Package kmspb holds the Cosmos-KMS SignerService gRPC client, generated from
// signerservice.proto.
package kmspb

//go:generate protoc --go_out=. --go_opt=paths=source_relative --go_opt=Msignerservice.proto=pkg.akt.dev/node/v2/operator/reporter/signer/kmspb;kmspb --go-grpc_out=. --go-grpc_opt=paths=source_relative --go-grpc_opt=Msignerservice.proto=pkg.akt.dev/node/v2/operator/reporter/signer/kmspb;kmspb signerservice.proto

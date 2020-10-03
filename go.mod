module github.com/ovrclk/akash

go 1.15

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/boz/go-lifecycle v0.1.1-0.20190620234137-5139c86739b8
	github.com/caarlos0/env v3.3.0+incompatible
	github.com/cosmos/cosmos-sdk v0.34.4-0.20200928165114-e17dd4fd5008
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.2-0.20190904063534-ff6b7dc882cf // indirect
	github.com/google/uuid v1.1.2-0.20190416172445-c2e93f3ae59f // indirect
	github.com/gorilla/context v1.1.1
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/grpc-ecosystem/grpc-gateway v1.15.0
	github.com/hashicorp/hcl v1.0.1-0.20191016231534-914dc3f8dd7c // indirect
	github.com/jmhodges/levigo v1.0.1-0.20191019112844-b572e7f4cdac // indirect
	github.com/libp2p/go-buffer-pool v0.0.3-0.20190619091711-d94255cb3dfc // indirect
	github.com/lithammer/shortuuid v1.0.1-0.20190319200910-1be5ab5d90f6
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rakyll/statik v0.1.7
	github.com/regen-network/cosmos-proto v0.3.0
	github.com/rs/cors v1.7.1-0.20191011001009-dcbccb712443 // indirect
	github.com/spf13/cast v1.3.1
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/objx v0.2.1-0.20190415111823-35313a95ee26 // indirect
	github.com/stretchr/testify v1.6.1
	github.com/subosito/gotenv v1.2.1-0.20190917103637-de67a6614a4d // indirect
	github.com/tendermint/tendermint v0.34.0-rc4
	github.com/tendermint/tm-db v0.6.2
	github.com/vektra/mockery v1.1.2
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/tools v0.0.0-20200323144430-8dcfad9e016e
	google.golang.org/appengine v1.6.6-0.20191016204603-16bce7d3dc4e // indirect
	google.golang.org/genproto v0.0.0-20200825200019-8632dd797987
	google.golang.org/grpc v1.32.0
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	k8s.io/api v0.18.2
	k8s.io/apimachinery v0.18.2
	k8s.io/client-go v0.18.2
	k8s.io/code-generator v0.18.2
	k8s.io/metrics v0.18.2
	sigs.k8s.io/kind v0.8.1
)

replace github.com/keybase/go-keychain => github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.2-alpha.regen.4

replace github.com/cosmos/cosmos-sdk => github.com/cosmos/cosmos-sdk v0.34.4-0.20200928165114-e17dd4fd5008

replace github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.14.7

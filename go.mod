module github.com/ovrclk/akash

go 1.15

require (
	github.com/avast/retry-go v2.7.0+incompatible
	github.com/blang/semver v3.5.1+incompatible
	github.com/boz/go-lifecycle v0.1.1-0.20190620234137-5139c86739b8
	github.com/cosmos/cosmos-sdk v0.41.3
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.3
	github.com/golang/protobuf v1.4.3
	github.com/gorilla/context v1.1.1
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/websocket v1.4.2
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/hashicorp/hcl v1.0.1-0.20191016231534-914dc3f8dd7c // indirect
	github.com/jmhodges/levigo v1.0.1-0.20191019112844-b572e7f4cdac // indirect
	github.com/libp2p/go-buffer-pool v0.0.3-0.20190619091711-d94255cb3dfc // indirect
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rakyll/statik v0.1.7
	github.com/regen-network/cosmos-proto v0.3.1
	github.com/rs/cors v1.7.1-0.20191011001009-dcbccb712443 // indirect
	github.com/rs/zerolog v1.20.0
	github.com/satori/go.uuid v1.2.0
	github.com/spf13/cast v1.3.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.1
	github.com/stretchr/objx v0.2.1-0.20190415111823-35313a95ee26 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/subosito/gotenv v1.2.1-0.20190917103637-de67a6614a4d // indirect
	github.com/tendermint/tendermint v0.34.7
	github.com/tendermint/tm-db v0.6.4
	github.com/vektra/mockery v1.1.2
	golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys v0.0.0-20210113181707-4bcb84eeeb78 // indirect
	golang.org/x/tools v0.0.0-20210106214847-113979e3529a
	google.golang.org/appengine v1.6.6-0.20191016204603-16bce7d3dc4e // indirect
	google.golang.org/genproto v0.0.0-20210114201628-6edceaf6022f
	google.golang.org/grpc v1.35.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.19.3
	k8s.io/code-generator v0.19.3
	k8s.io/metrics v0.19.3
	sigs.k8s.io/kind v0.10.0
)

replace github.com/keybase/go-keychain => github.com/99designs/go-keychain v0.0.0-20191008050251-8e49817e8af4

replace github.com/gogo/protobuf => github.com/regen-network/protobuf v1.3.3-alpha.regen.1

replace google.golang.org/grpc => google.golang.org/grpc v1.33.2

replace github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.14.7

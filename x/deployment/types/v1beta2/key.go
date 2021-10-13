package v1beta2

const (
	// ModuleName is the module name constant used in many places
	ModuleName = "deployment"

	// StoreKey is the store key string for deployment
	StoreKey = ModuleName

	// RouterKey is the message route for deployment
	RouterKey = ModuleName
)

func DeploymentPrefix() []byte {
	return []byte{0x01}
}

func GroupPrefix() []byte {
	return []byte{0x02}
}

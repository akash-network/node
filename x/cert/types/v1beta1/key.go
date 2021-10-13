package v1beta1

const (
	// ModuleName is the module name constant used in many places
	ModuleName = "cert"

	// StoreKey is the store key string for provider
	StoreKey = ModuleName

	// RouterKey is the message route for provider
	RouterKey = ModuleName
)

func PrefixCertificateID() []byte {
	return []byte{0x01}
}

package constants

const (

	// should be for every command
	FlagRootDir = "data"
	FlagNode    = "node"
	DefaultNode = "http://localhost:46657"

	// all non-query commands / actual transactions
	FlagNonce = "nonce"

	// only commands which need private key / signing
	FlagKey = "key"
	KeyDir  = "keys"
	Codec   = "english"

	// all key types should be standardized
	FlagKeyType = "type"

	// todo: interactive.
	Password = "0123456789"
)

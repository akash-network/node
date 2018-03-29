package constants

const (

	// should be for every command
	FlagRootDir = "data"
	FlagNode    = "node"

	// all non-query commands / actual transactions
	FlagNonce = "nonce"

	// only commands which need private key / signing
	FlagKey = "key"
	KeyDir  = "keys"
	Codec   = "english"

	// all key types should be standardized
	FlagKeyType = "type"

	// market commands
	FlagWait = "wait"

	// todo: interactive.
	Password = "0123456789"
)

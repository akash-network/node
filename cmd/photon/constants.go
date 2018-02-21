package main

const (

	// should be for every command
	flagRootDir = "data"
	flagNode    = "node"
	defaultNode = "http://localhost:46657"

	// all non-query commands / actual transactions
	flagNonce = "nonce"

	// only commands which need private key / signing
	flagKey = "key"
	keyDir  = "keys"
	codec   = "english"

	// all key types should be standardized
	flagKeyType = "type"

	// todo: interactive.
	password = "0123456789"
)

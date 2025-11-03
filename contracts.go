package node

import (
	"embed"
)

const ContractsDir = ".cache/cosmwasm/artifacts"

//go:embed .cache/cosmwasm/artifacts/*.wasm
var Contracts embed.FS

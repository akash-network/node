package common

import "os"

func DefaultCLIHome() string {
	return os.ExpandEnv("$HOME/.akash")
}

func DefaultNodeHome() string {
	return os.ExpandEnv("$HOME/.akashd")
}

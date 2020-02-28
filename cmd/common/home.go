package common

import "os"

// DefaultCLIHome default home directories for the application CLI
func DefaultCLIHome() string {
	return os.ExpandEnv("$HOME/.akash")
}

// DefaultNodeHome default home directories for the application daemon
func DefaultNodeHome() string {
	return os.ExpandEnv("$HOME/.akashd")
}

package version

import "github.com/ovrclk/akash/types"

var (
	version = "master"
	commit  = ""
	date    = ""
)

func Get() types.AkashVersion {
	return types.AkashVersion{
		Version: version,
		Commit:  commit,
		Date:    date,
	}
}

func Version() string {
	return version
}

func Commit() string {
	return commit
}

func Date() string {
	return date
}

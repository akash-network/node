package cmd

import "os"

// SetConfigWriter makes w write reporter.toml until restore runs.
func SetConfigWriter(w func(*os.File, []byte) (int, error)) (restore func()) {
	prev := writeFileData
	writeFileData = w

	return func() { writeFileData = prev }
}

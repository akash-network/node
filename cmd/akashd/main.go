package main

import "os"

func main() {
	root := baseCommand()
	root.AddCommand(initCommand())
	root.AddCommand(startCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

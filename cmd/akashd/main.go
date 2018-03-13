package main

func main() {
	root := baseCommand()
	root.AddCommand(initCommand())
	root.AddCommand(startCommand())
	root.Execute()
}

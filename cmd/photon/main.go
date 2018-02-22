package main

func main() {
	root := baseCommand()
	root.AddCommand(keyCommand())
	root.AddCommand(sendCommand())
	root.AddCommand(queryCommand())
	root.AddCommand(deploymentCommand())
	root.Execute()
}

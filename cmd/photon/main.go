package main

func main() {
	root := baseCommand()
	root.AddCommand(keyCommand())
	root.AddCommand(sendCommand())
	root.Execute()
}

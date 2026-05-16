package cli

func isHelpRequest(args []string) bool {
	return len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")
}

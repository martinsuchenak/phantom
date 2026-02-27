package commands

// resolveBaseDir checks if the given input is a registered project name.
// If it is, it returns the registered absolute path.
// Otherwise, it returns the input as is, treating it as a normal path.
func resolveBaseDir(input string) string {
	if cfg == nil || cfg.Projects == nil {
		return input
	}

	if projectPath, ok := cfg.Projects[input]; ok {
		return projectPath
	}

	return input
}

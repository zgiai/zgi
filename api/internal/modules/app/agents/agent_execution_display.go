package agents

import (
	"path"
	"strings"
)

// Only classifications enter the durable trace. Commands, package names and
// output bodies remain private model input and are never copied into this view.
func agentShellExecutionDisplay(command, stdout, stderr string, job agentExecutionJob) map[string]interface{} {
	runtime := "shell"
	words := strings.Fields(command)
	if len(words) > 0 {
		switch path.Base(words[0]) {
		case "python", "python3", "python3.13":
			runtime = "python"
		case "node", "nodejs":
			runtime = "node"
		case "pip", "pip3", "npm", "pnpm", "yarn", "apt", "apt-get":
			runtime = "package_manager"
		}
	}
	diagnostic := ""
	if job.Status == "failed" || (job.ExitCode != nil && *job.ExitCode != 0) {
		switch {
		case strings.Contains(stderr, "ModuleNotFoundError:") && strings.Contains(stderr, "No module named"):
			diagnostic = "missing_python_dependency"
		case strings.Contains(stderr, "Cannot find module") || strings.Contains(stderr, "ERR_MODULE_NOT_FOUND"):
			diagnostic = "missing_node_dependency"
		case strings.Contains(stderr, ": not found") || strings.Contains(stderr, ": command not found"):
			diagnostic = "command_not_found"
		}
	}
	return map[string]interface{}{
		"runtime": runtime, "diagnostic": diagnostic,
		"has_stdout": strings.TrimSpace(stdout) != "", "has_stderr": strings.TrimSpace(stderr) != "",
	}
}

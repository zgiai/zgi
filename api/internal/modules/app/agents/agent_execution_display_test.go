package agents

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentShellDisplayClassifiesWithoutDisclosingBodies(t *testing.T) {
	exitCode := 1
	for _, test := range []struct{ command, stderr, runtime, diagnostic string }{
		{"python3 /workspace/private.py --token=secret", "ModuleNotFoundError: No module named 'private'", "python", "missing_python_dependency"},
		{"node private.js", "Error: Cannot find module 'private'", "node", "missing_node_dependency"},
		{"node private.js", "ERR_MODULE_NOT_FOUND private", "node", "missing_node_dependency"},
		{"private-command --password=secret", "/bin/sh: private-command: not found", "shell", "command_not_found"},
		{"npm install private", "private error", "package_manager", ""},
		{"echo private", "private error", "shell", ""},
	} {
		display := agentShellExecutionDisplay(test.command, "private output", test.stderr, agentExecutionJob{Status: "failed", ExitCode: &exitCode})
		if display["runtime"] != test.runtime || display["diagnostic"] != test.diagnostic || display["has_stdout"] != true || display["has_stderr"] != true {
			t.Fatalf("display = %#v, want runtime %q diagnostic %q", display, test.runtime, test.diagnostic)
		}
		encoded, err := json.Marshal(display)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
			t.Fatalf("display leaked input: %s", encoded)
		}
	}
	exitCode = 0
	display := agentShellExecutionDisplay("python3 test.py", "", "ModuleNotFoundError: No module named 'private'", agentExecutionJob{Status: "succeeded", ExitCode: &exitCode})
	if display["diagnostic"] != "" {
		t.Fatalf("successful stderr treated as failure: %#v", display)
	}
}

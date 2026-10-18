package platformcli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const ExitAwaitingApproval = 75

// Run emits bounded JSON on stdout and safe diagnostics on stderr. A pending
// approval exits 75 so a Shell Job need not wait indefinitely for user input.
func Run(ctx context.Context, client *Client, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		fmt.Fprintln(stdout, "zgi file read --id ID --ref REF --path PATH\nzgi file export --id ID --path PATH\nzgi model call --id ID --prompt TEXT --max-tokens N\nzgi tool call --id ID --name TOOL --arguments JSON\nzgi request status --id ID\nExit 75 requires console continuation; do not retry mutations with a new ID.")
		return 0
	}
	request, statusOnly, err := parseArguments(args)
	if err != nil {
		fmt.Fprintln(stderr, "invalid arguments; use zgi file read|export, model call, tool call, or request status with --id")
		return 2
	}
	if client == nil {
		fmt.Fprintln(stderr, "platform CLI is unavailable for this Job")
		return 1
	}
	var response agentexecution.PlatformResponse
	if statusOnly {
		response, err = client.Status(ctx, request.ID)
	} else {
		response, err = client.Call(ctx, request)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(response); err != nil {
		fmt.Fprintln(stderr, "could not write platform CLI response")
		return 1
	}
	switch response.Status {
	case "completed":
		return 0
	case "awaiting_approval", "queued", "running":
		return ExitAwaitingApproval
	default:
		return 1
	}
}

func parseArguments(args []string) (agentexecution.PlatformRequest, bool, error) {
	invalid := fmt.Errorf("invalid platform CLI arguments")
	if len(args) < 2 {
		return agentexecution.PlatformRequest{}, false, invalid
	}
	flags := flag.NewFlagSet("zgi", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "stable operation ID")
	var action string
	var buildArguments func() map[string]any
	statusOnly := false
	switch strings.Join(args[:2], " ") {
	case "file read":
		action = "file.read"
		ref := flags.String("ref", "", "authorized input or Skill reference")
		path := flags.String("path", "", "relative workspace destination")
		buildArguments = func() map[string]any { return map[string]any{"ref": *ref, "path": *path} }
	case "file export":
		action = "file.export"
		path := flags.String("path", "", "relative result path")
		buildArguments = func() map[string]any { return map[string]any{"path": *path} }
	case "model call":
		action = "model.call"
		prompt := flags.String("prompt", "", "model input")
		maxTokens := flags.Int("max-tokens", 1024, "output limit")
		buildArguments = func() map[string]any { return map[string]any{"prompt": *prompt, "max_tokens": *maxTokens} }
	case "tool call":
		action = "tool.call"
		name := flags.String("name", "", "bound tool name")
		arguments := flags.String("arguments", "{}", "tool JSON object")
		buildArguments = func() map[string]any {
			var object map[string]json.RawMessage
			if json.Unmarshal([]byte(*arguments), &object) != nil || object == nil {
				return nil
			}
			return map[string]any{"name": *name, "arguments": object}
		}
	case "request status":
		statusOnly = true
	default:
		return agentexecution.PlatformRequest{}, false, invalid
	}
	if flags.Parse(args[2:]) != nil || flags.NArg() != 0 || strings.TrimSpace(*id) == "" {
		return agentexecution.PlatformRequest{}, false, invalid
	}
	request := agentexecution.PlatformRequest{ID: *id, Action: action}
	if !statusOnly {
		arguments := buildArguments()
		if arguments == nil {
			return agentexecution.PlatformRequest{}, false, invalid
		}
		request.Arguments, _ = json.Marshal(arguments)
	}
	return request, statusOnly, nil
}

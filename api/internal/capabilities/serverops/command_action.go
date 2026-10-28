// This file implements the command run action (ADR 0025): one shell command,
// written by the caller, runs as the SSH user and returns its exit code and
// bounded output.
//
// Unlike every other action, the command is free-form. It is executed exactly
// as supplied by the remote login shell of the SSH user, so it can read or
// change anything that user may. The capability does not classify, filter, or
// rewrite it; the boundary is the server account's own permissions and the
// approver who sees the exact command. A command that needs root writes
// "sudo -n" itself; the connection privilege does not apply to it.
package serverops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// ActionCommandRun runs one caller-written shell command.
const ActionCommandRun = "server.command.run"

const (
	// maxCommandBytes bounds one submitted command.
	maxCommandBytes = 8192
	// commandRunTimeout is how long one command may run. The approval that
	// runs it waits in one HTTP request, so a longer job is started detached
	// on the server and checked with later commands.
	commandRunTimeout = 45 * time.Second
	// maxCommandOutputRunes bounds each returned output stream so one command
	// cannot dominate the model context. It stays below the 16,000-character
	// output schema so redacting a short secret cannot overflow it.
	maxCommandOutputRunes = 15000
)

// CommandPattern accepts any command without a NUL byte. Its length is bounded
// separately by maxCommandBytes.
const CommandPattern = `^[^\x00]+$`

// CommandRun is the result of one command run. A non-zero exit is a result,
// not an error: ExitCode carries it, and Stdout and Stderr carry the bounded
// output.
type CommandRun struct {
	Attempt
	Command   string
	ExitCode  int
	Stdout    string
	Stderr    string
	Truncated bool
}

// commandFromInput validates the exactly-one-key input of the command run
// action. The command is returned exactly as supplied.
func commandFromInput(input map[string]any) (string, error) {
	if err := rejectUnknownInputKeys(input, "command"); err != nil {
		return "", err
	}
	raw, present := input["command"]
	if !present {
		return "", fmt.Errorf("%w: command is required", errReadActionInput)
	}
	command, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: command must be a string", errReadActionInput)
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("%w: command is blank", errReadActionInput)
	}
	if len(command) > maxCommandBytes {
		return "", fmt.Errorf("%w: command exceeds %d bytes", errReadActionInput, maxCommandBytes)
	}
	if !utf8.ValidString(command) {
		return "", fmt.Errorf("%w: command is not valid text", errReadActionInput)
	}
	// The approver must read exactly what runs: control characters other than
	// tab and newline, and invisible format characters such as bidirectional
	// overrides, could make the shown command differ from the executed one.
	for _, r := range command {
		if (unicode.IsControl(r) && r != '\t' && r != '\n') || unicode.In(r, unicode.Cf) {
			return "", fmt.Errorf("%w: command contains a control or format character", errReadActionInput)
		}
	}
	// A command that would destroy the server or cut off access never reaches
	// approval (ADR 0026).
	if ClassifyCommand(command) == CommandForbidden {
		return "", fmt.Errorf("%w: command is forbidden because it would destroy the server or cut off access to it", errReadActionInput)
	}
	return command, nil
}

// RunCommand runs one caller-written command as the SSH user. The command is
// validated before any credential is parsed or any factory is invoked. A
// non-zero exit returns a CommandRun with its exit code; a transport failure,
// a timeout, or a host-key mismatch returns the shared error classes.
func (s *Service) RunCommand(ctx context.Context, connection Connection, command string) (*CommandRun, error) {
	validated, err := commandFromInput(map[string]any{"command": command})
	if err != nil {
		return nil, newError(ErrInvalidInput, "", err)
	}
	runService := *s
	if runService.limits.CommandTimeout < commandRunTimeout {
		runService.limits.CommandTimeout = commandRunTimeout
	}
	executionRunner, requestID, err := runService.prepareRunner(connection)
	if err != nil {
		return nil, err
	}
	result, runErr := executionRunner.run(ctx, validated)
	exitCode := 0
	if runErr != nil {
		var exitErr *ssh.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, runError(requestID, runErr)
		}
		exitCode = exitErr.ExitStatus()
	}
	stdout, stdoutCut := boundedCommandOutput(result.Stdout)
	stderr, stderrCut := boundedCommandOutput(result.Stderr)
	return &CommandRun{
		Attempt:   Attempt{RequestID: requestID, AttemptCount: 1},
		Command:   validated,
		ExitCode:  exitCode,
		Stdout:    stdout,
		Stderr:    stderr,
		Truncated: result.Truncated || stdoutCut || stderrCut,
	}, nil
}

// boundedCommandOutput turns one output stream into valid text of at most
// maxCommandOutputRunes runes, keeping its end, where a failure usually is.
func boundedCommandOutput(output []byte) (string, bool) {
	text := strings.ToValidUTF8(string(output), "�")
	text = strings.ReplaceAll(text, "\x00", "")
	runes := []rune(text)
	if len(runes) <= maxCommandOutputRunes {
		return text, false
	}
	return string(runes[len(runes)-maxCommandOutputRunes:]), true
}

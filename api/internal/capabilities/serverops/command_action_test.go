package serverops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestRunCommandRunsTheCommandExactlyAsWritten(t *testing.T) {
	command := "df -h / && journalctl -u nginx --since '-10 min' | tail -n 5"
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: []byte("Filesystem Size\n"), Stderr: []byte("warning\n")}}
	result, err := newFakeService(t, factory).RunCommand(context.Background(), writeActionSudoConnection(), command)
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	runners := factory.runnersSnapshot()
	if len(runners) != 1 || runners[0].observedCommand() != command {
		t.Fatalf("the command was rewritten: %#v", runners)
	}
	if config := factory.configsSnapshot()[0]; config.CommandTimeout != commandRunTimeout {
		t.Fatalf("command timeout = %v, want %v", config.CommandTimeout, commandRunTimeout)
	}
	if result.Command != command || result.ExitCode != 0 || result.Stdout != "Filesystem Size\n" ||
		result.Stderr != "warning\n" || result.Truncated || result.RequestID != serviceTestRequestID {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCommandReturnsANonZeroExitAsAResult(t *testing.T) {
	factory := &serviceFakeFactory{
		result: sshRunResult{Stderr: []byte("ls: cannot access '/missing'")},
		runErr: fmt.Errorf("serverops ssh: remote command exited with status 2: %w", &ssh.ExitError{}),
	}
	result, err := newFakeService(t, factory).RunCommand(context.Background(), serviceTestConnection(), "ls /missing")
	if err != nil {
		t.Fatalf("a remote exit must be a result, got error %v", err)
	}
	if result.Stderr != "ls: cannot access '/missing'" {
		t.Fatalf("result = %#v", result)
	}
}

func TestRunCommandKeepsTransportFailuresAsErrors(t *testing.T) {
	factory := &serviceFakeFactory{runErr: fmt.Errorf("serverops ssh: run command: %w", context.DeadlineExceeded)}
	_, err := newFakeService(t, factory).RunCommand(context.Background(), serviceTestConnection(), "sleep 600")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
}

func TestRunCommandRejectsInvalidCommandsBeforeConnecting(t *testing.T) {
	for _, command := range []string{"", "   \n", "echo \x00", strings.Repeat("x", maxCommandBytes+1), "echo \xff", "echo ok\rrm -rf /", "echo ‮exe.sh"} {
		factory := &serviceFakeFactory{}
		_, err := newFakeService(t, factory).RunCommand(context.Background(), serviceTestConnection(), command)
		if !errors.Is(err, ErrInvalidInput) || factory.callCount() != 0 {
			t.Fatalf("command %q: error = %v, factory calls = %d", command, err, factory.callCount())
		}
	}
}

func TestCommandLineRendersTheCommandUnchanged(t *testing.T) {
	line, err := CommandLine(ActionCommandRun, map[string]any{"command": "sudo -n systemctl daemon-reload"}, PrivilegeNone)
	if err != nil || line != "sudo -n systemctl daemon-reload" {
		t.Fatalf("CommandLine() = %q, %v", line, err)
	}
	if _, err := CommandLine(ActionCommandRun, map[string]any{"command": "ls", "path": "/"}, PrivilegeNone); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("an undeclared key was accepted: %v", err)
	}
}

func TestBoundedCommandOutputKeepsTheEnd(t *testing.T) {
	output := strings.Repeat("a", maxCommandOutputRunes) + "END"
	bounded, cut := boundedCommandOutput([]byte(output))
	if !cut || !strings.HasSuffix(bounded, "END") || len([]rune(bounded)) != maxCommandOutputRunes {
		t.Fatalf("bounded output: cut=%v len=%d", cut, len([]rune(bounded)))
	}
}

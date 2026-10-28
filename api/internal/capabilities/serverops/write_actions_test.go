package serverops

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// writeActionCases is the complete write action table: each action id and its
// one fixed systemctl verb.
var writeActionCases = []struct {
	action string
	verb   string
}{
	{ActionServiceRestart, "restart"},
	{ActionServiceStart, "start"},
	{ActionServiceStop, "stop"},
	{ActionServiceReload, "reload"},
}

func writeActionSudoConnection() Connection {
	connection := serviceTestConnection()
	connection.Privilege = PrivilegeSudo
	return connection
}

func writeActionStateOutput(active, sub string) []byte {
	return []byte(serviceStatusHeader + "\n" +
		"LoadState=loaded\n" +
		"ActiveState=" + active + "\n" +
		"SubState=" + sub + "\n" +
		"Description=ZGI test unit\n")
}

func TestServiceWriteActionIdentitiesAreExact(t *testing.T) {
	for _, testCase := range writeActionCases {
		verb, ok := serviceChangeVerb(testCase.action)
		if !ok || verb != testCase.verb {
			t.Errorf("%s verb = (%q,%v), want %q", testCase.action, verb, ok, testCase.verb)
		}
		if !IsServiceWriteAction(testCase.action) {
			t.Errorf("%s must be a service write action", testCase.action)
		}
	}
	for _, action := range []string{
		"", ActionSystemSnapshot, ActionServiceStatus, ActionJournalTail,
		"server.service.enable", "server.service.disable", "server.service.mask", "server.service.kill",
		"restart", "server.service.restart ", "SERVER.SERVICE.RESTART",
	} {
		if IsServiceWriteAction(action) {
			t.Errorf("%q must not be a service write action", action)
		}
		if _, _, err := buildServiceChangeCommand(action, map[string]any{"service_name": "nginx.service"}, PrivilegeNone); !errors.Is(err, errReadActionInput) {
			t.Errorf("%q: build error = %v, want input rejection", action, err)
		}
	}
}

func TestServiceWriteCommandsAreExact(t *testing.T) {
	for _, testCase := range writeActionCases {
		for _, privilege := range []Privilege{PrivilegeNone, PrivilegeSudo} {
			command, serviceName, err := buildServiceChangeCommand(testCase.action, map[string]any{"service_name": "nginx.service"}, privilege)
			if err != nil {
				t.Fatalf("%s/%q: build error = %v", testCase.action, privilege, err)
			}
			if serviceName != "nginx.service" {
				t.Fatalf("%s: validated name = %q", testCase.action, serviceName)
			}
			prefix := ""
			if privilege == PrivilegeSudo {
				prefix = "sudo -n "
			}
			want := "LC_ALL=C\nexport LC_ALL\n" +
				prefix + "systemctl " + testCase.verb + " -- 'nginx.service' 1>&2 || exit $?\n" +
				"printf '" + serviceStatusHeader + "\\n'\n" +
				"systemctl show --no-pager --property=LoadState --property=ActiveState --property=SubState --property=Description -- 'nginx.service'\n"
			if command != want {
				t.Fatalf("%s/%q command =\n%q\nwant\n%q", testCase.action, privilege, command, want)
			}
			// The state read after the change is exactly the status read, and it
			// never runs with sudo.
			statusCommand, _, _ := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service"})
			if !strings.HasSuffix(command, strings.TrimPrefix(statusCommand, "LC_ALL=C\nexport LC_ALL\n")) {
				t.Fatalf("%s: state read differs from the status read", testCase.action)
			}
			if strings.Count(command, "sudo") != strings.Count(prefix, "sudo") {
				t.Fatalf("%s/%q: sudo appears outside the change line: %q", testCase.action, privilege, command)
			}
		}
	}
}

func TestServiceWriteCommandTokensAreFixed(t *testing.T) {
	// Only the fixed words of the script may appear as tokens; the unit name is
	// the single dynamic token, so no input can add a word.
	allowed := map[string]bool{
		"LC_ALL=C": true, "export": true, "LC_ALL": true, "sudo": true, "-n": true,
		"systemctl": true, "--": true, "1>": true, "2": true, "exit": true, "$?": true,
		"printf": true, serviceStatusHeader + "\\n": true, serviceStatusHeader + "n": true,
		"show": true, "--no-pager": true, "--property=LoadState": true, "--property=ActiveState": true,
		"--property=SubState": true, "--property=Description": true, "nginx@a:b_c-d.e.service": true,
	}
	for _, testCase := range writeActionCases {
		command, _, err := buildServiceChangeCommand(testCase.action, map[string]any{"service_name": "nginx@a:b_c-d.e.service"}, PrivilegeSudo)
		if err != nil {
			t.Fatalf("%s: %v", testCase.action, err)
		}
		for _, token := range readActionShellTokens(command) {
			if token == testCase.verb {
				continue
			}
			if !allowed[token] {
				t.Errorf("%s: unexpected command token %q", testCase.action, token)
			}
		}
		// No other systemctl verb may appear.
		for _, other := range writeActionCases {
			if other.verb != testCase.verb && strings.Contains(command, " "+other.verb+" ") {
				t.Errorf("%s: command carries the %q verb", testCase.action, other.verb)
			}
		}
	}
}

func TestServiceWriteCommandsRejectInvalidInputAndPrivilege(t *testing.T) {
	inputs := []map[string]any{
		nil,
		{},
		{"service_name": ""},
		{"service_name": 7},
		{"service_name": "nginx"},
		{"service_name": "nginx.service; reboot"},
		{"service_name": "nginx.service'"},
		{"service_name": "$(reboot).service"},
		{"service_name": " nginx.service"},
		{"service_name": "../nginx.service"},
		{"service_name": "nginx.service", "verb": "enable"},
		{"service_name": "nginx.service", "command": "reboot"},
		{"service_name": "nginx.service", "sudo": true},
	}
	for _, testCase := range writeActionCases {
		for _, input := range inputs {
			if command, _, err := buildServiceChangeCommand(testCase.action, input, PrivilegeNone); err == nil || command != "" {
				t.Errorf("%s accepted %#v: %q", testCase.action, input, command)
			}
		}
		for _, privilege := range []Privilege{"root", "SUDO", " sudo", "sudo ", "doas", "sudo -S"} {
			if _, _, err := buildServiceChangeCommand(testCase.action, map[string]any{"service_name": "nginx.service"}, privilege); err == nil {
				t.Errorf("%s accepted privilege %q", testCase.action, privilege)
			}
		}
	}
}

func TestChangeServiceRunsFixedCommandAndReturnsState(t *testing.T) {
	for _, testCase := range writeActionCases {
		for _, connection := range []Connection{serviceTestConnection(), writeActionSudoConnection()} {
			factory := &serviceFakeFactory{result: sshRunResult{Stdout: writeActionStateOutput("active", "running")}}
			service := newFakeService(t, factory)

			result, err := service.ChangeService(context.Background(), connection, testCase.action, "nginx.service")
			if err != nil {
				t.Fatalf("%s/%q: ChangeService() error = %v", testCase.action, connection.Privilege, err)
			}
			want := &ServiceChange{
				Attempt:     Attempt{RequestID: serviceTestRequestID, AttemptCount: 1},
				Action:      testCase.action,
				ServiceName: "nginx.service",
				LoadState:   "loaded",
				ActiveState: "active",
				SubState:    "running",
			}
			if !reflect.DeepEqual(result, want) {
				t.Fatalf("%s: result = %#v, want %#v", testCase.action, result, want)
			}
			wantCommand, _, err := buildServiceChangeCommand(testCase.action, map[string]any{"service_name": "nginx.service"}, connection.Privilege)
			if err != nil {
				t.Fatal(err)
			}
			runners := factory.runnersSnapshot()
			if len(runners) != 1 || runners[0].callCount() != 1 || runners[0].observedCommand() != wantCommand {
				t.Fatalf("%s: runner usage = %#v", testCase.action, runners)
			}
			hasSudo := strings.HasPrefix(strings.TrimPrefix(runners[0].observedCommand(), "LC_ALL=C\nexport LC_ALL\n"), "sudo -n systemctl "+testCase.verb+" -- ")
			if hasSudo != (connection.Privilege == PrivilegeSudo) {
				t.Fatalf("%s: sudo prefix = %v for privilege %q", testCase.action, hasSudo, connection.Privilege)
			}
		}
	}
}

func TestReadActionsNeverUseSudo(t *testing.T) {
	connection := writeActionSudoConnection()
	snapshotFactory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	if _, err := newFakeService(t, snapshotFactory).SystemSnapshot(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	statusFactory := &serviceFakeFactory{result: sshRunResult{Stdout: readActionServiceStatusOutput()}}
	if _, err := newFakeService(t, statusFactory).ServiceStatus(context.Background(), connection, "nginx.service"); err != nil {
		t.Fatal(err)
	}
	journalFactory := &serviceFakeFactory{result: sshRunResult{Stdout: []byte("line\n")}}
	if _, err := newFakeService(t, journalFactory).JournalTail(context.Background(), connection, "nginx.service", JournalOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, factory := range []*serviceFakeFactory{snapshotFactory, statusFactory, journalFactory} {
		runners := factory.runnersSnapshot()
		if len(runners) != 1 || strings.Contains(runners[0].observedCommand(), "sudo") {
			t.Fatalf("read action ran with sudo: %#v", runners)
		}
	}
}

func TestChangeServiceRejectsBeforeFactory(t *testing.T) {
	cases := []struct {
		name       string
		connection Connection
		action     string
		service    string
		want       error
	}{
		{"read action id", serviceTestConnection(), ActionServiceStatus, "nginx.service", ErrInvalidInput},
		{"unknown action id", serviceTestConnection(), "server.service.enable", "nginx.service", ErrInvalidInput},
		{"bad unit", serviceTestConnection(), ActionServiceRestart, "nginx; reboot", ErrInvalidInput},
		{"bad privilege", func() Connection { c := serviceTestConnection(); c.Privilege = "root"; return c }(), ActionServiceRestart, "nginx.service", ErrConnectionInvalid},
		{"capture connection", serviceTestCaptureConnection(), ActionServiceStop, "nginx.service", ErrConnectionInvalid},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stdout: writeActionStateOutput("active", "running")}}
		_, err := newFakeService(t, factory).ChangeService(context.Background(), testCase.connection, testCase.action, testCase.service)
		if !errors.Is(err, testCase.want) {
			t.Errorf("%s: error = %v, want %v", testCase.name, err, testCase.want)
		}
		if factory.callCount() != 0 {
			t.Errorf("%s: factory calls = %d, want 0", testCase.name, factory.callCount())
		}
	}
}

func TestBadPrivilegeIsRejectedByEveryAction(t *testing.T) {
	connection := serviceTestConnection()
	connection.Privilege = "always"
	factory := &serviceFakeFactory{result: sshRunResult{Stdout: validSnapshotOutput()}}
	service := newFakeService(t, factory)
	if _, err := service.SystemSnapshot(context.Background(), connection); !errors.Is(err, ErrConnectionInvalid) {
		t.Errorf("snapshot error = %v", err)
	}
	if _, err := service.ValidateConnection(context.Background(), connection); !errors.Is(err, ErrConnectionInvalid) {
		t.Errorf("validate connection error = %v", err)
	}
	if factory.callCount() != 0 {
		t.Errorf("factory calls = %d, want 0", factory.callCount())
	}
}

func TestChangeServiceFailureCarriesBoundedRemoteText(t *testing.T) {
	stderr := "Failed to restart nginx.service: Unit nginx.service not found.\n\x1b[31mred\x00" + strings.Repeat("x", 2000)
	factory := &serviceFakeFactory{
		result: sshRunResult{Stderr: []byte(stderr)},
		runErr: fmt.Errorf("serverops ssh: remote command exited with status 5: %w", &ssh.ExitError{}),
	}
	service := newFakeService(t, factory)
	_, err := service.ChangeService(context.Background(), writeActionSudoConnection(), ActionServiceRestart, "nginx.service")
	if !errors.Is(err, ErrProviderRejected) {
		t.Fatalf("error = %v, want ErrProviderRejected", err)
	}
	var exitErr *ssh.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatal("the remote exit error must stay discoverable")
	}
	var capabilityErr *Error
	if !errors.As(err, &capabilityErr) || capabilityErr.RequestID != serviceTestRequestID || capabilityErr.AttemptCount != 1 {
		t.Fatalf("attempt metadata = %#v", capabilityErr)
	}
	// The public message stays generic and never echoes remote text.
	if message := err.Error(); message != "server operations provider rejected the request" {
		t.Fatalf("public message = %q", message)
	}
	cause := errors.Unwrap(err)
	if cause == nil || !errors.Is(cause, errServiceChangeRejected) {
		t.Fatalf("cause = %v", cause)
	}
	text := cause.Error()
	if !strings.Contains(text, "Failed to restart nginx.service: Unit nginx.service not found.") {
		t.Fatalf("cause lacks the remote error text: %q", text)
	}
	if strings.ContainsAny(text, "\n\x1b\x00") {
		t.Fatalf("cause carries control characters: %q", text)
	}
	if utf8.RuneCountInString(text) > maxRemoteErrorRunes+200 {
		t.Fatalf("cause is unbounded: %d runes", utf8.RuneCountInString(text))
	}
}

func TestChangeServiceNonExitFailuresKeepSharedClasses(t *testing.T) {
	cases := []struct {
		runErr error
		want   error
	}{
		{context.DeadlineExceeded, ErrTimeout},
		{fmt.Errorf("%w: presented key", errSSHHostKeyMismatch), ErrAuthInvalid},
		{errors.New("boom"), ErrUpstream},
	}
	for _, testCase := range cases {
		factory := &serviceFakeFactory{result: sshRunResult{Stderr: []byte("leak-probe-stderr")}, runErr: testCase.runErr}
		_, err := newFakeService(t, factory).ChangeService(context.Background(), serviceTestConnection(), ActionServiceStop, "nginx.service")
		if !errors.Is(err, testCase.want) {
			t.Errorf("%v: error = %v, want %v", testCase.runErr, err, testCase.want)
		}
		if strings.Contains(fmt.Sprint(errors.Unwrap(err)), "leak-probe-stderr") {
			t.Errorf("%v: a non-exit failure must not carry remote output", testCase.runErr)
		}
	}
}

func TestChangeServiceRejectsCorruptOrTruncatedState(t *testing.T) {
	cases := []sshRunResult{
		{Stdout: writeActionStateOutput("active", "running"), Truncated: true},
		{Stdout: []byte("restarted\n" + string(writeActionStateOutput("active", "running")))},
		{Stdout: []byte(serviceStatusHeader + "\nLoadState=loaded\n")},
		{Stdout: nil},
	}
	for index, result := range cases {
		factory := &serviceFakeFactory{result: result}
		_, err := newFakeService(t, factory).ChangeService(context.Background(), serviceTestConnection(), ActionServiceReload, "nginx.service")
		if !errors.Is(err, ErrResponseInvalid) {
			t.Errorf("case %d: error = %v, want ErrResponseInvalid", index, err)
		}
	}
}

func TestBoundedRemoteErrorText(t *testing.T) {
	cases := map[string]string{
		"":                               "no error output",
		"  \n\t ":                        "no error output",
		"sudo: a password is required\n": "sudo: a password is required",
		"a\r\nb\x1b[0mc":                 "a b [0mc",
		"bad\xffutf8":                    "bad\uFFFDutf8",
		"zero\u200bwidth":                "zero width",
	}
	for input, want := range cases {
		if got := boundedRemoteErrorText([]byte(input)); got != want {
			t.Errorf("boundedRemoteErrorText(%q) = %q, want %q", input, got, want)
		}
	}
	long := boundedRemoteErrorText([]byte(strings.Repeat("é", maxRemoteErrorRunes*3)))
	if utf8.RuneCountInString(long) != maxRemoteErrorRunes+3 || !strings.HasSuffix(long, "...") {
		t.Fatalf("long text = %d runes", utf8.RuneCountInString(long))
	}
}

func TestCommandLineRendersEveryAction(t *testing.T) {
	cases := []struct {
		action    string
		input     map[string]any
		privilege Privilege
		want      string
	}{
		{ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, PrivilegeSudo, "sudo -n systemctl restart -- 'nginx.service'"},
		{ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, PrivilegeNone, "systemctl restart -- 'nginx.service'"},
		{ActionServiceStart, map[string]any{"service_name": "app@1.service"}, PrivilegeSudo, "sudo -n systemctl start -- 'app@1.service'"},
		{ActionServiceStop, map[string]any{"service_name": "nginx.service"}, PrivilegeNone, "systemctl stop -- 'nginx.service'"},
		{ActionServiceReload, map[string]any{"service_name": "nginx.service"}, PrivilegeSudo, "sudo -n systemctl reload -- 'nginx.service'"},
		{ActionServiceStatus, map[string]any{"service_name": "nginx.service"}, PrivilegeSudo,
			"systemctl show --no-pager --property=LoadState --property=ActiveState --property=SubState --property=Description -- 'nginx.service'"},
		{ActionJournalTail, map[string]any{"service_name": "nginx.service"}, PrivilegeNone,
			"journalctl --no-pager --quiet --output=cat --unit='nginx.service' --lines=100 --since=-60min"},
		{ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": float64(20), "since_minutes": float64(15)}, PrivilegeSudo,
			"journalctl --no-pager --quiet --output=cat --unit='nginx.service' --lines=20 --since=-15min"},
		{ActionSystemSnapshot, nil, PrivilegeSudo, systemSnapshotCommandSummary},
		{ActionSystemSnapshot, map[string]any{}, PrivilegeNone, systemSnapshotCommandSummary},
	}
	for _, testCase := range cases {
		got, err := CommandLine(testCase.action, testCase.input, testCase.privilege)
		if err != nil || got != testCase.want {
			t.Errorf("CommandLine(%s, %#v, %q) = (%q, %v), want %q", testCase.action, testCase.input, testCase.privilege, got, err, testCase.want)
		}
	}
}

func TestCommandLineMatchesExecutedCommand(t *testing.T) {
	// The rendered line is a verbatim line of the executed script for every
	// action that takes input, so the preview cannot drift from what runs.
	for _, privilege := range []Privilege{PrivilegeNone, PrivilegeSudo} {
		for _, testCase := range writeActionCases {
			input := map[string]any{"service_name": "nginx.service"}
			line, err := CommandLine(testCase.action, input, privilege)
			if err != nil {
				t.Fatal(err)
			}
			command, _, err := buildServiceChangeCommand(testCase.action, input, privilege)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Split(command, "\n")[2] != line+" 1>&2 || exit $?" {
				t.Fatalf("%s: rendered %q does not match the executed change line in %q", testCase.action, line, command)
			}
		}
	}
	statusLine, _ := CommandLine(ActionServiceStatus, map[string]any{"service_name": "nginx.service"}, PrivilegeNone)
	statusCommand, _, _ := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service"})
	if !strings.Contains(statusCommand, "\n"+statusLine+"\n") {
		t.Fatalf("status line %q is not part of %q", statusLine, statusCommand)
	}
	journalInput := map[string]any{"service_name": "nginx.service", "lines": 7}
	journalLine, _ := CommandLine(ActionJournalTail, journalInput, PrivilegeNone)
	journalCommand, _, _ := buildJournalTailCommand(journalInput)
	if !strings.Contains(journalCommand, "\n"+journalLine+"\n") {
		t.Fatalf("journal line %q is not part of %q", journalLine, journalCommand)
	}
}

func TestCommandLineRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		action    string
		input     map[string]any
		privilege Privilege
	}{
		{"", map[string]any{"service_name": "nginx.service"}, PrivilegeNone},
		{"server.service.enable", map[string]any{"service_name": "nginx.service"}, PrivilegeNone},
		{ActionServiceRestart, map[string]any{"service_name": "nginx.service"}, "root"},
		{ActionServiceStatus, map[string]any{"service_name": "nginx.service"}, "root"},
		{ActionServiceRestart, map[string]any{"service_name": "nginx"}, PrivilegeSudo},
		{ActionServiceRestart, map[string]any{"service_name": "nginx.service", "extra": 1}, PrivilegeSudo},
		{ActionServiceRestart, nil, PrivilegeSudo},
		{ActionServiceStatus, map[string]any{"service_name": "nginx.service; reboot"}, PrivilegeNone},
		{ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": 1000}, PrivilegeNone},
		{ActionJournalTail, map[string]any{"service_name": "nginx.service", "lines": 1.5}, PrivilegeNone},
		{ActionSystemSnapshot, map[string]any{"service_name": "nginx.service"}, PrivilegeNone},
	}
	for _, testCase := range cases {
		line, err := CommandLine(testCase.action, testCase.input, testCase.privilege)
		if line != "" || !errors.Is(err, ErrInvalidInput) {
			t.Errorf("CommandLine(%q, %#v, %q) = (%q, %v), want an input rejection", testCase.action, testCase.input, testCase.privilege, line, err)
		}
	}
}

func TestChangeServicePermissionRefusalIsItsOwnClass(t *testing.T) {
	for _, stderr := range []string{
		"sudo: a password is required",
		"Sorry, user ops is not allowed to execute '/usr/bin/systemctl restart -- ssh.service' as root on ops-test.",
		"ops is not in the sudoers file.",
		"Failed to restart nginx.service: Interactive authentication required.",
		"Failed to restart nginx.service: Access denied",
	} {
		factory := &serviceFakeFactory{
			result: sshRunResult{Stderr: []byte(stderr)},
			runErr: fmt.Errorf("serverops ssh: remote command exited with status 1: %w", &ssh.ExitError{}),
		}
		_, err := newFakeService(t, factory).ChangeService(context.Background(), writeActionSudoConnection(), ActionServiceRestart, "nginx.service")
		if !errors.Is(err, ErrPermissionDenied) || errors.Is(err, ErrProviderRejected) {
			t.Fatalf("stderr %q: error = %v, want only ErrPermissionDenied", stderr, err)
		}
		if message := err.Error(); message != "server operations permission was denied" {
			t.Fatalf("public message = %q", message)
		}
	}
}

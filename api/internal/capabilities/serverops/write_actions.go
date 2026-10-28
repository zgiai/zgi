// This file implements the private, pure command layer for the four controlled
// service write actions and the exported, side-effect-free command renderer
// used by approval previews.
//
// A write action changes the state of exactly one systemd service unit with one
// fixed systemctl verb. The verb comes only from the action id, never from
// input; the only dynamic string is the single-quoted unit name that matches
// SystemdServiceNamePattern exactly; the only privilege escalation is the fixed
// "sudo -n" prefix chosen by the connection. There is still no API that accepts
// an arbitrary command, argument, path, or shell fragment.
package serverops

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// systemSnapshotCommandSummary is the concise, faithful description of the
// fixed system snapshot script shown to an approver. The script itself is long
// and takes no input, so the preview names exactly what it reads instead.
const systemSnapshotCommandSummary = "read-only system snapshot: hostname, /etc/os-release, uname -sr, " +
	"/proc/uptime, /proc/loadavg, /proc/meminfo, df -P -k"

// sudoNonInteractivePrefix is the only privilege escalation a command may
// carry. "-n" makes sudo fail instead of prompting for a password.
const sudoNonInteractivePrefix = "sudo -n "

// serviceChangeVerb maps a service write action onto its one fixed systemctl
// verb. Any other action id has no verb.
func serviceChangeVerb(action string) (string, bool) {
	switch action {
	case ActionServiceRestart:
		return "restart", true
	case ActionServiceStart:
		return "start", true
	case ActionServiceStop:
		return "stop", true
	case ActionServiceReload:
		return "reload", true
	default:
		return "", false
	}
}

// IsServiceWriteAction reports whether action is one of the four service write
// actions.
func IsServiceWriteAction(action string) bool {
	_, ok := serviceChangeVerb(action)
	return ok
}

// serviceChangeLine is the single command line of one service write action. It
// is shared by the executed script and the rendered approval preview, so what
// an approver sees is exactly what runs.
func serviceChangeLine(verb, serviceName string, privilege Privilege) string {
	line := "systemctl " + verb + " -- " + shellSingleQuote(serviceName)
	if privilege == PrivilegeSudo {
		line = sudoNonInteractivePrefix + line
	}
	return line
}

// buildServiceChangeCommand validates the exactly-one-key input and returns the
// fixed script for one service write action plus the validated unit name. The
// script runs the change line with its standard output moved to standard error
// (so it can never forge a protocol record), stops with the change's own exit
// status when it fails, and otherwise reads the unit state again with the same
// unprivileged show line the service status action uses.
func buildServiceChangeCommand(action string, input map[string]any, privilege Privilege) (string, string, error) {
	verb, ok := serviceChangeVerb(action)
	if !ok {
		return "", "", fmt.Errorf("%w: action is not a service write action", errReadActionInput)
	}
	if !privilege.Valid() {
		return "", "", fmt.Errorf("%w: privilege is not supported", errReadActionInput)
	}
	if err := rejectUnknownInputKeys(input, "service_name"); err != nil {
		return "", "", err
	}
	serviceName, err := serviceNameFromInput(input)
	if err != nil {
		return "", "", err
	}
	command := "LC_ALL=C\nexport LC_ALL\n" +
		serviceChangeLine(verb, serviceName, privilege) + " 1>&2 || exit $?\n" +
		serviceStatusReadScript(serviceName)
	return command, serviceName, nil
}

// CommandLine renders the human-readable command line one action runs for the
// given input and connection privilege, for display to an approver. It is
// side-effect-free: it never dials, runs, or retains anything.
//
// It validates the input exactly as the executing action does and builds the
// line from the same pieces as the executed command:
//   - a service write action renders its change line, for example
//     "sudo -n systemctl restart -- 'nginx.service'";
//   - service status renders its systemctl show line;
//   - journal tail renders its journalctl line with the effective defaults;
//   - the system snapshot, whose fixed script takes no input, renders a concise
//     description of what it reads.
//
// Read actions never run with sudo, so their rendering ignores a valid
// privilege. An unknown action, an invalid privilege, or invalid input returns
// an ErrInvalidInput error and no line.
func CommandLine(action string, input map[string]any, privilege Privilege) (string, error) {
	if !privilege.Valid() {
		return "", newError(ErrInvalidInput, "", fmt.Errorf("%w: privilege is not supported", errReadActionInput))
	}
	switch action {
	case ActionSystemSnapshot:
		if len(input) != 0 {
			return "", newError(ErrInvalidInput, "", fmt.Errorf("%w: unexpected input key", errReadActionInput))
		}
		return systemSnapshotCommandSummary, nil
	case ActionServiceStatus:
		_, serviceName, err := buildServiceStatusCommand(input)
		if err != nil {
			return "", newError(ErrInvalidInput, "", err)
		}
		return serviceStatusShowLine(serviceName), nil
	case ActionJournalTail:
		_, validated, err := buildJournalTailCommand(input)
		if err != nil {
			return "", newError(ErrInvalidInput, "", err)
		}
		return journalTailLine(validated), nil
	}
	verb, ok := serviceChangeVerb(action)
	if !ok {
		return "", newError(ErrInvalidInput, "", fmt.Errorf("%w: action is unknown", errReadActionInput))
	}
	_, serviceName, err := buildServiceChangeCommand(action, input, privilege)
	if err != nil {
		return "", newError(ErrInvalidInput, "", err)
	}
	return serviceChangeLine(verb, serviceName, privilege), nil
}

// errServiceChangeRejected classifies a service write command that exited with
// a non-zero status.
var errServiceChangeRejected = errors.New("serverops service write action: command failed")

// serviceChangeRunError classifies one failed service write run. A non-zero
// remote exit keeps the ErrProviderRejected class and adds the bounded,
// control-free remote error text to the operator-only cause; every other
// failure uses the shared classification. Error() stays generic either way.
func serviceChangeRunError(requestID string, err error, stderr []byte) error {
	classified := runError(requestID, err)
	if !errors.Is(classified, ErrProviderRejected) {
		return classified
	}
	return newError(ErrProviderRejected, requestID,
		fmt.Errorf("%w: %s: %w", errServiceChangeRejected, boundedRemoteErrorText(stderr), err))
}

// boundedRemoteErrorText turns remote error output into one bounded line:
// invalid UTF-8 is replaced, control and format characters become spaces, runs
// of whitespace collapse, and the text is cut to maxRemoteErrorRunes runes.
func boundedRemoteErrorText(output []byte) string {
	text := strings.ToValidUTF8(string(output), "�")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "no error output"
	}
	if utf8.RuneCountInString(text) > maxRemoteErrorRunes {
		text = string([]rune(text)[:maxRemoteErrorRunes]) + "..."
	}
	return text
}

package serverops

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
)

// readActionShellTokens splits a shell command into tokens while honoring
// single and double quotes. Whitespace inside quotes is preserved, so a quoted
// program such as the awk script stays one token and an unquoted `>` or `;`
// remains a standalone token. This lets the command tests prove that only fixed
// words appear and that input cannot add tokens.
func readActionShellTokens(command string) []string {
	var (
		tokens  []string
		current strings.Builder
		single  bool
		double  bool
		escaped bool
	)
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, r := range command {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case single:
			if r == '\'' {
				single = false
			} else {
				current.WriteRune(r)
			}
		case double:
			if r == '"' {
				double = false
			} else {
				current.WriteRune(r)
			}
		case r == '\\':
			escaped = true
		case r == '\'':
			single = true
		case r == '"':
			double = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == ';' || r == '|' || r == '&':
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// readActionForbiddenWords are control, write, or escalation words that must
// never appear as a command token in a read-only action command.
func readActionForbiddenWords() map[string]bool {
	words := []string{
		"sudo", "doas", "su", "runuser", "setpriv", "sudoedit", "eval", "exec", "source",
		"sh", "bash", "zsh", "dash", "ksh",
		"rm", "cp", "mv", "dd", "tee", "truncate", "shred", "chmod", "chown", "chattr",
		"ln", "install", "mkdir", "rmdir", "touch", "mkfs", "mount", "umount", "mkswap",
		"apt", "apt-get", "yum", "dnf", "apk", "pacman", "pip", "pip3", "npm", "snap",
		"curl", "wget", "scp", "sftp", "rsync", "nc", "ncat", "socat", "telnet",
		"restart", "reload", "start", "stop", "enable", "disable", "mask", "unmask",
		"kill", "pkill", "killall", "reboot", "shutdown", "poweroff", "halt", "suspend",
		"hibernate", "isolate", "rescue", "emergency", "preset", "revert", "edit",
		"systemd-run", "nsenter", "chroot", "unshare", "useradd", "userdel", "passwd",
		"crontab", "at", "batch", "systemd-tmpfiles",
		"upload", "download", "restore", "dump", "import", "exportfs",
	}
	forbidden := make(map[string]bool, len(words))
	for _, word := range words {
		forbidden[word] = true
	}
	return forbidden
}

// readActionAssertCommandIsReadOnly asserts that no forbidden word appears as a
// command token and that no standalone shell redirection token exists.
func readActionAssertCommandIsReadOnly(t *testing.T, command string) {
	t.Helper()
	forbidden := readActionForbiddenWords()
	for _, token := range readActionShellTokens(command) {
		if forbidden[token] {
			t.Errorf("command contains forbidden token %q", token)
		}
		switch token {
		case ">", ">>", "<", "<<", "2>", "&>", "|&":
			t.Errorf("command contains a redirection token %q", token)
		}
	}
}

func readActionValidSnapshotLines() []string {
	return []string{
		systemSnapshotHeader,
		"hostname\tserver-1",
		"operating_system\tUbuntu 22.04.3 LTS",
		"kernel_version\tLinux 5.15.0-91-generic",
		"uptime_seconds\t123456",
		"load_average_1m\t0.42",
		"memory\t16000000000\t8000000000\t8000000000",
		"disk\t/dev/sda1\t/\t100000000000\t50000000000\t50000000000",
		"disk\ttmpfs\t/run\t1000000\t900000\t100000",
	}
}

func readActionJoinLines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n") + "\n")
}

func readActionReplaceLine(lines []string, index int, value string) []string {
	replaced := append([]string(nil), lines...)
	replaced[index] = value
	return replaced
}

func readActionSnapshotWithDisks(disks int) []byte {
	lines := append([]string(nil), readActionValidSnapshotLines()[:7]...)
	for index := 0; index < disks; index++ {
		lines = append(lines, readActionDiskLine(index))
	}
	return readActionJoinLines(lines)
}

func readActionDiskLine(index int) string {
	return "disk\t/dev/sd" + strconv.Itoa(index) + "\t/mnt/" + strconv.Itoa(index) + "\t1000\t600\t400"
}

func readActionServiceStatusOutput() []byte {
	return []byte(serviceStatusHeader + "\n" +
		"LoadState=loaded\n" +
		"ActiveState=active\n" +
		"SubState=running\n" +
		"Description=ZGI test unit\n")
}

func TestReadActionsShellSingleQuoteIsCorrectInItsOwnRight(t *testing.T) {
	cases := map[string]string{
		"nginx.service": "'nginx.service'",
		"a b":           "'a b'",
		"":              "''",
		"a'b":           `'a'\''b'`,
		"a'b'c":         `'a'\''b'\''c'`,
		"$(id)":         "'$(id)'",
		"`id`":          "'`id`'",
		"; rm -rf /":    "'; rm -rf /'",
		"a\nb":          "'a\nb'",
		`back\slash`:    `'back\slash'`,
		"quote\"inside": "'quote\"inside'",
	}
	for input, want := range cases {
		if got := shellSingleQuote(input); got != want {
			t.Errorf("shellSingleQuote(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReadActionsServiceNameInputValidation(t *testing.T) {
	valid := []string{
		"nginx.service",
		"sshd.service",
		"systemd-resolved.service",
		"my_app-1.service",
		"getty@tty1.service",
		"dbus.socket.service",
		"a.service",
		"foo:bar.service",
		"a.b-c_d@e.service",
		"9.service",
	}
	for _, name := range valid {
		statusCommand, returnedName, err := buildServiceStatusCommand(map[string]any{"service_name": name})
		if err != nil {
			t.Errorf("buildServiceStatusCommand(%q): %v", name, err)
			continue
		}
		if returnedName != name {
			t.Errorf("buildServiceStatusCommand(%q) returned name %q", name, returnedName)
		}
		quoted := "'" + name + "'"
		if strings.Count(statusCommand, quoted) != 1 {
			t.Errorf("service status command for %q does not quote the unit exactly once", name)
		}
		tokens := readActionShellTokens(statusCommand)
		if tokens[len(tokens)-1] != name {
			t.Errorf("service status command for %q does not end with the unit as one argument: %#v", name, tokens)
		}

		journalCommand, journalInput, err := buildJournalTailCommand(map[string]any{"service_name": name})
		if err != nil {
			t.Errorf("buildJournalTailCommand(%q): %v", name, err)
			continue
		}
		if journalInput.ServiceName != name {
			t.Errorf("buildJournalTailCommand(%q) returned name %q", name, journalInput.ServiceName)
		}
		if strings.Count(journalCommand, "--unit='"+name+"'") != 1 {
			t.Errorf("journal command for %q does not use one quoted unit argument", name)
		}
	}

	invalid := []string{
		"",
		" ",
		"   ",
		"\t",
		"\n",
		" nginx.service",
		"nginx.service ",
		"\tnginx.service",
		"nginx.service\t",
		"nginx.service\n",
		"nginx.service\r",
		"nginx",
		"nginx.socket",
		".service",
		"-nginx.service",
		"nginx.service; rm -rf /",
		"nginx.service; id",
		"nginx.service|cat /etc/shadow",
		"nginx.service&&id",
		"nginx.service extra",
		"/bin/sh",
		"a/b.service",
		"../etc/passwd",
		"$(id).service",
		"`id`.service",
		"ng*inx.service",
		"nginx'.service",
		"nginx\".service",
		"NGINX.SERVICE",
		"nginx.service\x00",
		strings.Repeat("a", maxServiceNameLength) + ".service",
	}
	for _, name := range invalid {
		if _, _, err := buildServiceStatusCommand(map[string]any{"service_name": name}); !errors.Is(err, errReadActionInput) {
			t.Errorf("service status input %q: error = %v, want errReadActionInput", name, err)
		}
		if _, _, err := buildJournalTailCommand(map[string]any{"service_name": name}); !errors.Is(err, errReadActionInput) {
			t.Errorf("journal input %q: error = %v, want errReadActionInput", name, err)
		}
	}

	nonStrings := []any{123, int64(1), true, nil, []string{"nginx.service"}, map[string]string{"a": "b"}}
	for _, value := range nonStrings {
		if _, _, err := buildServiceStatusCommand(map[string]any{"service_name": value}); !errors.Is(err, errReadActionInput) {
			t.Errorf("service_name %#v: error = %v, want errReadActionInput", value, err)
		}
	}

	if _, _, err := buildServiceStatusCommand(nil); !errors.Is(err, errReadActionInput) {
		t.Errorf("missing service_name: error = %v, want errReadActionInput", err)
	}
	if _, _, err := buildServiceStatusCommand(map[string]any{}); !errors.Is(err, errReadActionInput) {
		t.Errorf("empty input: error = %v, want errReadActionInput", err)
	}
}

func TestReadActionsServiceStatusCommandIsExact(t *testing.T) {
	command, name, err := buildServiceStatusCommand(map[string]any{"service_name": "nginx.service"})
	if err != nil {
		t.Fatalf("buildServiceStatusCommand: %v", err)
	}
	if name != "nginx.service" {
		t.Fatalf("service name = %q", name)
	}
	want := []string{
		"LC_ALL=C",
		"export",
		"LC_ALL",
		"printf",
		"ZGI_SERVER_SERVICE_STATUS_V1\\n",
		"systemctl",
		"show",
		"--no-pager",
		"--property=LoadState",
		"--property=ActiveState",
		"--property=SubState",
		"--property=Description",
		"--",
		"nginx.service",
	}
	if got := readActionShellTokens(command); !equalStringSlices(got, want) {
		t.Errorf("service status tokens = %#v, want %#v", got, want)
	}
	if !strings.HasPrefix(command, "LC_ALL=C\n") {
		t.Errorf("service status command does not start with LC_ALL=C: %q", command)
	}
	readActionAssertCommandIsReadOnly(t, command)

	// A different valid unit changes exactly one token and adds none.
	other, _, err := buildServiceStatusCommand(map[string]any{"service_name": "dbus.socket.service"})
	if err != nil {
		t.Fatalf("buildServiceStatusCommand(other): %v", err)
	}
	otherTokens := readActionShellTokens(other)
	if len(otherTokens) != len(want) {
		t.Fatalf("token count changed with input: %d vs %d", len(otherTokens), len(want))
	}
	for index := range otherTokens {
		if index == len(otherTokens)-1 {
			if otherTokens[index] != "dbus.socket.service" {
				t.Errorf("final token = %q", otherTokens[index])
			}
			continue
		}
		if otherTokens[index] != want[index] {
			t.Errorf("token %d changed with input: %q vs %q", index, otherTokens[index], want[index])
		}
	}
}

func TestReadActionsJournalTailCommandIsExact(t *testing.T) {
	command, input, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service"})
	if err != nil {
		t.Fatalf("buildJournalTailCommand: %v", err)
	}
	if input != (journalTailInput{ServiceName: "nginx.service", Lines: defaultJournalLines, SinceMinutes: defaultJournalSinceMinutes}) {
		t.Fatalf("default input = %#v", input)
	}
	want := []string{
		"LC_ALL=C",
		"export",
		"LC_ALL",
		"journalctl",
		"--no-pager",
		"--quiet",
		"--output=cat",
		"--unit=nginx.service",
		"--lines=100",
		"--since=-60min",
	}
	if got := readActionShellTokens(command); !equalStringSlices(got, want) {
		t.Errorf("journal tokens = %#v, want %#v", got, want)
	}
	readActionAssertCommandIsReadOnly(t, command)

	other, _, err := buildJournalTailCommand(map[string]any{
		"service_name":  "nginx.service",
		"lines":         200,
		"since_minutes": 1440,
	})
	if err != nil {
		t.Fatalf("buildJournalTailCommand(bounds): %v", err)
	}
	boundsTokens := readActionShellTokens(other)
	if !equalStringSlices(boundsTokens, []string{
		"LC_ALL=C", "export", "LC_ALL", "journalctl", "--no-pager", "--quiet", "--output=cat",
		"--unit=nginx.service", "--lines=200", "--since=-1440min",
	}) {
		t.Errorf("journal bounds tokens = %#v", boundsTokens)
	}
}

func TestReadActionsSnapshotCommandIsFixedAndDeterministic(t *testing.T) {
	first := buildSystemSnapshotCommand()
	second := buildSystemSnapshotCommand()
	if first != second {
		t.Fatal("buildSystemSnapshotCommand is not deterministic")
	}
	if strings.ContainsRune(first, 0) {
		t.Fatal("snapshot command contains NUL")
	}
	if !strings.HasPrefix(first, "LC_ALL=C\n") {
		t.Errorf("snapshot command does not start with LC_ALL=C: %q", first)
	}
	if !strings.Contains(first, "printf '"+systemSnapshotHeader+"\\n'") {
		t.Errorf("snapshot command does not print the exact header first: %q", first)
	}
	for _, required := range []string{
		"/etc/os-release",
		"/proc/uptime",
		"/proc/loadavg",
		"/proc/meminfo",
		"uname",
		"df -P -k",
	} {
		if !strings.Contains(first, required) {
			t.Errorf("snapshot command is missing fixed read %q", required)
		}
	}
	if !strings.Contains(first, "head -n "+strconv.Itoa(maxDiskEntries+1)+" ") {
		t.Errorf("snapshot command does not bound df to maxDiskEntries: %q", first)
	}
	// Numeric records must use the portable "%.0f" format for byte counters.
	if strings.Contains(first, "%d") {
		t.Errorf("snapshot command uses %%d for a counter instead of %%.0f: %q", first)
	}
	if got := strings.Count(first, "%.0f"); got != 6 {
		t.Errorf("snapshot command has %d %%.0f formats, want 6 (three memory, three disk): %q", got, first)
	}
	readActionAssertCommandIsReadOnly(t, first)
}

func TestReadActionsJournalIntegerRepresentations(t *testing.T) {
	for name, value := range map[string]any{
		"int":                  5,
		"int8":                 int8(5),
		"int16":                int16(5),
		"int32":                int32(5),
		"int64":                int64(5),
		"uint":                 uint(5),
		"uint8":                uint8(5),
		"uint16":               uint16(5),
		"uint32":               uint32(5),
		"uint64":               uint64(5),
		"float64":              float64(5),
		"json five":            json.Number("5"),
		"json five point zero": json.Number("5.0"),
		"json five e zero":     json.Number("5e0"),
	} {
		command, input, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "lines": value})
		if err != nil {
			t.Errorf("%s: buildJournalTailCommand(%#v): %v", name, value, err)
			continue
		}
		if input.Lines != 5 {
			t.Errorf("%s: lines = %d, want 5", name, input.Lines)
		}
		if !strings.Contains(command, "--lines=5 ") {
			t.Errorf("%s: command missing bounded lines: %q", name, command)
		}
	}

	command, input, err := buildJournalTailCommand(map[string]any{
		"service_name":  "nginx.service",
		"lines":         1,
		"since_minutes": 1,
	})
	if err != nil {
		t.Fatalf("minimum bounds: %v", err)
	}
	if input.Lines != 1 || input.SinceMinutes != 1 {
		t.Errorf("minimum bounds input = %#v", input)
	}
	if !strings.Contains(command, "--lines=1 ") || !strings.Contains(command, "--since=-1min") {
		t.Errorf("minimum bounds command = %q", command)
	}

	if _, input, err := buildJournalTailCommand(map[string]any{"service_name": "nginx.service", "lines": maxJournalLines, "since_minutes": maxJournalSinceMinutes}); err != nil {
		t.Fatalf("maximum bounds: %v", err)
	} else if input.Lines != maxJournalLines || input.SinceMinutes != maxJournalSinceMinutes {
		t.Errorf("maximum bounds input = %#v", input)
	}
}

func TestReadActionsJournalInputRejections(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]any
	}{
		{"unknown key", map[string]any{"service_name": "nginx.service", "command": "id"}},
		{"unknown path key", map[string]any{"service_name": "nginx.service", "path": "/etc/passwd"}},
		{"unknown since key", map[string]any{"service_name": "nginx.service", "since": 5}},
		{"unknown unit key", map[string]any{"service_name": "nginx.service", "unit": "other.service"}},
		{"missing service_name", map[string]any{}},
		{"lines string", map[string]any{"service_name": "nginx.service", "lines": "5"}},
		{"lines bool", map[string]any{"service_name": "nginx.service", "lines": true}},
		{"lines nil", map[string]any{"service_name": "nginx.service", "lines": nil}},
		{"lines slice", map[string]any{"service_name": "nginx.service", "lines": []int{5}}},
		{"lines map", map[string]any{"service_name": "nginx.service", "lines": map[string]int{"n": 5}}},
		{"lines fraction", map[string]any{"service_name": "nginx.service", "lines": 1.5}},
		{"lines float32 integer", map[string]any{"service_name": "nginx.service", "lines": float32(5)}},
		{"lines float32 fraction", map[string]any{"service_name": "nginx.service", "lines": float32(1.5)}},
		{"lines json fraction", map[string]any{"service_name": "nginx.service", "lines": json.Number("1.5")}},
		{"lines json rounded up trap", map[string]any{"service_name": "nginx.service", "lines": json.Number("5.0000000000000000000000000001")}},
		{"lines json rounded down trap", map[string]any{"service_name": "nginx.service", "lines": json.Number("199.9999999999999999999999999999")}},
		{"lines json tiny fraction", map[string]any{"service_name": "nginx.service", "lines": json.Number("5e-1")}},
		{"lines json malformed fraction", map[string]any{"service_name": "nginx.service", "lines": json.Number("1/1")}},
		{"lines json NaN", map[string]any{"service_name": "nginx.service", "lines": json.Number("NaN")}},
		{"lines json Inf", map[string]any{"service_name": "nginx.service", "lines": json.Number("Inf")}},
		{"lines json -Inf", map[string]any{"service_name": "nginx.service", "lines": json.Number("-Inf")}},
		{"lines json leading plus", map[string]any{"service_name": "nginx.service", "lines": json.Number("+5")}},
		{"lines json leading zero", map[string]any{"service_name": "nginx.service", "lines": json.Number("05")}},
		{"lines json trailing dot", map[string]any{"service_name": "nginx.service", "lines": json.Number("5.")}},
		{"lines json leading dot", map[string]any{"service_name": "nginx.service", "lines": json.Number(".5")}},
		{"lines json hex", map[string]any{"service_name": "nginx.service", "lines": json.Number("0x10")}},
		{"lines json huge exponent", map[string]any{"service_name": "nginx.service", "lines": json.Number("1e1000000000")}},
		{"lines json absurd exponent", map[string]any{"service_name": "nginx.service", "lines": json.Number("1e99999999999999999999")}},
		{"lines json exponent out of range", map[string]any{"service_name": "nginx.service", "lines": json.Number("1e20")}},
		{"lines json zero", map[string]any{"service_name": "nginx.service", "lines": json.Number("0")}},
		{"lines json negative", map[string]any{"service_name": "nginx.service", "lines": json.Number("-5")}},
		{"lines rational", map[string]any{"service_name": "nginx.service", "lines": 2.0000001}},
		{"lines NaN", map[string]any{"service_name": "nginx.service", "lines": math.NaN()}},
		{"lines +Inf", map[string]any{"service_name": "nginx.service", "lines": math.Inf(1)}},
		{"lines -Inf", map[string]any{"service_name": "nginx.service", "lines": math.Inf(-1)}},
		{"lines float32 NaN", map[string]any{"service_name": "nginx.service", "lines": float32(math.NaN())}},
		{"lines int64 overflow", map[string]any{"service_name": "nginx.service", "lines": int64(math.MaxInt64)}},
		{"lines uint64 overflow", map[string]any{"service_name": "nginx.service", "lines": uint64(math.MaxUint64)}},
		{"lines float overflow", map[string]any{"service_name": "nginx.service", "lines": 1e30}},
		{"lines json overflow", map[string]any{"service_name": "nginx.service", "lines": json.Number("99999999999999999999")}},
		{"lines zero", map[string]any{"service_name": "nginx.service", "lines": 0}},
		{"lines negative", map[string]any{"service_name": "nginx.service", "lines": -1}},
		{"lines above max", map[string]any{"service_name": "nginx.service", "lines": maxJournalLines + 1}},
		{"since zero", map[string]any{"service_name": "nginx.service", "since_minutes": 0}},
		{"since negative", map[string]any{"service_name": "nginx.service", "since_minutes": -1}},
		{"since fraction", map[string]any{"service_name": "nginx.service", "since_minutes": 1.5}},
		{"since NaN", map[string]any{"service_name": "nginx.service", "since_minutes": math.NaN()}},
		{"since above max", map[string]any{"service_name": "nginx.service", "since_minutes": maxJournalSinceMinutes + 1}},
	}
	for _, testCase := range cases {
		if _, _, err := buildJournalTailCommand(testCase.input); !errors.Is(err, errReadActionInput) {
			t.Errorf("%s: error = %v, want errReadActionInput", testCase.name, err)
		}
	}
}

// TestReadActionsJSONNumberExactIntegerValidation pins the exact mathematical
// contract for json.Number: float64 rounding must never turn a fractional
// literal into an accepted integer, and malformed or non-JSON spellings must
// never reach the exact-integer path.
func TestReadActionsJSONNumberExactIntegerValidation(t *testing.T) {
	accepted := map[string]int64{
		"5":     5,
		"5.0":   5,
		"5e0":   5,
		"5E0":   5,
		"50e-1": 5,
		"0":     0,
		"0.0":   0,
		"0e99":  0,
		"-0":    0,
	}
	for text, want := range accepted {
		value, err := coerceInputInteger(json.Number(text), 0, maxByteCount)
		if err != nil {
			t.Errorf("json.Number(%q): %v", text, err)
			continue
		}
		if value != want {
			t.Errorf("json.Number(%q) = %d, want %d", text, value, want)
		}
	}

	rejected := []string{
		"5.0000000000000000000000000001",
		"199.9999999999999999999999999999",
		"1.0000000000000000000000000000000001",
		"1/1",
		"NaN",
		"nan",
		"Inf",
		"-Inf",
		"+Inf",
		"+5",
		"05",
		"5.",
		".5",
		"0x10",
		"",
		"5e",
		"e5",
		"--5",
		"5 0",
		" 5",
		"5 ",
		"1e1000000000",
		"1e99999999999999999999",
		"1e-99999999999999999999",
		"1e9223372036854775807",
		"1e-9223372036854775808",
		"1.0e-9223372036854775808",
		"1e9223372036854775808",
		"1e-9223372036854775809",
		"1e20",
		"1.5",
		"5e-1",
	}
	for _, text := range rejected {
		value, err := coerceInputInteger(json.Number(text), 0, maxByteCount)
		if err == nil {
			t.Errorf("json.Number(%q) accepted as %d, want rejection", text, value)
			continue
		}
		if !errors.Is(err, errReadActionInput) {
			t.Errorf("json.Number(%q): error = %v, want errReadActionInput", text, err)
		}
		if text != "" && strings.Contains(err.Error(), text) {
			t.Errorf("json.Number(%q): error echoed the untrusted number: %v", text, err)
		}
	}

	// The byte ceiling is enforced exactly, not through a rounded float.
	if _, err := coerceInputInteger(json.Number(strconv.FormatInt(maxByteCount, 10)), 0, maxByteCount); err != nil {
		t.Errorf("maxByteCount rejected: %v", err)
	}
	if _, err := coerceInputInteger(json.Number(strconv.FormatInt(maxByteCount+1, 10)), 0, maxByteCount); !errors.Is(err, errReadActionInput) {
		t.Errorf("maxByteCount+1: error = %v, want errReadActionInput", err)
	}
}

// TestReadActionsJSONNumberExponentOverflowIsSafe is the regression for the
// int64 overflow/panic paths in the exact exponent handling. Every spelling
// below previously risked negating math.MinInt64, overflowing len+scaled, or
// underflowing exponent-fractionDigits. None may panic, and all must be
// rejected through both the private helper and the journal action builder.
func TestReadActionsJSONNumberExponentOverflowIsSafe(t *testing.T) {
	spellings := []string{
		"1e9223372036854775807",
		"1e-9223372036854775808",
		"1.0e-9223372036854775808",
		"1e9223372036854775808",
		"1e-9223372036854775809",
		"1e99999999999999999999",
		"1e-99999999999999999999",
		"5.0e9223372036854775807",
		"5.0e-9223372036854775808",
		"0.0e-9223372036854775808",
	}
	for _, text := range spellings {
		value, err := coerceInputInteger(json.Number(text), 1, maxJournalLines)
		if err == nil {
			t.Errorf("coerceInputInteger(json.Number(%q)) = %d, want rejection", text, value)
		} else if !errors.Is(err, errReadActionInput) {
			t.Errorf("json.Number(%q): error = %v, want errReadActionInput", text, err)
		}
		if _, _, err := buildJournalTailCommand(map[string]any{
			"service_name": "nginx.service",
			"lines":        json.Number(text),
		}); !errors.Is(err, errReadActionInput) {
			t.Errorf("buildJournalTailCommand(json.Number(%q)): error = %v, want errReadActionInput", text, err)
		}
	}

	// The zero-mantissa case resolves before exponent parsing and is still a
	// valid in-range zero when the range admits it.
	if value, err := coerceInputInteger(json.Number("0.0e-9223372036854775808"), 0, maxByteCount); err != nil {
		t.Errorf("zero mantissa with a minimum exponent: %v", err)
	} else if value != 0 {
		t.Errorf("zero mantissa = %d, want 0", value)
	}
}

func TestReadActionsSnapshotParserHappyPath(t *testing.T) {
	input := readActionJoinLines(readActionValidSnapshotLines())
	snapshot, err := parseSystemSnapshotOutput(input)
	if err != nil {
		t.Fatalf("parseSystemSnapshotOutput: %v", err)
	}
	if snapshot.Hostname != "server-1" {
		t.Errorf("hostname = %q", snapshot.Hostname)
	}
	if snapshot.OperatingSystem != "Ubuntu 22.04.3 LTS" {
		t.Errorf("operating system = %q", snapshot.OperatingSystem)
	}
	if snapshot.KernelVersion != "Linux 5.15.0-91-generic" {
		t.Errorf("kernel version = %q", snapshot.KernelVersion)
	}
	if snapshot.UptimeSeconds != 123456 {
		t.Errorf("uptime = %d", snapshot.UptimeSeconds)
	}
	if snapshot.LoadAverage1m != 0.42 {
		t.Errorf("load average = %v", snapshot.LoadAverage1m)
	}
	if snapshot.Memory != (memoryInfo{TotalBytes: 16000000000, AvailableBytes: 8000000000, UsedBytes: 8000000000}) {
		t.Errorf("memory = %#v", snapshot.Memory)
	}
	wantDisks := []diskInfo{
		{Filesystem: "/dev/sda1", MountPoint: "/", TotalBytes: 100000000000, AvailableBytes: 50000000000, UsedBytes: 50000000000},
		{Filesystem: "tmpfs", MountPoint: "/run", TotalBytes: 1000000, AvailableBytes: 900000, UsedBytes: 100000},
	}
	if !equalDiskSlices(snapshot.Disks, wantDisks) {
		t.Errorf("disks = %#v, want %#v", snapshot.Disks, wantDisks)
	}

	// A missing terminal newline is an ordinary variant, not corruption.
	withoutTrailing := []byte(strings.TrimSuffix(string(input), "\n"))
	if _, err := parseSystemSnapshotOutput(withoutTrailing); err != nil {
		t.Errorf("snapshot without trailing newline: %v", err)
	}

	// Empty text is in schema contract because the output schema has no minLength.
	emptyText := readActionJoinLines(readActionReplaceLine(readActionValidSnapshotLines(), 1, "hostname\t"))
	if snapshot, err := parseSystemSnapshotOutput(emptyText); err != nil {
		t.Errorf("empty hostname value: %v", err)
	} else if snapshot.Hostname != "" {
		t.Errorf("empty hostname parsed as %q", snapshot.Hostname)
	}

	// Exactly maxDiskEntries disks are accepted.
	if _, err := parseSystemSnapshotOutput(readActionSnapshotWithDisks(maxDiskEntries)); err != nil {
		t.Errorf("exactly maxDiskEntries disks: %v", err)
	}
}

func TestReadActionsSnapshotParserRejections(t *testing.T) {
	base := readActionValidSnapshotLines()
	cases := []struct {
		name   string
		output []byte
	}{
		{"empty output", nil},
		{"only newline", []byte("\n")},
		{"wrong header", readActionJoinLines(readActionReplaceLine(base, 0, "ZGI_SERVER_SNAPSHOT_V2"))},
		{"header not first", readActionJoinLines(readActionReplaceLine(base, 1, systemSnapshotHeader))},
		{"unknown record", readActionJoinLines(append(base[:1:1], append([]string{"bogus\tvalue"}, base[1:]...)...))},
		{"duplicate hostname", readActionJoinLines(append(append([]string(nil), base...), "hostname\tsecond"))},
		{"duplicate operating_system", readActionJoinLines(append(append([]string(nil), base...), "operating_system\tOther"))},
		{"duplicate kernel_version", readActionJoinLines(append(append([]string(nil), base...), "kernel_version\tOther"))},
		{"duplicate uptime", readActionJoinLines(append(append([]string(nil), base...), "uptime_seconds\t1"))},
		{"duplicate load", readActionJoinLines(append(append([]string(nil), base...), "load_average_1m\t1.0"))},
		{"duplicate memory", readActionJoinLines(append(append([]string(nil), base...), "memory\t1\t1\t0"))},
		{"missing hostname", readActionJoinLines(append([]string{base[0]}, base[2:]...))},
		{"missing operating_system", readActionJoinLines(append([]string{base[0], base[1]}, base[3:]...))},
		{"missing kernel_version", readActionJoinLines(append([]string{base[0], base[1], base[2]}, base[4:]...))},
		{"missing uptime", readActionJoinLines(append([]string{base[0], base[1], base[2], base[3]}, base[5:]...))},
		{"missing load", readActionJoinLines(append([]string{base[0], base[1], base[2], base[3], base[4]}, base[6:]...))},
		{"missing memory", readActionJoinLines(append([]string(nil), base[:6]...))},
		{"hostname extra field", readActionJoinLines(readActionReplaceLine(base, 1, "hostname\tserver\textra"))},
		{"operating_system extra field", readActionJoinLines(readActionReplaceLine(base, 2, "operating_system\tx\textra"))},
		{"kernel_version extra field", readActionJoinLines(readActionReplaceLine(base, 3, "kernel_version\tx\textra"))},
		{"uptime extra field", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t1\textra"))},
		{"load extra field", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t1.0\textra"))},
		{"memory wrong field count", readActionJoinLines(readActionReplaceLine(base, 6, "memory\t100\t50"))},
		{"disk wrong field count", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t100\t60"))},
		{"uptime letters", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\tabc"))},
		{"uptime fraction", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t1.5"))},
		{"uptime negative", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t-1"))},
		{"uptime signed", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t+1"))},
		{"uptime empty", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t"))},
		{"uptime hex", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t0x10"))},
		{"uptime space", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t 1"))},
		{"uptime overflow", readActionJoinLines(readActionReplaceLine(base, 4, "uptime_seconds\t"+strconv.FormatInt(maxUptimeSeconds+1, 10)))},
		{"load letters", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\tabc"))},
		{"load NaN", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\tNaN"))},
		{"load Inf", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\tInf"))},
		{"load negative", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t-1"))},
		{"load exponent", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t1e3"))},
		{"load leading dot", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t.5"))},
		{"load trailing dot", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t1."))},
		{"load empty", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t"))},
		{"load out of range", readActionJoinLines(readActionReplaceLine(base, 5, "load_average_1m\t"+strconv.FormatFloat(maxLoadAverage*2, 'f', -1, 64)))},
		{"memory negative", readActionJoinLines(readActionReplaceLine(base, 6, "memory\t-1\t0\t0"))},
		{"memory above max", readActionJoinLines(readActionReplaceLine(base, 6, "memory\t"+strconv.FormatInt(maxByteCount+1, 10)+"\t0\t0"))},
		{"memory available exceeds total", readActionJoinLines(readActionReplaceLine(base, 6, "memory\t100\t200\t0"))},
		{"memory inconsistent", readActionJoinLines(readActionReplaceLine(base, 6, "memory\t100\t40\t70"))},
		{"disk negative", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t-1\t0\t0"))},
		{"disk above max", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t"+strconv.FormatInt(maxByteCount+1, 10)+"\t0\t0"))},
		{"disk used exceeds total", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t100\t10\t150"))},
		{"disk available exceeds total", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t100\t150\t10"))},
		{"disk capacity inconsistent", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t/\t100\t60\t50"))},
		{"too many disks", readActionSnapshotWithDisks(maxDiskEntries + 1)},
		{"NUL", append(readActionJoinLines(base), 0)},
		{"invalid UTF-8", readActionJoinLines(readActionReplaceLine(base, 1, "hostname\t\xff\xfe"))},
		{"control character", readActionJoinLines(readActionReplaceLine(base, 1, "hostname\tbad\x01name"))},
		{"hostname too long", readActionJoinLines(readActionReplaceLine(base, 1, "hostname\t"+strings.Repeat("a", maxSnapshotHostnameLength+1)))},
		{"operating_system too long", readActionJoinLines(readActionReplaceLine(base, 2, "operating_system\t"+strings.Repeat("a", maxSnapshotOSLength+1)))},
		{"kernel_version too long", readActionJoinLines(readActionReplaceLine(base, 3, "kernel_version\t"+strings.Repeat("a", maxSnapshotKernelLength+1)))},
		{"filesystem too long", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t"+strings.Repeat("a", maxDiskFilesystemLength+1)+"\t/\t100\t50\t50"))},
		{"mount point too long", readActionJoinLines(readActionReplaceLine(base, 7, "disk\t/dev/sda1\t"+strings.Repeat("a", maxDiskMountPointLength+1)+"\t100\t50\t50"))},
	}
	for _, testCase := range cases {
		if _, err := parseSystemSnapshotOutput(testCase.output); !errors.Is(err, errReadActionOutput) {
			t.Errorf("%s: error = %v, want errReadActionOutput", testCase.name, err)
		}
	}
}

func TestReadActionsServiceStatusParser(t *testing.T) {
	status, err := parseServiceStatusOutput(readActionServiceStatusOutput())
	if err != nil {
		t.Fatalf("parseServiceStatusOutput: %v", err)
	}
	if status.LoadState != "loaded" || status.ActiveState != "active" || status.SubState != "running" {
		t.Errorf("status states = %#v", status)
	}
	if status.Description != "ZGI test unit" {
		t.Errorf("description = %q", status.Description)
	}

	valid := []struct {
		name   string
		output []byte
	}{
		{"value with equals", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=a=b=c\n")},
		{"empty description", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=\n")},
		{"no trailing newline", []byte(strings.TrimSuffix(string(readActionServiceStatusOutput()), "\n"))},
	}
	for _, testCase := range valid {
		if _, err := parseServiceStatusOutput(testCase.output); err != nil {
			t.Errorf("%s: %v", testCase.name, err)
		}
	}

	cases := []struct {
		name   string
		output []byte
	}{
		{"empty output", nil},
		{"wrong header", []byte("ZGI_SERVER_SERVICE_STATUS_V2\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=x\n")},
		{"missing load", []byte(serviceStatusHeader + "\nActiveState=active\nSubState=running\nDescription=x\n")},
		{"missing active", []byte(serviceStatusHeader + "\nLoadState=loaded\nSubState=running\nDescription=x\n")},
		{"missing sub", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nDescription=x\n")},
		{"missing description", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\n")},
		{"duplicate load", []byte(serviceStatusHeader + "\nLoadState=loaded\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=x\n")},
		{"duplicate description", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=x\nDescription=y\n")},
		{"unknown key", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=x\nRestart=always\n")},
		{"empty key", []byte(serviceStatusHeader + "\n=loaded\n")},
		{"line without equals", []byte(serviceStatusHeader + "\nLoadState\n")},
		{"blank structural line", []byte(serviceStatusHeader + "\nLoadState=loaded\n\nActiveState=active\nSubState=running\nDescription=x\n")},
		{"NUL", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=x\x00\n")},
		{"state too long", []byte(serviceStatusHeader + "\nLoadState=" + strings.Repeat("a", maxServiceStateLength+1) + "\nActiveState=active\nSubState=running\nDescription=x\n")},
		{"description too long", []byte(serviceStatusHeader + "\nLoadState=loaded\nActiveState=active\nSubState=running\nDescription=" + strings.Repeat("a", maxServiceDescriptionLength+1) + "\n")},
	}
	for _, testCase := range cases {
		if _, err := parseServiceStatusOutput(testCase.output); !errors.Is(err, errReadActionOutput) {
			t.Errorf("%s: error = %v, want errReadActionOutput", testCase.name, err)
		}
	}
}

func TestReadActionsJournalParser(t *testing.T) {
	longLine := strings.Repeat("\u00e9", maxJournalLineLength+5)

	valid := []struct {
		name               string
		output             []byte
		maxLines           int
		transportTruncated bool
		wantLines          []string
		wantTruncated      bool
	}{
		{"empty output", nil, 10, false, nil, false},
		{"terminal newline artifact", []byte("alpha\nbeta\n"), 10, false, []string{"alpha", "beta"}, false},
		{"no trailing newline", []byte("alpha\nbeta"), 10, false, []string{"alpha", "beta"}, false},
		{"interior blank", []byte("alpha\n\nbeta\n"), 10, false, []string{"alpha", "", "beta"}, false},
		{"trailing blank preserved", []byte("alpha\n\n"), 10, false, []string{"alpha", ""}, false},
		{"single newline is one blank line", []byte("\n"), 10, false, []string{""}, false},
		{"CRLF normalized", []byte("alpha\r\nbeta\r\n"), 10, false, []string{"alpha", "beta"}, false},
		{"mixed line endings", []byte("alpha\r\nbeta\ngamma\r\n"), 10, false, []string{"alpha", "beta", "gamma"}, false},
		{"whitespace preserved", []byte("  spaced  \n\tTabbed\t\n"), 10, false, []string{"  spaced  ", "\tTabbed\t"}, false},
		{"exactly max lines", []byte("a\nb\n"), 2, false, []string{"a", "b"}, false},
		{"line limit", []byte("a\nb\nc\nd\n"), 2, false, []string{"a", "b"}, true},
		{"transport truncated", []byte("only\n"), 10, true, []string{"only"}, true},
		{"line exactly at length", []byte(strings.Repeat("x", maxJournalLineLength) + "\n"), 10, false, []string{strings.Repeat("x", maxJournalLineLength)}, false},
		{"unicode line shortened", []byte(longLine + "\n"), 10, false, []string{strings.Repeat("\u00e9", maxJournalLineLength)}, true},
	}
	for _, testCase := range valid {
		journal, err := parseJournalTailOutput(testCase.output, testCase.maxLines, testCase.transportTruncated)
		if err != nil {
			t.Errorf("%s: %v", testCase.name, err)
			continue
		}
		if !equalStringSlices(journal.Lines, testCase.wantLines) {
			t.Errorf("%s: lines = %#v, want %#v", testCase.name, journal.Lines, testCase.wantLines)
		}
		if journal.Truncated != testCase.wantTruncated {
			t.Errorf("%s: truncated = %v, want %v", testCase.name, journal.Truncated, testCase.wantTruncated)
		}
	}

	// Unicode truncation is by code point, not by byte.
	journal, err := parseJournalTailOutput([]byte(longLine+"\n"), 10, false)
	if err != nil {
		t.Fatalf("unicode truncation: %v", err)
	}
	if got := len([]rune(journal.Lines[0])); got != maxJournalLineLength {
		t.Errorf("truncated rune count = %d, want %d", got, maxJournalLineLength)
	}
	if got := len(journal.Lines[0]); got != maxJournalLineLength*2 {
		t.Errorf("truncated byte length = %d, want %d", got, maxJournalLineLength*2)
	}

	cases := []struct {
		name     string
		output   []byte
		maxLines int
	}{
		{"NUL", []byte("alpha\x00beta\n"), 10},
		{"invalid UTF-8 high byte", []byte("alpha\xffbeta\n"), 10},
		{"invalid UTF-8 lone bytes", []byte{0xff, 0xfe, '\n'}, 10},
		{"invalid UTF-8 truncated rune", []byte("alpha\xc3\n"), 10},
		{"maxLines zero", []byte("alpha\n"), 0},
		{"maxLines negative", []byte("alpha\n"), -1},
		{"maxLines above cap", []byte("alpha\n"), maxJournalLines + 1},
	}
	for _, testCase := range cases {
		if _, err := parseJournalTailOutput(testCase.output, testCase.maxLines, false); !errors.Is(err, errReadActionOutput) {
			t.Errorf("%s: error = %v, want errReadActionOutput", testCase.name, err)
		}
	}

	// Invalid UTF-8 is rejected before any string conversion and the error
	// never echoes the untrusted bytes.
	if _, err := parseJournalTailOutput([]byte("secret\xffpayload\n"), 10, false); !errors.Is(err, errReadActionOutput) {
		t.Errorf("invalid UTF-8: error = %v, want errReadActionOutput", err)
	} else if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "payload") {
		t.Errorf("invalid UTF-8 error echoed journal output: %v", err)
	}
}

// TestReadActionsJournalParserTransportTruncatedUTF8Suffix covers the
// cross-layer contract with OPS-02: the raw output byte budget can split the
// final UTF-8 rune, so a transport-truncated tail may end with exactly one
// incomplete encoding. Only that case is recovered; every other invalid byte
// stays rejected, and no bytes are normalized or echoed.
func TestReadActionsJournalParserTransportTruncatedUTF8Suffix(t *testing.T) {
	recoverable := []struct {
		name   string
		body   string
		suffix []byte
		want   []string
	}{
		{"two byte prefix", "alpha\n", []byte{0xC3}, []string{"alpha"}},
		{"three byte prefix", "alpha\n", []byte{0xE2, 0x82}, []string{"alpha"}},
		{"four byte prefix", "alpha\n", []byte{0xF0, 0x9F, 0x98}, []string{"alpha"}},
		{"after valid multibyte", "caf\u00e9\n", []byte{0xE2, 0x82}, []string{"caf\u00e9"}},
		{"only incomplete rune", "", []byte{0xF0, 0x9F}, nil},
	}
	for _, testCase := range recoverable {
		output := append([]byte(testCase.body), testCase.suffix...)
		journal, err := parseJournalTailOutput(output, maxJournalLines, true)
		if err != nil {
			t.Errorf("%s: transport-truncated recovery: %v", testCase.name, err)
			continue
		}
		if !equalStringSlices(journal.Lines, testCase.want) {
			t.Errorf("%s: lines = %#v, want %#v", testCase.name, journal.Lines, testCase.want)
		}
		if !journal.Truncated {
			t.Errorf("%s: truncated = false, want true", testCase.name)
		}
		if _, err := parseJournalTailOutput(output, maxJournalLines, false); !errors.Is(err, errReadActionOutput) {
			t.Errorf("%s: without transport truncation error = %v, want errReadActionOutput", testCase.name, err)
		}
	}

	rejected := []struct {
		name   string
		output []byte
	}{
		{"embedded invalid byte", []byte("alpha\xffbeta\n")},
		{"invalid continuation sequence", []byte("alpha\xc3\x28")},
		{"invalid continuation after three byte start", []byte("alpha\xe2\x82\x28")},
		{"invalid continuation after four byte start", []byte("alpha\xf0\x9f\x28")},
		{"trailing standalone invalid byte", []byte("alpha\xff")},
		{"single invalid byte", []byte{0xff}},
		{"invalid two byte encoding", []byte("\xc3\x28")},
		{"invalid byte before incomplete suffix", []byte("alpha\xff\xc3")},
		{"incomplete rune not at end", []byte("alpha\xc3\n")},
		{"complete invalid encoding after valid rune", []byte("alpha\n\u00e9\xff")},
	}
	for _, testCase := range rejected {
		if _, err := parseJournalTailOutput(testCase.output, maxJournalLines, true); !errors.Is(err, errReadActionOutput) {
			t.Errorf("%s: error = %v, want errReadActionOutput", testCase.name, err)
		}
	}

	// Complete valid Unicode at the end is preserved and never peeled off.
	valid := []byte("alpha\n\u00e9\n")
	for _, truncated := range []bool{false, true} {
		journal, err := parseJournalTailOutput(valid, maxJournalLines, truncated)
		if err != nil {
			t.Fatalf("valid trailing unicode (truncated=%v): %v", truncated, err)
		}
		if !equalStringSlices(journal.Lines, []string{"alpha", "\u00e9"}) {
			t.Errorf("valid trailing unicode (truncated=%v): lines = %#v", truncated, journal.Lines)
		}
		if journal.Truncated != truncated {
			t.Errorf("valid trailing unicode (truncated=%v): Truncated = %v", truncated, journal.Truncated)
		}
	}

	// Rejections never echo the untrusted bytes.
	if _, err := parseJournalTailOutput([]byte("supersecret\xff\xc3"), maxJournalLines, true); !errors.Is(err, errReadActionOutput) {
		t.Errorf("truncated invalid UTF-8: error = %v, want errReadActionOutput", err)
	} else if strings.Contains(err.Error(), "supersecret") {
		t.Errorf("truncated invalid UTF-8 error echoed output: %v", err)
	}
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func equalDiskSlices(got, want []diskInfo) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// This file classifies a caller-written command (ADR 0026): read-only commands
// may run without approval, forbidden commands never run, and every other
// command is a change that needs approval.
//
// The proof is a parse of a deliberately narrow shell grammar, never a pattern
// match on raw text. Plain words, single- and double-quoted strings, the
// operators &&, ||, ; and |, the redirections 2>&1, output to /dev/null and
// reading a file with <, are understood. Anything else, including $ expansion,
// backticks, subshells, braces, background jobs, other redirections and
// escapes, cannot be proven and is therefore a change. Only the forbidden check
// also looks at the raw text, so a command the grammar cannot parse is still
// refused when it plainly destroys the server.
package serverops

import (
	"path"
	"regexp"
	"strings"
	"unicode"
)

// CommandClass is the approval class of one command.
type CommandClass string

const (
	// CommandReadOnly cannot change the server or reveal a sensitive file.
	CommandReadOnly CommandClass = "read_only"
	// CommandChange may change the server, or could not be proven read-only.
	CommandChange CommandClass = "change"
	// CommandForbidden would destroy the server or cut off access to it.
	CommandForbidden CommandClass = "forbidden"
)

// ClassifyCommand returns the approval class of one command.
func ClassifyCommand(command string) CommandClass {
	commands, parsed := parseCommandLine(command)
	if (parsed && anyForbidden(commands)) || (!parsed && forbiddenRaw(command)) {
		return CommandForbidden
	}
	if !parsed || len(commands) == 0 {
		return CommandChange
	}
	for _, simple := range commands {
		if !readOnlySimpleCommand(simple) {
			return CommandChange
		}
	}
	return CommandReadOnly
}

// IsReadOnlyInvocation reports whether one action with its input can only
// read the server. The fixed read actions always do; the command run action
// does when its command classifies read-only.
func IsReadOnlyInvocation(action string, input map[string]any) bool {
	switch action {
	case ActionSystemSnapshot, ActionServiceStatus, ActionJournalTail:
		return true
	case ActionCommandRun:
		command, err := commandFromInput(input)
		return err == nil && ClassifyCommand(command) == CommandReadOnly
	default:
		return false
	}
}

// simpleCommand is one program with its arguments and the files it reads
// through input redirection.
type simpleCommand struct {
	words  []string
	inputs []string
}

// parseCommandLine splits a command into simple commands. It reports false for
// anything outside the narrow grammar.
func parseCommandLine(command string) ([]simpleCommand, bool) {
	var (
		commands []simpleCommand
		current  simpleCommand
		word     strings.Builder
		inWord   bool
		quoted   bool
		// pending is true after &&, || or | until the next command starts.
		pending bool
	)
	flushWord := func() {
		if inWord {
			current.words = append(current.words, word.String())
		}
		word.Reset()
		inWord, quoted = false, false
	}
	// endCommand closes the current command before an operator, which needs
	// a command on its left.
	endCommand := func() bool {
		flushWord()
		if len(current.words) == 0 {
			return false
		}
		commands = append(commands, current)
		current, pending = simpleCommand{}, true
		return true
	}
	runes := []rune(command)
	readTarget := func(index int) (string, int, bool) {
		for index < len(runes) && (runes[index] == ' ' || runes[index] == '\t') {
			index++
		}
		var target strings.Builder
		for index < len(runes) {
			r := runes[index]
			if r == ' ' || r == '\t' || r == '\n' || strings.ContainsRune(";|&<>()$`\\'\"{}", r) {
				break
			}
			target.WriteRune(r)
			index++
		}
		return target.String(), index, target.Len() > 0
	}
	for index := 0; index < len(runes); index++ {
		r := runes[index]
		switch {
		case r == ' ' || r == '\t':
			flushWord()
		case r == '\n' || r == ';':
			// A separator may end an empty line; a newline may also follow an
			// operator, which then still waits for its right-hand command.
			flushWord()
			if len(current.words) > 0 {
				commands = append(commands, current)
				current, pending = simpleCommand{}, false
			} else if r == ';' && pending {
				return nil, false
			}
		case r == '\'':
			end := index + 1
			for end < len(runes) && runes[end] != '\'' {
				end++
			}
			if end >= len(runes) {
				return nil, false
			}
			word.WriteString(string(runes[index+1 : end]))
			inWord, quoted, index = true, true, end
		case r == '"':
			end := index + 1
			for end < len(runes) && runes[end] != '"' {
				if strings.ContainsRune("$`\\", runes[end]) {
					return nil, false
				}
				end++
			}
			if end >= len(runes) {
				return nil, false
			}
			word.WriteString(string(runes[index+1 : end]))
			inWord, quoted, index = true, true, end
		case r == '&':
			if index+1 < len(runes) && runes[index+1] == '&' {
				if !endCommand() {
					return nil, false
				}
				index++
				continue
			}
			if index+1 < len(runes) && runes[index+1] == '>' && !inWord {
				// &>/dev/null discards both streams.
				target, next, ok := readTarget(index + 2)
				if !ok || target != "/dev/null" {
					return nil, false
				}
				index = next - 1
				continue
			}
			return nil, false
		case r == '|':
			if index+1 < len(runes) && runes[index+1] == '&' {
				return nil, false
			}
			if !endCommand() {
				return nil, false
			}
			if index+1 < len(runes) && runes[index+1] == '|' {
				index++
			}
		case r == '>':
			// An optional file descriptor directly before the operator.
			if inWord && (quoted || strings.Trim(word.String(), "0123456789") != "") {
				return nil, false
			}
			word.Reset()
			inWord, quoted = false, false
			if index+1 < len(runes) && runes[index+1] == '>' {
				return nil, false
			}
			if index+1 < len(runes) && runes[index+1] == '&' {
				end := index + 2
				for end < len(runes) && runes[end] >= '0' && runes[end] <= '9' {
					end++
				}
				if end == index+2 {
					return nil, false
				}
				index = end - 1
				continue
			}
			target, next, ok := readTarget(index + 1)
			if !ok || target != "/dev/null" {
				return nil, false
			}
			index = next - 1
		case r == '<':
			if inWord || (index+1 < len(runes) && strings.ContainsRune("<(&", runes[index+1])) {
				return nil, false
			}
			target, next, ok := readTarget(index + 1)
			if !ok {
				return nil, false
			}
			current.inputs = append(current.inputs, target)
			index = next - 1
		case strings.ContainsRune("$`\\(){}", r):
			return nil, false
		case (r == '#' || r == '!') && !inWord:
			return nil, false
		default:
			word.WriteRune(r)
			inWord, pending = true, false
		}
	}
	flushWord()
	if len(current.words) > 0 {
		commands = append(commands, current)
	} else if pending {
		// A trailing operator leaves its command incomplete.
		return nil, false
	}
	return commands, true
}

// programName returns the program a simple command runs. A path is accepted
// only under the standard binary directories.
func programName(word string) (string, bool) {
	if !strings.Contains(word, "/") {
		return word, word != ""
	}
	switch path.Dir(word) {
	case "/bin", "/sbin", "/usr/bin", "/usr/sbin", "/usr/local/bin", "/usr/local/sbin":
		return path.Base(word), true
	}
	return "", false
}

// sensitivePathMarkers name files whose content would hand a secret to the
// model. Reading them always asks.
var sensitivePathMarkers = []string{
	"/etc/shadow", "/etc/gshadow", "sudoers", "/environ", ".env",
	"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "/.ssh/", ".pgpass", ".netrc",
	".git-credentials", ".aws/credentials", ".docker/config.json", ".kube/config",
}

func sensitivePath(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range sensitivePathMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key")
}

func readOnlySimpleCommand(simple simpleCommand) bool {
	for _, input := range simple.inputs {
		if sensitivePath(input) {
			return false
		}
	}
	for _, word := range simple.words[1:] {
		if sensitivePath(word) {
			return false
		}
	}
	name, ok := programName(simple.words[0])
	if !ok || strings.Contains(simple.words[0], "=") {
		return false
	}
	args := simple.words[1:]
	// Asking any installed program for its version only reports it.
	if len(args) == 1 && args[0] == "--version" {
		return true
	}
	if rule, known := readOnlyPrograms[name]; known {
		return rule(args)
	}
	return false
}

// readOnlyPrograms maps each allowlisted program to its argument rule. A
// program that is not listed is a change.
var readOnlyPrograms = map[string]func([]string) bool{}

func init() {
	anyArgs := func([]string) bool { return true }
	for _, name := range []string{
		"ls", "cat", "head", "wc", "cut", "tr", "echo", "printf", "pwd", "whoami", "id", "uptime",
		"uname", "nproc", "lscpu", "lsblk", "findmnt", "free", "df", "du", "stat", "file", "readlink",
		"realpath", "basename", "dirname", "md5sum", "sha1sum", "sha256sum", "which", "ps", "pgrep",
		"ss", "netstat", "lsof", "w", "who", "last", "getent", "grep", "egrep", "fgrep", "zgrep",
		"true", "false", "column", "nl", "rev", "comm", "diff", "base64", "od", "hexdump",
		"jq", "numfmt", "seq", "zcat", "vmstat", "iostat", "mpstat",
	} {
		readOnlyPrograms[name] = anyArgs
	}
	readOnlyPrograms["uniq"] = func(args []string) bool { return len(positional(args)) <= 1 }
	readOnlyPrograms["tail"] = rejectFlags("-f", "-F", "--follow")
	readOnlyPrograms["sort"] = rejectFlags("-o", "--output")
	readOnlyPrograms["date"] = rejectFlags("-s", "--set")
	readOnlyPrograms["dmesg"] = rejectFlags("-c", "-C", "-D", "-E", "-n", "--clear", "--read-clear", "--console-off", "--console-on", "--console-level")
	readOnlyPrograms["hostname"] = func(args []string) bool { return len(positional(args)) == 0 }
	readOnlyPrograms["mount"] = func(args []string) bool { return len(args) == 0 }
	// command runs its argument unless it only looks the name up.
	readOnlyPrograms["command"] = func(args []string) bool {
		return len(args) >= 2 && (args[0] == "-v" || args[0] == "-V")
	}
	readOnlyPrograms["type"] = anyArgs
	readOnlyPrograms["crontab"] = func(args []string) bool { return len(args) == 1 && args[0] == "-l" }
	readOnlyPrograms["top"] = func(args []string) bool { return hasFlag(args, "-b") && hasFlag(args, "-n") }
	readOnlyPrograms["nginx"] = func(args []string) bool {
		return (hasFlag(args, "-t") || hasFlag(args, "-T")) && onlyFlags(args, map[string]bool{"-t": true, "-T": true, "-q": true}, map[string]bool{"-c": true, "-p": true})
	}
	readOnlyPrograms["sed"] = readOnlySed
	readOnlyPrograms["awk"] = readOnlyAwk
	readOnlyPrograms["gawk"] = readOnlyAwk
	readOnlyPrograms["mawk"] = readOnlyAwk
	readOnlyPrograms["find"] = rejectFlags("-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls")
	readOnlyPrograms["journalctl"] = rejectFlags("-f", "--follow", "--vacuum-size", "--vacuum-time", "--vacuum-files", "--rotate", "--flush", "--sync", "--relinquish-var", "--smart-relinquish-var", "--setup-keys", "--update-catalog")
	readOnlyPrograms["systemctl"] = readOnlySystemctl
	readOnlyPrograms["git"] = readOnlyGit
	readOnlyPrograms["docker"] = readOnlyContainerCLI
	readOnlyPrograms["podman"] = readOnlyContainerCLI
	readOnlyPrograms["ip"] = readOnlyIP
	readOnlyPrograms["curl"] = readOnlyCurl
	statusOnly := func(args []string) bool {
		if len(args) == 0 {
			return true
		}
		verb := args[0]
		return verb == "status" || verb == "show" || strings.HasPrefix(verb, "list")
	}
	readOnlyPrograms["timedatectl"] = statusOnly
	readOnlyPrograms["hostnamectl"] = statusOnly
	readOnlyPrograms["loginctl"] = statusOnly
}

// positional returns the arguments that are not flags.
func positional(args []string) []string {
	out := []string{}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			out = append(out, arg)
		}
	}
	return out
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

// rejectFlags builds a rule that refuses any of the named flags, alone, with
// an =value, or as a combined short flag such as -fn.
func rejectFlags(flags ...string) func([]string) bool {
	return func(args []string) bool {
		for _, arg := range args {
			for _, flag := range flags {
				if arg == flag || strings.HasPrefix(arg, flag+"=") {
					return false
				}
				if len(flag) == 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(shortFlagLetters(arg), rune(flag[1])) {
					return false
				}
			}
		}
		return true
	}
}

// shortFlagLetters returns the letters of a combined short flag up to its
// first value character, so -fn100 yields fn.
func shortFlagLetters(arg string) string {
	letters := strings.TrimPrefix(arg, "-")
	for index, r := range letters {
		if !unicode.IsLetter(r) {
			return letters[:index]
		}
	}
	return letters
}

// onlyFlags accepts only the listed flags; valued flags consume the next word.
func onlyFlags(args []string, flags, valued map[string]bool) bool {
	for index := 0; index < len(args); index++ {
		switch {
		case flags[args[index]]:
		case valued[args[index]] && index+1 < len(args):
			index++
		default:
			return false
		}
	}
	return true
}

var sedPrintScript = regexp.MustCompile(`^(\d+|\$)(,(\d+|\$))?p$`)

// readOnlySed accepts only sed -n with line-range print scripts.
func readOnlySed(args []string) bool {
	quiet, scripts := false, 0
	for _, arg := range args {
		switch {
		case arg == "-n" || arg == "--quiet" || arg == "--silent":
			quiet = true
		case strings.HasPrefix(arg, "-"):
			return false
		case scripts == 0:
			if !sedPrintScript.MatchString(arg) {
				return false
			}
			scripts++
		}
	}
	return quiet && scripts == 1
}

var awkSideEffects = []string{"system", "getline", "|", ">", "close", "fflush", "printf >"}

// readOnlyAwk accepts an inline program without commands, pipes, output
// redirection, or reading other files.
func readOnlyAwk(args []string) bool {
	program := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "-F" || arg == "-v":
			index++
		case strings.HasPrefix(arg, "-F") || strings.HasPrefix(arg, "-v"):
		case strings.HasPrefix(arg, "-"):
			return false
		case !program:
			for _, marker := range awkSideEffects {
				if strings.Contains(arg, marker) {
					return false
				}
			}
			program = true
		}
	}
	return program
}

// systemctlValued are the systemctl flags whose value is the next word.
var systemctlValued = map[string]bool{"-p": true, "--property": true, "-t": true, "--type": true, "--state": true, "-n": true, "--lines": true, "-o": true, "--output": true, "-H": true, "--host": true, "-M": true, "--machine": true}

var systemctlReadVerbs = map[string]bool{
	"status": true, "show": true, "cat": true, "is-active": true, "is-enabled": true, "is-failed": true,
	"is-system-running": true, "list-units": true, "list-unit-files": true, "list-timers": true,
	"list-sockets": true, "list-dependencies": true, "list-jobs": true, "list-machines": true,
}

func readOnlySystemctl(args []string) bool {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if systemctlValued[arg] {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return systemctlReadVerbs[arg]
	}
	// Only flags, such as systemctl --failed, lists units.
	return true
}

// gitReadSubcommands may take any arguments except a file output.
var gitReadSubcommands = map[string]bool{
	"status": true, "log": true, "diff": true, "show": true, "rev-parse": true, "describe": true,
	"ls-files": true, "ls-tree": true, "blame": true, "shortlog": true, "cat-file": true, "rev-list": true,
}

func readOnlyGit(args []string) bool {
	index := 0
	for index < len(args) {
		switch {
		case args[index] == "-C" && index+1 < len(args):
			index += 2
		case args[index] == "--no-pager":
			index++
		default:
			goto subcommand
		}
	}
subcommand:
	if index >= len(args) {
		return false
	}
	sub, rest := args[index], args[index+1:]
	for _, arg := range rest {
		if strings.HasPrefix(arg, "--output") || strings.HasPrefix(arg, "--ext-diff") || strings.HasPrefix(arg, "--textconv") {
			return false
		}
	}
	if gitReadSubcommands[sub] {
		return true
	}
	switch sub {
	case "branch", "tag":
		for _, arg := range rest {
			switch arg {
			case "-a", "-r", "-v", "-vv", "-l", "--list", "--show-current", "--all", "--remotes":
			default:
				return false
			}
		}
		return true
	case "remote":
		return len(rest) == 0 || (len(rest) == 1 && rest[0] == "-v") || (len(rest) >= 1 && (rest[0] == "show" || rest[0] == "get-url"))
	case "stash":
		return len(rest) >= 1 && (rest[0] == "list" || rest[0] == "show")
	case "config":
		return len(rest) >= 1 && (rest[0] == "--get" || rest[0] == "--get-all" || rest[0] == "--list" || rest[0] == "-l" || rest[0] == "--get-regexp")
	}
	return false
}

var containerReadSubcommands = map[string]bool{
	"ps": true, "inspect": true, "images": true, "version": true, "info": true, "top": true, "port": true,
}

var containerReadGroups = map[string]map[string]bool{
	"container": {"ls": true, "list": true, "ps": true, "inspect": true, "logs": true, "port": true, "top": true},
	"image":     {"ls": true, "list": true, "inspect": true, "history": true},
	"network":   {"ls": true, "list": true, "inspect": true},
	"volume":    {"ls": true, "list": true, "inspect": true},
	"compose":   {"ps": true, "logs": true, "config": true, "ls": true, "images": true, "top": true, "version": true},
}

func readOnlyContainerCLI(args []string) bool {
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return false
	}
	sub := rest[0]
	followed := rejectFlags("-f", "--follow")
	switch {
	case containerReadSubcommands[sub]:
		return true
	case sub == "logs":
		return followed(rest[1:])
	case sub == "stats":
		return hasFlag(rest[1:], "--no-stream")
	}
	group, ok := containerReadGroups[sub]
	if !ok {
		return false
	}
	inner := rest[1:]
	for len(inner) > 0 && strings.HasPrefix(inner[0], "-") {
		if inner[0] == "-f" || inner[0] == "--file" || inner[0] == "-p" || inner[0] == "--project-name" {
			if len(inner) < 2 {
				return false
			}
			inner = inner[2:]
			continue
		}
		inner = inner[1:]
	}
	if len(inner) == 0 || !group[inner[0]] {
		return false
	}
	return followed(inner[1:])
}

var ipReadObjects = map[string]bool{"a": true, "addr": true, "address": true, "l": true, "link": true, "r": true, "route": true, "n": true, "neigh": true, "neighbor": true, "rule": true}

func readOnlyIP(args []string) bool {
	rest := args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
		rest = rest[1:]
	}
	if len(rest) == 0 || !ipReadObjects[rest[0]] {
		return false
	}
	return len(rest) == 1 || rest[1] == "show" || rest[1] == "list" || rest[1] == "ls" || rest[1] == "get"
}

// curlValued are the curl flags whose value is the next word.
var curlValued = map[string]bool{"-H": true, "--header": true, "-m": true, "--max-time": true, "-w": true, "--write-out": true, "-A": true, "--user-agent": true, "-e": true, "--referer": true, "--connect-timeout": true, "--resolve": true}

var curlWriting = []string{"-o", "--output", "-O", "--remote-name", "--remote-name-all", "-X", "--request", "-d", "--data", "--data-raw", "--data-binary", "--data-urlencode", "-F", "--form", "--form-string", "-T", "--upload-file", "-K", "--config", "--json", "-c", "--cookie-jar", "-D", "--dump-header", "--create-dirs", "-u", "--user"}

// readOnlyCurl accepts GET requests to a local address only, so a read can
// neither change a remote service nor send server data elsewhere.
func readOnlyCurl(args []string) bool {
	urls := 0
	for index := 0; index < len(args); index++ {
		arg := args[index]
		// Discarding the body keeps only the status line, as in a health check.
		if (arg == "-o" || arg == "--output") && index+1 < len(args) && args[index+1] == "/dev/null" {
			index++
			continue
		}
		if arg == "--output=/dev/null" {
			continue
		}
		for _, flag := range curlWriting {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return false
			}
		}
		if curlValued[arg] {
			if arg == "-D" || arg == "--dump-header" {
				return false
			}
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			// Combined short flags such as -sS; any that writes was refused.
			if !strings.HasPrefix(arg, "--") && strings.ContainsAny(arg[1:], "oOXdFTKcDu") {
				return false
			}
			continue
		}
		if !localURL(arg) {
			return false
		}
		urls++
	}
	return urls > 0
}

func localURL(value string) bool {
	rest := value
	if scheme := strings.Index(rest, "://"); scheme >= 0 {
		if s := rest[:scheme]; s != "http" && s != "https" {
			return false
		}
		rest = rest[scheme+3:]
	}
	if at := strings.Index(rest, "@"); at >= 0 {
		return false
	}
	host := rest
	if strings.HasPrefix(host, "[") {
		end := strings.Index(host, "]")
		if end < 0 {
			return false
		}
		host = host[:end+1]
	} else if cut := strings.IndexAny(host, ":/?#"); cut >= 0 {
		host = host[:cut]
	}
	switch host {
	case "127.0.0.1", "localhost", "[::1]", "0.0.0.0":
		return true
	}
	return false
}

// forbiddenRoots are directories whose recursive removal destroys the server.
var forbiddenRoots = map[string]bool{
	"/": true, "/*": true, "~": true, "~/": true, "/bin": true, "/boot": true, "/dev": true, "/etc": true,
	"/home": true, "/lib": true, "/lib64": true, "/opt": true, "/proc": true, "/root": true, "/sbin": true,
	"/srv": true, "/sys": true, "/usr": true, "/var": true,
}

var forbiddenPrograms = map[string]bool{
	"shutdown": true, "reboot": true, "poweroff": true, "halt": true, "mkswap": true,
	"fdisk": true, "sfdisk": true, "parted": true, "wipefs": true,
}

func anyForbidden(commands []simpleCommand) bool {
	for _, simple := range commands {
		if forbiddenSimpleCommand(simple.words) {
			return true
		}
	}
	return false
}

func forbiddenSimpleCommand(words []string) bool {
	if len(words) == 0 {
		return false
	}
	name, _ := programName(words[0])
	args := words[1:]
	if name == "sudo" || name == "command" || name == "exec" || name == "nohup" {
		// Judge the command these wrappers run.
		rest := args
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			if rest[0] == "-u" || rest[0] == "-g" {
				if len(rest) < 2 {
					return false
				}
				rest = rest[2:]
				continue
			}
			rest = rest[1:]
		}
		return forbiddenSimpleCommand(rest)
	}
	switch {
	case forbiddenPrograms[name] || strings.HasPrefix(name, "mkfs"):
		return true
	case name == "init" || name == "telinit":
		return len(args) == 1 && (args[0] == "0" || args[0] == "6")
	case name == "dd":
		for _, arg := range args {
			if strings.HasPrefix(arg, "of=/dev/") && arg != "of=/dev/null" {
				return true
			}
		}
	case name == "systemctl":
		for _, arg := range args {
			switch arg {
			case "reboot", "poweroff", "halt", "kexec", "emergency", "rescue":
				return true
			}
		}
	case name == "rm":
		recursive := false
		for _, arg := range args {
			if arg == "--recursive" || (strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsAny(arg, "rR")) {
				recursive = true
			}
		}
		if !recursive {
			return false
		}
		for _, arg := range positional(args) {
			if forbiddenRoots[strings.TrimRight(arg, "/")] || forbiddenRoots[arg] {
				return true
			}
		}
	}
	return false
}

// rawCommandPosition matches the start of a command: the text start, or after
// a separator, subshell, or backtick, optionally through sudo.
const rawCommandPosition = "(^|[;&|(\\n`])\\s*(sudo\\s+(-\\S+\\s+)*)?"

// forbiddenRawPatterns catch destructive commands the grammar cannot parse.
// Each anchors on a command position so a destructive word that is only an
// argument, as in grep reboot, does not match.
var forbiddenRawPatterns = []*regexp.Regexp{
	regexp.MustCompile(rawCommandPosition + `(shutdown|reboot|poweroff|halt|mkfs(\.\w+)?|mkswap|wipefs|fdisk|parted)(\s|$|[;&|)` + "`" + `])`),
	regexp.MustCompile(rawCommandPosition + `rm\s+(-\S*[rR]\S*\s+)+(--\s+)?(/|/\*|~/?)(\s|$|;)`),
	regexp.MustCompile(rawCommandPosition + `dd\s[^;&|]*of=/dev/(sd|vd|nvme|xvd|hd|mmcblk|dm-|mapper/)`),
	regexp.MustCompile(`:\s*\(\s*\)\s*\{`),
}

func forbiddenRaw(command string) bool {
	for _, pattern := range forbiddenRawPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// UnattendedAction reports whether an unattended run may ever call the action
// (ADR 0027). The fixed reads may; the command action may, but each command
// must still classify read-only when it runs. Service changes never may.
func UnattendedAction(action string) bool {
	switch action {
	case ActionSystemSnapshot, ActionServiceStatus, ActionJournalTail, ActionCommandRun:
		return true
	default:
		return false
	}
}

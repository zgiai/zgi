// This file implements the private, pure read-action layer for the three
// accepted server operations actions: it strictly validates action input,
// builds fixed read-only remote commands, and parses bounded command output into
// private typed values.
//
// It intentionally does not dial, handle connection identity or credentials, or
// run commands: Service wires these helpers to the SSH transport. There is
// deliberately no API that accepts an arbitrary command, command suffix, shell
// fragment, file path, environment variable, hostname, or binary name. The only
// dynamic string any remote command may contain is a service unit name that
// matches SystemdServiceNamePattern exactly; the only dynamic numbers are the
// two bounded journal integers.
package serverops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Private read-action error classes. Only this package builds or parses these
// commands, so the classes stay private: Service projects them onto the public
// Err* sentinels and no caller ever sees a private class.
var (
	// errReadActionInput classifies rejected action input.
	errReadActionInput = errors.New("serverops read action: invalid input")
	// errReadActionOutput classifies rejected or corrupted remote output.
	errReadActionOutput = errors.New("serverops read action: invalid output")
)

// Output text limits mirror the accepted output schemas so a parsed value can
// never exceed the schema bound.
const (
	maxSnapshotHostnameLength   = 255
	maxSnapshotOSLength         = 255
	maxSnapshotKernelLength     = 255
	maxDiskFilesystemLength     = 255
	maxDiskMountPointLength     = 1024
	maxServiceStateLength       = 64
	maxServiceDescriptionLength = 1024
)

// Exact record-protocol headers. A parser accepts nothing but the exact header
// as the first line of command output.
const (
	systemSnapshotHeader = "ZGI_SERVER_SNAPSHOT_V1"
	serviceStatusHeader  = "ZGI_SERVER_SERVICE_STATUS_V1"
)

// systemdServiceNameMatcher is the compiled accepted service-unit grammar. A
// name is validated exactly as supplied and is never trimmed or otherwise
// normalized into validity.
var systemdServiceNameMatcher = regexp.MustCompile(SystemdServiceNamePattern)

// protocolDecimalMatcher accepts the canonical unsigned base-10 decimal form
// used by the snapshot number protocol. Signs, exponents, NaN/Inf spellings,
// and incomplete fractions are rejected instead of reinterpreted.
var protocolDecimalMatcher = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)

// jsonNumberMatcher accepts exactly one standalone RFC 8259 JSON number and
// nothing else. Anchoring both ends is what keeps spellings such as "1/1",
// "5.", "NaN", "Inf", and hexadecimal forms out of the exact big-number path.
var jsonNumberMatcher = regexp.MustCompile(`^(-?)(0|[1-9][0-9]*)(?:\.([0-9]+))?(?:[eE]([-+]?[0-9]+))?$`)

// systemSnapshot is the validated private result of parsing one system
// snapshot. It deliberately uses typed scalars instead of an unstructured map.
type systemSnapshot struct {
	Hostname        string
	OperatingSystem string
	KernelVersion   string
	UptimeSeconds   int64
	LoadAverage1m   float64
	Memory          memoryInfo
	Disks           []diskInfo
}

// memoryInfo is the validated memory record of a system snapshot.
type memoryInfo struct {
	TotalBytes     int64
	AvailableBytes int64
	UsedBytes      int64
}

// diskInfo is one validated disk record of a system snapshot.
type diskInfo struct {
	Filesystem     string
	MountPoint     string
	TotalBytes     int64
	AvailableBytes int64
	UsedBytes      int64
}

// serviceStatus is the validated private result of parsing one service status.
type serviceStatus struct {
	LoadState   string
	ActiveState string
	SubState    string
	Description string
}

// journalTailInput is the fully validated journal tail action input.
type journalTailInput struct {
	ServiceName  string
	Lines        int
	SinceMinutes int
}

// journalTail is the validated private result of parsing one journal tail.
type journalTail struct {
	Lines     []string
	Truncated bool
}

// buildSystemSnapshotCommand returns the single fixed system snapshot script.
// The script takes no input, reads only hostname, /etc/os-release, uname,
// /proc/uptime, /proc/loadavg, /proc/meminfo, and df, and emits the strict
// tab-separated ZGI_SERVER_SNAPSHOT_V1 protocol. Scalar text is stripped of
// tabs, carriage returns, and newlines so remote values cannot inject records.
//
// Every byte counter is formatted with "%.0f" rather than "%d": the values are
// exact multiples of 1024 within the JSON-safe ceiling, and awk's "%d" width
// differs across implementations while "%.0f" is portable and never switches to
// exponent notation for these magnitudes.
func buildSystemSnapshotCommand() string {
	return systemSnapshotScriptPrefix +
		"df -P -k | head -n " + strconv.Itoa(maxDiskEntries+1) +
		" | awk 'NR > 1 && NF >= 6 { mount_point=$6; for (i=7; i<=NF; i++) mount_point=mount_point\" \"$i; " +
		"printf \"disk\\t%s\\t%s\\t%.0f\\t%.0f\\t%.0f\\n\", $1, mount_point, $2*1024, $4*1024, $3*1024 }'\n"
}

// systemSnapshotScriptPrefix is the fixed prefix of the system snapshot
// command. It sets LC_ALL=C, prints the exact header first, and emits every
// scalar and memory record with sanitized values. It contains no dynamic input.
const systemSnapshotScriptPrefix = `LC_ALL=C
export LC_ALL
printf 'ZGI_SERVER_SNAPSHOT_V1\n'
hostname_value=$(hostname | head -n 1 | tr -d '\011\015\012')
printf 'hostname\t%s\n' "$hostname_value"
operating_system_value=$(grep '^PRETTY_NAME=' /etc/os-release | head -n 1 | cut -d= -f2- | tr -d '\011\015\012')
printf 'operating_system\t%s\n' "$operating_system_value"
kernel_version_value=$(uname -sr | head -n 1 | tr -d '\011\015\012')
printf 'kernel_version\t%s\n' "$kernel_version_value"
uptime_seconds_value=$(cut -d' ' -f1 /proc/uptime | cut -d. -f1 | tr -d '\011\015\012')
printf 'uptime_seconds\t%s\n' "$uptime_seconds_value"
load_average_1m_value=$(cut -d' ' -f1 /proc/loadavg | tr -d '\011\015\012')
printf 'load_average_1m\t%s\n' "$load_average_1m_value"
memory_value=$(awk '/^MemTotal:/{total=$2} /^MemAvailable:/{available=$2} END{if (total == "" || available == "") exit 1; printf "%.0f\t%.0f\t%.0f", total*1024, available*1024, (total-available)*1024}' /proc/meminfo)
printf 'memory\t%s\n' "$memory_value"
`

// parseSystemSnapshotOutput validates the strict snapshot protocol and returns
// the typed snapshot. Corrupted output is rejected, never clamped: an absent or
// wrong header, unknown or duplicate records, missing required records, wrong
// field counts, out-of-contract numbers, memory or disk inconsistency, too many
// disks, NUL, invalid UTF-8, control characters, and over-length text all fail.
// Error text never echoes the untrusted output.
func parseSystemSnapshotOutput(output []byte) (systemSnapshot, error) {
	if bytes.IndexByte(output, 0) >= 0 {
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot output contains NUL", errReadActionOutput)
	}
	lines := protocolOutputLines(output)
	if len(lines) == 0 || lines[0] != systemSnapshotHeader {
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the %q header", errReadActionOutput, systemSnapshotHeader)
	}

	var (
		snapshot   systemSnapshot
		seenHost   bool
		seenOS     bool
		seenKernel bool
		seenUptime bool
		seenLoad   bool
		seenMemory bool
	)
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		switch fields[0] {
		case "hostname":
			if seenHost {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate hostname record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 2); err != nil {
				return systemSnapshot{}, err
			}
			if err := validateProtocolText(fields[1], maxSnapshotHostnameLength); err != nil {
				return systemSnapshot{}, err
			}
			snapshot.Hostname = fields[1]
			seenHost = true
		case "operating_system":
			if seenOS {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate operating_system record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 2); err != nil {
				return systemSnapshot{}, err
			}
			if err := validateProtocolText(fields[1], maxSnapshotOSLength); err != nil {
				return systemSnapshot{}, err
			}
			snapshot.OperatingSystem = fields[1]
			seenOS = true
		case "kernel_version":
			if seenKernel {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate kernel_version record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 2); err != nil {
				return systemSnapshot{}, err
			}
			if err := validateProtocolText(fields[1], maxSnapshotKernelLength); err != nil {
				return systemSnapshot{}, err
			}
			snapshot.KernelVersion = fields[1]
			seenKernel = true
		case "uptime_seconds":
			if seenUptime {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate uptime_seconds record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 2); err != nil {
				return systemSnapshot{}, err
			}
			value, err := parseProtocolInteger(fields[1], maxUptimeSeconds)
			if err != nil {
				return systemSnapshot{}, err
			}
			snapshot.UptimeSeconds = value
			seenUptime = true
		case "load_average_1m":
			if seenLoad {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate load_average_1m record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 2); err != nil {
				return systemSnapshot{}, err
			}
			value, err := parseProtocolNumber(fields[1], maxLoadAverage)
			if err != nil {
				return systemSnapshot{}, err
			}
			snapshot.LoadAverage1m = value
			seenLoad = true
		case "memory":
			if seenMemory {
				return systemSnapshot{}, fmt.Errorf("%w: duplicate memory record", errReadActionOutput)
			}
			if err := requireProtocolFieldCount(fields, 4); err != nil {
				return systemSnapshot{}, err
			}
			memory, err := parseMemoryRecord(fields[1:])
			if err != nil {
				return systemSnapshot{}, err
			}
			snapshot.Memory = memory
			seenMemory = true
		case "disk":
			if len(snapshot.Disks) >= maxDiskEntries {
				return systemSnapshot{}, fmt.Errorf("%w: system snapshot exceeds %d disk records", errReadActionOutput, maxDiskEntries)
			}
			if err := requireProtocolFieldCount(fields, 6); err != nil {
				return systemSnapshot{}, err
			}
			disk, err := parseDiskRecord(fields[1:])
			if err != nil {
				return systemSnapshot{}, err
			}
			snapshot.Disks = append(snapshot.Disks, disk)
		default:
			return systemSnapshot{}, fmt.Errorf("%w: system snapshot contains an unknown record", errReadActionOutput)
		}
	}

	switch {
	case !seenHost:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the hostname record", errReadActionOutput)
	case !seenOS:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the operating_system record", errReadActionOutput)
	case !seenKernel:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the kernel_version record", errReadActionOutput)
	case !seenUptime:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the uptime_seconds record", errReadActionOutput)
	case !seenLoad:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the load_average_1m record", errReadActionOutput)
	case !seenMemory:
		return systemSnapshot{}, fmt.Errorf("%w: system snapshot is missing the memory record", errReadActionOutput)
	}
	return snapshot, nil
}

// parseMemoryRecord validates the three memory counters and the invariant that
// used equals total minus available. It rejects negative or over-contract
// counters and inconsistent relationships instead of repairing them.
func parseMemoryRecord(fields []string) (memoryInfo, error) {
	total, err := parseProtocolInteger(fields[0], maxByteCount)
	if err != nil {
		return memoryInfo{}, err
	}
	available, err := parseProtocolInteger(fields[1], maxByteCount)
	if err != nil {
		return memoryInfo{}, err
	}
	used, err := parseProtocolInteger(fields[2], maxByteCount)
	if err != nil {
		return memoryInfo{}, err
	}
	if total < available {
		return memoryInfo{}, fmt.Errorf("%w: memory available exceeds memory total", errReadActionOutput)
	}
	if used != total-available {
		return memoryInfo{}, fmt.Errorf("%w: memory used is inconsistent with total and available", errReadActionOutput)
	}
	return memoryInfo{TotalBytes: total, AvailableBytes: available, UsedBytes: used}, nil
}

// parseDiskRecord validates one disk record and its capacity invariant. It
// rejects negative or over-contract counters and impossible capacities.
func parseDiskRecord(fields []string) (diskInfo, error) {
	if err := validateProtocolText(fields[0], maxDiskFilesystemLength); err != nil {
		return diskInfo{}, err
	}
	if err := validateProtocolText(fields[1], maxDiskMountPointLength); err != nil {
		return diskInfo{}, err
	}
	total, err := parseProtocolInteger(fields[2], maxByteCount)
	if err != nil {
		return diskInfo{}, err
	}
	available, err := parseProtocolInteger(fields[3], maxByteCount)
	if err != nil {
		return diskInfo{}, err
	}
	used, err := parseProtocolInteger(fields[4], maxByteCount)
	if err != nil {
		return diskInfo{}, err
	}
	if total < available || total < used || used+available > total {
		return diskInfo{}, fmt.Errorf("%w: disk capacity is inconsistent", errReadActionOutput)
	}
	return diskInfo{
		Filesystem:     fields[0],
		MountPoint:     fields[1],
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
	}, nil
}

// buildServiceStatusCommand validates the exactly-one-key input and returns the
// fixed LC_ALL=C systemctl show command, the validated service name, and any
// input error. The only dynamic string is the single-quoted unit literal.
func buildServiceStatusCommand(input map[string]any) (string, string, error) {
	if err := rejectUnknownInputKeys(input, "service_name"); err != nil {
		return "", "", err
	}
	serviceName, err := serviceNameFromInput(input)
	if err != nil {
		return "", "", err
	}
	command := "LC_ALL=C\nexport LC_ALL\nprintf '" + serviceStatusHeader + "\\n'\n" +
		"systemctl show --no-pager --property=LoadState --property=ActiveState --property=SubState --property=Description -- " +
		shellSingleQuote(serviceName) + "\n"
	return command, serviceName, nil
}

// parseServiceStatusOutput validates the header plus exactly one each of
// LoadState, ActiveState, SubState, and Description in key=value form. It
// rejects unknown, duplicate, and missing keys, structural line violations,
// NUL, and over-length values, and never executes or infers anything.
func parseServiceStatusOutput(output []byte) (serviceStatus, error) {
	if bytes.IndexByte(output, 0) >= 0 {
		return serviceStatus{}, fmt.Errorf("%w: service status output contains NUL", errReadActionOutput)
	}
	lines := protocolOutputLines(output)
	if len(lines) == 0 || lines[0] != serviceStatusHeader {
		return serviceStatus{}, fmt.Errorf("%w: service status is missing the %q header", errReadActionOutput, serviceStatusHeader)
	}

	var (
		status        serviceStatus
		seenLoad      bool
		seenActive    bool
		seenSub       bool
		seenDescribed bool
	)
	for _, line := range lines[1:] {
		separator := strings.IndexByte(line, '=')
		if separator <= 0 {
			return serviceStatus{}, fmt.Errorf("%w: service status line is not a key=value record", errReadActionOutput)
		}
		key := line[:separator]
		value := line[separator+1:]
		switch key {
		case "LoadState":
			if seenLoad {
				return serviceStatus{}, fmt.Errorf("%w: duplicate LoadState record", errReadActionOutput)
			}
			if err := validateProtocolText(value, maxServiceStateLength); err != nil {
				return serviceStatus{}, err
			}
			status.LoadState = value
			seenLoad = true
		case "ActiveState":
			if seenActive {
				return serviceStatus{}, fmt.Errorf("%w: duplicate ActiveState record", errReadActionOutput)
			}
			if err := validateProtocolText(value, maxServiceStateLength); err != nil {
				return serviceStatus{}, err
			}
			status.ActiveState = value
			seenActive = true
		case "SubState":
			if seenSub {
				return serviceStatus{}, fmt.Errorf("%w: duplicate SubState record", errReadActionOutput)
			}
			if err := validateProtocolText(value, maxServiceStateLength); err != nil {
				return serviceStatus{}, err
			}
			status.SubState = value
			seenSub = true
		case "Description":
			if seenDescribed {
				return serviceStatus{}, fmt.Errorf("%w: duplicate Description record", errReadActionOutput)
			}
			if err := validateProtocolText(value, maxServiceDescriptionLength); err != nil {
				return serviceStatus{}, err
			}
			status.Description = value
			seenDescribed = true
		default:
			return serviceStatus{}, fmt.Errorf("%w: service status contains an unknown key", errReadActionOutput)
		}
	}

	switch {
	case !seenLoad:
		return serviceStatus{}, fmt.Errorf("%w: service status is missing LoadState", errReadActionOutput)
	case !seenActive:
		return serviceStatus{}, fmt.Errorf("%w: service status is missing ActiveState", errReadActionOutput)
	case !seenSub:
		return serviceStatus{}, fmt.Errorf("%w: service status is missing SubState", errReadActionOutput)
	case !seenDescribed:
		return serviceStatus{}, fmt.Errorf("%w: service status is missing Description", errReadActionOutput)
	}
	return status, nil
}

// buildJournalTailCommand validates the journal tail input, applies documented
// defaults, and returns the fixed LC_ALL=C journalctl command plus the fully
// validated input. The only dynamic string is the single-quoted unit literal;
// the two numbers come only from validated integers formatted with strconv.
func buildJournalTailCommand(input map[string]any) (string, journalTailInput, error) {
	var validated journalTailInput
	if err := rejectUnknownInputKeys(input, "service_name", "lines", "since_minutes"); err != nil {
		return "", validated, err
	}
	serviceName, err := serviceNameFromInput(input)
	if err != nil {
		return "", validated, err
	}

	lines := defaultJournalLines
	if raw, present := input["lines"]; present {
		value, err := coerceInputInteger(raw, 1, maxJournalLines)
		if err != nil {
			return "", validated, err
		}
		lines = int(value)
	}
	sinceMinutes := defaultJournalSinceMinutes
	if raw, present := input["since_minutes"]; present {
		value, err := coerceInputInteger(raw, 1, maxJournalSinceMinutes)
		if err != nil {
			return "", validated, err
		}
		sinceMinutes = int(value)
	}

	validated = journalTailInput{ServiceName: serviceName, Lines: lines, SinceMinutes: sinceMinutes}
	command := "LC_ALL=C\nexport LC_ALL\n" +
		"journalctl --no-pager --quiet --output=cat --unit=" + shellSingleQuote(serviceName) +
		" --lines=" + strconv.Itoa(lines) +
		" --since=-" + strconv.Itoa(sinceMinutes) + "min\n"
	return command, validated, nil
}

// parseJournalTailOutput normalizes CRLF, removes only one terminal newline
// artifact, preserves intentional empty and interior lines, returns at most
// maxLines lines, and truncates each retained line to maxJournalLineLength
// Unicode code points. Truncated is set when the transport truncated the output,
// when excess lines were dropped, or when any retained line was shortened.
// NUL always rejects. Valid UTF-8 is unchanged. Invalid UTF-8 rejects unless the
// transport reports truncation and the invalidity is exactly one incomplete
// trailing rune caused by the byte budget; in that one case the incomplete
// suffix is dropped before parsing. An invalid maxLines is rejected, and an
// empty journal is valid.
func parseJournalTailOutput(output []byte, maxLines int, transportTruncated bool) (journalTail, error) {
	if maxLines < 1 || maxLines > maxJournalLines {
		return journalTail{}, fmt.Errorf("%w: journal maxLines %d is outside 1..%d", errReadActionOutput, maxLines, maxJournalLines)
	}
	if bytes.IndexByte(output, 0) >= 0 {
		return journalTail{}, fmt.Errorf("%w: journal output contains NUL", errReadActionOutput)
	}

	result := journalTail{Truncated: transportTruncated}
	if !utf8.Valid(output) {
		if !transportTruncated {
			return journalTail{}, fmt.Errorf("%w: journal output is not valid UTF-8", errReadActionOutput)
		}
		recovered, ok := trimTransportTruncatedUTF8Suffix(output)
		if !ok {
			return journalTail{}, fmt.Errorf("%w: journal output is not valid UTF-8", errReadActionOutput)
		}
		output = recovered
	}

	if len(output) == 0 {
		return result, nil
	}

	text := strings.ReplaceAll(string(output), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	retained := strings.Split(text, "\n")
	if len(retained) > maxLines {
		result.Truncated = true
		retained = retained[:maxLines]
	}

	result.Lines = make([]string, 0, len(retained))
	for _, line := range retained {
		runes := []rune(line)
		if len(runes) > maxJournalLineLength {
			line = string(runes[:maxJournalLineLength])
			result.Truncated = true
		}
		result.Lines = append(result.Lines, line)
	}
	return result, nil
}

// trimTransportTruncatedUTF8Suffix recovers the valid prefix of a raw byte
// stream whose only defect is one incomplete UTF-8 encoding at the very end,
// as produced when a byte budget cuts the final rune. It never normalizes
// RuneError and never echoes bytes.
//
// It only considers a suffix of at most utf8.UTFMax-1 bytes, requires the byte
// at the candidate start to be a rune start, requires everything before it to be
// valid UTF-8, and requires the suffix itself to be incomplete
// (!utf8.FullRune). utf8.FullRune is true for a complete invalid encoding, so an
// embedded invalid byte, an invalid complete encoding, and a trailing standalone
// invalid byte such as 0xff all fail this test and stay rejected.
func trimTransportTruncatedUTF8Suffix(output []byte) ([]byte, bool) {
	limit := len(output) - (utf8.UTFMax - 1)
	if limit < 0 {
		limit = 0
	}
	for start := len(output) - 1; start >= limit; start-- {
		if !utf8.RuneStart(output[start]) {
			continue
		}
		if !utf8.Valid(output[:start]) {
			continue
		}
		if utf8.FullRune(output[start:]) {
			continue
		}
		return output[:start], true
	}
	return nil, false
}

// serviceNameFromInput extracts and validates the single service_name string.
// The value is matched exactly as supplied: surrounding whitespace, an empty
// value, an over-length value, and anything outside SystemdServiceNamePattern
// are rejected, and a rejected value is never trimmed into validity.
func serviceNameFromInput(input map[string]any) (string, error) {
	raw, present := input["service_name"]
	if !present {
		return "", fmt.Errorf("%w: service_name is required", errReadActionInput)
	}
	name, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%w: service_name must be a string", errReadActionInput)
	}
	if name == "" {
		return "", fmt.Errorf("%w: service_name is empty", errReadActionInput)
	}
	if name != strings.TrimSpace(name) {
		return "", fmt.Errorf("%w: service_name has surrounding whitespace", errReadActionInput)
	}
	if len(name) > maxServiceNameLength {
		return "", fmt.Errorf("%w: service_name exceeds %d bytes", errReadActionInput, maxServiceNameLength)
	}
	if !systemdServiceNameMatcher.MatchString(name) {
		return "", fmt.Errorf("%w: service_name is not a valid systemd unit name", errReadActionInput)
	}
	return name, nil
}

// rejectUnknownInputKeys rejects any key outside the action's exact contract.
// The rejected key is deliberately not echoed, so untrusted input cannot reach
// a diagnostic message.
func rejectUnknownInputKeys(input map[string]any, allowed ...string) error {
	for key := range input {
		known := false
		for _, candidate := range allowed {
			if key == candidate {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("%w: unexpected input key", errReadActionInput)
		}
	}
	return nil
}

// coerceInputInteger accepts only a numeric value whose mathematical value is
// an exact integer inside [minimum, maximum]. The accepted Go type surface is
// deliberately narrow: production float64, the signed and unsigned Go integer
// types, and json.Number. float32 is rejected outright because a rounded float
// must never be treated as a validated count. bool, strings, nil, structs,
// maps, slices, NaN/Inf, fractions, unsigned overflow, and out-of-range values
// are rejected, and no error echoes the untrusted value.
func coerceInputInteger(value any, minimum, maximum int64) (int64, error) {
	switch typed := value.(type) {
	case int:
		return checkedIntegerRange(int64(typed), minimum, maximum)
	case int8:
		return checkedIntegerRange(int64(typed), minimum, maximum)
	case int16:
		return checkedIntegerRange(int64(typed), minimum, maximum)
	case int32:
		return checkedIntegerRange(int64(typed), minimum, maximum)
	case int64:
		return checkedIntegerRange(typed, minimum, maximum)
	case uint:
		return checkedUnsignedRange(uint64(typed), minimum, maximum)
	case uint8:
		return checkedUnsignedRange(uint64(typed), minimum, maximum)
	case uint16:
		return checkedUnsignedRange(uint64(typed), minimum, maximum)
	case uint32:
		return checkedUnsignedRange(uint64(typed), minimum, maximum)
	case uint64:
		return checkedUnsignedRange(typed, minimum, maximum)
	case float64:
		return checkedFloatRange(typed, minimum, maximum)
	case json.Number:
		return checkedJSONNumber(typed, minimum, maximum)
	default:
		return 0, fmt.Errorf("%w: value has unsupported type %T", errReadActionInput, value)
	}
}

// checkedIntegerRange bounds-checks an already-integral Go integer.
func checkedIntegerRange(value, minimum, maximum int64) (int64, error) {
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	return value, nil
}

// checkedUnsignedRange rejects unsigned overflow before narrowing to int64.
func checkedUnsignedRange(value uint64, minimum, maximum int64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	return checkedIntegerRange(int64(value), minimum, maximum)
}

// checkedFloatRange rejects NaN/Inf, fractional values, and out-of-range
// values. The conversion is guarded on both sides of the float comparison so a
// rounded float can never slip past the bounds.
func checkedFloatRange(value float64, minimum, maximum int64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%w: value is not a finite integer", errReadActionInput)
	}
	if value != math.Trunc(value) {
		return 0, fmt.Errorf("%w: value is not an integer", errReadActionInput)
	}
	if value < float64(minimum) || value > float64(maximum) {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	return checkedIntegerRange(int64(value), minimum, maximum)
}

// checkedJSONNumber validates a json.Number exactly, without any float64
// rounding. The spelling must be exactly one standalone JSON number; the
// mathematical value is then tested for integrality and range with decimal
// string arithmetic only, so a fractional literal such as
// "5.0000000000000000000000000001" is rejected even though float64 would round
// it to 5. No error echoes the untrusted spelling.
//
// Every step below is written so that no int/int64 arithmetic and no slice or
// strings.Repeat count can overflow, even for adversarial exponent spellings at
// the int64 boundaries:
//   - the exponent is parsed with a fixed 64-bit width and classified by sign on
//     overflow;
//   - the fraction-count subtraction is guarded against underflow before it runs;
//   - a negative scaled exponent is compared before it is negated, which bounds
//     the later negation, int conversion, and slice index;
//   - a nonnegative scaled exponent is compared only after the significant digit
//     count is proven to be at most 19, so the conversion to a Repeat count is
//     at most 18.
func checkedJSONNumber(number json.Number, minimum, maximum int64) (int64, error) {
	matches := jsonNumberMatcher.FindStringSubmatch(string(number))
	if matches == nil {
		return 0, fmt.Errorf("%w: numeric value is not a JSON number", errReadActionInput)
	}
	sign := matches[1]
	// digits joins the integer and fraction digits; fractionDigits records how
	// many of them sit after the decimal point.
	digits := matches[2] + matches[3]
	fractionDigits := len(matches[3])

	// A zero mantissa is exactly zero regardless of exponent and sign, so it is
	// resolved before any exponent arithmetic; this also keeps an enormous
	// exponent spelling such as "0e99999999999999999999" harmless.
	if strings.TrimLeft(digits, "0") == "" {
		return checkedIntegerRange(0, minimum, maximum)
	}

	// Parse the exponent with an explicit 64-bit bound instead of Atoi, whose
	// width is platform-dependent. On overflow the classification depends only
	// on the exponent sign: a nonzero mantissa with a negative exponent that
	// cannot be represented is an infinitesimal fraction and never an integer,
	// while a positive overflow can only be an astronomically large magnitude
	// and is therefore out of range.
	exponent := int64(0)
	if matches[4] != "" {
		parsed, err := strconv.ParseInt(matches[4], 10, 64)
		if err != nil {
			if matches[4][0] == '-' {
				return 0, fmt.Errorf("%w: numeric value is not an integer", errReadActionInput)
			}
			return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
		}
		exponent = parsed
	}

	// fractionDigits is a slice length, which always fits an int64 on every
	// platform this code can build for; the check makes that precondition
	// explicit rather than assumed.
	if uint64(fractionDigits) > uint64(math.MaxInt64) {
		return 0, fmt.Errorf("%w: numeric value is not an integer", errReadActionInput)
	}
	fraction := int64(fractionDigits)

	// Guard the subtraction before performing it. If exponent is so negative
	// that subtracting the fraction count would underflow int64, the value is an
	// infinitesimal fraction and can never be an integer.
	if exponent < math.MinInt64+fraction {
		return 0, fmt.Errorf("%w: numeric value is not an integer", errReadActionInput)
	}
	scaledExponent := exponent - fraction

	if scaledExponent < 0 {
		// Compare before negating. A required fractional shift at least as large
		// as the digit count could only be satisfied by an all-zero mantissa,
		// which was already handled, so the value is not an integer. This also
		// proves the negation, the int conversion, and the slice index below are
		// all bounded: 1 <= -scaledExponent <= len(digits)-1.
		if scaledExponent <= -int64(len(digits)) {
			return 0, fmt.Errorf("%w: numeric value is not an integer", errReadActionInput)
		}
		required := int(-scaledExponent)
		if strings.Trim(digits[len(digits)-required:], "0") != "" {
			return 0, fmt.Errorf("%w: numeric value is not an integer", errReadActionInput)
		}
		digits = digits[:len(digits)-required]
		scaledExponent = 0
	}

	significant := strings.TrimLeft(digits, "0")
	if significant == "" {
		return checkedIntegerRange(0, minimum, maximum)
	}
	// At most 19 decimal digits fit an int64, so a wider magnitude is out of
	// range. This is checked before any arithmetic that involves the exponent.
	if len(significant) > 19 {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	// len(significant) is now 1..19, so the headroom is a bounded int64 in 0..18
	// and neither this comparison nor the Repeat count conversion can overflow.
	if scaledExponent > int64(19-len(significant)) {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	value, err := strconv.ParseInt(significant+strings.Repeat("0", int(scaledExponent)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: value is outside the allowed range", errReadActionInput)
	}
	if sign == "-" {
		value = -value
	}
	return checkedIntegerRange(value, minimum, maximum)
}

// shellSingleQuote renders value as exactly one POSIX shell literal. The helper
// is correct on its own even though the service-name grammar already excludes
// apostrophes: every embedded apostrophe is closed, escaped, and reopened.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// protocolOutputLines splits a newline-delimited protocol payload. Exactly one
// terminal newline artifact is ignored; any other empty line remains and is
// rejected by the record parsers as a structural violation.
func protocolOutputLines(output []byte) []string {
	if len(output) == 0 {
		return nil
	}
	text := strings.TrimSuffix(string(output), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// requireProtocolFieldCount rejects a record with the wrong number of fields.
func requireProtocolFieldCount(fields []string, want int) error {
	if len(fields) != want {
		return fmt.Errorf("%w: record has %d fields, want %d", errReadActionOutput, len(fields), want)
	}
	return nil
}

// parseProtocolInteger accepts only a non-negative canonical base-10 integer
// within maximum. Signs, whitespace, fractions, exponents, and overflow fail.
func parseProtocolInteger(field string, maximum int64) (int64, error) {
	if field == "" {
		return 0, fmt.Errorf("%w: numeric field is empty", errReadActionOutput)
	}
	for index := 0; index < len(field); index++ {
		if field[index] < '0' || field[index] > '9' {
			return 0, fmt.Errorf("%w: numeric field is not a base-10 integer", errReadActionOutput)
		}
	}
	value, err := strconv.ParseInt(field, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: numeric field is not a base-10 integer", errReadActionOutput)
	}
	if value > maximum {
		return 0, fmt.Errorf("%w: numeric field is out of range", errReadActionOutput)
	}
	return value, nil
}

// parseProtocolNumber accepts only a non-negative canonical base-10 decimal
// number within maximum. NaN/Inf spellings, signs, exponents, and incomplete
// fractions fail before conversion.
func parseProtocolNumber(field string, maximum float64) (float64, error) {
	if !protocolDecimalMatcher.MatchString(field) {
		return 0, fmt.Errorf("%w: numeric field is not a base-10 number", errReadActionOutput)
	}
	value, err := strconv.ParseFloat(field, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: numeric field is not a base-10 number", errReadActionOutput)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maximum {
		return 0, fmt.Errorf("%w: numeric field is out of range", errReadActionOutput)
	}
	return value, nil
}

// validateProtocolText rejects invalid UTF-8, control characters, and text
// longer than the schema bound. An empty value is in contract when the schema
// declares no minimum length.
func validateProtocolText(value string, maxLength int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: text is not valid UTF-8", errReadActionOutput)
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] == 0x7f {
			return fmt.Errorf("%w: text contains a control character", errReadActionOutput)
		}
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%w: text exceeds %d characters", errReadActionOutput, maxLength)
	}
	return nil
}

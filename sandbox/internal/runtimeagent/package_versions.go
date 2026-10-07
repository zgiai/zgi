package runtimeagent

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const (
	packageLedgerVersion = agentexecution.SystemPackageLedgerVersion
	maxSystemPackages    = 4096
	packageQueryTimeout  = 30 * time.Second
)

var (
	debianArchitecture = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	debianVersion      = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+:~\-]{0,255}$`)
)

type packageLedgerEnvironment struct {
	Profile      string `json:"profile"`
	Sources      string `json:"sources"`
	Architecture string `json:"architecture"`
}

type systemPackage struct {
	Name         string `json:"name"`
	Architecture string `json:"architecture"`
	Version      string `json:"version"`
}

func (p systemPackage) key() string { return p.Name + ":" + p.Architecture }

func (p systemPackage) valid() bool {
	return aptPackageName.MatchString(p.Name) && debianArchitecture.MatchString(p.Architecture) && debianVersion.MatchString(p.Version)
}

func (b *Backend) packageLedgerEnvironment() packageLedgerEnvironment {
	return packageLedgerEnvironment{Profile: b.profileIdentity, Sources: b.packageSources.Digest(), Architecture: runtime.GOARCH}
}

func (b *Backend) validatePackageLedger(ledger packageLedger) error {
	if len(ledger.Entries) > maxPackageLedgerEntries || len(ledger.Packages) > maxSystemPackages {
		return agentexecution.ErrOutputLimit
	}
	switch ledger.Version {
	case 0:
		if len(ledger.Packages) != 0 || ledger.Environment != (packageLedgerEnvironment{}) {
			return errors.New("unversioned package ledger cannot contain version locks")
		}
	case packageLedgerVersion:
		if ledger.Environment != b.packageLedgerEnvironment() {
			return errors.New("package ledger profile, architecture, or source mismatch; recover with the original environment")
		}
	default:
		return errors.New("unsupported package ledger version")
	}
	for _, entry := range ledger.Entries {
		if err := validateAPTArgs(entry.Args); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(ledger.Packages))
	for _, pkg := range ledger.Packages {
		if !pkg.valid() || seen[pkg.key()] {
			return errors.New("package ledger contains an invalid or duplicate version lock")
		}
		seen[pkg.key()] = true
	}
	if ledger.Version == packageLedgerVersion {
		for _, entry := range ledger.Entries {
			if entry.Args[0] == "install" && len(ledger.Packages) == 0 {
				return errors.New("package installation ledger has no version locks")
			}
		}
	}
	return nil
}

// The query uses a fixed binary, format, database, and minimal environment.
// Neither the workspace nor Job environment can supply dpkg options or paths.
func (b *Backend) installedSystemPackages(ctx context.Context) ([]systemPackage, error) {
	ctx, cancel := context.WithTimeout(ctx, packageQueryTimeout)
	defer cancel()
	queryPath := b.packageQueryPath
	if queryPath == "" {
		queryPath = "/usr/bin/dpkg-query"
	}
	command := exec.CommandContext(ctx, queryPath, "--admindir=/var/lib/dpkg", "--show", "--showformat=${binary:Package}\t${Architecture}\t${Version}\t${db:Status-Status}\n")
	configurePackageProcess(command)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	stdout := packageOutputBuffer{maximum: maxPackageLedgerBytes}
	stderr := packageOutputBuffer{maximum: defaultPackageOutputBytes}
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("query installed system packages: %w: %s", err, stderr.String())
	}
	if stdout.truncated {
		return nil, agentexecution.ErrOutputLimit
	}
	return parseSystemPackages(stdout.String())
}

func parseSystemPackages(raw string) ([]systemPackage, error) {
	var packages []systemPackage
	seen := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimSuffix(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		columns := strings.Split(line, "\t")
		if len(columns) != 4 {
			return nil, errors.New("invalid system package query output")
		}
		if columns[3] != "installed" {
			continue // Removed or partially configured packages are not locks.
		}
		name, suffix, qualified := strings.Cut(columns[0], ":")
		pkg := systemPackage{Name: name, Architecture: columns[1], Version: columns[2]}
		if !pkg.valid() || (qualified && suffix != pkg.Architecture) || seen[pkg.key()] {
			return nil, errors.New("invalid or duplicate installed system package")
		}
		if len(packages) >= maxSystemPackages {
			return nil, agentexecution.ErrOutputLimit
		}
		seen[pkg.key()] = true
		packages = append(packages, pkg)
	}
	slices.SortFunc(packages, func(a, b systemPackage) int { return strings.Compare(a.key(), b.key()) })
	return packages, nil
}

// Only these generated package/version arguments reach the privileged helper;
// ledger commands, URLs, apt configuration, and arbitrary flags never do.
func (b *Backend) restoreSystemPackageVersions(ctx context.Context, expected []systemPackage) error {
	if len(expected) == 0 {
		return nil
	}
	actual, err := b.installedSystemPackages(ctx)
	if err != nil {
		return err
	}
	pins := missingSystemPackageVersions(expected, actual)
	if len(pins) == 0 {
		return nil // A pause/resume needs no network or repeated installation.
	}
	args := append([]string{"install", "-y", "--no-install-recommends", "--allow-downgrades", "--no-remove"}, pins...)
	result := b.runAPTWithMetadataRetry(ctx, args)
	if result.ExitCode != 0 {
		return fmt.Errorf("restore locked system packages failed (no latest-version fallback): %s: %s", result.Error, result.Stderr)
	}
	actual, err = b.installedSystemPackages(ctx)
	if err != nil {
		return err
	}
	if missing := missingSystemPackageVersions(expected, actual); len(missing) != 0 {
		return fmt.Errorf("restored system package version mismatch: %s", missing[0])
	}
	return nil
}

func missingSystemPackageVersions(expected, actual []systemPackage) []string {
	versions := make(map[string]string, len(actual))
	for _, pkg := range actual {
		versions[pkg.key()] = pkg.Version
	}
	var pins []string
	for _, pkg := range expected {
		if versions[pkg.key()] != pkg.Version {
			pins = append(pins, pkg.key()+"="+pkg.Version)
		}
	}
	return pins
}

package runner

import (
	"reflect"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func TestSecureRuntimeLimitsBuildBubblewrapArgs(t *testing.T) {
	limits := secureRuntimeLimits{
		CPUSeconds:    3,
		MemoryBytes:   134217728,
		ProcessLimit:  16,
		OpenFileLimit: 32,
	}

	got := limits.bwrapArgs()
	want := []string{
		"--rlimit", "cpu", "3",
		"--rlimit", "as", "134217728",
		"--rlimit", "nproc", "16",
		"--rlimit", "nofile", "32",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rlimit args:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSecureRuntimeLimitsSkipDisabledValues(t *testing.T) {
	limits := secureRuntimeLimits{
		MemoryBytes:   64,
		OpenFileLimit: 8,
	}

	got := limits.bwrapArgs()
	want := []string{
		"--rlimit", "as", "64",
		"--rlimit", "nofile", "8",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rlimit args:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSecureRuntimeLimitsFromConfig(t *testing.T) {
	cfg := config.Config{
		SecureRuntimeCPUSeconds:    4,
		SecureRuntimeMemoryBytes:   256,
		SecureRuntimeProcessLimit:  12,
		SecureRuntimeOpenFileLimit: 24,
	}

	got := secureRuntimeLimitsFromConfig(cfg)
	want := secureRuntimeLimits{
		CPUSeconds:    4,
		MemoryBytes:   256,
		ProcessLimit:  12,
		OpenFileLimit: 24,
	}
	if got != want {
		t.Fatalf("unexpected secure runtime limits: got %+v want %+v", got, want)
	}
}

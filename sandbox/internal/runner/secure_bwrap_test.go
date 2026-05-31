package runner

import "testing"

func TestBuildSecureBwrapArgsEnforcesIsolationContract(t *testing.T) {
	args := buildSecureBwrapArgs(secureBwrapSpec{
		RootFS:        "/runtime/rootfs",
		WorkDir:       "/workspace",
		Binary:        "python3",
		Args:          []string{"script.py"},
		EnableNetwork: false,
		Env: map[string]string{
			"Z_VAR": "z",
			"A_VAR": "a",
		},
	})

	assertArgPair(t, args, "--ro-bind", "/runtime/rootfs")
	assertArgPair(t, args, "--bind", "/workspace")
	assertArgPair(t, args, "--chdir", secureWorkspacePath)
	assertArgPair(t, args, "--uid", "65534")
	assertArgPair(t, args, "--gid", "65534")
	for _, flag := range []string{"--clearenv", "--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup", "--unshare-net"} {
		if !hasArg(args, flag) {
			t.Fatalf("expected %s in bwrap args: %#v", flag, args)
		}
	}

	aIndex := argPairIndex(args, "--setenv", "A_VAR")
	zIndex := argPairIndex(args, "--setenv", "Z_VAR")
	if aIndex < 0 || zIndex < 0 || aIndex > zIndex {
		t.Fatalf("expected deterministic env order, got %#v", args)
	}
	if args[len(args)-2] != "python3" || args[len(args)-1] != "script.py" {
		t.Fatalf("expected binary and args at end, got %#v", args)
	}
}

func TestBuildSecureBwrapArgsAllowsNetworkWhenRequested(t *testing.T) {
	args := buildSecureBwrapArgs(secureBwrapSpec{
		RootFS:        "/runtime/rootfs",
		WorkDir:       "/workspace",
		Binary:        "python3",
		EnableNetwork: true,
	})

	if hasArg(args, "--unshare-net") {
		t.Fatalf("expected network namespace to remain shared when network is enabled, got %#v", args)
	}
}

func assertArgPair(t *testing.T, args []string, key string, value string) {
	t.Helper()
	if argPairIndex(args, key, value) < 0 {
		t.Fatalf("expected %s %s in args: %#v", key, value, args)
	}
}

func argPairIndex(args []string, key string, value string) int {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == key && args[index+1] == value {
			return index
		}
	}
	return -1
}

func hasArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

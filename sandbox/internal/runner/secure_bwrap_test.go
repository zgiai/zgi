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
	assertArgPair(t, args, "--dir", "/proc")
	assertArgPair(t, args, "--chdir", secureWorkspacePath)
	assertArgPair(t, args, "--uid", "65534")
	assertArgPair(t, args, "--gid", "65534")
	for _, flag := range []string{"--clearenv", "--unshare-user", "--unshare-pid", "--unshare-ipc", "--unshare-uts", "--unshare-cgroup", "--unshare-net"} {
		if !hasArg(args, flag) {
			t.Fatalf("expected %s in bwrap args: %#v", flag, args)
		}
	}
	if hasArg(args, "--proc") {
		t.Fatalf("expected parent procfs to stay hidden, got %#v", args)
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

func TestBuildSecureBwrapArgsAddsProfileEnvAfterRequestEnv(t *testing.T) {
	profileEnv, err := secureDependencyProfileEnv("skill-office")
	if err != nil {
		t.Fatalf("profile env: %v", err)
	}
	args := buildSecureBwrapArgs(secureBwrapSpec{
		RootFS:              "/runtime/rootfs",
		WorkDir:             "/workspace",
		Binary:              "python3",
		ProfileHostDir:      "/runtime/rootfs/opt/zgi/profiles/skill-office",
		ProfileContainerDir: "/opt/zgi/profiles/skill-office",
		Env: map[string]string{
			"NODE_PATH": "/workspace/node_modules",
			"Z_VAR":     "z",
		},
		ProfileEnv: profileEnv,
	})

	if lastSetenvValue(args, "NODE_PATH") != "/opt/zgi/profiles/skill-office/node_modules" {
		t.Fatalf("expected profile NODE_PATH to win, got %#v", args)
	}
	if lastSetenvValue(args, "PYTHONNOUSERSITE") != "1" {
		t.Fatalf("expected python user site isolation, got %#v", args)
	}
	expectedPath := "/opt/zgi/profiles/skill-office/venv/bin:/opt/zgi/profiles/skill-office/node_modules/.bin:" + defaultSecurePath
	if lastSetenvValue(args, "PATH") != expectedPath {
		t.Fatalf("expected profile PATH, got %#v", args)
	}
	if argPairIndex(args, "--setenv", "Z_VAR") > lastSetenvKeyIndex(args, "PATH") {
		t.Fatalf("expected profile env after request env, got %#v", args)
	}
	assertArgPair(t, args, "--ro-bind", "/runtime/rootfs/opt/zgi/profiles/skill-office")
	bindIndex := argPairIndex(args, "--ro-bind", "/runtime/rootfs/opt/zgi/profiles/skill-office")
	if bindIndex+2 >= len(args) || args[bindIndex+2] != "/opt/zgi/profiles/skill-office" {
		t.Fatalf("expected profile bind target, got %#v", args)
	}
	if bindIndex > singleArgIndex(args, "--unshare-net") {
		t.Fatalf("expected profile bind before network isolation, got %#v", args)
	}
}

func TestSecureDependencyProfileEnvRejectsUnsafeName(t *testing.T) {
	_, err := secureDependencyProfileEnv("../skill-office")
	if err == nil {
		t.Fatal("expected unsafe dependency profile name rejection")
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

func singleArgIndex(args []string, value string) int {
	for index, arg := range args {
		if arg == value {
			return index
		}
	}
	return -1
}

func lastSetenvValue(args []string, name string) string {
	index := lastSetenvKeyIndex(args, name)
	if index < 0 || index+2 >= len(args) {
		return ""
	}
	return args[index+2]
}

func lastSetenvKeyIndex(args []string, name string) int {
	index := -1
	for i := 0; i < len(args)-2; i++ {
		if args[i] == "--setenv" && args[i+1] == name {
			index = i
		}
	}
	return index
}

func hasArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

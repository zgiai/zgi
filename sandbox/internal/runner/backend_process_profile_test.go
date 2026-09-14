package runner

import "testing"

func TestProcessDependencyProfileEnvProtectsImmutableProfile(t *testing.T) {
	env := processDependencyProfileEnv("/tmp/profile")
	if env["PYTHONDONTWRITEBYTECODE"] != "1" {
		t.Fatalf("PYTHONDONTWRITEBYTECODE = %q, want 1", env["PYTHONDONTWRITEBYTECODE"])
	}
	if env["PYTHONNOUSERSITE"] != "1" {
		t.Fatalf("PYTHONNOUSERSITE = %q, want 1", env["PYTHONNOUSERSITE"])
	}
}

func TestProcessRuntimeProfileEnvRedirectsGeneratedCachesToWorkspace(t *testing.T) {
	env := processRuntimeProfileEnv(processDependencyProfileEnv("/tmp/profile"), "/tmp/workspace")
	want := map[string]string{
		"XDG_CACHE_HOME":  "/tmp/workspace/.zgi-runtime-cache",
		"MPLCONFIGDIR":    "/tmp/workspace/.zgi-runtime-cache/matplotlib",
		"NUMBA_CACHE_DIR": "/tmp/workspace/.zgi-runtime-cache/numba",
	}
	for key, value := range want {
		if env[key] != value {
			t.Fatalf("%s = %q, want %q", key, env[key], value)
		}
	}
	if _, ok := env["PYTHONPYCACHEPREFIX"]; ok {
		t.Fatal("PYTHONPYCACHEPREFIX must remain unset so immutable profile bytecode remains reusable")
	}
}

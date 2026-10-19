package mcpclient

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLocalProcessLauncherEnvironment(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	t.Setenv("LD_LIBRARY_PATH", "/opt/toolcache/lib")
	t.Setenv("ZGI_TEST_PARENT_SECRET", "must-not-leak")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	process, err := LocalProcessLauncher{}.Launch(ctx, ProcessSpec{Command: sh, Args: []string{"-c", `printf '%s|%s|%s' "$LD_LIBRARY_PATH" "$ZGI_TEST_PARENT_SECRET" "$EXPLICIT"`}, Env: map[string]string{"EXPLICIT": "set"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(process.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "/opt/toolcache/lib||set" {
		t.Fatalf("child environment = %q, want loader path and explicit env without parent secrets", got)
	}
}

package runner

import (
	"context"
	"testing"
	"time"
)

func TestRunPython(t *testing.T) {
	service := NewService(1, 2*time.Second, 4096)

	result, err := service.Run(context.Background(), Request{
		Language: "python3",
		Code:     "print('sandbox-ok')",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("expected zero exit code, got %d", result.ExitCode)
	}

	if result.Stdout != "sandbox-ok\n" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
}

func TestRunUnsupportedLanguage(t *testing.T) {
	service := NewService(1, 2*time.Second, 4096)

	_, err := service.Run(context.Background(), Request{
		Language: "ruby",
		Code:     "puts 'nope'",
	})
	if err == nil {
		t.Fatal("expected an error for unsupported language")
	}
}

func TestSafeBaseEnvDropsDangerousKeys(t *testing.T) {
	env := safeBaseEnv([]string{
		"PATH=/usr/bin",
		"LD_PRELOAD=x",
		"DYLD_INSERT_LIBRARIES=x",
		"NODE_OPTIONS=--require x",
		"ZGI_OK=1",
	})

	for _, item := range env {
		if item == "LD_PRELOAD=x" || item == "DYLD_INSERT_LIBRARIES=x" || item == "NODE_OPTIONS=--require x" {
			t.Fatalf("expected dangerous env to be dropped, got %v", env)
		}
	}
	if len(env) != 2 {
		t.Fatalf("expected safe env entries only, got %v", env)
	}
}

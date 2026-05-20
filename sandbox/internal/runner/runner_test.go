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

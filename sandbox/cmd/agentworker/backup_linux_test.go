//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

func TestRecoveryStorageIdentityAndExplicitTarget(t *testing.T) {
	first := workerConfig{CheckpointDir: t.TempDir()}
	second := workerConfig{CheckpointDir: t.TempDir()}
	a, err := recoveryStorageDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	b, err := recoveryStorageDigest(second)
	if err != nil || a == b {
		t.Fatalf("separate target digest=%s error=%v", b, err)
	}
	link := filepath.Join(t.TempDir(), "source-link")
	if err := os.Symlink(first.CheckpointDir, link); err != nil {
		t.Fatal(err)
	}
	c, err := recoveryStorageDigest(workerConfig{CheckpointDir: link})
	if err != nil || c != a {
		t.Fatal("source alias not recognized")
	}
	s3 := workerConfig{CheckpointS3: workspacecheckpoint.S3Config{Bucket: "private", Prefix: "source"}}
	a, err = recoveryStorageDigest(s3)
	if err != nil {
		t.Fatal(err)
	}
	s3.CheckpointS3.Prefix = "source/"
	b, err = recoveryStorageDigest(s3)
	if err != nil || a != b {
		t.Fatal("S3 prefix alias not recognized")
	}
	t.Setenv("ZGI_AGENT_RUNTIME_RECOVERY_TARGET", "")
	if err := runCheckpointBackup(t.Context(), workerConfig{}, "--restore-checkpoints", t.TempDir()); err == nil {
		t.Fatal("unguarded restore accepted")
	}
}

func TestBackupArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--backup-checkpoints"}, {"--restore-checkpoints", ""}, {"--not-backup", "private"}, {"--backup-checkpoints", "private", "extra"}} {
		if _, _, err := backupArguments(args); err == nil {
			t.Fatalf("invalid arguments accepted: %v", args)
		}
	}
	if _, _, err := backupArguments([]string{"--backup-checkpoints", "private"}); err != nil {
		t.Fatal(err)
	}
}

//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/checkpointbackup"
	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/storage"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

func checkpointStore(ctx context.Context, cfg workerConfig, readOnly bool) (agentexecution.WorkspaceCheckpointStore, func() error, error) {
	if cfg.CheckpointS3.Bucket != "" {
		store, err := workspacecheckpoint.NewS3Store(ctx, cfg.CheckpointS3)
		return store, func() error { return nil }, err
	}
	if cfg.CheckpointDir == "" {
		return nil, nil, errors.New("durable checkpoint storage is required")
	}
	var store *workspacecheckpoint.FileStore
	var err error
	if readOnly {
		store, err = workspacecheckpoint.OpenFileStore(cfg.CheckpointDir)
	} else {
		store, err = workspacecheckpoint.NewFileStore(cfg.CheckpointDir)
	}
	if err != nil {
		return nil, nil, err
	}
	return store, store.Close, nil
}

func recoveryProfiles(cfg workerConfig) (map[string]string, error) {
	profiles := map[string]string{}
	add := func(id string, profile any) error {
		data, err := json.Marshal(profile)
		if err != nil {
			return err
		}
		var digest string
		digest, err = agentexecution.ReleaseDigest(bytes.NewReader(data))
		profiles[id] = digest
		return err
	}
	if cfg.Backend == e2bBackendName {
		for id, profile := range cfg.E2BProfiles {
			if err := add(id, profile); err != nil {
				return nil, err
			}
		}
	} else {
		for id, profile := range cfg.Profiles {
			if err := add(id, profile); err != nil {
				return nil, err
			}
		}
	}
	return profiles, nil
}

func recoveryStorageDigest(cfg workerConfig) (string, error) {
	var location any
	if cfg.CheckpointS3.Bucket != "" {
		location = struct{ Endpoint, Bucket, Prefix string }{strings.TrimSpace(cfg.CheckpointS3.Endpoint), strings.TrimSpace(cfg.CheckpointS3.Bucket), strings.Trim(cfg.CheckpointS3.Prefix, "/")}
	} else {
		path, err := filepath.EvalSymlinks(cfg.CheckpointDir)
		if err != nil {
			return "", errors.New("recovery checkpoint directory must already exist")
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return "", err
		}
		location = path
	}
	data, err := json.Marshal(location)
	if err != nil {
		return "", err
	}
	return agentexecution.ReleaseDigest(bytes.NewReader(data))
}

func runCheckpointBackup(parent context.Context, cfg workerConfig, command, directory string) error {
	if _, _, err := backupArguments([]string{command, directory}); err != nil {
		return err
	}
	if command == "--restore-checkpoints" && os.Getenv("ZGI_AGENT_RUNTIME_RECOVERY_TARGET") != "true" {
		return errors.New("restore requires ZGI_AGENT_RUNTIME_RECOVERY_TARGET=true and an isolated empty target")
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	started := time.Now()
	var result checkpointbackup.Manifest
	if command == "--verify-checkpoint-backup" {
		var err error
		result, err = checkpointbackup.Verify(ctx, directory)
		if err != nil {
			return err
		}
	} else {
		if _, err := verifyWorkerRelease(cfg); err != nil {
			return err
		}
		profiles, err := recoveryProfiles(cfg)
		if err != nil {
			return err
		}
		storageDigest, err := recoveryStorageDigest(cfg)
		if err != nil {
			return err
		}
		if command == "--backup-checkpoints" {
			bindings, err := storage.ReadCheckpointBindings(ctx, cfg.DatabaseURL, cfg.Backend)
			if err != nil {
				return err
			}
			source, closeSource, err := checkpointStore(ctx, cfg, true)
			if err != nil {
				return err
			}
			defer closeSource()
			_, configuration, err := workerReleaseFingerprint(cfg)
			if err != nil {
				return err
			}
			result, err = checkpointbackup.Export(ctx, directory, checkpointbackup.Manifest{Version: checkpointbackup.Version, Backend: cfg.Backend, CreatedAt: time.Now().UTC(), ConfigurationDigest: configuration, SourceStorageDigest: storageDigest, Profiles: profiles, Bindings: bindings}, source)
			if err != nil {
				return err
			}
		} else {
			result, err = checkpointbackup.Verify(ctx, directory)
			if err != nil {
				return err
			}
			if result.Backend != cfg.Backend {
				return errors.New("recovery backend does not match target")
			}
			if result.SourceStorageDigest == storageDigest {
				return errors.New("recovery requires a separate checkpoint directory or S3 prefix")
			}
			if cfg.CheckpointS3.Bucket == "" {
				backupStorage, err := recoveryStorageDigest(workerConfig{CheckpointDir: filepath.Join(directory, "checkpoints")})
				if err != nil {
					return err
				}
				if storageDigest == backupStorage {
					return errors.New("recovery target must not use the backup's own checkpoint directory")
				}
			}
			for _, binding := range result.Bindings {
				if result.Profiles[binding.ProfileID] != profiles[binding.ProfileID] {
					return errors.New("required recovery profile is missing or changed")
				}
			}
			if err := storage.CheckRecoveryTarget(ctx, cfg.DatabaseURL); err != nil {
				return err
			}
			target, closeTarget, err := checkpointStore(ctx, cfg, false)
			if err != nil {
				return err
			}
			defer closeTarget()
			if _, err = checkpointbackup.CopyTo(ctx, directory, target); err != nil {
				return err
			}
			store, err := storage.Open(config.Config{DatabaseURL: cfg.DatabaseURL, MaxWorkers: 1, AgentRuntimeAdmission: &cfg.Admission})
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.RestoreCheckpointBindings(ctx, cfg.Backend, result.Bindings); err != nil {
				return err
			}
		}
	}
	var total int64
	for _, object := range result.Objects {
		total += object.Size
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Operation   string `json:"operation"`
		Backend     string `json:"backend"`
		Bindings    int    `json:"bindings"`
		Checkpoints int    `json:"checkpoints"`
		Bytes       int64  `json:"bytes"`
		ElapsedMS   int64  `json:"elapsed_ms"`
		Result      string `json:"result"`
	}{command, result.Backend, len(result.Bindings), len(result.Objects), total, time.Since(started).Milliseconds(), "verified"})
}

// Keep diagnostics at the operator CLI, never expose tenant metadata as metrics.
func backupArguments(args []string) (string, string, error) {
	if len(args) != 2 || args[1] == "" {
		return "", "", errors.New("usage: agentworker backup-command private-directory")
	}
	switch args[0] {
	case "--backup-checkpoints", "--verify-checkpoint-backup", "--restore-checkpoints":
	default:
		return "", "", fmt.Errorf("unknown checkpoint backup command %q", args[0])
	}
	return args[0], args[1], nil
}

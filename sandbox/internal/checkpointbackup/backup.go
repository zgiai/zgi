// Package checkpointbackup exports verified, private recovery sets for suspended
// Agent workspaces. A set is not a backup of application data, Jobs or credentials.
package checkpointbackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

const (
	Version                = 1
	maxManifestBytes       = 16 << 20
	maxBindings            = 10000
	maxObjectBytes         = (1 << 30) + (40 << 20)
	maxSetBytes      int64 = 8 << 30
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Object struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Manifest contains confidential tenant metadata, but no Jobs, connection tokens
// or provider compute references. Profiles are hashes of operator configuration.
type Manifest struct {
	Version             int                      `json:"version"`
	Backend             string                   `json:"backend"`
	CreatedAt           time.Time                `json:"created_at"`
	ConfigurationDigest string                   `json:"configuration_digest"`
	SourceStorageDigest string                   `json:"source_storage_digest"`
	Profiles            map[string]string        `json:"profiles"`
	Bindings            []agentexecution.Binding `json:"bindings"`
	Objects             []Object                 `json:"objects"`
}

// ValidateBindings is also used before the restore transaction. Recovery never
// extends retention or trusts an old provider reference to identify live compute.
func ValidateBindings(bindings []agentexecution.Binding, backend string, now time.Time) error {
	if backend == "" || len(bindings) == 0 || len(bindings) > maxBindings {
		return errors.New("recovery requires a bounded nonempty set of suspended bindings")
	}
	ids, workspaces, requests := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, b := range bindings {
		request := b.OrganizationID + "\x00" + b.SubjectID + "\x00" + b.IdempotencyKey
		if b.ID == "" || b.OrganizationID == "" || b.SubjectID == "" || b.AgentID == "" || b.ConversationID == "" || b.ProfileID == "" || b.WorkspaceID == "" || b.IdempotencyKey == "" || b.RequestHash == "" ||
			b.Backend != backend || b.Status != agentexecution.BindingStatusSuspended || b.Generation < 1 || b.Generation == math.MaxInt64 ||
			b.BackendRef != "" || b.WorkspaceRef != "" || b.PendingCheckpointKey != "" || b.FailureReason != "" ||
			!digestPattern.MatchString(b.CheckpointKey) || !b.RetentionExpiresAt.After(now) ||
			ids[b.ID] || workspaces[b.WorkspaceID] || requests[request] {
			return errors.New("invalid, duplicate, expired or provider-bound recovery binding")
		}
		ids[b.ID], workspaces[b.WorkspaceID], requests[request] = true, true, true
	}
	return nil
}

func (m Manifest) Validate(now time.Time) error {
	if m.Version != Version || m.CreatedAt.IsZero() || !regexpReleaseDigest(m.ConfigurationDigest) || !regexpReleaseDigest(m.SourceStorageDigest) {
		return errors.New("invalid recovery manifest version or provenance")
	}
	if err := ValidateBindings(m.Bindings, m.Backend, now); err != nil {
		return err
	}
	required := map[string]bool{}
	for _, b := range m.Bindings {
		if !regexpReleaseDigest(m.Profiles[b.ProfileID]) {
			return errors.New("recovery profile fingerprint is missing")
		}
		required[b.CheckpointKey] = true
	}
	seen := map[string]bool{}
	var total int64
	for _, object := range m.Objects {
		if !required[object.Key] || seen[object.Key] || !digestPattern.MatchString(object.SHA256) || object.Size < 0 || object.Size > maxObjectBytes {
			return errors.New("invalid or duplicate recovery object")
		}
		total += object.Size
		if total > maxSetBytes {
			return errors.New("recovery set exceeds 8 GiB")
		}
		seen[object.Key] = true
	}
	if len(seen) != len(required) {
		return errors.New("recovery set is incomplete")
	}
	return nil
}

func regexpReleaseDigest(value string) bool {
	return len(value) == 71 && value[:7] == "sha256:" && digestPattern.MatchString(value[7:])
}

// Export requires a new directory. It publishes manifest.json only after every
// source and destination object has passed full-stream checksum verification.
// An interrupted set is retained for diagnosis, never treated as ready.
func Export(ctx context.Context, directory string, m Manifest, source agentexecution.WorkspaceCheckpointStore) (Manifest, error) {
	if source == nil {
		return Manifest{}, errors.New("checkpoint source is required")
	}
	if err := ValidateBindings(m.Bindings, m.Backend, time.Now()); err != nil {
		return Manifest{}, err
	}
	if m.Version != Version || !regexpReleaseDigest(m.ConfigurationDigest) || !regexpReleaseDigest(m.SourceStorageDigest) || m.CreatedAt.IsZero() {
		return Manifest{}, errors.New("invalid backup provenance")
	}
	for _, b := range m.Bindings {
		if !regexpReleaseDigest(m.Profiles[b.ProfileID]) {
			return Manifest{}, errors.New("backup profile fingerprint is missing")
		}
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return Manifest{}, fmt.Errorf("create new private backup directory: %w", err)
	}
	destination, err := workspacecheckpoint.NewFileStore(filepath.Join(directory, "checkpoints"))
	if err != nil {
		return Manifest{}, err
	}
	defer destination.Close()
	m.Objects = nil
	seen := map[string]bool{}
	var total int64
	for _, binding := range m.Bindings {
		key := binding.CheckpointKey
		if seen[key] {
			continue
		}
		seen[key] = true
		reader, err := source.Open(ctx, key)
		if err != nil {
			return Manifest{}, fmt.Errorf("open backup source checkpoint: %w", err)
		}
		digest := sha256.New()
		counter := &countReader{Reader: io.TeeReader(reader, digest)}
		err = destination.Save(ctx, key, counter)
		err = errors.Join(err, reader.Close())
		if err != nil {
			return Manifest{}, fmt.Errorf("copy backup checkpoint: %w", err)
		}
		object := Object{Key: key, SHA256: hex.EncodeToString(digest.Sum(nil)), Size: counter.size}
		total += object.Size
		if total > maxSetBytes {
			return Manifest{}, errors.New("backup set exceeds 8 GiB")
		}
		if err := verifyObject(ctx, destination, object); err != nil {
			return Manifest{}, err
		}
		m.Objects = append(m.Objects, object)
	}
	if err := m.Validate(time.Now()); err != nil {
		return Manifest{}, err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, errors.New("backup manifest exceeds 16 MiB")
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "manifest.pending"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Manifest{}, err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(filepath.Join(directory, "manifest.pending"), filepath.Join(directory, "manifest.json")); err != nil {
		return Manifest{}, err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return Manifest{}, err
	}
	err = errors.Join(dir.Sync(), dir.Close())
	return m, err
}

// Verify is read-only and refuses partial, expired, corrupt or symlinked sets.
func Verify(ctx context.Context, directory string) (Manifest, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return Manifest{}, errors.New("backup must be a private real directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Manifest{}, err
	}
	defer root.Close()
	info, err = root.Lstat("manifest.json")
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxManifestBytes || info.Mode().Perm()&0o077 != 0 {
		return Manifest{}, errors.New("backup has no private completed manifest")
	}
	file, err := root.Open("manifest.json")
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(io.LimitReader(file, maxManifestBytes+1))
	decoder.DisallowUnknownFields()
	var m Manifest
	err = decoder.Decode(&m)
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		err = errors.New("trailing backup manifest data")
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return Manifest{}, fmt.Errorf("read completed backup manifest: %w", err)
	}
	if err := m.Validate(time.Now()); err != nil {
		return Manifest{}, err
	}
	store, err := workspacecheckpoint.OpenFileStore(filepath.Join(directory, "checkpoints"))
	if err != nil {
		return Manifest{}, err
	}
	defer store.Close()
	for _, object := range m.Objects {
		if err := verifyObject(ctx, store, object); err != nil {
			return Manifest{}, err
		}
	}
	return m, nil
}

// CopyTo verifies the complete set before publishing any object, and verifies
// destination bytes even when a store reports an idempotent no-op. Database
// records must be restored separately, only after this returns successfully.
func CopyTo(ctx context.Context, directory string, target agentexecution.WorkspaceCheckpointStore) (Manifest, error) {
	if target == nil {
		return Manifest{}, errors.New("recovery checkpoint target is required")
	}
	m, err := Verify(ctx, directory)
	if err != nil {
		return Manifest{}, err
	}
	source, err := workspacecheckpoint.OpenFileStore(filepath.Join(directory, "checkpoints"))
	if err != nil {
		return Manifest{}, err
	}
	defer source.Close()
	for _, object := range m.Objects {
		reader, err := source.Open(ctx, object.Key)
		if err != nil {
			return Manifest{}, err
		}
		err = target.Save(ctx, object.Key, reader)
		err = errors.Join(err, reader.Close())
		if err != nil {
			return Manifest{}, fmt.Errorf("publish recovery object: %w", err)
		}
		if err := verifyObject(ctx, target, object); err != nil {
			return Manifest{}, fmt.Errorf("verify recovery destination: %w", err)
		}
	}
	return m, nil
}

func verifyObject(ctx context.Context, store agentexecution.WorkspaceCheckpointStore, object Object) error {
	reader, err := store.Open(ctx, object.Key)
	if err != nil {
		return fmt.Errorf("open recovery object: %w", err)
	}
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(reader, maxObjectBytes+1))
	err = errors.Join(err, reader.Close())
	if err != nil {
		return fmt.Errorf("verify recovery object: %w", err)
	}
	if size != object.Size || hex.EncodeToString(digest.Sum(nil)) != object.SHA256 {
		return errors.New("recovery object does not match completed manifest")
	}
	return nil
}

type countReader struct {
	io.Reader
	size int64
}

func (r *countReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.size += int64(n)
	return n, err
}

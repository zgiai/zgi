// Package socialops persists an employee's SOP work across conversations and
// restarts. Remote transitions are a server integration boundary, not proof
// supplied by a model; the runtime must obtain readback from a native connector.
package socialops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

const MaxArtifactBytes = 8 << 20

type Service struct {
	db   *gorm.DB
	root *os.Root
	now  func() time.Time
}

func New(db *gorm.DB, dir string, now func() time.Time) (*Service, error) {
	if db == nil || strings.TrimSpace(dir) == "" {
		return nil, invalid()
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, invalid()
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, root: root, now: now}, nil
}
func (s *Service) Close() error { return s.root.Close() }
func invalid() error            { return core.NewError(core.ErrorCodeInvalidInput, false, nil) }
func conflict() error           { return core.NewError(core.ErrorCodeConflict, false, nil) }
func validScope(scope Scope) bool {
	return scope.OrganizationID != uuid.Nil && scope.WorkspaceID != uuid.Nil && scope.OwnerID != uuid.Nil && scope.EmployeeID != uuid.Nil
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func scopeKey(scope Scope) string {
	return hash([]byte(fmt.Sprintf("%s/%s/%s/%s", scope.OrganizationID, scope.WorkspaceID, scope.OwnerID, scope.EmployeeID)))
}
func scoped(db *gorm.DB, scope Scope) *gorm.DB {
	return db.Where("organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ?", scope.OrganizationID, scope.WorkspaceID, scope.OwnerID, scope.EmployeeID)
}
func validKey(s string) bool {
	return s != "" && len(s) <= 128 && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "/\\\x00\r\n") && s != "." && s != ".."
}
func (s *Service) PutArtifact(ctx context.Context, scope Scope, mime string, data []byte) (Artifact, error) {
	if !validScope(scope) || (len(data) == 0 && mime != "text/plain") || len(data) > MaxArtifactBytes || len(mime) > 128 || !strings.Contains(mime, "/") || strings.ContainsAny(mime, "\r\n\x00") {
		return Artifact{}, invalid()
	}
	a := Artifact{ID: uuid.New(), Scope: scope, MIME: mime, Size: int64(len(data)), Digest: hash(data), CreatedAt: s.now().UTC()}
	f, err := s.root.OpenFile(a.ID.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Artifact{}, err
	}
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = s.db.WithContext(ctx).Create(&a).Error
	}
	if err != nil {
		_ = s.root.Remove(a.ID.String())
		return Artifact{}, err
	}
	return a, nil
}
func (s *Service) artifact(ctx context.Context, scope Scope, id uuid.UUID) (Artifact, error) {
	if !validScope(scope) || id == uuid.Nil {
		return Artifact{}, invalid()
	}
	var a Artifact
	if err := scoped(s.db.WithContext(ctx), scope).Where("id = ?", id).Take(&a).Error; err != nil {
		return Artifact{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return a, nil
}

func (s *Service) GetArtifact(ctx context.Context, scope Scope, id uuid.UUID) (Artifact, error) {
	return s.artifact(ctx, scope, id)
}
func (s *Service) ReadArtifact(ctx context.Context, scope Scope, id uuid.UUID) ([]byte, error) {
	a, err := s.artifact(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	info, err := s.root.Lstat(a.ID.String())
	if err != nil || !info.Mode().IsRegular() || info.Size() != a.Size || a.Size > MaxArtifactBytes {
		return nil, invalid()
	}
	f, err := s.root.Open(a.ID.String())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != a.Size || hash(b) != a.Digest {
		return nil, conflict()
	}
	return b, nil
}
func (s *Service) SaveRecord(ctx context.Context, scope Scope, kind, key string, expected int64, artifact uuid.UUID) (Record, error) {
	if !validKey(key) || expected < 0 {
		return Record{}, invalid()
	}
	switch kind {
	case "configuration", "source", "product_fact", "topic", "draft", "review", "feedback", "checkpoint", "weekly_review", "writing_rule", "archive":
	default:
		return Record{}, invalid()
	}
	if _, err := s.artifact(ctx, scope, artifact); err != nil {
		return Record{}, err
	}
	now := s.now().UTC()
	r := Record{ID: uuid.New(), Scope: scope, ScopeKey: scopeKey(scope), Kind: kind, Key: key, Revision: 1, ArtifactID: artifact, UpdatedAt: now}
	if expected == 0 {
		if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
			return Record{}, conflict()
		}
		return r, nil
	}
	result := scoped(s.db.WithContext(ctx).Model(&Record{}), scope).Where("kind = ? AND record_key = ? AND revision = ?", kind, key, expected).Updates(map[string]any{"revision": expected + 1, "artifact_id": artifact, "updated_at": now})
	if result.Error != nil {
		return Record{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Record{}, conflict()
	}
	return s.GetRecord(ctx, scope, kind, key)
}
func (s *Service) GetRecord(ctx context.Context, scope Scope, kind, key string) (Record, error) {
	if !validScope(scope) || !validKey(kind) || !validKey(key) {
		return Record{}, invalid()
	}
	var r Record
	err := scoped(s.db.WithContext(ctx), scope).Where("kind = ? AND record_key = ?", kind, key).Take(&r).Error
	return r, err
}
func (s *Service) ListRecords(ctx context.Context, scope Scope, kind string, after uuid.UUID, limit int) ([]Record, error) {
	if !validScope(scope) || !validKey(kind) || limit < 1 || limit > 100 {
		return nil, invalid()
	}
	query := scoped(s.db.WithContext(ctx), scope).Where("kind = ?", kind)
	if after != uuid.Nil {
		query = query.Where("id > ?", after)
	}
	var out []Record
	err := query.Order("id").Limit(limit).Find(&out).Error
	return out, err
}

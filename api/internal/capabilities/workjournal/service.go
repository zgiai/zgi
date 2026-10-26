package workjournal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func New(db *gorm.DB, now func() time.Time) (*Service, error) {
	if db == nil {
		return nil, invalid()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db, now}, nil
}
func invalid() error  { return core.NewError(core.ErrorCodeInvalidInput, false, nil) }
func conflict() error { return core.NewError(core.ErrorCodeConflict, false, nil) }
func missing() error  { return core.NewError(core.ErrorCodeNotFound, false, nil) }
func validScope(s Scope) bool {
	return s.OrganizationID != uuid.Nil && s.WorkspaceID != uuid.Nil && s.OwnerID != uuid.Nil && s.EmployeeID != uuid.Nil
}
func scoped(db *gorm.DB, s Scope) *gorm.DB {
	return db.Where("organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ?", s.OrganizationID, s.WorkspaceID, s.OwnerID, s.EmployeeID)
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func scopeKey(s Scope) string {
	return hash([]byte(fmt.Sprintf("%s/%s/%s/%s", s.OrganizationID, s.WorkspaceID, s.OwnerID, s.EmployeeID)))
}
func validKey(k string) bool {
	return k != "" && len(k) <= 128 && strings.TrimSpace(k) == k && !strings.ContainsAny(k, "\x00\r\n")
}
func pageLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 100 {
		return 100
	}
	return n
}
func canonical(raw datatypes.JSON, object bool) (datatypes.JSON, error) {
	if len(raw) == 0 || len(raw) > MaxPayloadBytes {
		return nil, invalid()
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// Keep numeric lexemes intact; frozen operation inputs are stored as JSON
	// text and dispatched without conversion through floating point.
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, invalid()
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, invalid()
	}
	if object {
		if _, ok := value.(map[string]any); !ok {
			return nil, invalid()
		}
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) > MaxPayloadBytes {
		return nil, invalid()
	}
	return b, nil
}
func readErr(err error) error {
	if err == gorm.ErrRecordNotFound {
		return missing()
	}
	return err
}
func (s *Service) GetRecord(ctx context.Context, sc Scope, id uuid.UUID) (Record, error) {
	var r Record
	if !validScope(sc) || id == uuid.Nil {
		return r, invalid()
	}
	err := scoped(s.db.WithContext(ctx), sc).First(&r, "id = ?", id).Error
	return r, readErr(err)
}
func (s *Service) GetRecordByKey(ctx context.Context, sc Scope, kind, key string) (Record, error) {
	var r Record
	if !validScope(sc) || !validKey(kind) || !validKey(key) {
		return r, invalid()
	}
	err := scoped(s.db.WithContext(ctx), sc).Where("kind = ? AND key = ?", kind, key).First(&r).Error
	return r, readErr(err)
}
func (s *Service) ListRecords(ctx context.Context, sc Scope, kind string, after uuid.UUID, limit int) ([]Record, error) {
	if !validScope(sc) || (kind != "" && !validKey(kind)) {
		return nil, invalid()
	}
	q := scoped(s.db.WithContext(ctx), sc)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if after != uuid.Nil {
		q = q.Where("id > ?", after)
	}
	rows := []Record{}
	err := q.Order("id ASC").Limit(pageLimit(limit)).Find(&rows).Error
	return rows, err
}
func (s *Service) ListRecordVersions(ctx context.Context, sc Scope, id uuid.UUID, after uint64, limit int) ([]RecordVersion, error) {
	if _, err := s.GetRecord(ctx, sc, id); err != nil {
		return nil, err
	}
	rows := []RecordVersion{}
	err := scoped(s.db.WithContext(ctx), sc).Where("record_id = ? AND revision > ?", id, after).Order("revision ASC").Limit(pageLimit(limit)).Find(&rows).Error
	return rows, err
}

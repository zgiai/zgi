package workjournal

import (
	"context"
	"math"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) SaveRecord(ctx context.Context, sc Scope, req SaveRecordRequest) (Record, error) {
	var out Record
	if req.ExpectedRevision >= math.MaxInt64 || !validScope(sc) || !validKey(req.Kind) || !validKey(req.Key) || !validKey(req.State) {
		return out, invalid()
	}
	content, err := canonical(req.Content, false)
	if err != nil {
		return out, err
	}
	if len(req.Sources) == 0 {
		req.Sources = datatypes.JSON(`[]`)
	}
	sources, err := canonical(req.Sources, false)
	if err != nil {
		return out, err
	}
	if len(sources) == 0 || sources[0] != '[' {
		return out, invalid()
	}
	now := s.now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.ExpectedRevision == 0 {
			out = Record{ID: uuid.New(), Scope: sc, ScopeKey: scopeKey(sc), Kind: req.Kind, Key: req.Key, State: req.State, Content: content, Sources: sources, Revision: 1, CreatedAt: now, UpdatedAt: now}
			r := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&out)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return conflict()
			}
		} else {
			if e := scoped(tx, sc).Where("kind = ? AND key = ?", req.Kind, req.Key).First(&out).Error; e != nil {
				return readErr(e)
			}
			if out.Revision != req.ExpectedRevision {
				return conflict()
			}
			r := scoped(tx.Model(&Record{}), sc).Where("id = ? AND revision = ?", out.ID, req.ExpectedRevision).Updates(map[string]any{"state": req.State, "content": content, "sources": sources, "revision": req.ExpectedRevision + 1, "updated_at": now})
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return conflict()
			}
			out.State = req.State
			out.Content = content
			out.Sources = sources
			out.Revision++
			out.UpdatedAt = now
		}
		v := RecordVersion{ID: uuid.New(), Scope: sc, RecordID: out.ID, Revision: out.Revision, Kind: out.Kind, Key: out.Key, State: out.State, Content: out.Content, Sources: out.Sources, CreatedAt: now}
		return tx.Create(&v).Error
	})
	return out, err
}

package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type imageInspectionEvidence struct {
	ArtifactID            uuid.UUID `json:"artifact_id"`
	Digest                string    `json:"digest"`
	ConfigurationRevision int64     `json:"configuration_revision"`
	Approved              *bool     `json:"approved"`
	Issues                []string  `json:"issues"`
	Description           string    `json:"description"`
	Provider              string    `json:"provider"`
	Model                 string    `json:"model"`
	ObservedAt            time.Time `json:"observed_at"`
}

// RecordImageInspection converts a server-owned vision result into publication
// policy evidence. It is deliberately absent from the model tool catalog.
func (s *Service) RecordImageInspection(ctx context.Context, scope Scope, artifactID, reviewArtifactID uuid.UUID, approved bool) error {
	if s == nil || !validScope(scope) || artifactID == uuid.Nil || reviewArtifactID == uuid.Nil || artifactID == reviewArtifactID {
		return invalid()
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var configurationRecord Record
		if err := scoped(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND record_key = ?", CurrentConfigurationRecordKind, CurrentConfigurationRecordKey).Take(&configurationRecord).Error; err != nil {
			return err
		}
		local := &Service{db: tx, root: s.root, now: s.now}
		configuration, _, err := local.LoadCurrentConfiguration(ctx, scope)
		if err != nil || configuration.Revision < 1 || configuration.Revision != configurationRecord.Revision {
			if err != nil {
				return err
			}
			return invalid()
		}
		image, err := local.artifact(ctx, scope, artifactID)
		if err != nil {
			return err
		}
		imageData, err := local.ReadArtifact(ctx, scope, artifactID)
		if err != nil {
			return err
		}
		if _, _, err = ImageDimensions(imageData); err != nil || image.Digest != hash(imageData) {
			return invalid()
		}
		review, err := local.artifact(ctx, scope, reviewArtifactID)
		if err != nil {
			return err
		}
		if review.MIME != "application/json" {
			return invalid()
		}
		reviewData, err := local.ReadArtifact(ctx, scope, reviewArtifactID)
		if err != nil {
			return err
		}
		var evidence imageInspectionEvidence
		if json.Unmarshal(reviewData, &evidence) != nil || evidence.Approved == nil ||
			evidence.ArtifactID != artifactID || evidence.Digest != image.Digest || evidence.ConfigurationRevision != configuration.Revision ||
			*evidence.Approved != approved {
			return invalid()
		}
		now := s.now().UTC()
		inspection := ImageInspection{ID: uuid.New(), Scope: scope, ScopeKey: scopeKey(scope), ArtifactID: artifactID,
			ArtifactDigest: image.Digest, ConfigurationRevision: configuration.Revision, ReviewArtifactID: reviewArtifactID,
			Approved: approved, ReviewedAt: now}
		err = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope_key"}, {Name: "artifact_id"}, {Name: "configuration_revision"}},
			DoUpdates: clause.Assignments(map[string]any{"artifact_digest": inspection.ArtifactDigest, "review_artifact_id": reviewArtifactID, "approved": approved, "reviewed_at": now}),
		}).Create(&inspection).Error
		return err
	})
}

func (s *Service) requireApprovedImageInspections(ctx context.Context, scope Scope, configurationRevision int64, images []uuid.UUID, lock bool) error {
	if configurationRevision < 1 {
		return invalid()
	}
	seen := map[uuid.UUID]bool{}
	for _, artifactID := range images {
		if artifactID == uuid.Nil || seen[artifactID] {
			return invalid()
		}
		seen[artifactID] = true
		imageData, err := s.ReadArtifact(ctx, scope, artifactID)
		if err != nil {
			return err
		}
		query := scoped(s.db.WithContext(ctx), scope).Where("scope_key = ? AND artifact_id = ? AND configuration_revision = ? AND approved = ?", scopeKey(scope), artifactID, configurationRevision, true)
		if lock {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var inspection ImageInspection
		if err = query.Take(&inspection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return conflict()
			}
			return err
		}
		if inspection.ArtifactDigest != hash(imageData) || inspection.ReviewArtifactID == uuid.Nil {
			return conflict()
		}
		if _, err = s.artifact(ctx, scope, inspection.ReviewArtifactID); err != nil {
			return err
		}
	}
	return nil
}

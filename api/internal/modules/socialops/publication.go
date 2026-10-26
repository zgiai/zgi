package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

type PublicationRequest struct {
	AccountID           string
	ParentPublicationID *uuid.UUID
	Scope
	ConnectionID    uuid.UUID
	ContentKey      string
	Platform        string
	DraftArtifactID uuid.UUID
	ParentRemoteID  string
}
type RemoteMedia struct {
	ID         string     `json:"id"`
	URL        string     `json:"url,omitempty"`
	ArtifactID *uuid.UUID `json:"artifact_id,omitempty"`
}
type RemotePost struct {
	QuotedID    string        `json:"quoted_id,omitempty"`
	RepostedID  string        `json:"reposted_id,omitempty"`
	Media       []RemoteMedia `json:"media,omitempty"`
	ID          string        `json:"id"`
	URL         string        `json:"url"`
	Text        string        `json:"text"`
	ParentID    string        `json:"parent_id,omitempty"`
	PublishedAt time.Time     `json:"published_at"`
}

func (s *Service) PreparePublication(ctx context.Context, r PublicationRequest) (Publication, error) {
	if len(r.AccountID) > 255 || strings.ContainsAny(r.AccountID, "\x00\r\n*") || strings.TrimSpace(r.AccountID) != r.AccountID || r.ConnectionID == uuid.Nil || !validKey(r.ContentKey) || (r.Platform != "x" && r.Platform != "linkedin" && r.Platform != "facebook") || len(r.ParentRemoteID) > 255 {
		return Publication{}, invalid()
	}
	if _, err := s.artifact(ctx, r.Scope, r.DraftArtifactID); err != nil {
		return Publication{}, err
	}
	configurationRevision, archiveTimezone, err := s.publicationConfigurationSnapshot(ctx, r)
	if err != nil {
		return Publication{}, err
	}
	if r.ParentPublicationID != nil {
		parent, err := s.GetPublication(ctx, r.Scope, *r.ParentPublicationID)
		if err != nil {
			return Publication{}, err
		}
		if parent.ConnectionID != r.ConnectionID || parent.AccountID != r.AccountID || parent.Platform != r.Platform || (parent.State != "verified" && parent.State != "archived") || parent.VerifiedAt == nil || parent.RemoteID == "" || (r.ParentRemoteID != "" && r.ParentRemoteID != parent.RemoteID) {
			return Publication{}, conflict()
		}
		r.ParentRemoteID = parent.RemoteID
	}
	now := s.now().UTC()
	p := Publication{AccountID: r.AccountID, ParentPublicationID: r.ParentPublicationID, ID: uuid.New(), Scope: r.Scope, ScopeKey: scopeKey(r.Scope), ConnectionID: r.ConnectionID, ContentKey: r.ContentKey, Platform: r.Platform, DraftArtifactID: r.DraftArtifactID, State: "draft", Revision: 1, ParentRemoteID: r.ParentRemoteID, CreatedAt: now, UpdatedAt: now}
	p.ConfigurationRevision = configurationRevision
	p.ArchiveTimezone = archiveTimezone
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&p)
	if result.Error != nil {
		return Publication{}, result.Error
	}
	if result.RowsAffected == 1 {
		return p, nil
	}
	var prior Publication
	if err := scoped(s.db.WithContext(ctx), r.Scope).Where("connection_id = ? AND account_id = ? AND content_key = ?", r.ConnectionID, r.AccountID, r.ContentKey).Take(&prior).Error; err != nil {
		return Publication{}, err
	}
	if prior.DraftArtifactID != r.DraftArtifactID || prior.Platform != r.Platform || prior.ParentRemoteID != r.ParentRemoteID || !sameOptionalID(prior.ParentPublicationID, r.ParentPublicationID) {
		return Publication{}, conflict()
	}
	return prior, nil
}
func (s *Service) GetPublication(ctx context.Context, scope Scope, id uuid.UUID) (Publication, error) {
	if !validScope(scope) || id == uuid.Nil {
		return Publication{}, invalid()
	}
	var p Publication
	err := scoped(s.db.WithContext(ctx), scope).Where("id = ?", id).Take(&p).Error
	return p, err
}

// ReplaceDraft invalidates review and cannot mutate a reserved or submitted
// operation. Previously persisted artifacts remain available as history.
func (s *Service) ReplaceDraft(ctx context.Context, scope Scope, id uuid.UUID, revision int64, artifactID uuid.UUID) (Publication, error) {
	if _, err := s.artifact(ctx, scope, artifactID); err != nil {
		return Publication{}, err
	}
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if p.State != "draft" && p.State != "reviewed" {
		return Publication{}, conflict()
	}
	return s.transition(ctx, scope, id, revision, p.State, "draft", map[string]any{"draft_artifact_id": artifactID, "reviewed_digest": ""})
}
func (s *Service) transition(ctx context.Context, scope Scope, id uuid.UUID, revision int64, from, to string, fields map[string]any) (Publication, error) {
	if !validScope(scope) || id == uuid.Nil || revision < 1 {
		return Publication{}, invalid()
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["state"] = to
	fields["revision"] = revision + 1
	fields["updated_at"] = s.now().UTC()
	result := scoped(s.db.WithContext(ctx).Model(&Publication{}), scope).Where("id = ? AND revision = ? AND state = ?", id, revision, from).Updates(fields)
	if result.Error != nil {
		return Publication{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Publication{}, conflict()
	}
	return s.GetPublication(ctx, scope, id)
}
func (s *Service) ReviewPublication(ctx context.Context, scope Scope, id uuid.UUID, revision int64, digest string) (Publication, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	data, err := s.ReadArtifact(ctx, scope, p.DraftArtifactID)
	if err != nil {
		return Publication{}, err
	}
	if digest != hash(data) {
		return Publication{}, conflict()
	}
	if p.ConfigurationRevision > 0 {
		bundle, bundleErr := s.draftBundle(ctx, scope, p)
		if bundleErr != nil {
			return Publication{}, bundleErr
		}
		if err = s.requireApprovedImageInspections(ctx, scope, p.ConfigurationRevision, bundle.Images, false); err != nil {
			return Publication{}, err
		}
	}
	return s.transition(ctx, scope, id, revision, "draft", "reviewed", map[string]any{"reviewed_digest": digest})
}
func (s *Service) ReservePublication(ctx context.Context, scope Scope, id uuid.UUID, revision int64) (Publication, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	data, err := s.ReadArtifact(ctx, scope, p.DraftArtifactID)
	if err != nil {
		return Publication{}, err
	}
	if hash(data) != p.ReviewedDigest {
		return Publication{}, conflict()
	}
	if p.ParentPublicationID != nil {
		parent, err := s.GetPublication(ctx, scope, *p.ParentPublicationID)
		if err != nil {
			return Publication{}, err
		}
		if parent.ConnectionID != p.ConnectionID || parent.AccountID != p.AccountID || parent.Platform != p.Platform || parent.RemoteID != p.ParentRemoteID || (parent.State != "verified" && parent.State != "archived") {
			return Publication{}, conflict()
		}
	}
	return s.reserveWithPolicy(ctx, p, revision)
}
func (s *Service) StartDispatch(ctx context.Context, scope Scope, id uuid.UUID, revision int64) (Publication, error) {
	var result Publication
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		local := &Service{db: tx, root: s.root, now: s.now}
		p, err := local.GetPublication(ctx, scope, id)
		if err != nil {
			return err
		}
		var record Record
		err = scoped(tx, scope).Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ? AND record_key = ?", "configuration", "current").Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && p.ConfigurationRevision == 0 {
			result, err = local.transition(ctx, scope, id, revision, "reserved", "dispatching", nil)
			return err
		}
		if err != nil {
			return err
		}
		cfg, _, err := local.LoadCurrentConfiguration(ctx, scope)
		if err != nil {
			return err
		}
		if cfg.Revision != p.ConfigurationRevision || p.ReservedAt == nil {
			return conflict()
		}
		bundle, err := local.draftBundle(ctx, scope, p)
		if err != nil {
			return err
		}
		if err = local.requireApprovedImageInspections(ctx, scope, p.ConfigurationRevision, bundle.Images, true); err != nil {
			return err
		}
		location, err := time.LoadLocation(cfg.Timezone)
		if err != nil || p.ReservedAt.In(location).Format("2006-01-02") != s.now().In(location).Format("2006-01-02") {
			return conflict()
		}
		result, err = local.transition(ctx, scope, id, revision, "reserved", "dispatching", nil)
		return err
	})
	return result, err
}
func (s *Service) RecordUnknown(ctx context.Context, scope Scope, id uuid.UUID, revision int64) (Publication, error) {
	return s.transition(ctx, scope, id, revision, "dispatching", "outcome_unknown", nil)
}
func validURL(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}
func validReceipt(r RemotePost, now time.Time) bool {
	if !validKey(r.ID) || len(r.ID) > 255 || (r.URL != "" && !validURL(r.URL)) || (!r.PublishedAt.IsZero() && (r.PublishedAt.Before(time.Unix(0, 0)) || r.PublishedAt.After(now))) || len(r.Media) > 4 {
		return false
	}
	seen := map[string]bool{}
	for _, m := range r.Media {
		if !validKey(m.ID) || seen[m.ID] || (m.URL != "" && !validURL(m.URL)) {
			return false
		}
		seen[m.ID] = true
	}
	return true
}
func validRemote(r RemotePost, now time.Time) bool {
	if !validReceipt(r, now) || !validURL(r.URL) || !r.PublishedAt.After(time.Unix(0, 0)) || len(r.Text) > MaxArtifactBytes {
		return false
	}
	for _, m := range r.Media {
		if !validURL(m.URL) {
			return false
		}
	}
	return true
}
func receiptFields(r RemotePost) map[string]any {
	media, _ := json.Marshal(r.Media)
	fields := map[string]any{"remote_id": r.ID, "remote_url": r.URL, "media_json": string(media)}
	if !r.PublishedAt.IsZero() {
		fields["published_at"] = r.PublishedAt.UTC()
	}
	return fields
}

// RecordCreated persists an authoritative native ID before readback. Creation
// APIs need not return a URL or timestamp; verification obtains those later.
func (s *Service) RecordCreated(ctx context.Context, scope Scope, id uuid.UUID, revision int64, r RemotePost) (Publication, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if !validReceipt(r, s.now().UTC()) || (r.ParentID != "" && r.ParentID != p.ParentRemoteID) {
		return Publication{}, invalid()
	}
	return s.transition(ctx, scope, id, revision, "dispatching", "remote_created", receiptFields(r))
}

// RecordVerified is called only with a native readback. Known upload IDs must
// match actual remote media before local image references enter the archive.
func (s *Service) RecordVerified(ctx context.Context, scope Scope, id uuid.UUID, revision int64, r RemotePost, actual uuid.UUID, images ...uuid.UUID) (Publication, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if !validRemote(r, s.now().UTC()) || p.RemoteID != r.ID || (p.RemoteURL != "" && p.RemoteURL != r.URL) || p.ParentRemoteID != r.ParentID || (p.PublishedAt != nil && !p.PublishedAt.Equal(r.PublishedAt)) {
		return Publication{}, conflict()
	}
	data, err := s.ReadArtifact(ctx, scope, actual)
	if err != nil {
		return Publication{}, err
	}
	if string(data) != r.Text {
		return Publication{}, conflict()
	}
	var submitted []RemoteMedia
	if p.MediaJSON != "" && json.Unmarshal([]byte(p.MediaJSON), &submitted) != nil {
		return Publication{}, conflict()
	}
	if len(images) > 0 && (len(images) != len(submitted) || len(images) != len(r.Media)) {
		return Publication{}, conflict()
	}
	actualMedia := append([]RemoteMedia{}, r.Media...)
	for i := range actualMedia {
		actualMedia[i].ArtifactID = nil
	}
	for i, imageID := range images {
		if submitted[i].ID != actualMedia[i].ID {
			return Publication{}, conflict()
		}
		if _, err = s.ReadArtifact(ctx, scope, imageID); err != nil {
			return Publication{}, err
		}
		local := imageID
		actualMedia[i].ArtifactID = &local
	}
	media, _ := json.Marshal(actualMedia)
	return s.transition(ctx, scope, id, revision, "remote_created", "verified", map[string]any{"actual_artifact_id": actual, "verified_at": s.now().UTC(), "remote_url": r.URL, "published_at": r.PublishedAt.UTC(), "media_json": string(media), "quoted_id": r.QuotedID, "reposted_id": r.RepostedID})
}
func (s *Service) Archive(ctx context.Context, scope Scope, id uuid.UUID, revision int64, timezone string) (Publication, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Publication{}, err
	}
	if p.ArchiveTimezone != "" {
		timezone = p.ArchiveTimezone
	}
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		return Publication{}, invalid()
	}

	if p.VerifiedAt == nil || p.ActualArtifactID == nil || p.PublishedAt == nil {
		return Publication{}, conflict()
	}
	if _, err = s.ReadArtifact(ctx, scope, *p.ActualArtifactID); err != nil {
		return Publication{}, err
	}
	return s.transition(ctx, scope, id, revision, "verified", "archived", map[string]any{"archive_date": p.PublishedAt.In(zone).Format("2006-01-02")})
}
func (s *Service) RecordMetric(ctx context.Context, scope Scope, id uuid.UUID, values map[string]*int64) (Metric, error) {
	p, err := s.GetPublication(ctx, scope, id)
	if err != nil {
		return Metric{}, err
	}
	now := s.now().UTC()
	if p.PublishedAt == nil || (p.State != "verified" && p.State != "archived") || now.Before(p.PublishedAt.Add(7*24*time.Hour)) || len(values) > 32 {
		return Metric{}, invalid()
	}
	for name, value := range values {
		if !validKey(name) || (value != nil && *value < 0) {
			return Metric{}, invalid()
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return Metric{}, invalid()
	}
	age := int64(now.Sub(*p.PublishedAt) / time.Second)
	m := Metric{ID: uuid.New(), PublicationID: id, ObservedAt: now, AgeSeconds: age, Late: now.After(p.PublishedAt.Add(7 * 24 * time.Hour)), ValuesJSON: string(data)}
	if err = s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return Metric{}, conflict()
	}
	return m, nil
}

// DueMetrics returns only actual verified posts; missing platform metrics stay
// absent until a real connector observation is committed with RecordMetric.
func (s *Service) DueMetrics(ctx context.Context, scope Scope, limit int) ([]Publication, error) {
	if !validScope(scope) || limit < 1 || limit > 100 {
		return nil, invalid()
	}
	var out []Publication
	query := scoped(s.db.WithContext(ctx), scope).Where("state IN ? AND published_at <= ?", []string{"verified", "archived"}, s.now().UTC().Add(-7*24*time.Hour)).Where("NOT EXISTS (SELECT 1 FROM social_operation_metrics m WHERE m.publication_id = social_operation_publications.id)")
	err := query.Order("published_at, id").Limit(limit).Find(&out).Error
	return out, err
}

func sameOptionalID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

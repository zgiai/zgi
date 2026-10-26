package socialops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// WriteDailyArchive rebuilds one date index from verified actual content. A
// rerun replaces the scoped record with CAS rather than appending duplicate
// headings or mixing internal research/review into public publication history.
func (s *Service) WriteDailyArchive(ctx context.Context, scope Scope, date string) (Record, error) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date || !validScope(scope) {
		return Record{}, invalid()
	}
	var posts []Publication
	err = scoped(s.db.WithContext(ctx), scope).Where("state = ? AND archive_date = ?", "archived", date).Order("platform, published_at, id").Limit(501).Find(&posts).Error
	if err != nil {
		return Record{}, err
	}
	if len(posts) > 500 {
		return Record{}, invalid()
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n", date)
	platform := ""
	for _, p := range posts {
		if p.ActualArtifactID == nil || p.VerifiedAt == nil {
			return Record{}, conflict()
		}
		data, readErr := s.ReadArtifact(ctx, scope, *p.ActualArtifactID)
		if readErr != nil {
			return Record{}, readErr
		}
		if platform != p.Platform {
			platform = p.Platform
			fmt.Fprintf(&out, "\n## %s\n", platform)
		}
		fmt.Fprintf(&out, "\n### %s\n\n", p.RemoteID)
		fence := "```"
		for strings.Contains(string(data), fence) {
			fence += "`"
		}
		fmt.Fprintf(&out, "%stext\n%s\n%s\n\n%s\n", fence, data, fence, p.RemoteURL)
		var media []RemoteMedia
		if p.MediaJSON != "" && json.Unmarshal([]byte(p.MediaJSON), &media) != nil {
			return Record{}, conflict()
		}
		for _, m := range media {
			if m.ArtifactID != nil {
				if _, err := s.ReadArtifact(ctx, scope, *m.ArtifactID); err != nil {
					return Record{}, err
				}
				fmt.Fprintf(&out, "\n![Published image](%s)\n", m.ArtifactID.String())
			}
			if m.URL != "" {
				fmt.Fprintf(&out, "\n[Remote image](%s)\n", m.URL)
			}
		}
		if p.QuotedID != "" {
			fmt.Fprintf(&out, "\nQuoted post: %s\n", p.QuotedID)
		}
		if p.RepostedID != "" {
			fmt.Fprintf(&out, "\nReposted post: %s\n", p.RepostedID)
		}
		if p.ParentRemoteID != "" {
			fmt.Fprintf(&out, "\nParent post: %s\n", p.ParentRemoteID)
		}
		if out.Len() > MaxArtifactBytes {
			return Record{}, invalid()
		}
	}
	previous, err := s.GetRecord(ctx, scope, "archive", date)
	var expected int64
	if err == nil {
		expected = previous.Revision
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return Record{}, err
	}
	a, err := s.PutArtifact(ctx, scope, "text/markdown", []byte(out.String()))
	if err != nil {
		return Record{}, err
	}
	return s.SaveRecord(ctx, scope, "archive", date, expected, a.ID)
}

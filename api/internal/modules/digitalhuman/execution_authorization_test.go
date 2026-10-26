package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// These fixtures exercise the real authorization queries, not a permissive
// authorizer stub. Runtime surface tables contain only synthetic public grants.
func linuxAccessFixture(t *testing.T, surface string) (*AccessService, Scope, uuid.UUID, uuid.UUID) {
	t.Helper()
	_, repo := newTestService(t)
	db := repo.db
	if err := db.AutoMigrate(&APIKey{}); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{
		`CREATE TABLE published_runtime_surfaces (id TEXT PRIMARY KEY, resource_type TEXT, resource_id TEXT, organization_id TEXT, workspace_id TEXT, surface TEXT, enabled BOOLEAN, compatibility_source TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE published_runtime_surface_grants (id TEXT PRIMARY KEY, surface_id TEXT, subject_type TEXT, subject_id TEXT, enabled BOOLEAN, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
	} {
		if err := db.Exec(schema).Error; err != nil {
			t.Fatal(err)
		}
	}
	run, resolved, event, _ := linuxExecutionFixture(t, surface)
	a, err := executionAccessFor(run, resolved, event)
	if err != nil {
		t.Fatal(err)
	}
	webApp := a.webApp
	if webApp == uuid.Nil {
		webApp = uuid.New()
	}
	definition := Definition{ID: a.application, WebAppID: webApp, OrganizationID: a.organization, WorkspaceID: a.workspace,
		Name: "synthetic execution", Status: StatusActive, PublishedVersionID: &a.publication}
	version := Version{ID: a.publication, DigitalHumanID: a.application, OrganizationID: a.organization, WorkspaceID: a.workspace,
		State: VersionStatePublished, ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: true},
		Goals: datatypes.JSON(`[]`), Rules: datatypes.JSON(`[]`), MemoryPolicy: datatypes.JSON(`{}`),
		TriggerPolicy: datatypes.JSON(`{}`), PresentationPolicy: datatypes.JSON(`{}`)}
	for _, row := range []any{&definition, &version} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []string{"webapp", "api"} {
		sid := uuid.New()
		if err := db.Exec(`INSERT INTO published_runtime_surfaces (id,resource_type,resource_id,organization_id,workspace_id,surface,enabled) VALUES (?,?,?,?,?,?,?)`,
			sid, "digital_human", a.application, a.organization, a.workspace, entry, true).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO published_runtime_surface_grants (id,surface_id,subject_type,enabled) VALUES (?,?,?,?)`, uuid.New(), sid, "public", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	if surface == SurfaceAPI {
		if err := db.Create(&APIKey{ID: a.principal, DigitalHumanID: a.application, OrganizationID: a.organization, WorkspaceID: a.workspace,
			KeyHash: "synthetic-unused-hash", KeyPrefix: "synthetic", Name: "test", Status: APIKeyStatusActive, UsageCount: 9}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return NewAccessService(db), a.scope(), a.application, a.publication
}

func TestDigitalHumanLinuxAccessRechecksPublicationAndOwnership(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			s, scope, app, publication := linuxAccessFixture(t, surface)
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, publication); err != nil {
				t.Fatal(err)
			}
			for _, mutate := range []func(*Scope){
				func(s *Scope) { s.OrganizationID = uuid.New() },
				func(s *Scope) { s.WorkspaceID = uuid.New() },
				func(s *Scope) { s.UserSubjectID = nil },
				func(s *Scope) { s.ActorID = uuid.New() },
				func(s *Scope) { s.PrincipalID = uuid.New() },
				func(s *Scope) { s.Surface = SurfaceTask },
			} {
				other := scope
				mutate(&other)
				if err := s.AuthorizeLinuxExecution(t.Context(), other, app, publication); err == nil {
					t.Fatalf("conflicting identity was allowed: %+v", other)
				}
			}
			for _, ids := range [][2]uuid.UUID{{uuid.New(), publication}, {app, uuid.New()}} {
				if err := s.AuthorizeLinuxExecution(t.Context(), scope, ids[0], ids[1]); err == nil {
					t.Fatal("different application/publication allowed")
				}
			}
			if err := s.db.Model(&Version{}).Where("id = ?", publication).Update("execution_runtime", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, publication); err == nil {
				t.Fatal("disabled Linux opt-in allowed")
			}
		})
	}
}

func TestDigitalHumanAttachmentAccessDoesNotRequireLinux(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			s, scope, app, publication := linuxAccessFixture(t, surface)
			if err := s.db.Model(&Version{}).Where("id = ?", publication).Update("execution_runtime", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeAttachmentAccess(t.Context(), scope, app, publication); err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, publication); err == nil {
				t.Fatal("attachment access enabled Linux")
			}
			if err := s.AuthorizeAttachmentAccess(t.Context(), scope, app, uuid.New()); err == nil {
				t.Fatal("different publication accepted")
			}
			if surface == SurfaceAPI {
				if err := s.db.Model(&APIKey{}).Where("id = ?", scope.PrincipalID).Update("status", APIKeyStatusRevoked).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.AuthorizeAttachmentAccess(t.Context(), scope, app, publication); err == nil {
					t.Fatal("revoked API credential accepted")
				}
			}
		})
	}
}

func TestDigitalHumanLinuxContinuationUsesFrozenPublicationAndLiveAccess(t *testing.T) {
	for _, surface := range []string{SurfaceConsole, SurfaceWebApp, SurfaceAPI} {
		t.Run(surface, func(t *testing.T) {
			s, scope, app, original := linuxAccessFixture(t, surface)
			var next Version
			if err := s.db.First(&next, "id = ?", original).Error; err != nil {
				t.Fatal(err)
			}
			next.ID = uuid.New()
			if err := s.db.Create(&next).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.db.Model(&Definition{}).Where("id = ?", app).Update("published_version_id", next.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, original); err == nil {
				t.Fatal("ordinary execution accepted a stale publication")
			}
			if err := s.AuthorizeLinuxContinuation(t.Context(), scope, app, original); err != nil {
				t.Fatalf("verified continuation lost frozen publication: %v", err)
			}
			for _, mutate := range []map[string]any{
				{"digital_human_id": uuid.New()}, {"organization_id": uuid.New()}, {"workspace_id": uuid.New()},
				{"state": VersionStateDraft}, {"execution_runtime": nil},
			} {
				tx := s.db.Begin()
				if err := tx.Model(&Version{}).Where("id = ?", original).Updates(mutate).Error; err != nil {
					t.Fatal(err)
				}
				probe := NewAccessService(tx)
				err := probe.AuthorizeLinuxContinuation(t.Context(), scope, app, original)
				if rollback := tx.Rollback().Error; rollback != nil {
					t.Fatal(rollback)
				}
				if err == nil {
					t.Fatalf("invalid frozen version accepted: %v", mutate)
				}
			}
			if err := s.db.Model(&Version{}).Where("id = ?", next.ID).Update("execution_runtime", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeLinuxContinuation(t.Context(), scope, app, original); err == nil {
				t.Fatal("current execution revocation ignored")
			}
		})
	}
}

func TestDigitalHumanLinuxAccessLiveExternalRevocation(t *testing.T) {
	for _, test := range []struct {
		name, surface string
		revoke        func(*gorm.DB, Scope, uuid.UUID, time.Time) error
	}{
		{"webapp policy", SurfaceWebApp, func(db *gorm.DB, _ Scope, app uuid.UUID, _ time.Time) error {
			return db.Exec(`UPDATE published_runtime_surfaces SET enabled = false WHERE resource_id = ? AND surface = 'webapp'`, app).Error
		}},
		{"API policy", SurfaceAPI, func(db *gorm.DB, _ Scope, app uuid.UUID, _ time.Time) error {
			return db.Exec(`UPDATE published_runtime_surfaces SET enabled = false WHERE resource_id = ? AND surface = 'api'`, app).Error
		}},
		{"API key revoked", SurfaceAPI, func(db *gorm.DB, s Scope, _ uuid.UUID, _ time.Time) error {
			return db.Model(&APIKey{}).Where("id = ?", s.PrincipalID).Update("status", APIKeyStatusRevoked).Error
		}},
		{"API key expired", SurfaceAPI, func(db *gorm.DB, s Scope, _ uuid.UUID, now time.Time) error {
			return db.Model(&APIKey{}).Where("id = ?", s.PrincipalID).Update("expires_at", now.Add(-time.Hour)).Error
		}},
		{"application disabled", SurfaceAPI, func(db *gorm.DB, _ Scope, app uuid.UUID, _ time.Time) error {
			return db.Model(&Definition{}).Where("id = ?", app).Update("status", StatusDisabled).Error
		}},
		{"publication replaced", SurfaceWebApp, func(db *gorm.DB, _ Scope, app uuid.UUID, _ time.Time) error {
			return db.Model(&Definition{}).Where("id = ?", app).Update("published_version_id", uuid.New()).Error
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, scope, app, publication := linuxAccessFixture(t, test.surface)
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, publication); err != nil {
				t.Fatal(err)
			}
			if test.surface == SurfaceAPI {
				var key APIKey
				if err := s.db.First(&key, "id = ?", scope.PrincipalID).Error; err != nil || key.UsageCount != 9 || key.LastUsedAt != nil {
					t.Fatal("authorization changed credential usage")
				}
			}
			if err := test.revoke(s.db, scope, app, s.now()); err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeLinuxExecution(t.Context(), scope, app, publication); err == nil {
				t.Fatal("revoked external access still allowed")
			}
			if err := s.AuthorizeLinuxContinuation(t.Context(), scope, app, publication); err == nil {
				t.Fatal("continuation ignored live access revocation")
			}
		})
	}
}

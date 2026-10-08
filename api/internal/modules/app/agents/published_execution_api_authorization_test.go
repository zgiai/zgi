package agents

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPublishedExecutionAPIRechecksCredentialAndSurface(t *testing.T) {
	tests := map[string]func(*gorm.DB, *Agent, *uuid.UUID) error{
		"active":      nil,
		"missing key": func(_ *gorm.DB, _ *Agent, key *uuid.UUID) error { *key = uuid.Nil; return nil },
		"unknown key": func(_ *gorm.DB, _ *Agent, key *uuid.UUID) error { *key = uuid.New(); return nil },
		"revoked": func(db *gorm.DB, _ *Agent, _ *uuid.UUID) error {
			return db.Table("agent_api_keys").Update("status", "inactive").Error
		},
		"expired": func(db *gorm.DB, _ *Agent, _ *uuid.UUID) error {
			return db.Table("agent_api_keys").Update("expires_at", time.Now().Add(-time.Hour)).Error
		},
		"other application key": func(db *gorm.DB, _ *Agent, _ *uuid.UUID) error {
			return db.Table("agent_api_keys").Update("agent_id", uuid.New()).Error
		},
		"other credential workspace": func(db *gorm.DB, _ *Agent, _ *uuid.UUID) error {
			return db.Table("agent_api_keys").Update("tenant_id", uuid.New()).Error
		},
		"offline application":         func(_ *gorm.DB, a *Agent, _ *uuid.UUID) error { a.WebAppStatus = AgentWebAppStatusInactive; return nil },
		"disabled API":                func(_ *gorm.DB, a *Agent, _ *uuid.UUID) error { a.EnableAPI = false; return nil },
		"deleted application":         func(_ *gorm.DB, a *Agent, _ *uuid.UUID) error { now := time.Now(); a.DeletedAt = &now; return nil },
		"non agent":                   func(_ *gorm.DB, a *Agent, _ *uuid.UUID) error { a.AgentsType = "workflow"; return nil },
		"other application workspace": func(_ *gorm.DB, a *Agent, _ *uuid.UUID) error { a.TenantID = uuid.New(); return nil },
		"surface revoked": func(db *gorm.DB, a *Agent, _ *uuid.UUID) error {
			return db.Table("published_runtime_surfaces").Create(map[string]interface{}{"id": uuid.New(), "resource_type": "agent", "resource_id": a.ID, "surface": "api", "enabled": false}).Error
		},
		"surface store unavailable": func(db *gorm.DB, _ *Agent, _ *uuid.UUID) error {
			return db.Exec("DROP TABLE published_runtime_surfaces").Error
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{AllowGlobalUpdate: true})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := sqlDB.Close(); err != nil {
					t.Error(err)
				}
			})
			for _, schema := range []string{
				"CREATE TABLE agent_api_keys (id TEXT PRIMARY KEY, agent_id TEXT, tenant_id TEXT, status TEXT, expires_at DATETIME)",
				"CREATE TABLE published_runtime_surfaces (id TEXT PRIMARY KEY, resource_type TEXT, resource_id TEXT, surface TEXT, enabled BOOLEAN, deleted_at DATETIME)",
				"CREATE TABLE published_runtime_surface_grants (id TEXT PRIMARY KEY, surface_id TEXT, subject_type TEXT, subject_id TEXT, enabled BOOLEAN, created_at DATETIME, deleted_at DATETIME)",
			} {
				if err := db.Exec(schema).Error; err != nil {
					t.Fatal(err)
				}
			}
			key, workspace := uuid.New(), uuid.New()
			a := &Agent{ID: uuid.New(), TenantID: workspace, AgentsType: "AGENT", WebAppStatus: AgentWebAppStatusActive, EnableAPI: true}
			if err := db.Table("agent_api_keys").Create(map[string]interface{}{"id": key, "agent_id": a.ID, "tenant_id": workspace, "status": "active"}).Error; err != nil {
				t.Fatal(err)
			}
			if mutate != nil {
				if err := mutate(db, a, &key); err != nil {
					t.Fatal(err)
				}
			}
			s := &agentsService{db: db, agentsRepo: &publishedRuntimeRepo{agent: a}}
			err = s.authorizePublishedExecutionAPI(t.Context(), a.ID, key, workspace)
			if (err == nil) != (name == "active") {
				t.Fatalf("authorization=%v", err)
			}
		})
	}
	if err := (&agentsService{}).authorizePublishedExecutionAPI(t.Context(), uuid.New(), uuid.New(), uuid.New()); err == nil {
		t.Fatal("missing stores authorized execution")
	}
}

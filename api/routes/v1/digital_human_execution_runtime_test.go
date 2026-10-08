package v1

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDigitalHumanProductionLinuxRequiresIdentityEvenWithOperatorAdmission(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	organization := uuid.New()
	t.Setenv("ZGI_AGENT_RUNTIME_ENABLED", "true")
	t.Setenv("ZGI_AGENT_RUNTIME_URL", "http://127.0.0.1:1")
	t.Setenv("ZGI_AGENT_RUNTIME_API_KEY", "synthetic-worker-credential")
	t.Setenv("ZGI_AGENT_RUNTIME_APPROVED_PROFILES_JSON", `{"standard":{"backend":"local-gvisor","organization_ids":["`+organization.String()+`"],"policy_ids":["offline"]}}`)
	for _, policy := range []string{
		`{"digital_human:console":["` + organization.String() + `"],"digital_human:webapp":["` + organization.String() + `"],"digital_human:external-api":["` + organization.String() + `"]}`,
		`{"malformed":true}`,
	} {
		t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", policy)
		provider := newDigitalHumanLinuxExecution(DigitalHumanRuntimeDeps{DB: db})
		for _, surface := range []string{digitalhuman.SurfaceConsole, digitalhuman.SurfaceWebApp, digitalhuman.SurfaceAPI} {
			run := &digitalhuman.Run{OrganizationID: organization, Surface: surface}
			resolved := &digitalhuman.ResolvedExecution{Snapshot: digitalhuman.ExecutionSnapshot{ExecutionRuntime: &digitalhuman.ExecutionRuntimeConfig{Enabled: true}}}
			factory, err := provider.Prepare(t.Context(), run, resolved, invocation.Event{})
			if factory != nil || (surface != digitalhuman.SurfaceWebApp && err == nil) {
				t.Fatalf("operator env bypassed verified identity: surface=%s err=%v", surface, err)
			}
		}
	}
}

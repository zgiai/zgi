package runtimeauth

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublishedRuntimeStoreFallsBackToLegacyAgentPolicy(t *testing.T) {
	resourceID := uuid.New()
	auth, err := NewStore(nil).GetResourceAuthorization(context.Background(), PublishedRuntimeResourceAgent, resourceID, PolicyFromAgentFields(WebAppStatusInactive, true))
	if err != nil {
		t.Fatalf("GetResourceAuthorization error = %v", err)
	}

	policy := PolicyFromAuthorization(PolicyFromAgentFields(WebAppStatusInactive, true), auth)
	if policy.Allows(PublishedRuntimeSurfaceWebApp) {
		t.Fatal("webapp should stay disabled from legacy fallback")
	}
	if !policy.Allows(PublishedRuntimeSurfaceAPI) {
		t.Fatal("api should stay enabled from legacy fallback")
	}
	if policy.Allows(PublishedRuntimeSurfaceBuiltinApp) {
		t.Fatal("builtin app should stay disabled while the legacy webapp is inactive")
	}
	if !policy.Allows(PublishedRuntimeSurfaceInternal) {
		t.Fatal("internal invocation should stay enabled from legacy fallback")
	}
}

func TestPublishedRuntimeStoreOverlaysPersistedSurfacesAndBuiltinAudience(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	resourceID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()
	departmentID := uuid.New()
	internalSurfaceID := uuid.New()
	builtinSurfaceID := uuid.New()
	apiSurfaceID := uuid.New()
	webappSurfaceID := uuid.New()

	expectPublishedRuntimeSurfaceLookup(mock, resourceID, organizationID, workspaceID, []runtimeSurfaceExpectation{
		{id: apiSurfaceID, surface: PublishedRuntimeSurfaceAPI, enabled: true},
		{id: builtinSurfaceID, surface: PublishedRuntimeSurfaceBuiltinApp, enabled: true},
		{id: internalSurfaceID, surface: PublishedRuntimeSurfaceInternal, enabled: true},
		{id: webappSurfaceID, surface: PublishedRuntimeSurfaceWebApp, enabled: false},
	})
	expectPublishedRuntimeGrantLookup(mock, []runtimeGrantExpectation{
		{surfaceID: builtinSurfaceID, subjectType: PublishedRuntimeSubjectAccount, subjectID: &accountID, enabled: true},
		{surfaceID: builtinSurfaceID, subjectType: PublishedRuntimeSubjectDepartment, subjectID: &departmentID, enabled: true},
		{surfaceID: internalSurfaceID, subjectType: PublishedRuntimeSubjectInternal, enabled: true},
	})

	auth, err := NewStore(db).GetResourceAuthorization(ctx, PublishedRuntimeResourceAgent, resourceID, PolicyFromAgentFields(WebAppStatusActive, false))
	if err != nil {
		t.Fatalf("GetResourceAuthorization error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
	if auth.OrganizationID != organizationID {
		t.Fatalf("organization id = %s, want %s", auth.OrganizationID, organizationID)
	}
	if auth.WorkspaceID == nil || *auth.WorkspaceID != workspaceID {
		t.Fatalf("workspace id = %v, want %s", auth.WorkspaceID, workspaceID)
	}
	if got, want := len(auth.Surfaces), 4; got != want {
		t.Fatalf("surface count = %d, want %d", got, want)
	}

	policy := PolicyFromAuthorization(PolicyFromAgentFields(WebAppStatusActive, false), auth)
	if policy.Allows(PublishedRuntimeSurfaceWebApp) {
		t.Fatal("persisted webapp false should override active legacy status")
	}
	if !policy.Allows(PublishedRuntimeSurfaceAPI) {
		t.Fatal("persisted api true should override disabled legacy api")
	}
	if !policy.Allows(PublishedRuntimeSurfaceBuiltinApp) {
		t.Fatal("persisted builtin true should enable builtin app")
	}
	if !policy.Allows(PublishedRuntimeSurfaceInternal) {
		t.Fatal("persisted internal true should keep internal invocation enabled")
	}
	if got, want := policy.AllowedBuiltinAccountIDs, []string{accountID.String()}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("allowed builtin accounts = %v, want %v", got, want)
	}
	if got, want := policy.AllowedBuiltinDeptIDs, []string{departmentID.String()}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("allowed builtin departments = %v, want %v", got, want)
	}
}

func TestPublishedRuntimeStoreTreatsLegacyBuiltinSeedAsActiveWebAppCompatibility(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	resourceID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	builtinSurfaceID := uuid.New()

	expectPublishedRuntimeSurfaceLookup(mock, resourceID, organizationID, workspaceID, []runtimeSurfaceExpectation{{
		id:      builtinSurfaceID,
		surface: PublishedRuntimeSurfaceBuiltinApp,
		enabled: false,
		source:  PublishedRuntimeSourceLegacyAgentFields,
	}})
	expectPublishedRuntimeGrantLookup(mock, nil)

	auth, err := NewStore(db).GetResourceAuthorization(ctx, PublishedRuntimeResourceAgent, resourceID, PolicyFromAgentFields(WebAppStatusActive, false))
	if err != nil {
		t.Fatalf("GetResourceAuthorization error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
	surface, ok := auth.Surface(PublishedRuntimeSurfaceBuiltinApp)
	if !ok {
		t.Fatalf("builtin app surface missing")
	}
	if !surface.Enabled {
		t.Fatalf("legacy builtin seed should be enabled for active webapp compatibility")
	}
}

func TestPublishedRuntimeStoreSaveResourceAuthorizationCreatesSurfaceAndGrants(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	resourceID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	accountID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND surface = $3 AND deleted_at IS NULL LIMIT $4`)).
		WithArgs(string(PublishedRuntimeResourceAgent), resourceID, string(PublishedRuntimeSurfaceBuiltinApp), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "published_runtime_surfaces"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "published_runtime_surface_grants" SET "deleted_at"=`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "published_runtime_surface_grants"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := NewStore(db).SaveResourceAuthorization(ctx, ResourceAuthorization{
		ResourceType:   PublishedRuntimeResourceAgent,
		ResourceID:     resourceID,
		OrganizationID: organizationID,
		WorkspaceID:    &workspaceID,
		Surfaces: []SurfaceAuthorization{{
			Surface:             PublishedRuntimeSurfaceBuiltinApp,
			Enabled:             true,
			CompatibilitySource: PublishedRuntimeSourceGrant,
			Grants: []SurfaceGrant{{
				SubjectType: PublishedRuntimeSubjectAccount,
				SubjectID:   &accountID,
				Enabled:     true,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("SaveResourceAuthorization error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPublishedRuntimeStoreRejectsUnsupportedSurfaceGrantSubjectsBeforeSQL(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	resourceID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name    string
		surface PublishedRuntimeSurface
		subject PublishedRuntimeSubjectType
		want    string
	}{
		{
			name:    "webapp rejects account grant",
			surface: PublishedRuntimeSurfaceWebApp,
			subject: PublishedRuntimeSubjectAccount,
			want:    "webapp runtime grants must use public subject",
		},
		{
			name:    "api rejects organization grant",
			surface: PublishedRuntimeSurfaceAPI,
			subject: PublishedRuntimeSubjectOrganization,
			want:    "api runtime grants must use public subject",
		},
		{
			name:    "internal rejects public grant",
			surface: PublishedRuntimeSurfaceInternal,
			subject: PublishedRuntimeSubjectPublic,
			want:    "internal runtime grants must use internal subject",
		},
		{
			name:    "builtin rejects public grant",
			surface: PublishedRuntimeSurfaceBuiltinApp,
			subject: PublishedRuntimeSubjectPublic,
			want:    "builtin app grants must target organization, account, or department",
		},
	}

	store := NewStore(db)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SaveResourceAuthorization(ctx, ResourceAuthorization{
				ResourceType:   PublishedRuntimeResourceAgent,
				ResourceID:     resourceID,
				OrganizationID: organizationID,
				Surfaces: []SurfaceAuthorization{{
					Surface:             tt.surface,
					Enabled:             true,
					CompatibilitySource: PublishedRuntimeSourceGrant,
					Grants: []SurfaceGrant{{
						SubjectType: tt.subject,
						SubjectID:   &accountID,
						Enabled:     true,
					}},
				}},
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("SaveResourceAuthorization error = %v, want containing %q", err, tt.want)
			}
		})
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected sql calls: %v", err)
	}
}

func TestPublishedRuntimeStoreListAuthorizedResourceIDsEvaluatesAudienceGrants(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000100")
	accountID := uuid.MustParse("00000000-0000-0000-0000-000000000200")
	departmentID := uuid.MustParse("00000000-0000-0000-0000-000000000300")
	otherAccountID := uuid.MustParse("00000000-0000-0000-0000-000000000201")

	openResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	organizationResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	accountResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	departmentResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000004")
	otherAccountResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000005")
	disabledGrantResourceID := uuid.MustParse("00000000-0000-0000-0000-000000000006")

	openSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001001")
	organizationSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001002")
	accountSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001003")
	departmentSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001004")
	otherAccountSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001005")
	disabledGrantSurfaceID := uuid.MustParse("00000000-0000-0000-0000-000000001006")

	now := time.Now()
	surfaceRows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).
		AddRow(openSurfaceID.String(), string(PublishedRuntimeResourceAgent), openResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(organizationSurfaceID.String(), string(PublishedRuntimeResourceAgent), organizationResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(accountSurfaceID.String(), string(PublishedRuntimeResourceAgent), accountResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(departmentSurfaceID.String(), string(PublishedRuntimeResourceAgent), departmentResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(otherAccountSurfaceID.String(), string(PublishedRuntimeResourceAgent), otherAccountResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(disabledGrantSurfaceID.String(), string(PublishedRuntimeResourceAgent), disabledGrantResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND surface = $2 AND organization_id = $3 AND enabled = $4 AND deleted_at IS NULL ORDER BY resource_id ASC`)).
		WithArgs(string(PublishedRuntimeResourceAgent), string(PublishedRuntimeSurfaceBuiltinApp), organizationID, true).
		WillReturnRows(surfaceRows)

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	}).
		AddRow(uuid.New().String(), organizationSurfaceID.String(), string(PublishedRuntimeSubjectOrganization), organizationID.String(), true, now, now, nil).
		AddRow(uuid.New().String(), accountSurfaceID.String(), string(PublishedRuntimeSubjectAccount), accountID.String(), true, now, now, nil).
		AddRow(uuid.New().String(), departmentSurfaceID.String(), string(PublishedRuntimeSubjectDepartment), departmentID.String(), true, now, now, nil).
		AddRow(uuid.New().String(), otherAccountSurfaceID.String(), string(PublishedRuntimeSubjectAccount), otherAccountID.String(), true, now, now, nil).
		AddRow(uuid.New().String(), disabledGrantSurfaceID.String(), string(PublishedRuntimeSubjectOrganization), organizationID.String(), false, now, now, nil)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WithArgs(openSurfaceID, organizationSurfaceID, accountSurfaceID, departmentSurfaceID, otherAccountSurfaceID, disabledGrantSurfaceID).
		WillReturnRows(grantRows)

	got, err := NewStore(db).ListAuthorizedResourceIDs(ctx, PublishedRuntimeResourceAgent, PublishedRuntimeSurfaceBuiltinApp, organizationID, RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
		DepartmentIDs:  []uuid.UUID{departmentID},
	})
	if err != nil {
		t.Fatalf("ListAuthorizedResourceIDs error = %v", err)
	}
	want := []uuid.UUID{openResourceID, organizationResourceID, accountResourceID, departmentResourceID}
	assertUUIDElements(t, got, want)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPublishedRuntimeStoreFilterAuthorizedResourceIDsAppliesFallbackAndPersistedOverlay(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openPublishedRuntimeAuthMockDB(t)
	defer cleanup()

	organizationID := uuid.MustParse("10000000-0000-0000-0000-000000000000")
	accountID := uuid.MustParse("20000000-0000-0000-0000-000000000000")
	otherAccountID := uuid.MustParse("20000000-0000-0000-0000-000000000001")

	legacyOpenResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000001")
	persistedDisabledResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000002")
	accountResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000003")
	otherAccountResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000004")
	legacySeedResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000005")
	inactiveFallbackResourceID := uuid.MustParse("30000000-0000-0000-0000-000000000006")

	disabledSurfaceID := uuid.MustParse("40000000-0000-0000-0000-000000000002")
	accountSurfaceID := uuid.MustParse("40000000-0000-0000-0000-000000000003")
	otherAccountSurfaceID := uuid.MustParse("40000000-0000-0000-0000-000000000004")
	legacySeedSurfaceID := uuid.MustParse("40000000-0000-0000-0000-000000000005")

	activeFallback := PolicyFromAgentFields(WebAppStatusActive, false)
	inactiveFallback := PolicyFromAgentFields(WebAppStatusInactive, false)
	candidates := []ResourceAuthorizationCandidate{
		{ResourceID: legacyOpenResourceID, Fallback: activeFallback},
		{ResourceID: persistedDisabledResourceID, Fallback: activeFallback},
		{ResourceID: accountResourceID, Fallback: activeFallback},
		{ResourceID: otherAccountResourceID, Fallback: activeFallback},
		{ResourceID: legacySeedResourceID, Fallback: activeFallback},
		{ResourceID: inactiveFallbackResourceID, Fallback: inactiveFallback},
	}

	now := time.Now()
	surfaceRows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	}).
		AddRow(disabledSurfaceID.String(), string(PublishedRuntimeResourceAgent), persistedDisabledResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), false, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(accountSurfaceID.String(), string(PublishedRuntimeResourceAgent), accountResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(otherAccountSurfaceID.String(), string(PublishedRuntimeResourceAgent), otherAccountResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), true, PublishedRuntimeSourceGrant, now, now, nil).
		AddRow(legacySeedSurfaceID.String(), string(PublishedRuntimeResourceAgent), legacySeedResourceID.String(), organizationID.String(), nil, string(PublishedRuntimeSurfaceBuiltinApp), false, PublishedRuntimeSourceLegacyAgentFields, now, now, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND surface = $2 AND organization_id = $3 AND resource_id IN ($4,$5,$6,$7,$8,$9) AND deleted_at IS NULL ORDER BY resource_id ASC`)).
		WithArgs(
			string(PublishedRuntimeResourceAgent),
			string(PublishedRuntimeSurfaceBuiltinApp),
			organizationID,
			legacyOpenResourceID,
			persistedDisabledResourceID,
			accountResourceID,
			otherAccountResourceID,
			legacySeedResourceID,
			inactiveFallbackResourceID,
		).
		WillReturnRows(surfaceRows)

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	}).
		AddRow(uuid.New().String(), accountSurfaceID.String(), string(PublishedRuntimeSubjectAccount), accountID.String(), true, now, now, nil).
		AddRow(uuid.New().String(), otherAccountSurfaceID.String(), string(PublishedRuntimeSubjectAccount), otherAccountID.String(), true, now, now, nil)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surface_grants" WHERE surface_id IN ($1,$2,$3,$4) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`)).
		WithArgs(disabledSurfaceID, accountSurfaceID, otherAccountSurfaceID, legacySeedSurfaceID).
		WillReturnRows(grantRows)

	got, err := NewStore(db).FilterAuthorizedResourceIDs(ctx, PublishedRuntimeResourceAgent, PublishedRuntimeSurfaceBuiltinApp, organizationID, candidates, RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
	})
	if err != nil {
		t.Fatalf("FilterAuthorizedResourceIDs error = %v", err)
	}
	assertUUIDSequence(t, got, []uuid.UUID{legacyOpenResourceID, accountResourceID, legacySeedResourceID})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

type runtimeSurfaceExpectation struct {
	id      uuid.UUID
	surface PublishedRuntimeSurface
	enabled bool
	source  string
}

type runtimeGrantExpectation struct {
	surfaceID   uuid.UUID
	subjectType PublishedRuntimeSubjectType
	subjectID   *uuid.UUID
	enabled     bool
}

func openPublishedRuntimeAuthMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm: %v", err)
	}

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func expectPublishedRuntimeSurfaceLookup(mock sqlmock.Sqlmock, resourceID, organizationID, workspaceID uuid.UUID, surfaces []runtimeSurfaceExpectation) {
	rows := sqlmock.NewRows([]string{
		"id",
		"resource_type",
		"resource_id",
		"organization_id",
		"workspace_id",
		"surface",
		"enabled",
		"compatibility_source",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	now := time.Now()
	for _, surface := range surfaces {
		rows.AddRow(
			surface.id.String(),
			string(PublishedRuntimeResourceAgent),
			resourceID.String(),
			organizationID.String(),
			workspaceID.String(),
			string(surface.surface),
			surface.enabled,
			runtimeSurfaceSource(surface.source),
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(PublishedRuntimeResourceAgent), resourceID).
		WillReturnRows(rows)
}

func runtimeSurfaceSource(source string) string {
	if source == "" {
		return PublishedRuntimeSourceGrant
	}
	return source
}

func expectPublishedRuntimeGrantLookup(mock sqlmock.Sqlmock, grants []runtimeGrantExpectation) {
	rows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	now := time.Now()
	for _, grant := range grants {
		var subjectID interface{}
		if grant.subjectID != nil {
			subjectID = grant.subjectID.String()
		}
		rows.AddRow(
			uuid.New().String(),
			grant.surfaceID.String(),
			string(grant.subjectType),
			subjectID,
			grant.enabled,
			now,
			now,
			nil,
		)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(rows)
}

func assertUUIDElements(t *testing.T, got, want []uuid.UUID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("uuid count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	remaining := make(map[uuid.UUID]int, len(want))
	for _, id := range want {
		remaining[id]++
	}
	for _, id := range got {
		if remaining[id] == 0 {
			t.Fatalf("unexpected uuid %s in %v, want %v", id, got, want)
		}
		remaining[id]--
	}
}

func assertUUIDSequence(t *testing.T, got, want []uuid.UUID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("uuid count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uuid at %d = %s, want %s; full result %v", i, got[i], want[i], got)
		}
	}
}

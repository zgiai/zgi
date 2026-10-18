package agents

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestNormalizeAgentCatalogTagName(t *testing.T) {
	name, normalized, err := normalizeAgentCatalogTagName("  Customer\t Success  ")
	require.NoError(t, err)
	require.Equal(t, "Customer Success", name)
	require.Equal(t, "customer success", normalized)
}

func TestNormalizeAgentCatalogTagNameRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"   ", "bad\x00name", strings.Repeat("界", maxAgentCatalogTagRunes+1)} {
		_, _, err := normalizeAgentCatalogTagName(value)
		require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	}
}

func TestParseRunnableWebAppTagIDsDeduplicates(t *testing.T) {
	first := uuid.NewString()
	second := uuid.NewString()
	ids, err := parseRunnableWebAppTagIDs(first + ", " + second + "," + first)
	require.NoError(t, err)
	require.Equal(t, []string{first, second}, ids)
}

func TestParseRunnableWebAppTagIDsRejectsInvalidValues(t *testing.T) {
	ids, err := parseRunnableWebAppTagIDs(uuid.NewString() + ",invalid")
	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Nil(t, ids)

	tooMany := make([]string, maxRunnableWebAppTagIDs+1)
	for index := range tooMany {
		tooMany[index] = uuid.NewString()
	}
	ids, err = parseRunnableWebAppTagIDs(strings.Join(tooMany, ","))
	require.ErrorIs(t, err, runtimeservice.ErrInvalidInput)
	require.Nil(t, ids)
}

func TestResolveAgentCatalogTagReturnsExistingRowAfterNameConflict(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	organizationID := uuid.New()
	actorID := uuid.New()
	existingID := uuid.New()
	now := time.Now()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "agent_catalog_tags" ("organization_id","name","normalized_name","created_by","id") VALUES ($1,$2,$3,$4,$5) ON CONFLICT ("organization_id","normalized_name") DO NOTHING RETURNING "id","created_at","updated_at"`)).
		WithArgs(organizationID, "Finance", "finance", actorID, sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT \* FROM "agent_catalog_tags" WHERE organization_id = \$1 AND normalized_name = \$2 LIMIT \$3`).
		WithArgs(organizationID, "finance", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "name", "normalized_name", "created_by", "created_at", "updated_at",
		}).AddRow(existingID, organizationID, "Finance", "finance", actorID, now, now))

	resolved, err := resolveAgentCatalogTag(t.Context(), db, organizationID, actorID, dto.AgentCatalogTagInput{Name: "Finance"})
	require.NoError(t, err)
	require.Equal(t, existingID, resolved.ID)
	require.Equal(t, "Finance", resolved.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentCatalogTagsByAgentIDsGroupsRows(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	firstAgentID := uuid.New()
	secondAgentID := uuid.New()
	financeTagID := uuid.New()
	operationsTagID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bindings.agent_id, tags.id AS tag_id, tags.name FROM agent_catalog_tag_bindings AS bindings JOIN agent_catalog_tags AS tags ON tags.id = bindings.tag_id WHERE bindings.agent_id IN ($1,$2) ORDER BY tags.name ASC, tags.id ASC`)).
		WithArgs(firstAgentID, secondAgentID).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "tag_id", "name"}).
			AddRow(firstAgentID, financeTagID, "Finance").
			AddRow(firstAgentID, operationsTagID, "Operations").
			AddRow(secondAgentID, financeTagID, "Finance"))

	service := &agentsService{db: db}
	tagsByAgent, err := service.agentCatalogTagsByAgentIDs(t.Context(), []uuid.UUID{
		firstAgentID,
		secondAgentID,
	})
	require.NoError(t, err)
	require.Equal(t, []dto.AgentCatalogTag{
		{ID: financeTagID.String(), Name: "Finance"},
		{ID: operationsTagID.String(), Name: "Operations"},
	}, tagsByAgent[firstAgentID])
	require.Equal(t, []dto.AgentCatalogTag{
		{ID: financeTagID.String(), Name: "Finance"},
	}, tagsByAgent[secondAgentID])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAgentCatalogTagFacetsScopesByWorkspaceAndAssetKind(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)

	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	tagID := uuid.New()
	mock.ExpectQuery(`SELECT tags\.id AS tag_id, tags\.name, COUNT\(DISTINCT agents\.id\) AS count FROM agent_catalog_tags AS tags .*agents\.agent_type IN.*`).
		WithArgs(organizationID, workspaceID, "AGENT").
		WillReturnRows(sqlmock.NewRows([]string{"tag_id", "name", "count"}).AddRow(tagID, "Finance", 2))

	service := &agentsService{
		db: db,
		tenantService: &stubWorkspaceManagementServiceForAgentsList{
			currentOrganization: &workspace_model.OrganizationMember{
				OrganizationID: organizationID,
				AccountID:      "account-1",
				Role:           workspace_model.OrganizationRoleNormal,
			},
			workspaceIDsByOrganization: []string{workspaceID},
		},
		enterpriseService: &stubOrganizationServiceForAgentsList{
			workspaces:               []*workspace_model.Workspace{{ID: workspaceID, Status: workspace_model.WorkspaceStatusNormal}},
			permissionsByWorkspaceID: map[string]bool{workspaceID: true},
		},
	}

	resp, err := service.GetAgentCatalogTagFacets(t.Context(), "account-1", dto.GetAgentCatalogTagFacetsRequest{
		WorkspaceID: workspaceID,
		AssetKind:   "agent",
	})
	require.NoError(t, err)
	require.Equal(t, []dto.AgentCatalogTagFacet{{
		AgentCatalogTag: dto.AgentCatalogTag{ID: tagID.String(), Name: "Finance"},
		Count:           2,
	}}, resp.Items)
	require.NoError(t, mock.ExpectationsWereMet())
}

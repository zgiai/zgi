package runtimeworkspace

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCatalogTagsByAgentIDsGroupsRowsAndInitializesEmptyItems(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	require.NoError(t, err)

	agentWithTagsID, agentWithoutTagsID := uuid.New(), uuid.New()
	firstTagID, secondTagID := uuid.New(), uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT bindings.agent_id, tags.id AS tag_id, tags.name FROM agent_catalog_tag_bindings AS bindings JOIN agent_catalog_tags AS tags ON tags.id = bindings.tag_id WHERE bindings.agent_id IN ($1,$2) ORDER BY tags.name ASC, tags.id ASC`)).
		WithArgs(agentWithTagsID, agentWithoutTagsID).
		WillReturnRows(sqlmock.NewRows([]string{"agent_id", "tag_id", "name"}).
			AddRow(agentWithTagsID, firstTagID, "Customer Service").
			AddRow(agentWithTagsID, secondTagID, "Sales"))

	tagsByAgent, err := NewService(db).catalogTagsByAgentIDs(t.Context(), []uuid.UUID{agentWithTagsID, agentWithoutTagsID})
	require.NoError(t, err)
	require.Equal(t, []CatalogTag{
		{ID: firstTagID.String(), Name: "Customer Service"},
		{ID: secondTagID.String(), Name: "Sales"},
	}, tagsByAgent[agentWithTagsID])
	require.Empty(t, tagsByAgent[agentWithoutTagsID])
	require.NotNil(t, tagsByAgent[agentWithoutTagsID])
	require.NoError(t, mock.ExpectationsWereMet())
}

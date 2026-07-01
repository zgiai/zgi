package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestDatasetViewPermissionCodesExcludeCreateOnlyPermissions(t *testing.T) {
	codes := datasetViewPermissionCodes()

	require.Contains(t, codes, model.WorkspacePermissionKnowledgeBaseView)
	require.Contains(t, codes, model.WorkspacePermissionKnowledgeBaseDocumentView)
	require.NotContains(t, codes, model.WorkspacePermissionKnowledgeBaseCreate)
	require.NotContains(t, codes, model.WorkspacePermissionKnowledgeBaseDocumentCreate)
}

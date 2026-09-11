package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type ragEvaluationPermissionChecker struct {
	interfaces.OrganizationService
	granted                                []workspace_model.WorkspacePermissionCode
	codes                                  []workspace_model.WorkspacePermissionCode
	organizationID, workspaceID, accountID string
	err                                    error
}

func (p *ragEvaluationPermissionChecker) CheckWorkspaceOrganizationAnyPermission(_ context.Context, organizationID, workspaceID, accountID string, codes ...workspace_model.WorkspacePermissionCode) (bool, error) {
	p.organizationID, p.workspaceID, p.accountID = organizationID, workspaceID, accountID
	p.codes = codes
	if p.err != nil {
		return false, p.err
	}
	for _, code := range codes {
		if workspace_model.WorkspacePermissionCodesAllow(p.granted, code) {
			return true, nil
		}
	}
	return false, nil
}

func TestRAGEvaluationRequiresExactRetrievalPermission(t *testing.T) {
	for _, tc := range []struct {
		permission workspace_model.WorkspacePermissionCode
		allowed    bool
	}{
		{workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest, true},
		{workspace_model.WorkspacePermissionKnowledgeBaseView, false},
		{workspace_model.WorkspacePermissionKnowledgeBaseUpdate, false},
		{workspace_model.WorkspacePermissionKnowledgeBaseManage, false},
	} {
		t.Run(string(tc.permission), func(t *testing.T) {
			checker := &ragEvaluationPermissionChecker{granted: []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseView, tc.permission}}
			handler := &RAGEvaluationHandler{organization: checker}
			allowed, err := handler.checkRetrievalPermission(context.Background(), datasetservice.KnowledgeScope{OrganizationID: " org-1 ", WorkspaceID: "scope-workspace", AccountID: " account-1 "}, " asset-workspace ")
			require.NoError(t, err)
			require.Equal(t, tc.allowed, allowed)
			require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest}, checker.codes)
			require.Equal(t, "org-1", checker.organizationID)
			require.Equal(t, "asset-workspace", checker.workspaceID)
			require.Equal(t, "account-1", checker.accountID)
		})
	}
}

func TestRAGEvaluationPermissionPreservesScopeFallbackAndErrors(t *testing.T) {
	wantErr := errors.New("permission store unavailable")
	checker := &ragEvaluationPermissionChecker{err: wantErr}
	allowed, err := (&RAGEvaluationHandler{organization: checker}).checkRetrievalPermission(context.Background(), datasetservice.KnowledgeScope{OrganizationID: "org-1", WorkspaceID: " fallback-workspace ", AccountID: "account-1"}, " ")
	require.False(t, allowed)
	require.ErrorIs(t, err, wantErr)
	require.Equal(t, "fallback-workspace", checker.workspaceID)
}

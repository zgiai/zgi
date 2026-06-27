package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/dto"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetrepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_service "github.com/zgiai/zgi/api/internal/modules/shared/service"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

func TestAuthorizeDatasetViewAccessRejectsCrossOrganizationDataset(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-2",
				WorkspaceID:    "workspace-1",
			},
		},
	}

	_, ok := authorizeDatasetViewAccess(c, datasets, &datasetAccessAuthorizationService{}, "dataset-1")

	if ok {
		t.Fatalf("authorizeDatasetViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeDatasetViewAccessRejectsDatasetWithoutWorkspace(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
			},
		},
	}

	_, ok := authorizeDatasetViewAccess(c, datasets, &datasetAccessAuthorizationService{}, "dataset-1")

	if ok {
		t.Fatalf("authorizeDatasetViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeDatasetDocumentViewAccessRejectsDocumentOutsideDataset(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetAccessDocumentService{
		documents: map[string]*dataset_model.Document{
			"document-1": {
				ID:             "document-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-2",
			},
		},
	}

	_, _, ok := authorizeDatasetDocumentViewAccess(c, datasets, documents, &datasetAccessAuthorizationService{allow: true}, "dataset-1", "document-1")

	if ok {
		t.Fatalf("authorizeDatasetDocumentViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestAuthorizeDatasetUpdateAccessUsesDatasetUpdatePermission(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	auth := &datasetAccessAuthorizationService{allow: true}

	dataset, ok := authorizeDatasetUpdateAccess(c, datasets, auth, "dataset-1")

	if !ok {
		t.Fatalf("authorizeDatasetUpdateAccess ok = false, want true")
	}
	if dataset == nil || dataset.ID != "dataset-1" {
		t.Fatalf("dataset = %#v, want dataset-1", dataset)
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if auth.lastRequest.OrganizationID != "org-1" || auth.lastRequest.WorkspaceID != "workspace-1" || auth.lastRequest.AccountID != "account-1" {
		t.Fatalf("auth request = %#v, want org/workspace/account scope", auth.lastRequest)
	}
}

func TestAuthorizeDatasetViewAccessUsesFineKnowledgeBaseViewPermissions(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	auth := &datasetAccessAuthorizationService{allow: true}

	dataset, ok := authorizeDatasetViewAccess(c, datasets, auth, "dataset-1")

	if !ok {
		t.Fatalf("authorizeDatasetViewAccess ok = false, want true")
	}
	if dataset == nil || dataset.ID != "dataset-1" {
		t.Fatalf("dataset = %#v, want dataset-1", dataset)
	}
	want := knowledgeBaseViewPermissionCodes()
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	assertNoCoarseKnowledgeBasePermissions(t, auth.lastRequest.PermissionCodes)
}

func TestAuthorizeDatasetFolderViewAccessRejectsCrossOrganizationFolder(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	folders := &datasetAccessFolderService{
		folders: map[string]*dataset_model.DatasetFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-2",
				WorkspaceID:    "workspace-1",
				CreatedBy:      "account-1",
				Permission:     string(dataset_model.DatasetPermissionAllTeam),
			},
		},
	}

	_, ok := authorizeDatasetFolderViewAccess(c, folders, &datasetAccessAuthorizationService{allow: true}, "folder-1")

	if ok {
		t.Fatalf("authorizeDatasetFolderViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeDatasetFolderViewAccessIgnoresLegacyOnlyMeForNonCreator(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	folders := &datasetAccessFolderService{
		folders: map[string]*dataset_model.DatasetFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
				CreatedBy:      "account-2",
				Permission:     string(dataset_model.DatasetPermissionOnlyMe),
			},
		},
	}

	folder, ok := authorizeDatasetFolderViewAccess(c, folders, &datasetAccessAuthorizationService{allow: true}, "folder-1")

	if !ok {
		t.Fatalf("authorizeDatasetFolderViewAccess ok = false, want true")
	}
	if folder == nil || folder.ID != "folder-1" {
		t.Fatalf("folder = %#v, want folder-1", folder)
	}
}

func TestAuthorizeDatasetFolderViewAccessUsesFineKnowledgeBaseFolderViewPermissions(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	folders := &datasetAccessFolderService{
		folders: map[string]*dataset_model.DatasetFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
				CreatedBy:      "account-2",
				Permission:     string(dataset_model.DatasetPermissionAllTeam),
			},
		},
	}
	auth := &datasetAccessAuthorizationService{allow: true}

	folder, ok := authorizeDatasetFolderViewAccess(c, folders, auth, "folder-1")

	if !ok {
		t.Fatalf("authorizeDatasetFolderViewAccess ok = false, want true")
	}
	if folder == nil || folder.ID != "folder-1" {
		t.Fatalf("folder = %#v, want folder-1", folder)
	}
	want := knowledgeBaseFolderViewPermissionCodes()
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if containsWorkspacePermission(auth.lastRequest.PermissionCodes, workspace_model.WorkspacePermissionKnowledgeBaseView) ||
		containsWorkspacePermission(auth.lastRequest.PermissionCodes, workspace_model.WorkspacePermissionKnowledgeBaseManage) {
		t.Fatalf("folder view permissions should not include coarse knowledge base view/manage: %#v", auth.lastRequest.PermissionCodes)
	}
}

func TestAuthorizeDatasetDocumentAndSegmentViewUseFinePermissions(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetAccessDocumentService{
		documents: map[string]*dataset_model.Document{
			"document-1": {
				ID:             "document-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
			},
		},
	}
	segments := &datasetAccessSegmentService{
		segments: map[string]*dataset_model.DocumentSegment{
			"segment-1": {
				ID:             "segment-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				DocumentID:     "document-1",
			},
		},
	}

	documentAuth := &datasetAccessAuthorizationService{allow: true}
	_, _, ok := authorizeDatasetDocumentViewAccess(c, datasets, documents, documentAuth, "dataset-1", "document-1")
	if !ok {
		t.Fatalf("authorizeDatasetDocumentViewAccess ok = false, want true")
	}
	if !reflect.DeepEqual(documentAuth.lastRequest.PermissionCodes, knowledgeBaseDocumentViewPermissionCodes()) {
		t.Fatalf("document permissions = %#v, want %#v", documentAuth.lastRequest.PermissionCodes, knowledgeBaseDocumentViewPermissionCodes())
	}
	assertNoCoarseKnowledgeBasePermissions(t, documentAuth.lastRequest.PermissionCodes)

	segmentAuth := &datasetAccessAuthorizationService{allow: true}
	_, _, _, ok = authorizeDatasetSegmentViewAccess(c, datasets, documents, segments, segmentAuth, "dataset-1", "document-1", "segment-1")
	if !ok {
		t.Fatalf("authorizeDatasetSegmentViewAccess ok = false, want true")
	}
	if !reflect.DeepEqual(segmentAuth.lastRequest.PermissionCodes, knowledgeBaseSegmentViewPermissionCodes()) {
		t.Fatalf("segment permissions = %#v, want %#v", segmentAuth.lastRequest.PermissionCodes, knowledgeBaseSegmentViewPermissionCodes())
	}
	assertNoCoarseKnowledgeBasePermissions(t, segmentAuth.lastRequest.PermissionCodes)
}

func TestAuthorizeDatasetFolderManageAccessUsesFolderManagePermission(t *testing.T) {
	c, _ := newDatasetAccessTestContext("account-1", "org-1")
	folders := &datasetAccessFolderService{
		folders: map[string]*dataset_model.DatasetFolder{
			"folder-1": {
				ID:             "folder-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
				CreatedBy:      "account-2",
				Permission:     string(dataset_model.DatasetPermissionAllTeam),
			},
		},
	}
	auth := &datasetAccessAuthorizationService{allow: true}

	folder, ok := authorizeDatasetFolderManageAccess(c, folders, auth, "folder-1")

	if !ok {
		t.Fatalf("authorizeDatasetFolderManageAccess ok = false, want true")
	}
	if folder == nil || folder.ID != "folder-1" {
		t.Fatalf("folder = %#v, want folder-1", folder)
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseFolderManage}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
}

func TestAuthorizeDatasetWorkspaceFolderManageAccessRejectsEmptyWorkspace(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")

	ok := authorizeDatasetWorkspaceFolderManageAccess(c, &datasetAccessAuthorizationService{allow: true}, "")

	if ok {
		t.Fatalf("authorizeDatasetWorkspaceFolderManageAccess ok = true, want false")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeDatasetSegmentViewAccessRejectsSegmentOutsideDocument(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetAccessDocumentService{
		documents: map[string]*dataset_model.Document{
			"document-1": {
				ID:             "document-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
			},
		},
	}
	segments := &datasetAccessSegmentService{
		segments: map[string]*dataset_model.DocumentSegment{
			"segment-1": {
				ID:             "segment-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				DocumentID:     "document-2",
			},
		},
	}

	_, _, _, ok := authorizeDatasetSegmentViewAccess(c, datasets, documents, segments, &datasetAccessAuthorizationService{allow: true}, "dataset-1", "document-1", "segment-1")

	if ok {
		t.Fatalf("authorizeDatasetSegmentViewAccess ok = true, want false")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestAuthorizeDatasetChildChunkAccessRejectsChildOutsideSegment(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	datasets := &datasetAccessDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			"dataset-1": {
				ID:             "dataset-1",
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetAccessDocumentService{
		documents: map[string]*dataset_model.Document{
			"document-1": {
				ID:             "document-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
			},
		},
	}
	segments := &datasetAccessSegmentService{
		segments: map[string]*dataset_model.DocumentSegment{
			"segment-1": {
				ID:             "segment-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				DocumentID:     "document-1",
			},
		},
		childChunks: map[string]*dataset_model.ChildChunk{
			"child-1": {
				ID:             "child-1",
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				DocumentID:     "document-1",
				SegmentID:      "segment-2",
			},
		},
	}

	_, ok := authorizeDatasetChildChunkAccess(
		c,
		datasets,
		documents,
		segments,
		&datasetAccessAuthorizationService{allow: true},
		"dataset-1",
		"document-1",
		"segment-1",
		"child-1",
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentView,
	)

	if ok {
		t.Fatalf("authorizeDatasetChildChunkAccess ok = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestUpdateDocumentRequiresDocumentUpdateBeforeBindingRequest(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	auth := &datasetAccessAuthorizationService{}
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	handler := &DocumentHandler{
		datasetService:  datasets,
		documentService: documents,
		authService:     auth,
	}
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPatch, "/datasets/"+datasetID+"/documents/"+documentID, bytes.NewBufferString("{"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "dataset_id", Value: datasetID},
		{Key: "document_id", Value: documentID},
	}

	handler.UpdateDocument(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if documents.updateDocumentCalls != 0 {
		t.Fatalf("UpdateDocument calls = %d, want 0", documents.updateDocumentCalls)
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
}

func TestRetryDocumentRequiresIndexManageBeforeBindingRequest(t *testing.T) {
	datasetID := "dataset-1"
	auth := &datasetAccessAuthorizationService{}
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{}
	handler := &DocumentHandler{
		datasetService:  datasets,
		documentService: documents,
		authService:     auth,
	}
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPost, "/datasets/"+datasetID+"/retry", bytes.NewBufferString("{"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}

	handler.RetryDocument(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if documents.retryDocumentsCalls != 0 {
		t.Fatalf("RetryDocuments calls = %d, want 0", documents.retryDocumentsCalls)
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseIndexManage}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
}

func TestUpdateDocumentStatusRequiresDocumentUpdateBeforeDocumentIDValidation(t *testing.T) {
	datasetID := "11111111-1111-1111-1111-111111111111"
	auth := &datasetAccessAuthorizationService{}
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{}
	handler := &DocumentHandler{
		datasetService:  datasets,
		documentService: documents,
		authService:     auth,
	}
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	c.Request = httptest.NewRequest(http.MethodPatch, "/datasets/"+datasetID+"/documents/status/enable/batch?document_id=not-a-uuid", nil)
	c.Params = gin.Params{
		{Key: "dataset_id", Value: datasetID},
		{Key: "action", Value: "enable"},
	}

	handler.UpdateDocumentStatus(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if documents.updateDocumentStatusCalls != 0 {
		t.Fatalf("UpdateDocumentStatus calls = %d, want 0", documents.updateDocumentStatusCalls)
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
}

func TestCreateDocumentSegmentQuestionRequiresSegmentUpdateBeforeBindingRequest(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
	}
	auth := &datasetAccessAuthorizationService{}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     auth,
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodPost,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions",
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"{",
	)

	handler.CreateDocumentSegmentQuestion(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if segments.createQuestionCalls != 0 {
		t.Fatalf("CreateDocumentSegmentQuestion calls = %d, want 0", segments.createQuestionCalls)
	}
}

func TestBatchCreateDocumentSegmentQuestionsRequiresSegmentUpdateBeforeBindingRequest(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
	}
	auth := &datasetAccessAuthorizationService{}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     auth,
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodPost,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/batch",
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"{",
	)

	handler.BatchCreateDocumentSegmentQuestions(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if segments.batchCreateQuestionCalls != 0 {
		t.Fatalf("BatchCreateDocumentSegmentQuestions calls = %d, want 0", segments.batchCreateQuestionCalls)
	}
}

func TestGenerateQuestionsForSegmentRequiresSegmentUpdateBeforeCountValidation(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
	}
	auth := &datasetAccessAuthorizationService{}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     auth,
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodPost,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/generate?count=not-a-number",
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"",
	)

	handler.GenerateQuestionsForSegment(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if segments.generateQuestionCalls != 0 {
		t.Fatalf("GenerateQuestionsForSegment calls = %d, want 0", segments.generateQuestionCalls)
	}
}

func TestUpdateDocumentSegmentQuestionRequiresSegmentUpdateBeforeBindingRequest(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	questionID := "question-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
	}
	auth := &datasetAccessAuthorizationService{}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     auth,
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodPut,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/"+questionID,
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"{",
	)
	c.Params = append(c.Params, gin.Param{Key: "question_id", Value: questionID})

	handler.UpdateDocumentSegmentQuestion(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, want) {
		t.Fatalf("permissions = %#v, want %#v", auth.lastRequest.PermissionCodes, want)
	}
	if segments.getQuestionCalls != 0 {
		t.Fatalf("GetDocumentSegmentQuestionByID calls = %d, want 0", segments.getQuestionCalls)
	}
	if segments.updateQuestionCalls != 0 {
		t.Fatalf("UpdateDocumentSegmentQuestion calls = %d, want 0", segments.updateQuestionCalls)
	}
}

func TestGetDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeResponse(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	questionID := "question-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
		question: &dto.DocumentSegmentQuestionResponse{
			ID:         questionID,
			DatasetID:  datasetID,
			DocumentID: documentID,
			SegmentID:  "segment-2",
			Question:   "foreign question",
		},
	}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     &datasetAccessAuthorizationService{allow: true},
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodGet,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/"+questionID,
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"",
	)
	c.Params = append(c.Params, gin.Param{Key: "question_id", Value: questionID})

	handler.GetDocumentSegmentQuestion(c)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want non-200 for question outside route; body=%s", recorder.Code, recorder.Body.String())
	}
	if segments.getQuestionCalls != 1 {
		t.Fatalf("GetDocumentSegmentQuestionByID calls = %d, want 1", segments.getQuestionCalls)
	}
	if segments.updateQuestionCalls != 0 || segments.deleteQuestionCalls != 0 {
		t.Fatalf("mutation calls = update:%d delete:%d, want 0/0", segments.updateQuestionCalls, segments.deleteQuestionCalls)
	}
}

func TestUpdateDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	questionID := "question-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
		question: &dto.DocumentSegmentQuestionResponse{
			ID:         questionID,
			DatasetID:  datasetID,
			DocumentID: documentID,
			SegmentID:  "segment-2",
		},
	}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     &datasetAccessAuthorizationService{allow: true},
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodPut,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/"+questionID,
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		`{"question":"updated"}`,
	)
	c.Params = append(c.Params, gin.Param{Key: "question_id", Value: questionID})

	handler.UpdateDocumentSegmentQuestion(c)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want non-200 for question outside route; body=%s", recorder.Code, recorder.Body.String())
	}
	if segments.getQuestionCalls != 1 {
		t.Fatalf("GetDocumentSegmentQuestionByID calls = %d, want 1", segments.getQuestionCalls)
	}
	if segments.updateQuestionCalls != 0 {
		t.Fatalf("UpdateDocumentSegmentQuestion calls = %d, want 0", segments.updateQuestionCalls)
	}
}

func TestDeleteDocumentSegmentQuestionRejectsQuestionOutsideRouteBeforeMutation(t *testing.T) {
	datasetID := "dataset-1"
	documentID := "document-1"
	segmentID := "segment-1"
	questionID := "question-1"
	datasets := &datasetDocumentPermissionDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {
				ID:             datasetID,
				OrganizationID: "org-1",
				WorkspaceID:    "workspace-1",
			},
		},
	}
	documents := &datasetDocumentPermissionDocumentService{
		documents: map[string]*dataset_model.Document{
			documentID: {
				ID:             documentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
			},
		},
	}
	segments := &datasetSegmentQuestionService{
		segments: map[string]*dataset_model.DocumentSegment{
			segmentID: {
				ID:             segmentID,
				OrganizationID: "org-1",
				DatasetID:      datasetID,
				DocumentID:     documentID,
			},
		},
		question: &dto.DocumentSegmentQuestionResponse{
			ID:         questionID,
			DatasetID:  datasetID,
			DocumentID: documentID,
			SegmentID:  "segment-2",
		},
	}
	handler := &SegmentHandler{
		datasetService:  datasets,
		documentService: documents,
		segmentService:  segments,
		authService:     &datasetAccessAuthorizationService{allow: true},
	}
	c, recorder := newDatasetSegmentQuestionContext(
		http.MethodDelete,
		"/datasets/"+datasetID+"/documents/"+documentID+"/segments/"+segmentID+"/questions/"+questionID,
		datasetID,
		documentID,
		segmentID,
		"account-1",
		"org-1",
		"",
	)
	c.Params = append(c.Params, gin.Param{Key: "question_id", Value: questionID})

	handler.DeleteDocumentSegmentQuestion(c)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want non-200 for question outside route; body=%s", recorder.Code, recorder.Body.String())
	}
	if segments.getQuestionCalls != 1 {
		t.Fatalf("GetDocumentSegmentQuestionByID calls = %d, want 1", segments.getQuestionCalls)
	}
	if segments.deleteQuestionCalls != 0 {
		t.Fatalf("DeleteDocumentSegmentQuestion calls = %d, want 0", segments.deleteQuestionCalls)
	}
}

func TestGetBatchHitTestingTaskStatusRejectsTaskOutsideRouteDataset(t *testing.T) {
	datasetID := "11111111-1111-1111-1111-111111111111"
	otherDatasetID := "22222222-2222-2222-2222-222222222222"
	datasetService := &datasetBatchTaskDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {ID: datasetID, OrganizationID: "org-1", WorkspaceID: "workspace-1"},
		},
	}
	manager, taskID := newDatasetBatchTaskManager(t, otherDatasetID, "account-1", "org-1", "pending")
	handler := &DatasetHandler{
		datasetService:   datasetService,
		batchTaskManager: manager,
	}
	c, recorder := newDatasetBatchTaskContext(http.MethodGet, datasetID, taskID, "account-1", "org-1", "")

	handler.GetBatchHitTestingTaskStatus(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if datasetService.permissionChecks != 0 {
		t.Fatalf("dataset permission checks = %d, want 0 before route/task mismatch is rejected", datasetService.permissionChecks)
	}
}

func TestGetBatchHitTestingTaskReportRejectsForeignAccountBeforeReport(t *testing.T) {
	datasetID := "11111111-1111-1111-1111-111111111111"
	datasetService := &datasetBatchTaskDatasetService{
		datasets: map[string]*dataset_model.Dataset{
			datasetID: {ID: datasetID, OrganizationID: "org-1", WorkspaceID: "workspace-1"},
		},
	}
	manager, taskID := newDatasetBatchTaskManager(t, datasetID, "account-2", "org-1", "completed")
	handler := &DatasetHandler{
		datasetService:   datasetService,
		batchTaskManager: manager,
	}
	c, recorder := newDatasetBatchTaskContext(http.MethodGet, datasetID, taskID, "account-1", "org-1", "")

	handler.GetBatchHitTestingTaskReport(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if datasetService.permissionChecks != 0 {
		t.Fatalf("dataset permission checks = %d, want 0 before task ownership is rejected", datasetService.permissionChecks)
	}
}

func TestStopBatchHitTestingTaskRejectsTaskOutsideRouteDatasetBeforeStop(t *testing.T) {
	datasetID := "11111111-1111-1111-1111-111111111111"
	otherDatasetID := "22222222-2222-2222-2222-222222222222"
	manager, taskID := newDatasetBatchTaskManager(t, otherDatasetID, "account-1", "org-1", "processing")
	handler := &DatasetHandler{
		datasetService: &datasetBatchTaskDatasetService{
			datasets: map[string]*dataset_model.Dataset{
				datasetID: {ID: datasetID, OrganizationID: "org-1", WorkspaceID: "workspace-1"},
			},
		},
		batchTaskManager: manager,
	}
	c, recorder := newDatasetBatchTaskContext(http.MethodPost, datasetID, taskID, "account-1", "org-1", "")

	handler.StopBatchHitTestingTask(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	task, ok := manager.GetTask(taskID)
	if !ok {
		t.Fatalf("task not found after stop denial")
	}
	if task.Status != "processing" {
		t.Fatalf("task status = %q, want processing after denied stop", task.Status)
	}
}

func TestSaveBatchHitTestingResultsRejectsForeignAccountBeforeBindingRequest(t *testing.T) {
	datasetID := "11111111-1111-1111-1111-111111111111"
	manager, taskID := newDatasetBatchTaskManager(t, datasetID, "account-2", "org-1", "completed")
	queryService := &datasetBatchTaskQueryService{}
	handler := &DatasetHandler{
		datasetService: &datasetBatchTaskDatasetService{
			datasets: map[string]*dataset_model.Dataset{
				datasetID: {ID: datasetID, OrganizationID: "org-1", WorkspaceID: "workspace-1"},
			},
		},
		datasetQueryService: queryService,
		batchTaskManager:    manager,
	}
	c, recorder := newDatasetBatchTaskContext(http.MethodPost, datasetID, taskID, "account-1", "org-1", "{")

	handler.SaveBatchHitTestingResults(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if queryService.saveCalls != 0 {
		t.Fatalf("SaveBatchHitTestingResults calls = %d, want 0", queryService.saveCalls)
	}
}

func newDatasetAccessTestContext(accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/datasets/dataset-1", nil)
	c.Set("account_id", accountID)
	util.SetOrganizationScopeCompat(c, organizationID)
	return c, recorder
}

func newDatasetSegmentQuestionContext(method, target, datasetID, documentID, segmentID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	c, recorder := newDatasetAccessTestContext(accountID, organizationID)
	c.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "dataset_id", Value: datasetID},
		{Key: "document_id", Value: documentID},
		{Key: "segment_id", Value: segmentID},
	}
	return c, recorder
}

func newDatasetBatchTaskContext(method, datasetID, taskID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	c, recorder := newDatasetAccessTestContext(accountID, organizationID)
	target := "/datasets/" + datasetID + "/batch-hit-testing/tasks/" + taskID
	c.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "dataset_id", Value: datasetID},
		{Key: "task_id", Value: taskID},
	}
	return c, recorder
}

func newDatasetBatchTaskManager(t *testing.T, datasetID, accountID, organizationID, status string) (*datasetservice.BatchHitTestingTaskManager, string) {
	t.Helper()
	manager := datasetservice.NewBatchHitTestingTaskManager(&datasetBatchTaskRepository{
		tasks: map[string]*dataset_model.BatchHitTestingTask{},
	})
	taskID := manager.CreateTask(datasetID, accountID, organizationID, &dto.AsyncBatchHitTestingRequest{
		Queries: []string{"what is covered?"},
	})
	if status != "" && status != "pending" {
		manager.UpdateTaskStatus(taskID, status)
	}
	return manager, taskID
}

type datasetAccessDatasetService struct {
	datasets map[string]*dataset_model.Dataset
}

func (s *datasetAccessDatasetService) GetDatasetByID(ctx context.Context, id string) (*dataset_model.Dataset, error) {
	return s.datasets[id], nil
}

type datasetAccessDocumentService struct {
	documents map[string]*dataset_model.Document
}

func (s *datasetAccessDocumentService) GetDocumentByID(ctx context.Context, documentID string) (*dataset_model.Document, error) {
	return s.documents[documentID], nil
}

type datasetDocumentPermissionDatasetService struct {
	datasetservice.DatasetService
	datasets map[string]*dataset_model.Dataset
}

func (s *datasetDocumentPermissionDatasetService) GetDatasetByID(ctx context.Context, id string) (*dataset_model.Dataset, error) {
	return s.datasets[id], nil
}

type datasetDocumentPermissionDocumentService struct {
	datasetservice.DocumentService
	documents                 map[string]*dataset_model.Document
	updateDocumentCalls       int
	retryDocumentsCalls       int
	updateDocumentStatusCalls int
}

func (s *datasetDocumentPermissionDocumentService) GetDocumentByID(ctx context.Context, documentID string) (*dataset_model.Document, error) {
	return s.documents[documentID], nil
}

func (s *datasetDocumentPermissionDocumentService) UpdateDocument(ctx context.Context, req *dto.DocumentUpdateRequest, datasetID, documentID string) error {
	s.updateDocumentCalls++
	return nil
}

func (s *datasetDocumentPermissionDocumentService) RetryDocuments(ctx context.Context, datasetID string, documentIDs []string, userID string) error {
	s.retryDocumentsCalls++
	return nil
}

func (s *datasetDocumentPermissionDocumentService) UpdateDocumentStatus(ctx context.Context, datasetID, action string, documentIDs []string, accountID string) error {
	s.updateDocumentStatusCalls++
	return nil
}

type datasetAccessFolderService struct {
	folders map[string]*dataset_model.DatasetFolder
}

func (s *datasetAccessFolderService) GetFolderByID(ctx context.Context, folderID string) (*dataset_model.DatasetFolder, error) {
	return s.folders[folderID], nil
}

type datasetAccessSegmentService struct {
	segments    map[string]*dataset_model.DocumentSegment
	childChunks map[string]*dataset_model.ChildChunk
}

func (s *datasetAccessSegmentService) GetChunkByID(ctx context.Context, id string) (*dataset_model.DocumentSegment, error) {
	return s.segments[id], nil
}

func (s *datasetAccessSegmentService) GetChildChunkByID(ctx context.Context, childChunkID string) (*dataset_model.ChildChunk, error) {
	return s.childChunks[childChunkID], nil
}

type datasetSegmentQuestionService struct {
	datasetservice.SegmentService
	segments                 map[string]*dataset_model.DocumentSegment
	question                 *dto.DocumentSegmentQuestionResponse
	createQuestionCalls      int
	batchCreateQuestionCalls int
	generateQuestionCalls    int
	getQuestionCalls         int
	updateQuestionCalls      int
	deleteQuestionCalls      int
}

func (s *datasetSegmentQuestionService) GetChunkByID(ctx context.Context, id string) (*dataset_model.DocumentSegment, error) {
	return s.segments[id], nil
}

func (s *datasetSegmentQuestionService) CreateDocumentSegmentQuestion(ctx context.Context, req *dto.DocumentSegmentQuestionCreateRequest, userID, organizationID string) (*dto.DocumentSegmentQuestionResponse, error) {
	s.createQuestionCalls++
	return &dto.DocumentSegmentQuestionResponse{}, nil
}

func (s *datasetSegmentQuestionService) BatchCreateDocumentSegmentQuestions(ctx context.Context, req *dto.DocumentSegmentQuestionBatchCreateRequest, userID, organizationID string, segmentID string) (*dto.DocumentSegmentQuestionBatchCreateResponse, error) {
	s.batchCreateQuestionCalls++
	return &dto.DocumentSegmentQuestionBatchCreateResponse{}, nil
}

func (s *datasetSegmentQuestionService) GenerateQuestionsForSegment(ctx context.Context, segmentID string, count int, userID, organizationID string, model *dto.ModelSpec) (*dto.DocumentSegmentQuestionBatchCreateResponse, error) {
	s.generateQuestionCalls++
	return &dto.DocumentSegmentQuestionBatchCreateResponse{}, nil
}

func (s *datasetSegmentQuestionService) GetDocumentSegmentQuestionByID(ctx context.Context, questionID string) (*dto.DocumentSegmentQuestionResponse, error) {
	s.getQuestionCalls++
	if s.question != nil {
		return s.question, nil
	}
	return &dto.DocumentSegmentQuestionResponse{}, nil
}

func (s *datasetSegmentQuestionService) UpdateDocumentSegmentQuestion(ctx context.Context, questionID string, req *dto.DocumentSegmentQuestionUpdateRequest, userID string) (*dto.DocumentSegmentQuestionResponse, error) {
	s.updateQuestionCalls++
	return &dto.DocumentSegmentQuestionResponse{}, nil
}

func (s *datasetSegmentQuestionService) DeleteDocumentSegmentQuestion(ctx context.Context, questionID string) error {
	s.deleteQuestionCalls++
	return nil
}

type datasetBatchTaskDatasetService struct {
	datasetservice.DatasetService
	datasets         map[string]*dataset_model.Dataset
	permissionChecks int
}

func (s *datasetBatchTaskDatasetService) GetDatasetWithPermissionCheck(ctx context.Context, datasetID, accountID, organizationID string) (*dataset_model.Dataset, error) {
	s.permissionChecks++
	dataset := s.datasets[datasetID]
	if dataset == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return dataset, nil
}

type datasetBatchTaskQueryService struct {
	datasetservice.DatasetQueryService
	saveCalls int
}

func (s *datasetBatchTaskQueryService) SaveBatchHitTestingResults(ctx context.Context, req *datasetservice.SaveBatchHitTestingResultsRequest) error {
	s.saveCalls++
	return nil
}

type datasetBatchTaskRepository struct {
	tasks map[string]*dataset_model.BatchHitTestingTask
}

func (r *datasetBatchTaskRepository) Create(ctx context.Context, task *dataset_model.BatchHitTestingTask) error {
	copied := *task
	r.tasks[task.TaskID] = &copied
	return nil
}

func (r *datasetBatchTaskRepository) GetByID(ctx context.Context, taskID string) (*dataset_model.BatchHitTestingTask, error) {
	task := r.tasks[taskID]
	if task == nil {
		return nil, gorm.ErrRecordNotFound
	}
	copied := *task
	return &copied, nil
}

func (r *datasetBatchTaskRepository) Update(ctx context.Context, task *dataset_model.BatchHitTestingTask) error {
	copied := *task
	r.tasks[task.TaskID] = &copied
	return nil
}

func (r *datasetBatchTaskRepository) UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, finishedAt *time.Time) error {
	task := r.tasks[taskID]
	if task == nil {
		return gorm.ErrRecordNotFound
	}
	task.Status = status
	if startedAt != nil {
		task.StartedAt = *startedAt
	}
	if finishedAt != nil {
		task.FinishedAt = *finishedAt
	}
	return nil
}

func (r *datasetBatchTaskRepository) UpdateQueryTaskStatus(ctx context.Context, taskID string, queryIndex int, status string, result *dataset_model.QueryTask) error {
	task := r.tasks[taskID]
	if task == nil {
		return gorm.ErrRecordNotFound
	}
	if queryIndex >= len(task.Queries) {
		return nil
	}
	task.Queries[queryIndex] = *result
	return nil
}

func (r *datasetBatchTaskRepository) ListByOrganizationID(ctx context.Context, organizationID string, page, limit int) ([]*dataset_model.BatchHitTestingTask, int64, error) {
	return nil, 0, nil
}

func (r *datasetBatchTaskRepository) ListByDatasetID(ctx context.Context, datasetID string, page, limit int) ([]*dataset_model.BatchHitTestingTask, int64, error) {
	return nil, 0, nil
}

func (r *datasetBatchTaskRepository) WithTx(tx *gorm.DB) datasetrepo.BatchHitTestingTaskRepository {
	return r
}

type datasetAccessAuthorizationService struct {
	interfaces.AuthorizationService
	allow       bool
	lastRequest interfaces.WorkspaceScopeRequest
}

func (s *datasetAccessAuthorizationService) RequireWorkspacePermission(ctx context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error) {
	s.lastRequest = req
	if !s.allow {
		return nil, shared_service.ErrAuthorizationDenied
	}
	return &interfaces.WorkspaceScope{
		WorkspaceID:     req.WorkspaceID,
		PermissionCodes: req.PermissionCodes,
	}, nil
}

func containsWorkspacePermission(codes []workspace_model.WorkspacePermissionCode, want workspace_model.WorkspacePermissionCode) bool {
	for _, code := range codes {
		if code == want {
			return true
		}
	}
	return false
}

func assertNoCoarseKnowledgeBasePermissions(t *testing.T, codes []workspace_model.WorkspacePermissionCode) {
	t.Helper()

	disallowed := []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
	}
	for _, code := range disallowed {
		if containsWorkspacePermission(codes, code) {
			t.Fatalf("permissions should not include coarse knowledge base permission %s: %#v", code, codes)
		}
	}
}

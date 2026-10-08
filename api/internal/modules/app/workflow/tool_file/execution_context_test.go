package tool_file

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

func TestPlatformBusinessFilesInheritVerifiedOwnerNotBillingAccount(t *testing.T) {
	manager, _, params, access := executionFileFixture(t)
	origin := toolgovernance.ExecutionOrigin{OrganizationID: access.OrganizationID, WorkspaceID: access.WorkspaceID,
		ApplicationType: access.ApplicationType, ApplicationID: access.ApplicationID, Entry: access.Entry,
		SubjectKind: access.SubjectKind, SubjectID: access.SubjectID, ConversationID: access.ConversationID,
		MessageID: *params.ExecutionMessageID, PublicationRef: uuid.NewString()}
	ctx, err := WithExecutionOrigin(t.Context(), origin)
	if err != nil {
		t.Fatal(err)
	}
	params.UserID, params.TenantID = uuid.NewString(), uuid.NewString()
	params.ExecutionIdentity, params.ExecutionMessageID, params.ConversationID = nil, nil, nil
	file, err := manager.CreateFileByRaw(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if file.UserID != origin.SubjectID || file.TenantID != origin.OrganizationID || file.ExecutionIdentity == nil ||
		file.ExecutionIdentity.ApplicationID != origin.ApplicationID || file.ExecutionMessageID == nil || *file.ExecutionMessageID != origin.MessageID {
		t.Fatalf("business file lost execution owner: %+v", file)
	}
	if _, _, err := manager.GetExecutionFileBinary(ctx, access, file.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.GetFileBinary(ctx, file.ID); err == nil {
		t.Fatal("legacy reader bypassed protected file")
	}
	origin.SubjectID = ""
	if _, err := WithExecutionOrigin(t.Context(), origin); err == nil {
		t.Fatal("invalid origin accepted")
	}
}

func TestPlatformRemoteFilesAreProtectedAndNeverPublishSourceURLs(t *testing.T) {
	manager, _, params, access := executionFileFixture(t)
	origin := toolgovernance.ExecutionOrigin{OrganizationID: access.OrganizationID, WorkspaceID: access.WorkspaceID,
		ApplicationType: access.ApplicationType, ApplicationID: access.ApplicationID, Entry: access.Entry,
		SubjectKind: access.SubjectKind, SubjectID: access.SubjectID, ConversationID: access.ConversationID,
		MessageID: *params.ExecutionMessageID, PublicationRef: uuid.NewString()}
	ctx, err := WithExecutionOrigin(t.Context(), origin)
	if err != nil {
		t.Fatal(err)
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("private result"))
	}))
	defer source.Close()
	file, err := manager.CreateFileByURL(ctx, CreateFileByURLParams{UserID: uuid.NewString(), TenantID: uuid.NewString(), FileURL: source.URL + "/result?token=secret"})
	if err != nil {
		t.Fatal(err)
	}
	if file.UserID != origin.SubjectID || file.TenantID != origin.OrganizationID || file.ExecutionIdentity == nil || file.OriginalURL != nil {
		t.Fatal("remote file lost protected ownership or retained source URL")
	}
	data, _, err := manager.GetExecutionFileBinary(ctx, access, file.ID)
	if err != nil || string(data) != "private result" {
		t.Fatalf("authenticated read: %q %v", data, err)
	}
	if _, _, err := manager.GetFileBinary(ctx, file.ID); err == nil {
		t.Fatal("legacy reader accepted remote protected file")
	}
	if url, err := PublicGeneratedFileURL(file, ".txt"); err != nil || url != "" {
		t.Fatalf("protected file signed: %q %v", url, err)
	}
	metadata := map[string]interface{}{"file_id": file.ID, "url": source.URL, "download_url": source.URL}
	ProtectGeneratedFileMetadata(file, metadata)
	if metadata["access_mode"] != "authenticated" || metadata["url"] != nil || metadata["download_url"] != nil || metadata["file_id"] != file.ID {
		t.Fatalf("protected metadata: %+v", metadata)
	}
}

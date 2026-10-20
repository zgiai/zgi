package container

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
)

type socialImageStoreStub struct {
	file  *toolfile.ToolFile
	reads int
}

func (s *socialImageStoreStub) GetToolFileByID(context.Context, string) (*toolfile.ToolFile, error) {
	return s.file, nil
}
func (s *socialImageStoreStub) GetFileBinary(context.Context, string) ([]byte, string, error) {
	s.reads++
	return []byte("image-fixture"), "image/png", nil
}
func TestSocialImageImportRechecksFileOwnerAndConversationBeforeBinaryRead(t *testing.T) {
	scope := socialops.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}
	conversation := uuid.NewString()
	id := uuid.New()
	for _, mode := range []string{"valid", "wrong_owner", "wrong_tenant", "wrong_conversation", "too_large", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			file := &toolfile.ToolFile{ID: id.String(), TenantID: scope.OrganizationID.String(), UserID: scope.OwnerID.String(), ConversationID: &conversation, Size: 10}
			switch mode {
			case "wrong_owner":
				file.UserID = uuid.NewString()
			case "wrong_tenant":
				file.TenantID = uuid.NewString()
			case "wrong_conversation":
				other := uuid.NewString()
				file.ConversationID = &other
			case "too_large":
				file.Size = 9 << 20
			case "deleted":
				now := time.Now()
				file.DeletedAt = &now
			}
			store := &socialImageStoreStub{file: file}
			_, err := importSocialImageFromStore(context.Background(), scope, conversation, id, store)
			if mode == "valid" {
				if err != nil || store.reads != 1 {
					t.Fatal(err, store.reads)
				}
			} else if err == nil || store.reads != 0 {
				t.Fatal("unauthorized binary access", err, store.reads)
			}
		})
	}
}

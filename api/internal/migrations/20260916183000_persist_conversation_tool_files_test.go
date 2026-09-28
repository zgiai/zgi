package migrations

import (
	"testing"
	"time"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPersistConversationToolFilesPreservesWorkflowExpiry(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:conversation-tool-file-lifecycle?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE tool_files (id TEXT PRIMARY KEY, conversation_id TEXT, lifecycle TEXT, expires_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	for _, row := range []struct {
		id             string
		conversationID *string
		lifecycle      string
		expiresAt      *time.Time
	}{
		{id: "agent-temp", conversationID: ptrString("conversation-1"), lifecycle: "temporary", expiresAt: &expiresAt},
		{id: "agent-stale", conversationID: ptrString("conversation-1"), lifecycle: "persistent", expiresAt: &expiresAt},
		{id: "workflow-temp", lifecycle: "temporary", expiresAt: &expiresAt},
	} {
		if err := db.Exec("INSERT INTO tool_files (id, conversation_id, lifecycle, expires_at) VALUES (?, ?, ?, ?)", row.id, row.conversationID, row.lifecycle, row.expiresAt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := upPersistConversationToolFiles(mschema.New(db)); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		id        string
		lifecycle string
		expires   bool
	}{
		{id: "agent-temp", lifecycle: "persistent"},
		{id: "agent-stale", lifecycle: "persistent"},
		{id: "workflow-temp", lifecycle: "temporary", expires: true},
	} {
		var actual struct {
			Lifecycle string
			ExpiresAt *time.Time
		}
		if err := db.Table("tool_files").Where("id = ?", row.id).Take(&actual).Error; err != nil {
			t.Fatal(err)
		}
		if actual.Lifecycle != row.lifecycle || (actual.ExpiresAt != nil) != row.expires {
			t.Errorf("%s = %q, %v; want %q, expiry=%v", row.id, actual.Lifecycle, actual.ExpiresAt, row.lifecycle, row.expires)
		}
	}
}

func ptrString(value string) *string { return &value }

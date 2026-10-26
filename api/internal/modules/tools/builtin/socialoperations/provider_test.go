package socialoperations

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type scopeSpy struct {
	calls    int
	scope    socialops.Scope
	identity InvocationIdentity
	image    []byte
}

func (s *scopeSpy) ImportSocialImage(context.Context, InvocationIdentity, uuid.UUID) ([]byte, error) {
	return s.image, nil
}

func (s *scopeSpy) ResolveSocialOperations(_ context.Context, i InvocationIdentity) (socialops.Scope, socialops.PublicationGateway, error) {
	s.calls++
	s.identity = i
	return s.scope, nil, nil
}

func TestImportGeneratedImagePersistsPreparedLocalBytes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	defer sql.Close()
	if err = db.AutoMigrate(&socialops.Artifact{}); err != nil {
		t.Fatal(err)
	}
	service, err := socialops.New(db, t.TempDir(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var imageBytes bytes.Buffer
	if err = png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 160, 90))); err != nil {
		t.Fatal(err)
	}
	resolver := &scopeSpy{scope: socialops.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}, image: imageBytes.Bytes()}
	provider := NewProvider(func() (*socialops.Service, error) { return service, nil }, resolver)
	tool, err := provider.GetTool("import_image")
	if err != nil {
		t.Fatal(err)
	}
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: resolver.scope.OrganizationID.String()})
	result, err := tool.Invoke(context.Background(), "actor", map[string]any{"input": `{"source_file_id":"` + uuid.NewString() + `"}`}, nil, nil, nil)
	if err != nil || len(result) != 1 {
		t.Fatal(result, err)
	}
	var count int64
	if err = db.Model(&socialops.Artifact{}).Where("mime = ?", "image/png").Count(&count).Error; err != nil || count != 1 {
		t.Fatal(count, err)
	}
}
func TestToolsRestoreScopeAndRejectIdentityArguments(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	defer sql.Close()
	if err = db.AutoMigrate(&socialops.Artifact{}, &socialops.Record{}, &socialops.Publication{}, &socialops.Metric{}); err != nil {
		t.Fatal(err)
	}
	service, err := socialops.New(db, t.TempDir(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	resolver := &scopeSpy{scope: socialops.Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerID: uuid.New(), EmployeeID: uuid.New()}}
	provider := NewProvider(func() (*socialops.Service, error) { return service, nil }, resolver)
	base, err := provider.GetTool("save_record")
	if err != nil {
		t.Fatal(err)
	}
	tool := base.ForkToolRuntime(&tools.ToolRuntime{TenantID: resolver.scope.OrganizationID.String()})
	conv, app, msg := uuid.NewString(), resolver.scope.EmployeeID.String(), uuid.NewString()
	result, err := tool.Invoke(context.Background(), "actor", map[string]any{"input": `{"kind":"source","key":"source-1","content":"verified original","mime":"text/plain","expected_revision":0}`}, &conv, &app, &msg)
	if err != nil || len(result) != 1 || resolver.calls != 1 || resolver.identity.ConversationID != conv || resolver.identity.EmployeeID != app {
		t.Fatal(result, err, resolver)
	}
	rows, err := service.ListRecords(context.Background(), resolver.scope, "source", uuid.Nil, 20)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	for _, input := range []map[string]any{{"input": `{"kind":"source","key":"bad","content":"bad","owner_id":"forged"}`}, {"input": `{"kind":"source","key":"bad","content":"bad"}`, "employee_id": "forged"}} {
		if _, err = tool.Invoke(context.Background(), "actor", input, &conv, &app, &msg); err == nil {
			t.Fatal("accepted model-authored scope")
		}
	}
	if _, err = tool.Invoke(context.Background(), "actor", map[string]any{"input": `{"kind":"configuration","key":"current","content":"{}","expected_revision":0}`}, &conv, &app, &msg); err == nil {
		t.Fatal("model changed server-managed operating configuration")
	}
	if _, err = tool.Invoke(context.Background(), "actor", map[string]any{"input": `{"kind":"archive","key":"2026-10-20","content":"unverified publication","expected_revision":0}`}, &conv, &app, &msg); err == nil {
		t.Fatal("model fabricated a public publication archive")
	}
}

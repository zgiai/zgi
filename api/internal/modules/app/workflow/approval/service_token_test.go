package approval

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGenerateTokenReturnsEightURLSafeCharacters(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Za-z0-9]{8}$`)

	for i := 0; i < 100; i++ {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken returned error: %v", err)
		}
		if !pattern.MatchString(token) {
			t.Fatalf("token = %q, want 8 URL-safe alphanumeric characters", token)
		}
	}
}

func TestCreateRuntimeFormRetriesTokenCollision(t *testing.T) {
	ctx := context.Background()
	db := newApprovalTokenTestDB(t)

	if err := db.Create(&Recipient{
		ID:               uuid.NewString(),
		FormID:           "existing-form",
		DeliveryID:       "existing-delivery",
		RecipientType:    RecipientTypeWebApp,
		RecipientPayload: `{}`,
		AccessToken:      "DUPLICAT",
	}).Error; err != nil {
		t.Fatalf("create existing recipient: %v", err)
	}

	restoreTokenGenerator := newApprovalToken
	defer func() { newApprovalToken = restoreTokenGenerator }()
	tokens := []string{"DUPLICAT", "UNIQUE01"}
	newApprovalToken = func() (string, error) {
		if len(tokens) == 0 {
			t.Fatal("token generator called more times than expected")
		}
		token := tokens[0]
		tokens = tokens[1:]
		return token, nil
	}

	webEnabled := true
	service := NewService(db)
	form, err := service.CreateOrGetRuntimeForm(ctx, CreateRuntimeFormParams{
		TenantID:      uuid.NewString(),
		AppID:         uuid.NewString(),
		WorkflowRunID: uuid.NewString(),
		NodeID:        "approval",
		NodeTitle:     "Approval",
		Rendered:      "Review",
		Config: NodeConfig{
			Content: "Review",
			Fields:  []FieldConfig{{Key: "comment", Label: "Comment", Type: "textarea"}},
			Actions: []Action{{ID: "approve", Label: "Approve"}},
			SubmitMethods: SubmitMethods{
				WebApp: WebAppSubmitMethod{Enabled: &webEnabled},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	if form.Payload.Token != "UNIQUE01" {
		t.Fatalf("payload token = %q, want regenerated token", form.Payload.Token)
	}
}

func newApprovalTokenTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Form{}, &Delivery{}, &Recipient{}); err != nil {
		t.Fatalf("auto migrate approval tables: %v", err)
	}
	return db
}

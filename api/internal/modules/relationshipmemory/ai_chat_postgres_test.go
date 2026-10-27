package relationshipmemory

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// AI chat keeps one memory per person, outside any organization (ADR 0021).
func TestAIChatMemoryIsThePersonsAcrossOrganizations(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run relationship memory PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	service := NewService(tx)
	adapter := &AgentAdapter{service: service}
	accountID, otherAccountID := uuid.New(), uuid.New()
	firstOrganization, secondOrganization := uuid.New(), uuid.New()

	// Listing creates the person's AI chat memory, in no organization.
	page, err := service.ListAccountRelationships(t.Context(), ListRelationshipsRequest{OrganizationID: firstOrganization, AccountID: accountID})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ConsumerType != ConsumerAIChat || page.Items[0].OrganizationID != uuid.Nil || !page.Items[0].CanManage {
		t.Fatalf("first organization page = %#v", page)
	}
	relationshipID := page.Items[0].ID

	// The chat runtime reaches the same memory through the adapter, and it
	// is the same from another organization.
	runtimeRelationship, err := adapter.relationship(t.Context(), uuid.Nil, AIChatConsumerID, agentmemory.UserScopeAccount, accountID)
	if err != nil || runtimeRelationship.ID != relationshipID {
		t.Fatalf("runtime relationship = %#v, err = %v", runtimeRelationship, err)
	}
	detail, err := service.GetAccountRelationship(t.Context(), secondOrganization, accountID, relationshipID)
	if err != nil || !detail.CanManage {
		t.Fatalf("detail from another organization = %#v, err = %v", detail, err)
	}
	if _, err := service.GetAccountRelationship(t.Context(), firstOrganization, otherAccountID, relationshipID); !errors.Is(err, ErrRelationshipNotFound) {
		t.Fatalf("another person's read error = %v, want not found", err)
	}

	// Its contract is always on; its settings are the person's.
	enabled, automatic, err := adapter.RelationshipRuntimeSettings(t.Context(), uuid.Nil, AIChatConsumerID, agentmemory.UserScopeAccount, accountID)
	if err != nil || !enabled || !automatic {
		t.Fatalf("runtime settings = %v, %v, err = %v", enabled, automatic, err)
	}

	// Nobody else reaches it: not an API end user, not another scope.
	if _, err := adapter.relationship(t.Context(), uuid.Nil, AIChatConsumerID, agentmemory.UserScopeEndUser, accountID); !errors.Is(err, agentmemory.ErrUnauthorized) {
		t.Fatalf("API end user error = %v, want unauthorized", err)
	}
	if _, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{OrganizationID: firstOrganization, WorkspaceID: uuid.New(), Subject: usersubject.AccountSubject(accountID), ConsumerType: ConsumerAIChat, ConsumerID: AIChatConsumerID}); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("AI chat memory in an organization error = %v, want rejected", err)
	}
	if _, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{Subject: usersubject.AccountSubject(accountID), ConsumerType: ConsumerAgent, ConsumerID: uuid.New()}); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Agent memory without a workspace error = %v, want denied", err)
	}
}

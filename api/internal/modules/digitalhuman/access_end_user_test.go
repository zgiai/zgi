package digitalhuman

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureAPIEndUserSubjectRegistersBeforeAnyChat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&usersubject.Subject{}); err != nil {
		t.Fatal(err)
	}
	service := NewAccessService(db)
	workspaceID, digitalHumanID := uuid.New(), uuid.New()
	subject, err := usersubject.DigitalHumanAPIEndUserSubject(workspaceID, digitalHumanID, "customer_123")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Task and file APIs require a registered subject; a first request from
	// a user who never chatted used to fail here.
	if _, err := usersubject.NewService(db).Require(ctx, subject); err == nil {
		t.Fatal("subject unexpectedly registered before the API request")
	}
	if err := service.EnsureAPIEndUserSubject(ctx, digitalHumanID, subject); err != nil {
		t.Fatalf("ensure subject: %v", err)
	}
	if _, err := usersubject.NewService(db).Require(ctx, subject); err != nil {
		t.Fatalf("registered subject is not usable: %v", err)
	}
	if err := service.EnsureAPIEndUserSubject(ctx, digitalHumanID, subject); err != nil {
		t.Fatalf("second ensure is not idempotent: %v", err)
	}

	// Relationship memory registers the same subject with the same issuer
	// during chat; both paths must agree.
	issuer := digitalHumanID
	if _, err := usersubject.NewService(db).Ensure(ctx, usersubject.EnsureRequest{
		Subject: subject, IssuerType: digitalHumanAPIIssuerType, IssuerID: &issuer,
	}); err != nil {
		t.Fatalf("relationship memory registration conflicts: %v", err)
	}

	if err := service.EnsureAPIEndUserSubject(ctx, digitalHumanID, usersubject.AccountSubject(uuid.New())); err == nil {
		t.Fatal("an account subject must not be registered as an API end user")
	}
}

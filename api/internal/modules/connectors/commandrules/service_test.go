package commandrules

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Rule{}); err != nil {
		t.Fatal(err)
	}
	return NewService(db)
}

func TestCommandRulesAreKeptPerOrganization(t *testing.T) {
	service := newTestService(t)
	ctx := context.Background()
	organization, other, actor := uuid.New(), uuid.New(), uuid.New()
	rule, err := service.Create(ctx, organization, actor, "  git   push ", serverops.RuleForbid, "no pushes from servers")
	if err != nil || rule.Prefix != "git push" {
		t.Fatalf("Create() = %#v, %v", rule, err)
	}
	if _, err := service.Create(ctx, organization, actor, "git push", serverops.RuleAsk, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a duplicate prefix was accepted: %v", err)
	}
	for _, bad := range []struct {
		prefix string
		action serverops.RuleAction
	}{{"", serverops.RuleAsk}, {"rm", "allow"}} {
		if _, err := service.Create(ctx, organization, actor, bad.prefix, bad.action, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("Create(%q, %q) error = %v", bad.prefix, bad.action, err)
		}
	}
	rules, err := service.CommandRules(ctx, organization)
	if err != nil || len(rules) != 1 || rules[0] != (serverops.CommandRule{Prefix: "git push", Action: serverops.RuleForbid}) {
		t.Fatalf("CommandRules() = %#v, %v", rules, err)
	}
	if rules, _ := service.CommandRules(ctx, other); len(rules) != 0 {
		t.Fatal("rules leaked into another organization")
	}
	if err := service.Delete(ctx, other, rule.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another organization deleted the rule: %v", err)
	}
	if err := service.Delete(ctx, organization, rule.ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := service.CommandRules(ctx, organization); len(rules) != 0 {
		t.Fatal("the rule was not deleted")
	}
}

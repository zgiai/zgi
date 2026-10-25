package digitalhuman

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimerepository "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost user=dry dbname=dry sslmode=disable"}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func boundarySQL(t *testing.T, db *gorm.DB, run *Run, event invocation.Event) string {
	t.Helper()
	boundary := conversationSearchBoundary(db, run, event)
	if boundary == nil {
		return ""
	}
	var ids []uuid.UUID
	return boundary.Session(&gorm.Session{DryRun: true}).Find(&ids).Statement.SQL.String()
}

func TestConversationSearchStaysInsideTheRunsBoundary(t *testing.T) {
	db := dryRunDB(t)
	subject := uuid.New()
	trust := string(SourceTrustChannelParticipant)
	base := Run{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New()}

	person := base
	person.Surface, person.UserSubjectID = SurfaceAPI, &subject
	if sql := boundarySQL(t, db, &person, invocation.Event{}); !strings.Contains(sql, "user_subject_id = $") || !strings.Contains(sql, "digital_human_id = $") {
		t.Fatalf("person boundary = %s", sql)
	}

	anonymous := base
	anonymous.Surface = SurfaceWebApp
	if sql := boundarySQL(t, db, &anonymous, invocation.Event{}); sql != "" {
		t.Fatalf("a run without a verified person may search: %s", sql)
	}

	group := base
	group.Surface, group.SourceTrust = SurfaceChannel, &trust
	groupEvent := invocation.Event{Context: map[string]any{"conversation_type": "group"}}
	if sql := boundarySQL(t, db, &group, groupEvent); !strings.Contains(sql, "conversation_type' = $") || strings.Contains(sql, "reply_to") || strings.Contains(sql, "user_subject_id") {
		t.Fatalf("group boundary = %s", sql)
	}

	direct := base
	direct.Surface, direct.SourceTrust = SurfaceChannel, &trust
	directEvent := invocation.Event{Context: map[string]any{"conversation_type": "single"}, ReplyTo: &invocation.ReplyAddress{Provider: "dingtalk", InstallationID: "install-1", ConversationID: "user:abc"}}
	if sql := boundarySQL(t, db, &direct, directEvent); !strings.Contains(sql, "reply_to' ->> 'conversation_id' = $") {
		t.Fatalf("direct chat boundary = %s", sql)
	}
	if sql := boundarySQL(t, db, &direct, invocation.Event{Context: map[string]any{"conversation_type": "single"}}); sql != "" {
		t.Fatalf("a direct chat without a reply address may search: %s", sql)
	}

	task := base
	task.Surface = SurfaceTask
	if sql := boundarySQL(t, db, &task, invocation.Event{}); sql != "" {
		t.Fatalf("a scheduled task may search: %s", sql)
	}
}

func TestConversationSearchWithholdsSecretsAndIdentifiers(t *testing.T) {
	result := conversationSearchModelResult([]runtimerepository.KeywordMatch{
		{ConversationID: uuid.New(), Query: "记住我的数据库密码是 Zx9!kq2#Lm8w", Answer: "我不会保存密码。"},
		{ConversationID: uuid.New(), Query: "我的身份证号是 110101199003071234", Answer: "好的。"},
		{ConversationID: uuid.New(), Query: "客户答谢会定在湖滨酒店。", Answer: "收到。"},
	})
	matches := result["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["user"] != "客户答谢会定在湖滨酒店。" || result["withheld"] != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestConversationSearchWithholdsWhatThePersonForgot(t *testing.T) {
	inner := skillloop.RuntimeTool{Definition: adapter.Tool{Function: adapter.Function{Name: conversationSearchToolName}}, Handler: func(context.Context, adapter.ToolCall) skillloop.RuntimeToolResult {
		return skillloop.RuntimeToolResult{Status: "success", ModelResult: conversationSearchModelResult([]runtimerepository.KeywordMatch{
			{ConversationID: uuid.New(), Query: "请记住：我住在上海浦东。", Answer: "好的。"},
			{ConversationID: uuid.New(), Query: "客户答谢会定在湖滨酒店。", Answer: "收到。"},
		})}
	}}
	forgotten := func(context.Context) ([]string, error) { return []string{"住在上海浦东。"}, nil }
	result := withoutForgottenTurns([]skillloop.RuntimeTool{inner}, forgotten)[0].Handler(context.Background(), adapter.ToolCall{})
	matches := result.ModelResult["matches"].([]map[string]any)
	if len(matches) != 1 || matches[0]["user"] != "客户答谢会定在湖滨酒店。" || result.ModelResult["withheld"] != 1 {
		t.Fatalf("result = %+v", result.ModelResult)
	}
	failing := func(context.Context) ([]string, error) { return nil, errors.New("database down") }
	if result := withoutForgottenTurns([]skillloop.RuntimeTool{inner}, failing)[0].Handler(context.Background(), adapter.ToolCall{}); result.Status == "success" {
		t.Fatal("search answered without knowing what was forgotten")
	}
}

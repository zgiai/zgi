package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestFinishWaitingApprovalMessagePromotesLeaf(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".*"dialogue_count"=CASE WHEN current_leaf_message_id = .* THEN dialogue_count ELSE dialogue_count \+ 1 END.* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), messageID, sqlmock.AnyArg(), sqlmock.AnyArg(), conversationID, messageID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.FinishWaitingApprovalMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("FinishWaitingApprovalMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestFinishContinuationMessageKeepsSameLeafWithoutIncrement(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), conversationID, messageID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.FinishContinuationMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("FinishContinuationMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSearchByCallerScopedMapsResults(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	updatedAt := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"type",
		"conversation_id",
		"conversation_title",
		"message_id",
		"match_text",
		"updated_at",
		"rank",
	}).AddRow(
		"message",
		conversationID.String(),
		"Release notes",
		messageID.String(),
		"Generate release notes",
		updatedAt,
		1,
	)
	mock.ExpectQuery(`(?s).*chat_runtime_conversations AS c.*chat_runtime_messages AS m.*ORDER BY rank ASC, updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"%release%",
			"%release%",
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"%release%",
			"%release%",
			20,
		).
		WillReturnRows(rows)

	results, err := repo.SearchByCallerScoped(context.Background(), organizationID, accountID, runtimemodel.ConversationCallerAIChat, nil, runtimemodel.ConversationTypeChat, "", nil, "", "release", 20)
	if err != nil {
		t.Fatalf("SearchByCallerScoped: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ConversationID != conversationID {
		t.Fatalf("conversation id = %s, want %s", results[0].ConversationID, conversationID)
	}
	if results[0].MessageID == nil || *results[0].MessageID != messageID {
		t.Fatalf("message id = %v, want %s", results[0].MessageID, messageID)
	}
	if results[0].MatchText != "Generate release notes" {
		t.Fatalf("match text = %q", results[0].MatchText)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSearchByCallerScopedAppliesSurfaceFilter(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()

	rows := sqlmock.NewRows([]string{
		"type",
		"conversation_id",
		"conversation_title",
		"message_id",
		"match_text",
		"updated_at",
		"rank",
	})
	mock.ExpectQuery(`(?s).*c\.metadata->>'surface'.*m_surface\.metadata->>'surface'.*ORDER BY rank ASC, updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"contextual_sidebar",
			"contextual_sidebar",
			"%asset%",
			"%asset%",
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"contextual_sidebar",
			"contextual_sidebar",
			"%asset%",
			"%asset%",
			10,
		).
		WillReturnRows(rows)

	results, err := repo.SearchByCallerScoped(context.Background(), organizationID, accountID, runtimemodel.ConversationCallerAIChat, nil, runtimemodel.ConversationTypeChat, "", nil, "contextual_sidebar", "asset", 10)
	if err != nil {
		t.Fatalf("SearchByCallerScoped: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListByCallerSurfaceScopedAppliesSidebarSurfaceFilter(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()

	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*metadata->>'surface'.*EXISTS.*m\.metadata->>'surface'.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"contextual_sidebar",
			"contextual_sidebar",
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*metadata->>'surface'.*EXISTS.*m\.metadata->>'surface'.*ORDER BY updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"contextual_sidebar",
			"contextual_sidebar",
			20,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	results, total, err := repo.ListByCallerSurfaceScoped(context.Background(), organizationID, accountID, runtimemodel.ConversationCallerAIChat, nil, "contextual_sidebar", 20, 0)
	if err != nil {
		t.Fatalf("ListByCallerSurfaceScoped: %v", err)
	}
	if total != 0 || len(results) != 0 {
		t.Fatalf("results = %d total = %d, want empty", len(results), total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListByCallerSurfaceScopedWorkChatKeepsLegacyOnlyWhenNoOtherSurface(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()

	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*NOT EXISTS.*m\.metadata->>'surface'.*<>.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"work_chat",
			"work_chat",
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*NOT EXISTS.*m\.metadata->>'surface'.*<>.*ORDER BY updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAIChat,
			runtimemodel.ConversationTypeChat,
			"work_chat",
			"work_chat",
			20,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	results, total, err := repo.ListByCallerSurfaceScoped(context.Background(), organizationID, accountID, runtimemodel.ConversationCallerAIChat, nil, "work_chat", 20, 0)
	if err != nil {
		t.Fatalf("ListByCallerSurfaceScoped: %v", err)
	}
	if total != 0 || len(results) != 0 {
		t.Fatalf("results = %d total = %d, want empty", len(results), total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListByCallerSourceScopedFiltersWebAppIdentity(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	webAppID := uuid.New()

	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*caller_type.*caller_id.*source.*source_web_app_id.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAgent,
			agentID,
			runtimemodel.ConversationTypeChat,
			runtimemodel.ConversationSourceWebApp,
			webAppID,
		).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s).*FROM "chat_runtime_conversations".*caller_type.*caller_id.*source.*source_web_app_id.*ORDER BY updated_at DESC.*LIMIT.*`).
		WithArgs(
			organizationID,
			accountID,
			runtimemodel.ConversationCallerAgent,
			agentID,
			runtimemodel.ConversationTypeChat,
			runtimemodel.ConversationSourceWebApp,
			webAppID,
			20,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	results, total, err := repo.ListByCallerSourceScoped(
		context.Background(),
		organizationID,
		accountID,
		runtimemodel.ConversationCallerAgent,
		&agentID,
		runtimemodel.ConversationTypeChat,
		runtimemodel.ConversationSourceWebApp,
		&webAppID,
		20,
		0,
	)
	if err != nil {
		t.Fatalf("ListByCallerSourceScoped: %v", err)
	}
	if total != 0 || len(results) != 0 {
		t.Fatalf("results = %d total = %d, want empty", len(results), total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpdateAfterMessagePromotesLeafWhenCurrentLeafIsParent(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()
	parentID := uuid.New()

	messageRows := sqlmock.NewRows([]string{"id", "parent_id"}).AddRow(messageID, parentID)
	mock.ExpectQuery(`(?s)SELECT .* FROM "chat_runtime_messages" .*id = .*deleted_at IS NULL.*LIMIT`).
		WillReturnRows(messageRows)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".* WHERE .*id = .*deleted_at IS NULL.*current_leaf_message_id = .* OR current_leaf_message_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.UpdateAfterMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("UpdateAfterMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestUpdateAfterMessageDoesNotPromoteLeafAfterBranchSwitch(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewConversationRepository(db)
	conversationID := uuid.New()
	messageID := uuid.New()
	parentID := uuid.New()

	messageRows := sqlmock.NewRows([]string{"id", "parent_id"}).AddRow(messageID, parentID)
	mock.ExpectQuery(`(?s)SELECT .* FROM "chat_runtime_messages" .*id = .*deleted_at IS NULL.*LIMIT`).
		WillReturnRows(messageRows)
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"current_leaf_message_id".* WHERE .*id = .*deleted_at IS NULL.*current_leaf_message_id = .* OR current_leaf_message_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"active_message_id".* WHERE id = .* AND active_message_id = .* AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.UpdateAfterMessage(context.Background(), conversationID, messageID); err != nil {
		t.Fatalf("UpdateAfterMessage: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func newConversationRepositoryMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("open gorm mock db: %v", err)
	}
	return db, mock
}

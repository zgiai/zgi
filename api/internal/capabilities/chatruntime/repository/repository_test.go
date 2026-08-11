package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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

func TestListBranchPageReadsBeyondOneHundredWithoutSemanticTruncation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:context-branch-page?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	conversationID := uuid.New()
	ids := make([]uuid.UUID, 205)
	var parentID *uuid.UUID
	for index := range ids {
		ids[index] = uuid.New()
		message := &runtimemodel.Message{ID: ids[index], ConversationID: conversationID, ParentID: parentID, Query: "query", Answer: "answer", Status: runtimemodel.MessageStatusCompleted, ModelName: "test-model", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
		if err := db.Create(message).Error; err != nil {
			t.Fatal(err)
		}
		id := ids[index]
		parentID = &id
	}

	repo := NewMessageRepository(db)
	leaf := ids[len(ids)-1]
	total := 0
	pages := 0
	for {
		page, err := repo.ListBranchPage(context.Background(), conversationID, leaf, nil, 100)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		total += len(page.Messages)
		if page.ReachedRoot {
			break
		}
		if page.NextLeafID == nil {
			t.Fatal("missing continuation")
		}
		leaf = *page.NextLeafID
	}
	if total != len(ids) || pages != 3 {
		t.Fatalf("read total=%d pages=%d, want total=%d pages=3", total, pages, len(ids))
	}

	boundary := ids[99]
	page, err := repo.ListBranchPage(context.Background(), conversationID, ids[104], &boundary, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !page.ReachedBoundary || len(page.Messages) != 5 {
		t.Fatalf("boundary page reached=%v messages=%d, want true/5", page.ReachedBoundary, len(page.Messages))
	}
}

func TestListBranchPageRejectsCrossConversationParentAndCycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:context-branch-invalid?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	repo := NewMessageRepository(db)
	conversationID := uuid.New()
	otherConversationID := uuid.New()
	foreignParentID := uuid.New()
	childID := uuid.New()
	messages := []*runtimemodel.Message{
		{ID: foreignParentID, ConversationID: otherConversationID, Query: "foreign", Status: runtimemodel.MessageStatusCompleted, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}},
		{ID: childID, ConversationID: conversationID, ParentID: &foreignParentID, Query: "child", Status: runtimemodel.MessageStatusCompleted, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}},
	}
	if err := db.Create(messages).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListBranchPage(context.Background(), conversationID, childID, nil, 100); err == nil {
		t.Fatal("expected cross-conversation parent error")
	}

	firstID := uuid.New()
	secondID := uuid.New()
	cycle := []*runtimemodel.Message{
		{ID: firstID, ConversationID: conversationID, ParentID: &secondID, Query: "first", Status: runtimemodel.MessageStatusCompleted, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}},
		{ID: secondID, ConversationID: conversationID, ParentID: &firstID, Query: "second", Status: runtimemodel.MessageStatusCompleted, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}},
	}
	if err := db.Create(cycle).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ListBranchPage(context.Background(), conversationID, firstID, nil, 100); err == nil {
		t.Fatal("expected cycle error")
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

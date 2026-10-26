package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSearchMessagesByKeywordsRanksMatchesInsideTheBoundary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	mine, other := uuid.New(), uuid.New()
	now := time.Now()
	add := func(conversation uuid.UUID, query, answer, status string, age time.Duration) uuid.UUID {
		t.Helper()
		message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation, Query: query, Answer: answer, Status: status, ModelName: "test-model",
			ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}, CreatedAt: now.Add(-age)}
		if err := db.Create(message).Error; err != nil {
			t.Fatal(err)
		}
		return message.ID
	}
	both := add(mine, "客户答谢会定在湖滨会所", "好的，湖滨会所", runtimemodel.MessageStatusCompleted, 3*time.Hour)
	one := add(mine, "答谢会邀请 30 位客户", "收到", runtimemodel.MessageStatusCompleted, time.Hour)
	add(mine, "答谢会的湖滨方案还在生成", "", "streaming", time.Minute)
	add(other, "另一个人的答谢会在湖滨会所", "x", runtimemodel.MessageStatusCompleted, time.Minute)
	current := add(mine, "上次答谢会在哪？", "", runtimemodel.MessageStatusCompleted, 0)

	boundary := db.Table("chat_runtime_messages").Select("conversation_id").Where("conversation_id = ?", mine)
	matches, err := SearchMessagesByKeywords(context.Background(), db, boundary, []string{" 答谢会 ", "湖滨", "答谢会", ""}, current, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 2 || matches[0].MessageID != both || matches[0].Score != 2 || matches[1].MessageID != one || matches[1].Score != 1 {
		t.Fatalf("matches = %+v", matches)
	}
	for _, match := range matches {
		if match.ConversationID != mine {
			t.Fatalf("a match left the boundary: %+v", match)
		}
	}
	// Wildcards in keywords are matched literally.
	if literal, _ := SearchMessagesByKeywords(context.Background(), db, boundary, []string{"%"}, uuid.Nil, 5); len(literal) != 0 {
		t.Fatalf("%% matched %d messages", len(literal))
	}
	if empty, _ := SearchMessagesByKeywords(context.Background(), db, boundary, []string{"  "}, uuid.Nil, 5); len(empty) != 0 {
		t.Fatal("blank keywords matched")
	}
}

func TestNormalizeSearchKeywordsBoundsTheRequest(t *testing.T) {
	got := NormalizeSearchKeywords([]string{"A", "a", " b ", "c", "d", "e", "f", strings.Repeat("长", 60)})
	if len(got) != 5 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("keywords = %q", got)
	}
}

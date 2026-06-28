package workflow

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
)

type fakeWorkflowConversationTitleGen struct {
	called   bool
	title    string
	source   string
	messages []titlegen.Message
}

func (f *fakeWorkflowConversationTitleGen) Generate(ctx context.Context, req titlegen.GenerateRequest) (*titlegen.GenerateResult, error) {
	f.called = true
	f.messages = append([]titlegen.Message(nil), req.Messages...)
	return &titlegen.GenerateResult{Title: f.title, Source: f.source}, nil
}

func TestIsDefaultWorkflowConversationName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "Conversation 2026-05-09 13:04:05", want: true},
		{name: " Conversation 2026-05-09 13:04:05 ", want: true},
		{name: "Conversation about refund", want: false},
		{name: "退款进度查询", want: false},
		{name: "", want: false},
	}

	for _, tt := range tests {
		if got := isDefaultWorkflowConversationName(tt.name); got != tt.want {
			t.Fatalf("isDefaultWorkflowConversationName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBuildWorkflowConversationTitleMessagesUsesFirstTurns(t *testing.T) {
	t.Parallel()

	messages := []*conversation.AgentMessage{
		{Query: " 第一问 ", Answer: " 第一答 "},
		{Query: "第二问", Answer: ""},
		nil,
		{Query: "", Answer: ""},
		{Query: "第三问", Answer: "第三答"},
		{Query: "第四问", Answer: "第四答"},
	}

	got := buildWorkflowConversationTitleMessages(messages)
	want := []titlegen.Message{
		{Role: "user", Content: "第一问"},
		{Role: "assistant", Content: "第一答"},
		{Role: "user", Content: "第二问"},
		{Role: "user", Content: "第三问"},
		{Role: "assistant", Content: "第三答"},
	}
	if len(got) != len(want) {
		t.Fatalf("message count = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("message[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestGenerateWebAppConversationTitleUpdatesOnlyDefaultName(t *testing.T) {
	ctx := context.Background()
	conv := &conversation.AgentConversation{
		ID:   uuid.New(),
		Name: "Conversation 2026-05-09 13:04:05",
	}
	gen := &fakeWorkflowConversationTitleGen{
		title:  "退款进度查询",
		source: titlegen.SourceModel,
	}
	conversationSvc := &fakeWorkflowTitleConversationService{conversation: conv}
	service := newWorkflowConversationTitleTestService(conversationSvc, &fakeWorkflowTitleMessageService{
		messages: []*conversation.AgentMessage{
			{Query: "怎么查询退款进度", Answer: "你可以在订单页面查看退款状态"},
			{Query: "多久到账", Answer: "一般 1 到 3 个工作日到账"},
			{Query: "需要人工处理吗", Answer: "通常不需要人工介入"},
			{Query: "能不能取消退款", Answer: "提交后通常不能取消"},
		},
	}, gen)

	agentID := uuid.New()
	accountID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	webAppID := uuid.New().String()
	conv.AgentID = agentID
	conv.FromAccountID = &accountID

	if err := service.generateWebAppConversationTitle(ctx, workflowConversationTitleParams{
		WorkspaceID:    workspaceID.String(),
		OrganizationID: organizationID.String(),
		AgentID:        agentID.String(),
		AccountID:      accountID,
		ConversationID: conv.ID,
		WebAppID:       webAppID,
	}); err != nil {
		t.Fatalf("generate title: %v", err)
	}
	if conv.Name != "退款进度查询" {
		t.Fatalf("conversation name = %q, want semantic title", conv.Name)
	}
	wantMessages := []titlegen.Message{
		{Role: "user", Content: "怎么查询退款进度"},
		{Role: "assistant", Content: "你可以在订单页面查看退款状态"},
		{Role: "user", Content: "多久到账"},
		{Role: "assistant", Content: "一般 1 到 3 个工作日到账"},
		{Role: "user", Content: "需要人工处理吗"},
		{Role: "assistant", Content: "通常不需要人工介入"},
	}
	if len(gen.messages) != len(wantMessages) {
		t.Fatalf("generated title message count = %d, want %d: %#v", len(gen.messages), len(wantMessages), gen.messages)
	}
	for i := range wantMessages {
		if gen.messages[i] != wantMessages[i] {
			t.Fatalf("generated title message[%d] = %#v, want %#v", i, gen.messages[i], wantMessages[i])
		}
	}

	semanticConversation := &conversation.AgentConversation{
		ID:   uuid.New(),
		Name: "已有标题",
	}
	conversationSvc.conversation = semanticConversation
	semanticConversation.AgentID = agentID
	semanticConversation.FromAccountID = &accountID
	if err := service.generateWebAppConversationTitle(ctx, workflowConversationTitleParams{
		WorkspaceID:    workspaceID.String(),
		OrganizationID: organizationID.String(),
		AgentID:        agentID.String(),
		AccountID:      accountID,
		ConversationID: semanticConversation.ID,
		WebAppID:       webAppID,
	}); err != nil {
		t.Fatalf("generate title for semantic conversation: %v", err)
	}
	if semanticConversation.Name != "已有标题" {
		t.Fatalf("semantic conversation name = %q, want unchanged", semanticConversation.Name)
	}
}

func TestGenerateWebAppConversationTitleDoesNotStoreFallback(t *testing.T) {
	ctx := context.Background()
	conv := &conversation.AgentConversation{
		ID:   uuid.New(),
		Name: "Conversation 2026-05-09 13:04:05",
	}
	gen := &fakeWorkflowConversationTitleGen{
		title:  "Conversation 2026-05-09 13:04:05",
		source: titlegen.SourceFallback,
	}
	service := newWorkflowConversationTitleTestService(&fakeWorkflowTitleConversationService{conversation: conv}, &fakeWorkflowTitleMessageService{
		messages: []*conversation.AgentMessage{
			{Query: "怎么查询退款进度", Answer: "你可以在订单页面查看退款状态"},
		},
	}, gen)

	agentID := uuid.New()
	accountID := uuid.New()
	organizationID := uuid.New()
	workspaceID := uuid.New()
	webAppID := uuid.New().String()
	conv.AgentID = agentID
	conv.FromAccountID = &accountID

	err := service.generateWebAppConversationTitle(ctx, workflowConversationTitleParams{
		WorkspaceID:    workspaceID.String(),
		OrganizationID: organizationID.String(),
		AgentID:        agentID.String(),
		AccountID:      accountID,
		ConversationID: conv.ID,
		WebAppID:       webAppID,
	})
	if err == nil {
		t.Fatalf("generate title should fail when generator returns fallback")
	}
	if conv.Name != "Conversation 2026-05-09 13:04:05" {
		t.Fatalf("conversation name = %q, want original fallback title", conv.Name)
	}
}

func TestGenerateWebAppConversationTitleRejectsForeignAccountBeforeMessages(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conv := &conversation.AgentConversation{
		ID:            uuid.New(),
		AgentID:       agentID,
		Name:          "Conversation 2026-05-09 13:04:05",
		FromAccountID: &ownerID,
	}
	conversationSvc := &fakeWorkflowTitleConversationService{conversation: conv}
	messageSvc := &fakeWorkflowTitleMessageService{
		messages: []*conversation.AgentMessage{
			{Query: "secret", Answer: "hidden"},
		},
	}
	gen := &fakeWorkflowConversationTitleGen{
		title:  "should not be used",
		source: titlegen.SourceModel,
	}
	service := newWorkflowConversationTitleTestService(conversationSvc, messageSvc, gen)

	err := service.generateWebAppConversationTitle(ctx, workflowConversationTitleParams{
		WorkspaceID:    uuid.NewString(),
		OrganizationID: uuid.NewString(),
		AgentID:        agentID.String(),
		AccountID:      callerID,
		ConversationID: conv.ID,
		WebAppID:       uuid.NewString(),
	})

	if err == nil {
		t.Fatalf("generate title error = nil, want foreign-account denial")
	}
	if !conversationSvc.scopedCalled {
		t.Fatalf("scoped conversation lookup was not called")
	}
	if messageSvc.called {
		t.Fatalf("message lookup was called for a foreign-account conversation")
	}
	if gen.called {
		t.Fatalf("title generator was called for a foreign-account conversation")
	}
	if conv.Name != "Conversation 2026-05-09 13:04:05" {
		t.Fatalf("conversation name = %q, want unchanged", conv.Name)
	}
}

func TestGenerateWebAppConversationTitleStoresSemanticFallback(t *testing.T) {
	ctx := context.Background()
	agentID := uuid.New()
	accountID := uuid.New()
	conv := &conversation.AgentConversation{
		ID:            uuid.New(),
		AgentID:       agentID,
		Name:          "Conversation 2026-05-09 13:04:05",
		FromAccountID: &accountID,
	}
	gen := &fakeWorkflowConversationTitleGen{
		title:  "生产一个胖胖的猫咪",
		source: titlegen.SourceFallback,
	}
	service := newWorkflowConversationTitleTestService(&fakeWorkflowTitleConversationService{conversation: conv}, &fakeWorkflowTitleMessageService{
		messages: []*conversation.AgentMessage{
			{Query: "生产一个胖胖的猫咪", Answer: "![cat.png](http://example.test/cat.png)"},
		},
	}, gen)

	organizationID := uuid.New()
	workspaceID := uuid.New()
	webAppID := uuid.New().String()

	if err := service.generateWebAppConversationTitle(ctx, workflowConversationTitleParams{
		WorkspaceID:    workspaceID.String(),
		OrganizationID: organizationID.String(),
		AgentID:        agentID.String(),
		AccountID:      accountID,
		ConversationID: conv.ID,
		WebAppID:       webAppID,
	}); err != nil {
		t.Fatalf("generate title: %v", err)
	}
	if conv.Name != "生产一个胖胖的猫咪" {
		t.Fatalf("conversation name = %q, want semantic fallback title", conv.Name)
	}
}

func newWorkflowConversationTitleTestService(conversationSvc conversation.AgentConversationService, messageSvc conversation.AgentMessageService, gen titlegen.Service) *WorkflowService {
	handler := &AdvancedChatWorkflowHandler{
		conversationService: conversationSvc,
		messageService:      messageSvc,
	}

	return &WorkflowService{
		advancedChatHandler:  handler,
		conversationTitleGen: gen,
	}
}

type fakeWorkflowTitleConversationService struct {
	conversation.AgentConversationService

	conversation  *conversation.AgentConversation
	scopedCalled  bool
	scopedAgentID uuid.UUID
}

func (s *fakeWorkflowTitleConversationService) GetConversation(ctx context.Context, id uuid.UUID) (*conversation.AgentConversation, error) {
	return s.conversation, nil
}

func (s *fakeWorkflowTitleConversationService) GetConversationByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*conversation.AgentConversation, error) {
	s.scopedCalled = true
	s.scopedAgentID = agentID
	if s.conversation == nil || s.conversation.ID != id || s.conversation.AgentID != agentID {
		return nil, errWebAppConversationNotFound
	}
	return s.conversation, nil
}

func (s *fakeWorkflowTitleConversationService) UpdateConversationNameIfCurrent(ctx context.Context, conversationID uuid.UUID, currentName, nextName string) (bool, error) {
	if s.conversation == nil || s.conversation.ID != conversationID || s.conversation.Name != currentName {
		return false, nil
	}
	s.conversation.Name = nextName
	return true, nil
}

type fakeWorkflowTitleMessageService struct {
	conversation.AgentMessageService

	messages []*conversation.AgentMessage
	called   bool
}

func (s *fakeWorkflowTitleMessageService) GetConversationMessages(ctx context.Context, conversationID uuid.UUID) ([]*conversation.AgentMessage, error) {
	s.called = true
	return s.messages, nil
}

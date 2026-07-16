package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	"gorm.io/gorm"
)

func TestAIChatCreateConversationAllowsOrganizationMemberWithoutWorkspace(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationRepo := &scopedAIChatConversationRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Access:       fakeAccessRepository{},
		},
	}

	conversation, err := svc.CreateConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, "Product chat")

	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	if conversation.OrganizationID != organizationID {
		t.Fatalf("organization id = %s, want %s", conversation.OrganizationID, organizationID)
	}
	if conversation.AccountID != accountID {
		t.Fatalf("account id = %s, want %s", conversation.AccountID, accountID)
	}
	if conversation.WorkspaceID != nil {
		t.Fatalf("workspace id = %v, want nil for organization-mode product chat", conversation.WorkspaceID)
	}
	if conversationRepo.created == nil || conversationRepo.created.ID == uuid.Nil {
		t.Fatalf("conversation was not persisted with an id: %#v", conversationRepo.created)
	}
}

func TestAIChatConversationHistoryUsesOrganizationAndAccountScope(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	visibleConversationID := uuid.New()
	conversationRepo := &scopedAIChatConversationRepository{
		conversations: []*aichatmodel.Conversation{
			{
				ID:             visibleConversationID,
				OrganizationID: organizationID,
				AccountID:      accountID,
				Title:          "visible",
			},
			{
				ID:             uuid.New(),
				OrganizationID: organizationID,
				AccountID:      uuid.New(),
				Title:          "other account",
			},
			{
				ID:             uuid.New(),
				OrganizationID: uuid.New(),
				AccountID:      accountID,
				Title:          "other organization",
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Access:       fakeAccessRepository{},
		},
	}

	conversations, total, err := svc.ListConversations(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, 1, 20)

	if err != nil {
		t.Fatalf("ListConversations returned error: %v", err)
	}
	if total != 1 || len(conversations) != 1 {
		t.Fatalf("conversation count = %d/%d, want 1/1", len(conversations), total)
	}
	if conversations[0].ID != visibleConversationID {
		t.Fatalf("conversation id = %s, want %s", conversations[0].ID, visibleConversationID)
	}
	if conversationRepo.lastListOrganizationID != organizationID {
		t.Fatalf("list organization id = %s, want %s", conversationRepo.lastListOrganizationID, organizationID)
	}
	if conversationRepo.lastListAccountID != accountID {
		t.Fatalf("list account id = %s, want %s", conversationRepo.lastListAccountID, accountID)
	}
}

func TestAIChatGetConversationRejectsOtherAccountConversation(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	conversationRepo := &scopedAIChatConversationRepository{
		conversations: []*aichatmodel.Conversation{
			{
				ID:             conversationID,
				OrganizationID: organizationID,
				AccountID:      uuid.New(),
				Title:          "other account",
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Access:       fakeAccessRepository{},
		},
	}

	_, err := svc.GetConversation(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
	if conversationRepo.lastGetOrganizationID != organizationID {
		t.Fatalf("get organization id = %s, want %s", conversationRepo.lastGetOrganizationID, organizationID)
	}
	if conversationRepo.lastGetAccountID != accountID {
		t.Fatalf("get account id = %s, want %s", conversationRepo.lastGetAccountID, accountID)
	}
}

func TestAIChatListMessagesRejectsOtherAccountConversation(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageRepo := &scopedAIChatMessageRepository{
		conversationOwners: map[uuid.UUID]aichatConversationOwner{
			conversationID: {
				organizationID: organizationID,
				accountID:      uuid.New(),
			},
		},
		messages: []*aichatmodel.Message{
			{ID: uuid.New(), ConversationID: conversationID, Query: "secret"},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message: messageRepo,
			Access:  fakeAccessRepository{},
		},
	}

	messages, total, err := svc.ListMessages(context.Background(), Scope{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}, conversationID, 1, 20)

	if err != nil {
		t.Fatalf("ListMessages returned error: %v", err)
	}
	if total != 0 || len(messages) != 0 {
		t.Fatalf("message count = %d/%d, want 0/0 for another account conversation", len(messages), total)
	}
	if messageRepo.lastListOrganizationID != organizationID {
		t.Fatalf("message organization id = %s, want %s", messageRepo.lastListOrganizationID, organizationID)
	}
	if messageRepo.lastListAccountID != accountID {
		t.Fatalf("message account id = %s, want %s", messageRepo.lastListAccountID, accountID)
	}
}

func TestAIChatMessageActionsRejectOtherAccountMessage(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	callerAccountID := uuid.New()
	ownerAccountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	messageRepo := &scopedAIChatMessageRepository{
		conversationOwners: map[uuid.UUID]aichatConversationOwner{
			conversationID: {
				organizationID: organizationID,
				accountID:      ownerAccountID,
			},
		},
		messages: []*aichatmodel.Message{
			{
				ID:             messageID,
				ConversationID: conversationID,
				Query:          "secret",
				Answer:         "answer",
				Status:         aichatmodel.MessageStatusCompleted,
				ModelName:      "test-model",
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message: messageRepo,
			Access:  fakeAccessRepository{},
		},
	}

	scope := Scope{
		OrganizationID: organizationID,
		AccountID:      callerAccountID,
	}
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "delete",
			run: func() error {
				return svc.DeleteMessage(ctx, scope, messageID)
			},
		},
		{
			name: "stop",
			run: func() error {
				_, err := svc.StopMessage(ctx, scope, messageID)
				return err
			},
		},
		{
			name: "regenerate",
			run: func() error {
				_, err := svc.PrepareRootRegeneration(ctx, scope, messageID, aichatdto.RegenerateMessageRequest{})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
			if messageRepo.lastGetOrganizationID != organizationID {
				t.Fatalf("message get organization id = %s, want %s", messageRepo.lastGetOrganizationID, organizationID)
			}
			if messageRepo.lastGetAccountID != callerAccountID {
				t.Fatalf("message get account id = %s, want %s", messageRepo.lastGetAccountID, callerAccountID)
			}
		})
	}
}

type scopedAIChatConversationRepository struct {
	recordingConversationRepository

	created                *aichatmodel.Conversation
	conversations          []*aichatmodel.Conversation
	lastListOrganizationID uuid.UUID
	lastListAccountID      uuid.UUID
	lastGetOrganizationID  uuid.UUID
	lastGetAccountID       uuid.UUID
}

func (r *scopedAIChatConversationRepository) Create(_ context.Context, conversation *aichatmodel.Conversation) error {
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	copy := *conversation
	r.created = &copy
	return nil
}

func (r *scopedAIChatConversationRepository) ListScoped(_ context.Context, organizationID, accountID uuid.UUID, _ int, _ int) ([]*aichatmodel.Conversation, int64, error) {
	r.lastListOrganizationID = organizationID
	r.lastListAccountID = accountID

	items := make([]*aichatmodel.Conversation, 0, len(r.conversations))
	for _, conversation := range r.conversations {
		if conversation.OrganizationID == organizationID && conversation.AccountID == accountID {
			items = append(items, conversation)
		}
	}
	return items, int64(len(items)), nil
}

func (r *scopedAIChatConversationRepository) GetScoped(_ context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Conversation, error) {
	r.lastGetOrganizationID = organizationID
	r.lastGetAccountID = accountID

	for _, conversation := range r.conversations {
		if conversation.ID == id && conversation.OrganizationID == organizationID && conversation.AccountID == accountID {
			return conversation, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

type aichatConversationOwner struct {
	organizationID uuid.UUID
	accountID      uuid.UUID
}

type scopedAIChatMessageRepository struct {
	recordingMessageRepository

	conversationOwners     map[uuid.UUID]aichatConversationOwner
	messages               []*aichatmodel.Message
	lastListOrganizationID uuid.UUID
	lastListAccountID      uuid.UUID
	lastGetOrganizationID  uuid.UUID
	lastGetAccountID       uuid.UUID
}

func (r *scopedAIChatMessageRepository) GetScoped(_ context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Message, error) {
	r.lastGetOrganizationID = organizationID
	r.lastGetAccountID = accountID

	for _, message := range r.messages {
		if message.ID != id {
			continue
		}
		owner, ok := r.conversationOwners[message.ConversationID]
		if ok && owner.organizationID == organizationID && owner.accountID == accountID {
			return message, nil
		}
		break
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *scopedAIChatMessageRepository) ListByConversationScoped(_ context.Context, conversationID, organizationID, accountID uuid.UUID, _ int, _ int) ([]*aichatmodel.Message, int64, error) {
	r.lastListOrganizationID = organizationID
	r.lastListAccountID = accountID

	owner, ok := r.conversationOwners[conversationID]
	if !ok || owner.organizationID != organizationID || owner.accountID != accountID {
		return []*aichatmodel.Message{}, 0, nil
	}

	items := make([]*aichatmodel.Message, 0, len(r.messages))
	for _, message := range r.messages {
		if message.ConversationID == conversationID {
			items = append(items, message)
		}
	}
	return items, int64(len(items)), nil
}

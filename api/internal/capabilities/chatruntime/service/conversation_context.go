package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

const contextBranchPageSize = 100

// loadedContextState contains the database-backed bootstrap history used to
// construct a new Agent run.
type loadedContextState struct {
	RawMessages []*runtimemodel.Message
}

func (s *service) loadContextState(ctx context.Context, scope Scope, conversationID uuid.UUID, parentID *uuid.UUID) (*loadedContextState, error) {
	state := &loadedContextState{RawMessages: []*runtimemodel.Message{}}
	if parentID == nil || *parentID == uuid.Nil {
		return state, nil
	}
	parent, err := s.repos.Message.GetScoped(ctx, *parentID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if parent.ConversationID != conversationID {
		return nil, fmt.Errorf("context parent belongs to another conversation")
	}
	raw, err := s.loadBranchMessages(ctx, conversationID, *parentID, nil)
	if err != nil {
		return nil, err
	}
	state.RawMessages = raw
	return state, nil
}

func (s *service) loadBranchMessages(ctx context.Context, conversationID, leafID uuid.UUID, stopExclusive *uuid.UUID) ([]*runtimemodel.Message, error) {
	if leafID == uuid.Nil {
		return []*runtimemodel.Message{}, nil
	}
	leafToOldest := make([]*runtimemodel.Message, 0, contextBranchPageSize)
	seen := map[uuid.UUID]struct{}{}
	next := leafID
	for {
		page, err := s.repos.Message.ListBranchPage(ctx, conversationID, next, stopExclusive, contextBranchPageSize)
		if err != nil {
			return nil, err
		}
		for _, message := range page.Messages {
			if message == nil {
				continue
			}
			if _, ok := seen[message.ID]; ok {
				return nil, fmt.Errorf("cycle detected across context branch pages")
			}
			seen[message.ID] = struct{}{}
			leafToOldest = append(leafToOldest, message)
		}
		if page.ReachedBoundary || page.ReachedRoot {
			if stopExclusive != nil && *stopExclusive != uuid.Nil && !page.ReachedBoundary {
				return nil, fmt.Errorf("context branch boundary is not reachable")
			}
			break
		}
		if page.NextLeafID == nil || *page.NextLeafID == uuid.Nil || len(page.Messages) == 0 {
			return nil, fmt.Errorf("context branch pagination ended before root or boundary")
		}
		next = *page.NextLeafID
	}
	for left, right := 0, len(leafToOldest)-1; left < right; left, right = left+1, right-1 {
		leafToOldest[left], leafToOldest[right] = leafToOldest[right], leafToOldest[left]
	}
	return leafToOldest, nil
}

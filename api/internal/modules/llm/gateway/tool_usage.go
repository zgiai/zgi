package gateway

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"log"
	"time"
)

func (s *llmGatewayServiceImpl) privateToolObserver(bc *BillingContext, selection *ProviderSelection, request []byte) *toolusage.Observer {
	if s.db == nil || bc == nil || selection == nil || selection.UseSystemProvider || bc.UseSystemProvider {
		return nil
	}
	lane, err := usageBillingLaneFromContext(selection, bc)
	if err != nil || lane != UsageBillingLanePrivate {
		return nil
	}
	org, err := uuid.Parse(bc.OrganizationID)
	if err != nil {
		return nil
	}
	channel := getChannelID(selection)
	if channel == nil {
		return nil
	}
	return toolusage.NewObserver(toolusage.Identity{OrganizationID: org, RequestID: bc.RequestID, AttemptID: bc.AttemptID, Lane: toolusage.LanePrivate, ModelName: bc.ModelName, ModelID: &bc.ModelID, Provider: bc.ProviderName, ChannelID: *channel, UpstreamScope: bc.CredentialID.String()}, request)
}
func (s *llmGatewayServiceImpl) recordPrivateToolObserver(ctx context.Context, o *toolusage.Observer, failed bool, contexts ...*BillingContext) {
	if o == nil {
		return
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	events, err := o.Events(true, failed)
	if err != nil {
		log.Printf("[Gateway] tool usage projection failed: %v", err)
		return
	}
	for i := range events {
		stored, err := toolusage.NewRepository(s.db).Record(recordCtx, &events[i])
		if err != nil {
			log.Printf("[Gateway] tool usage persistence failed: %v", err)
			continue
		}
		if events[i].Tool == toolusage.ToolCodeInterpreter && stored.AttemptID != events[i].AttemptID && stored.Status == toolusage.StatusSucceeded {
			reuse := events[i]
			reuse.ID = uuid.Nil
			reuse.DedupeKey = "reuse:" + events[i].AttemptID + ":" + stored.EventID
			reuse.EventID = reuse.DedupeKey
			reuse.Quantity, reuse.Status = 0, toolusage.StatusFailed
			_ = reuse.SetDetails(map[string]any{"free_reason": "session_reused", "related_event_id": stored.EventID})
			if _, err := toolusage.NewRepository(s.db).Record(recordCtx, &reuse); err != nil {
				log.Printf("[Gateway] tool session reuse audit failed: %v", err)
			}
		}
		if len(contexts) > 0 && contexts[0] != nil && contexts[0].ToolSettlement != nil && stored.AttemptID != contexts[0].AttemptID && stored.Tool == toolusage.ToolCodeInterpreter {
			state := contexts[0].ToolSettlement
			if state.Reused == nil {
				state.Reused = map[string]string{}
			}
			state.Reused[stored.Tool] = stored.EventID
		}
	}
}

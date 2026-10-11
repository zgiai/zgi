package skillloop

import (
	"context"
	"errors"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type planningStreamContextClient struct {
	*runnerTestLLMClient
	openedContext chan context.Context
	openError     error
}

func (c *planningStreamContextClient) AppChatStream(ctx context.Context, _ *llmclient.AppContext, _ *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	c.openedContext <- ctx
	if c.openError != nil {
		return nil, c.openError
	}
	stream := make(chan adapter.StreamResponse, 1)
	stream <- adapter.StreamResponse{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "finished"}, FinishReason: "stop"}}, Done: true}
	close(stream)
	return stream, nil
}

func TestOpenSkillPlanningStreamTransfersCancelOwnership(t *testing.T) {
	for _, openError := range []error{nil, errors.New("fixture open failure")} {
		client := &planningStreamContextClient{openedContext: make(chan context.Context, 1), openError: openError}
		runner := &Runner{LLMClient: client}
		prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})
		stream, stop, _, err := runner.openSkillPlanningStream(t.Context(), prepared, prepared.LLMRequest, nil, true)
		if stop == nil {
			t.Fatal("stream owner did not receive a cancel function")
		}
		t.Cleanup(stop)
		openedContext := <-client.openedContext
		if openError == nil {
			if err != nil || stream == nil || openedContext.Err() != nil {
				t.Fatalf("successful stream canceled before consumption: error=%v context=%v", err, openedContext.Err())
			}
		} else if !errors.Is(err, openError) || !errors.Is(openedContext.Err(), context.Canceled) {
			t.Fatalf("failed stream context not canceled: error=%v context=%v", err, openedContext.Err())
		}
		stop()
		if !errors.Is(openedContext.Err(), context.Canceled) {
			t.Fatal("owner cancellation did not stop the stream context")
		}
	}
}

func TestSkillPlanningRoundCancelsCompletedStream(t *testing.T) {
	client := &planningStreamContextClient{openedContext: make(chan context.Context, 1)}
	runner := &Runner{LLMClient: client}
	prepared := NewPreparedChat("conv-1", "msg-1", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})
	_, ok, err := runner.runSkillPlanningStream(t.Context(), prepared, prepared.LLMRequest, 0, nil, false, false, false)
	if err != nil || !ok {
		t.Fatalf("planning round failed: ok=%v error=%v", ok, err)
	}
	if openedContext := <-client.openedContext; !errors.Is(openedContext.Err(), context.Canceled) {
		t.Fatal("completed planning round retained its model stream context")
	}
}

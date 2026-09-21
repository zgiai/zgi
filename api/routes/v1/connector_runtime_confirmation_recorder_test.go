package v1

import (
	"context"
	"testing"

	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
)

func TestConnectorRuntimeConfirmationSurfaceRecorderDispatchesFrozenOrigin(t *testing.T) {
	tests := []struct {
		name          string
		surface       connectorcore.TargetType
		wantChatCalls int
		wantFlowCalls int
	}{
		{name: "chat", surface: connectorcore.TargetTypeChat, wantChatCalls: 1},
		{name: "agent", surface: connectorcore.TargetTypeAgent, wantChatCalls: 1},
		{name: "digital human", surface: connectorcore.TargetTypeDigitalHuman, wantChatCalls: 1},
		{name: "workflow", surface: connectorcore.TargetTypeWorkflow, wantFlowCalls: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			chatRecorder := &connectorRuntimeConfirmationRecorderSpy{}
			workflowRecorder := &connectorRuntimeConfirmationRecorderSpy{}
			recorder := connectorRuntimeConfirmationSurfaceRecorder{
				chatAndAgent: chatRecorder,
				workflow:     workflowRecorder,
			}
			result := connectorconfirmation.Result{Origin: connectorexecution.ConfirmationOrigin{
				Surface: testCase.surface,
			}}
			if err := recorder.RecordConnectorConfirmationResolution(
				context.Background(), connectorconfirmation.Command{}, result,
			); err != nil {
				t.Fatalf("RecordConnectorConfirmationResolution() error = %v", err)
			}
			if chatRecorder.calls != testCase.wantChatCalls || workflowRecorder.calls != testCase.wantFlowCalls {
				t.Fatalf("chat calls = %d, workflow calls = %d", chatRecorder.calls, workflowRecorder.calls)
			}
		})
	}
}

func TestConnectorRuntimeConfirmationSurfaceRecorderFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		recorder connectorRuntimeConfirmationSurfaceRecorder
		surface  connectorcore.TargetType
	}{
		{
			name: "unknown surface",
			recorder: connectorRuntimeConfirmationSurfaceRecorder{
				chatAndAgent: &connectorRuntimeConfirmationRecorderSpy{},
				workflow:     &connectorRuntimeConfirmationRecorderSpy{},
			},
			surface: connectorcore.TargetType("forged"),
		},
		{
			name: "missing chat recorder",
			recorder: connectorRuntimeConfirmationSurfaceRecorder{
				workflow: &connectorRuntimeConfirmationRecorderSpy{},
			},
			surface: connectorcore.TargetTypeChat,
		},
		{
			name: "missing workflow recorder",
			recorder: connectorRuntimeConfirmationSurfaceRecorder{
				chatAndAgent: &connectorRuntimeConfirmationRecorderSpy{},
			},
			surface: connectorcore.TargetTypeWorkflow,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.recorder.RecordConnectorConfirmationResolution(
				context.Background(),
				connectorconfirmation.Command{},
				connectorconfirmation.Result{Origin: connectorexecution.ConfirmationOrigin{Surface: testCase.surface}},
			)
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInternal) {
				t.Fatalf("error = %v, want internal", err)
			}
		})
	}
}

type connectorRuntimeConfirmationRecorderSpy struct {
	calls int
	err   error
}

func (recorder *connectorRuntimeConfirmationRecorderSpy) RecordConnectorConfirmationResolution(
	_ context.Context,
	_ connectorconfirmation.Command,
	_ connectorconfirmation.Result,
) error {
	recorder.calls++
	return recorder.err
}

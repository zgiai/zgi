package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type projectedActionBindingCommitMessageRepo struct {
	repository.MessageRepository
	err      error
	updates  int
	metadata map[string]interface{}
}

func (r *projectedActionBindingCommitMessageRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updates++
	r.metadata = cloneProjectedActionBindingMetadata(metadata)
	return r.err
}

func TestCommitProjectedActionBindingPublishesMetadataOnlyAfterDurableWrite(t *testing.T) {
	trace := projectedActionBindingCommitTrace()
	initial := map[string]interface{}{
		"operation_plan": map[string]interface{}{
			"phases": []interface{}{map[string]interface{}{
				"id": "advisory", "status": "pending", "phase_source": "model_advisory",
			}},
		},
		"unrelated": map[string]interface{}{"value": "preserved"},
	}

	t.Run("failed write leaves working snapshot unchanged", func(t *testing.T) {
		writeErr := errors.New("database unavailable")
		repo := &projectedActionBindingCommitMessageRepo{err: writeErr}
		prepared := &PreparedChat{Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: cloneProjectedActionBindingMetadata(initial),
		}}
		before := cloneProjectedActionBindingMetadata(prepared.Message.Metadata)
		svc := &service{repos: &repository.Repositories{Message: repo}}

		err := svc.commitProjectedActionBinding(t.Context(), prepared, trace)
		if !errors.Is(err, writeErr) {
			t.Fatalf("commitProjectedActionBinding() error = %v, want wrapped repository error", err)
		}
		if repo.updates != 1 {
			t.Fatalf("UpdateMetadata calls = %d, want 1", repo.updates)
		}
		if !reflect.DeepEqual(prepared.Message.Metadata, before) {
			t.Fatalf("failed commit mutated working metadata:\n got: %#v\nwant: %#v", prepared.Message.Metadata, before)
		}
		if reflect.DeepEqual(repo.metadata, before) {
			t.Fatal("repository did not receive the generated binding metadata")
		}
	})

	t.Run("successful write publishes once and trace recorder does not rewrite", func(t *testing.T) {
		repo := &projectedActionBindingCommitMessageRepo{}
		prepared := &PreparedChat{Message: &runtimemodel.Message{
			ID:       uuid.New(),
			Metadata: cloneProjectedActionBindingMetadata(initial),
		}}
		svc := &service{repos: &repository.Repositories{Message: repo}}

		if err := svc.commitProjectedActionBinding(t.Context(), prepared, trace); err != nil {
			t.Fatalf("commitProjectedActionBinding() error = %v", err)
		}
		if !reflect.DeepEqual(prepared.Message.Metadata, repo.metadata) {
			t.Fatalf("published metadata differs from durable write:\n got: %#v\nwant: %#v", prepared.Message.Metadata, repo.metadata)
		}
		tracePhases := mapSliceFromAny(trace.Result["plan"])
		if got := stringFromAny(tracePhases[0]["status"]); got != "pending" {
			t.Fatalf("commit mutated caller trace status = %q, want pending", got)
		}
		plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
		phases := mapSliceFromAny(plan["phases"])
		if len(phases) != 1 || stringFromAny(phases[0]["id"]) != "phase-external" {
			t.Fatalf("committed phases = %#v, want generated server phase", phases)
		}

		recorder := newProcessTimelineRecorder(t.Context(), t.Context(), svc, prepared, nil)
		recorder.RecordTrace([]skills.SkillTrace{trace}, trace)
		if repo.updates != 1 {
			t.Fatalf("UpdateMetadata calls after RecordTrace = %d, want exactly one durable commit", repo.updates)
		}
	})
}

func projectedActionBindingCommitTrace() skills.SkillTrace {
	return skills.SkillTrace{
		Kind:     "plan_update",
		SkillID:  skills.SkillExternalApps,
		ToolName: skills.MetaToolUpdatePlan,
		Status:   "success",
		Arguments: map[string]interface{}{
			"source":  "server_projected_action_auto_binding",
			"round":   1,
			"call_id": "call-1",
		},
		Result: map[string]interface{}{
			"plan": []interface{}{map[string]interface{}{
				"id":                                   "phase-external",
				"status":                               "pending",
				"required":                             true,
				"phase_source":                         "server_outcome",
				"_server_projected_ledger_epoch":       "epoch-1",
				"_server_projected_connection_binding": "connection-1",
				"expected_action": map[string]interface{}{
					"skill_id":                              skills.SkillExternalApps,
					"tool_name":                             "execute_action",
					"_server_projected_tool_name":           "mail_search_messages",
					"_server_projected_binding_fingerprint": "binding-1",
					"target": map[string]interface{}{
						"integration_id": "mail",
						"action_id":      "mail.message.search",
					},
				},
			}},
		},
	}
}

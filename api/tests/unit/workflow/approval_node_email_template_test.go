package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	approvalruntime "github.com/zgiai/ginext/internal/modules/app/workflow/approval"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	approvalnode "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/approval"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/database"
)

func TestApprovalNodeRendersEmailTemplatesBeforePersistingDelivery(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	oldDB := database.GetDB()
	database.SetDB(db)
	defer database.SetDB(oldDB)

	variablePool := entities.NewVariablePool()
	variablePool.SystemVariables.WorkflowRunID = "run-" + uuid.NewString()
	variablePool.Add([]string{"start", "test"}, "review value")
	runtimeState := entities.NewGraphRuntimeState(variablePool)

	node, err := approvalnode.New(
		"approval-instance",
		map[string]any{
			"id": "approval-node",
			"data": map[string]any{
				"title": "Approval",
				"approval": map[string]any{
					"content": "Please review {{#start.test#}}",
					"fields":  []map[string]any{},
					"actions": []map[string]any{
						{"id": "approve", "label": "Approve"},
						{"id": "reject", "label": "Reject"},
					},
				},
				"submit_methods": map[string]any{
					"webapp": map[string]any{"enabled": false},
					"email": map[string]any{
						"enabled":    true,
						"subject":    "Review {{#start.test#}}",
						"body":       "Open review: {{#start.test#}} {{#url#}}",
						"recipients": []map[string]any{},
					},
				},
				"timeout": map[string]any{"duration": 1, "unit": "hour"},
			},
		},
		entities.GraphInitParams{
			TenantID:     uuid.NewString(),
			AppID:        uuid.NewString(),
			WorkflowID:   uuid.NewString(),
			WorkflowType: entities.WorkflowTypeWorkflow,
		},
		nil,
		runtimeState,
		nil,
	)
	if err != nil {
		t.Fatalf("create approval node: %v", err)
	}

	events := make(chan *shared.NodeEventCh, 2)
	if err := node.Run(ctx, events); err != nil {
		t.Fatalf("run approval node: %v", err)
	}

	var delivery approvalruntime.Delivery
	if err := db.First(&delivery, "delivery_method_type = ?", approvalruntime.DeliveryTypeEmail).Error; err != nil {
		t.Fatalf("load email delivery: %v", err)
	}
	var method approvalruntime.EmailSubmitMethod
	if err := json.Unmarshal([]byte(delivery.ChannelPayload), &method); err != nil {
		t.Fatalf("decode email delivery payload: %v", err)
	}
	if method.Subject != "Review review value" {
		t.Fatalf("email subject = %q, want rendered workflow variable", method.Subject)
	}
	wantBody := "Open review: review value {{#url#}}"
	if method.Body != wantBody {
		t.Fatalf("email body = %q, want %q", method.Body, wantBody)
	}
}

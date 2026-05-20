package createscheduledtask

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

func TestBuildCreateTaskRequest_CompilesFixedOnceSchedule(t *testing.T) {
	node := newTestNodeFromConfig(t, map[string]any{
		"id": "create-task-node-1",
		"data": map[string]any{
			"type":  "create-scheduled-task",
			"title": "Create Scheduled Task",
			"task": map[string]any{
				"name":        "Contract reminder",
				"description": "Follow up with the customer",
				"schedule": map[string]any{
					"type": "once",
					"once": map[string]any{
						"input_mode": "fixed",
						"run_at":     "2026-04-11T10:00:00+08:00",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"owner@example.com"},
							"subject":    "Reminder",
							"body":       "<p>Hello</p>",
						},
					},
				},
			},
		},
	}, nil)

	req, err := node.buildCreateTaskRequest()
	if err != nil {
		t.Fatalf("buildCreateTaskRequest returned error: %v", err)
	}

	if req.Name != "Contract reminder" {
		t.Fatalf("Name = %q, want %q", req.Name, "Contract reminder")
	}
	if req.ScheduleType != automationmodel.AutomationScheduleTypeOnce {
		t.Fatalf("ScheduleType = %q, want %q", req.ScheduleType, automationmodel.AutomationScheduleTypeOnce)
	}
	if req.Timezone != defaultWorkflowScheduleTimezone {
		t.Fatalf("Timezone = %q, want %q", req.Timezone, defaultWorkflowScheduleTimezone)
	}
	if got := req.ScheduleConfig["run_at"]; got != "2026-04-11T10:00:00+08:00" {
		t.Fatalf("run_at = %#v, want %q", got, "2026-04-11T10:00:00+08:00")
	}
	if len(req.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(req.Actions))
	}
	if got := req.Actions[0].Config["body_type"]; got != defaultEmailBodyType {
		t.Fatalf("body_type = %#v, want %q", got, defaultEmailBodyType)
	}
	if got := req.SourceSnapshot["schedule_type"]; got != automationmodel.AutomationScheduleTypeOnce {
		t.Fatalf("source_snapshot.schedule_type = %#v, want %q", got, automationmodel.AutomationScheduleTypeOnce)
	}
}

func TestBuildCreateTaskRequest_ResolvesVariableOnceRunAtRFC3339(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "run_at"}, "2026-04-12T09:30:00Z")

	node := newTestNodeFromConfig(t, map[string]any{
		"id": "create-task-node-2",
		"data": map[string]any{
			"type": "create-scheduled-task",
			"task": map[string]any{
				"name": "Variable reminder",
				"schedule": map[string]any{
					"type": "once",
					"once": map[string]any{
						"input_mode": "variable",
						"run_at":     "{{#start.run_at#}}",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"owner@example.com"},
							"subject":    "Reminder",
							"body":       "Check the contract",
						},
					},
				},
			},
		},
	}, vp)

	req, err := node.buildCreateTaskRequest()
	if err != nil {
		t.Fatalf("buildCreateTaskRequest returned error: %v", err)
	}

	if got := req.ScheduleConfig["run_at"]; got != "2026-04-12T09:30:00Z" {
		t.Fatalf("run_at = %#v, want %q", got, "2026-04-12T09:30:00Z")
	}
}

func TestBuildCreateTaskRequest_RejectsVariableOnceRunAtWhenNotRFC3339(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "run_at"}, "tomorrow 9am")

	node := newTestNodeFromConfig(t, map[string]any{
		"id": "create-task-node-3",
		"data": map[string]any{
			"type": "create-scheduled-task",
			"task": map[string]any{
				"name": "Invalid variable reminder",
				"schedule": map[string]any{
					"type": "once",
					"once": map[string]any{
						"input_mode": "variable",
						"run_at":     "{{#start.run_at#}}",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"owner@example.com"},
							"subject":    "Reminder",
							"body":       "Check the contract",
						},
					},
				},
			},
		},
	}, vp)

	_, err := node.buildCreateTaskRequest()
	if err == nil {
		t.Fatal("buildCreateTaskRequest returned nil error, want non-RFC3339 validation failure")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "run_at") || !strings.Contains(got, "RFC3339") {
		t.Fatalf("error = %q, want run_at RFC3339 validation context", got)
	}
}

func TestBuildCreateTaskRequest_CompilesCronSchedule(t *testing.T) {
	node := newTestNodeFromConfig(t, map[string]any{
		"id": "create-task-node-4",
		"data": map[string]any{
			"type": "create-scheduled-task",
			"task": map[string]any{
				"name": "Weekly reminder",
				"schedule": map[string]any{
					"type":     "cron",
					"timezone": "Asia/Tokyo",
					"cron": map[string]any{
						"expr": "0 9 * * 1",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"owner@example.com"},
							"subject":    "Reminder",
							"body":       "Check the contract",
						},
					},
				},
			},
		},
	}, nil)

	req, err := node.buildCreateTaskRequest()
	if err != nil {
		t.Fatalf("buildCreateTaskRequest returned error: %v", err)
	}

	if req.ScheduleType != automationmodel.AutomationScheduleTypeCron {
		t.Fatalf("ScheduleType = %q, want %q", req.ScheduleType, automationmodel.AutomationScheduleTypeCron)
	}
	if req.Timezone != "Asia/Tokyo" {
		t.Fatalf("Timezone = %q, want %q", req.Timezone, "Asia/Tokyo")
	}
	if got := req.ScheduleConfig["cron_expr"]; got != "0 9 * * 1" {
		t.Fatalf("cron_expr = %#v, want %q", got, "0 9 * * 1")
	}
}

func TestBuildCreateTaskRequest_SkipsDisabledActions(t *testing.T) {
	node := newTestNodeFromConfig(t, map[string]any{
		"id": "create-task-node-5",
		"data": map[string]any{
			"type": "create-scheduled-task",
			"task": map[string]any{
				"name": "Reminder with skipped action",
				"schedule": map[string]any{
					"type": "once",
					"once": map[string]any{
						"input_mode": "fixed",
						"run_at":     "2026-04-11T10:00:00+08:00",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"enabled":      false,
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"skip@example.com"},
							"subject":    "Skip",
							"body":       "Skip me",
						},
					},
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"send@example.com"},
							"subject":    "Send",
							"body":       "Send me",
						},
					},
				},
			},
		},
	}, nil)

	req, err := node.buildCreateTaskRequest()
	if err != nil {
		t.Fatalf("buildCreateTaskRequest returned error: %v", err)
	}

	if len(req.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(req.Actions))
	}
	if req.Actions[0].ActionOrder != 1 {
		t.Fatalf("ActionOrder = %d, want 1", req.Actions[0].ActionOrder)
	}
	to, ok := req.Actions[0].Config["to"].([]string)
	if !ok || len(to) != 1 || to[0] != "send@example.com" {
		t.Fatalf("to = %#v, want []string{\"send@example.com\"}", req.Actions[0].Config["to"])
	}
}

func TestParseNodeDataFromConfig_AllowsDisabledDraftActionWithoutRecipients(t *testing.T) {
	_, _, err := parseNodeDataFromConfig(map[string]any{
		"id": "create-task-node-6",
		"data": map[string]any{
			"type": "create-scheduled-task",
			"task": map[string]any{
				"name": "Reminder with disabled draft action",
				"schedule": map[string]any{
					"type": "once",
					"once": map[string]any{
						"input_mode": "fixed",
						"run_at":     "2026-04-11T10:00:00+08:00",
					},
				},
				"actions": []map[string]any{
					{
						"action_type":  "send_notification",
						"enabled":      false,
						"channel_type": "email",
						"notification": map[string]any{},
					},
					{
						"action_type":  "send_notification",
						"channel_type": "email",
						"notification": map[string]any{
							"recipients": []string{"send@example.com"},
							"subject":    "Send",
							"body":       "Send me",
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseNodeDataFromConfig returned error: %v", err)
	}
}

func newTestNodeFromConfig(t *testing.T, config map[string]any, vp *entities.VariablePool) *Node {
	t.Helper()

	nodeData, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		t.Fatalf("parseNodeDataFromConfig returned error: %v", err)
	}

	if vp == nil {
		vp = entities.NewVariablePool()
	}
	vp.Add([]string{"sys", "organization_id"}, "org-1")
	vp.Add([]string{"sys", "workspace_id"}, "workspace-1")
	vp.Add([]string{"sys", "workflow_run_id"}, "run-1")

	return &Node{
		NodeStruct: base.NodeStruct{
			NodeID:            nodeID,
			NodeType:          shared.CreateScheduledTask,
			TenantID:          "workspace-legacy",
			APPID:             "app-1",
			WorkflowID:        "workflow-1",
			UserID:            "user-1",
			InvokeFrom:        string(entities.InvokeFromWorkflow),
			GraphRuntimeState: entities.NewGraphRuntimeState(vp),
		},
		nodeData: nodeData,
	}
}

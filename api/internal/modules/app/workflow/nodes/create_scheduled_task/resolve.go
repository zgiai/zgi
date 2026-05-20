package createscheduledtask

import (
	"fmt"
	"strings"
	"time"
)

func (n *Node) resolveTemplateVariables(template string) string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || !strings.Contains(template, "{{#") {
		return template
	}

	result := template
	for {
		startIdx := strings.Index(result, "{{#")
		if startIdx == -1 {
			break
		}

		endIdx := strings.Index(result[startIdx:], "#}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx

		varPath := result[startIdx+3 : endIdx]
		selector := strings.Split(varPath, ".")

		replacement := ""
		if variable := n.GraphRuntimeState.VariablePool.GetWithPath(selector); variable != nil {
			replacement = fmt.Sprintf("%v", variable.ToObject())
		}

		result = result[:startIdx] + replacement + result[endIdx+3:]
	}

	return result
}

func (n *Node) resolveOnceRunAt(schedule OnceScheduleData) (string, error) {
	var candidate string

	switch schedule.InputMode {
	case OnceInputModeFixed:
		candidate = strings.TrimSpace(schedule.RunAt)
	case OnceInputModeVariable:
		candidate = strings.TrimSpace(n.resolveTemplateVariables(schedule.RunAt))
	default:
		return "", fmt.Errorf("task.schedule.once.input_mode %q is not supported", schedule.InputMode)
	}

	if candidate == "" {
		return "", fmt.Errorf("task.schedule.once.run_at is required")
	}

	runAt, err := time.Parse(time.RFC3339, candidate)
	if err != nil {
		if schedule.InputMode == OnceInputModeVariable {
			return "", fmt.Errorf("task.schedule.once.run_at must resolve to an RFC3339 string: %w", err)
		}
		return "", fmt.Errorf("task.schedule.once.run_at must be an RFC3339 string: %w", err)
	}

	return runAt.Format(time.RFC3339), nil
}

func (n *Node) lookupSystemString(key string) string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return ""
	}

	variable := n.GraphRuntimeState.VariablePool.Get([]string{"sys", key})
	if variable == nil {
		return ""
	}

	value, ok := variable.ToObject().(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

func (n *Node) resolveWorkspaceID() string {
	if workspaceID := n.lookupSystemString("workspace_id"); workspaceID != "" {
		return workspaceID
	}
	if tenantID := n.lookupSystemString("tenant_id"); tenantID != "" {
		return tenantID
	}
	return strings.TrimSpace(n.TenantID)
}

func (n *Node) resolveOrganizationID() string {
	return n.lookupSystemString("organization_id")
}

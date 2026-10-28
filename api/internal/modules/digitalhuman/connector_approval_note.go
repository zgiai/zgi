package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/gorm"
)

// connectorApprovalNote tells the model how each SSH connection asks before a
// call in this run (ADR 0026–0028), so it never claims a call needs approval
// when its owner removed that, nor the reverse. The platform still decides
// every call; this only describes it.
func connectorApprovalNote(ctx context.Context, db *gorm.DB, run *Run, event invocation.Event) (string, error) {
	if db == nil || run == nil {
		return "", nil
	}
	var rows []*ResourceBinding
	if err := db.WithContext(ctx).
		Where("organization_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND binding_type = ? AND metadata->>'approval_policy' <> ''",
			run.OrganizationID, run.DigitalHumanID, run.DigitalHumanVersionID, resourcebindings.BindingTypeConnector).
		Order("created_at").Find(&rows).Error; err != nil {
		return "", fmt.Errorf("list connector approval policies: %w", err)
	}
	requester := connectorRequester(event)
	interactive := connectorInteractive(event)
	lines := []string{}
	for _, row := range rows {
		var metadata struct {
			Policy string `json:"approval_policy"`
		}
		_ = json.Unmarshal(row.Metadata, &metadata)
		ownerPresent := row.AuthorizedBy != nil && *row.AuthorizedBy != uuid.Nil && requester == row.AuthorizedBy.String()
		if line := connectorApprovalLine(row.DisplayName, metadata.Policy, interactive, ownerPresent); line != "" {
			if interactive {
				line += " " + connectorScheduledApproval(metadata.Policy)
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	return "How your server connections ask before a call in this run:\n" + strings.Join(lines, "\n") +
		"\nForbidden commands are always refused. Running without approval is not permission to do more: change only what the task needs, verify each change, and report every change you made.", nil
}

func connectorApprovalLine(name, policy string, interactive, ownerPresent bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "SSH connection"
	}
	switch {
	case policy == ConnectorApprovalTrustedUnattended && !interactive,
		(policy == ConnectorApprovalTrusted || policy == ConnectorApprovalTrustedUnattended) && interactive && ownerPresent:
		return "- " + name + ": its owner fully trusts it here, so every call runs without approval."
	case policy == ConnectorApprovalReadOnlyAuto && interactive,
		(policy == ConnectorApprovalTrusted || policy == ConnectorApprovalTrustedUnattended) && interactive:
		return "- " + name + ": commands the platform proves read-only run without approval; every other call waits for the person's approval."
	case !interactive:
		return "- " + name + ": no one can approve here, so only checks the platform proves read-only can run, and only with the owner's task grant."
	}
	return ""
}

// connectorScheduledApproval says what the owner's scheduled task runs may do
// with the connection, so the model can answer for them too.
func connectorScheduledApproval(policy string) string {
	if policy == ConnectorApprovalTrustedUnattended {
		return "In the owner's scheduled and background task runs, every call also runs without approval."
	}
	return "In scheduled and background task runs, only checks the platform proves read-only can run, and only with the owner's task grant."
}

package container

import (
	"context"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	socialtools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
)

// Keep the journal alongside, rather than inside, the public file backend.
// Initialization is lazy: tool discovery and nil-DB composition tests must not
// create runtime directories or query database tables.
func (c *ServiceContainer) GetSocialOperationsService() (*socialops.Service, error) {
	c.socialOperationsOnce.Do(func() {
		if c.config == nil || c.db == nil {
			c.socialOperationsErr = backgroundDenied(nil)
			return
		}
		base := c.config.Storage.OpenDALBasePath
		if base == "" {
			base = "./storage/opendal"
		}
		root := filepath.Join(filepath.Dir(filepath.Clean(base)), "social-operations")
		c.socialOperations, c.socialOperationsErr = socialops.New(c.db, root, nil)
	})
	return c.socialOperations, c.socialOperationsErr
}
func (c *ServiceContainer) newSocialOperationsProvider() tools.ToolProvider {
	return socialtools.NewProvider(c.GetSocialOperationsService, &socialOperationsRuntime{container: c})
}

type socialOperationsRuntime struct{ container *ServiceContainer }

func (s *socialOperationsRuntime) restoreSocialOperationsRun(ctx context.Context, identity socialtools.InvocationIdentity) (socialops.Scope, ActiveBackgroundTaskRun, error) {
	if s == nil || s.container == nil || s.container.db == nil || identity.Runtime == nil || identity.Runtime.InvokeFrom != tools.ToolInvokeFromDigitalHuman {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	org, orgErr := uuid.Parse(identity.Runtime.TenantID)
	conversation, conversationErr := uuid.Parse(identity.ConversationID)
	employee, employeeErr := uuid.Parse(identity.EmployeeID)
	message, messageErr := uuid.Parse(identity.MessageID)
	if orgErr != nil || conversationErr != nil || employeeErr != nil || messageErr != nil || org == uuid.Nil || conversation == uuid.Nil || employee == uuid.Nil || message == uuid.Nil {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	// The version comes from the active server run, never from a skill input.
	var runs []digitalhuman.Run
	err := s.container.db.WithContext(ctx).Select("digital_human_version_id").Where("organization_id = ? AND conversation_id = ? AND message_id = ? AND digital_human_id = ? AND surface = ? AND status = ? AND cancel_requested_at IS NULL", org, conversation, message, employee, digitalhuman.SurfaceTask, digitalhuman.RunStatusRunning).Limit(2).Find(&runs).Error
	if err != nil {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundInternal("restore social operations run", err)
	}
	if len(runs) != 1 {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	restored, err := s.container.GetBackgroundTaskRunResolver().Restore(ctx, ActiveBackgroundTaskRunRequest{OrganizationID: org, ConversationID: conversation, MessageID: &message, EmployeeID: employee, VersionID: runs[0].DigitalHumanVersionID})
	if err != nil {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, err
	}
	if !restored.SocialOperations {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	// ChatRuntime uses the task creator for account billing. It remains distinct
	// from the environment owner; both identities must agree with the restored
	// task before a journal can be addressed.
	if identity.ActorID != restored.OwnerID.String() {
		return socialops.Scope{}, ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	scope := socialops.Scope{OrganizationID: org, WorkspaceID: restored.WorkspaceID, OwnerID: restored.OwnerID, EmployeeID: employee}
	return scope, restored, nil
}

func (s *socialOperationsRuntime) ResolveSocialOperations(ctx context.Context, identity socialtools.InvocationIdentity) (socialops.Scope, socialops.PublicationGateway, error) {
	scope, restored, err := s.restoreSocialOperationsRun(ctx, identity)
	if err != nil {
		return socialops.Scope{}, nil, err
	}
	store, err := s.container.GetSocialOperationsService()
	if err != nil {
		return socialops.Scope{}, nil, err
	}
	gateway, err := s.container.newSocialOperationsGateway(restored, store)
	if err != nil {
		return socialops.Scope{}, nil, err
	}
	return scope, gateway, nil
}

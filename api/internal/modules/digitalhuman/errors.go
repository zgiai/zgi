package digitalhuman

import "errors"

var (
	// ErrInvalidRequest indicates invalid or incomplete DigitalHuman input.
	ErrInvalidRequest = errors.New("invalid digital human request")
	// ErrNotFound indicates that a scoped DigitalHuman does not exist.
	ErrNotFound = errors.New("digital human not found")
	// ErrVersionNotFound indicates that a requested configuration version does not exist.
	ErrVersionNotFound = errors.New("digital human version not found")
	// ErrNameConflict indicates that a workspace already uses the requested name.
	ErrNameConflict = errors.New("digital human name conflict")
	// ErrRevisionConflict indicates an optimistic concurrency conflict.
	ErrRevisionConflict = errors.New("digital human revision conflict")
	// ErrPublishedImmutable indicates an attempted mutation of a published version.
	ErrPublishedImmutable = errors.New("published digital human version is immutable")
	// ErrPublishInvalid indicates that the current draft cannot be published.
	ErrPublishInvalid = errors.New("digital human draft is not publishable")
	// ErrDeleteBlocked indicates that durable references prevent deletion.
	ErrDeleteBlocked = errors.New("digital human deletion is blocked")
	// ErrRoleTemplateNotFound indicates that a scoped reusable role does not exist.
	ErrRoleTemplateNotFound = errors.New("digital human role template not found")
	// ErrRoleVersionNotFound indicates that a role version does not exist.
	ErrRoleVersionNotFound = errors.New("digital human role template version not found")
	// ErrRoleNameConflict indicates a duplicate reusable role name.
	ErrRoleNameConflict = errors.New("digital human role template name conflict")
	// ErrRolePublishInvalid indicates an incomplete reusable role draft.
	ErrRolePublishInvalid = errors.New("digital human role template draft is not publishable")
	// ErrResourceBindingInvalid indicates an unsafe or incomplete binding.
	ErrResourceBindingInvalid = errors.New("digital human resource binding is invalid")
	// ErrRunNotFound indicates that a scoped DigitalHuman run does not exist.
	ErrRunNotFound = errors.New("digital human run not found")
	// ErrRunConflict indicates an invalid durable run state transition.
	ErrRunConflict = errors.New("digital human run state conflict")
	// ErrConversationNotFound indicates that a caller-scoped conversation does not exist.
	ErrConversationNotFound = errors.New("digital human conversation not found")
	// ErrMemoryNotFound indicates that scoped DigitalHuman memory does not exist.
	ErrMemoryNotFound = errors.New("digital human memory not found")
	// ErrMemoryConflict indicates a stale revision, configuration, epoch, or operation.
	ErrMemoryConflict = errors.New("digital human memory conflict")
	// ErrMemoryPolicyRejected indicates that a proposal failed privacy or evidence policy.
	ErrMemoryPolicyRejected = errors.New("digital human memory proposal rejected")
	// ErrMemoryUndoUnavailable indicates an expired, consumed, or stale undo record.
	ErrMemoryUndoUnavailable = errors.New("digital human memory undo unavailable")
	// ErrChannelInstallationNotFound indicates a missing scoped channel installation.
	ErrChannelInstallationNotFound = errors.New("digital human channel installation not found")
	// ErrChannelBindingNotFound indicates a missing scoped channel binding.
	ErrChannelBindingNotFound = errors.New("digital human channel binding not found")
	// ErrChannelInstallationInUse indicates that live conversation bindings
	// still reference a workspace-shared channel installation.
	ErrChannelInstallationInUse = errors.New("digital human channel installation is in use")
	// ErrChannelConflict indicates a duplicate channel identity or stale revision.
	ErrChannelConflict = errors.New("digital human channel conflict")
	// ErrChannelNotReady indicates an invalid or incomplete provider connection.
	ErrChannelNotReady = errors.New("digital human channel is not ready")
	// ErrChannelDeliveryNotFound indicates a missing scoped delivery record.
	ErrChannelDeliveryNotFound = errors.New("digital human channel delivery not found")
	// ErrChannelDeliveryConflict indicates an unsafe delivery state transition.
	ErrChannelDeliveryConflict = errors.New("digital human channel delivery conflict")
)

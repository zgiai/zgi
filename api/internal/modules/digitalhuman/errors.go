package digitalhuman

import (
	"errors"
	"fmt"
)

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
	// ErrVersionRestoreImpactChanged indicates that a restore preview is stale or invalid.
	ErrVersionRestoreImpactChanged = errors.New("digital human version restore impact changed")
	// ErrDeleteBlocked indicates that durable references prevent deletion.
	ErrDeleteBlocked = errors.New("digital human deletion is blocked")
	// ErrPersonaTemplateNotFound indicates that a scoped editing preset does not exist.
	ErrPersonaTemplateNotFound = errors.New("digital human persona template not found")
	// ErrPersonaTemplateNameConflict indicates a duplicate workspace-scoped preset name.
	ErrPersonaTemplateNameConflict = errors.New("digital human persona template name conflict")
	// ErrResourceBindingInvalid indicates an unsafe or incomplete binding.
	ErrResourceBindingInvalid = errors.New("digital human resource binding is invalid")
	// ErrRunNotFound indicates that a scoped DigitalHuman run does not exist.
	ErrRunNotFound = errors.New("digital human run not found")
	// ErrRunConflict indicates an invalid durable run state transition.
	ErrRunConflict = errors.New("digital human run state conflict")
	// ErrConversationNotFound indicates that a caller-scoped conversation does not exist.
	ErrConversationNotFound = errors.New("digital human conversation not found")
	// ErrConsoleFileNotAccessible hides whether a requested console attachment
	// exists outside the current account and workspace boundary.
	ErrConsoleFileNotAccessible = errors.New("digital human console file not accessible")
	// ErrMemoryNotFound indicates that scoped DigitalHuman memory does not exist.
	ErrMemoryNotFound = errors.New("digital human memory not found")
	// ErrMemoryConflict indicates a stale revision, configuration, epoch, or operation.
	ErrMemoryConflict = errors.New("digital human memory conflict")
	// ErrMemoryPolicyRejected indicates that a proposal failed privacy or evidence policy.
	ErrMemoryPolicyRejected = errors.New("digital human memory proposal rejected")
	// ErrMemoryUndoUnavailable indicates an expired, consumed, or stale undo record.
	ErrMemoryUndoUnavailable = errors.New("digital human memory undo unavailable")
	// ErrMemoryProjectionUnavailable indicates that this deployment cannot rebuild
	// the hidden memory projection.
	ErrMemoryProjectionUnavailable = errors.New("digital human memory projection unsupported")
	// ErrMemoryProjectionRepairFailed indicates that a projection rebuild failed.
	ErrMemoryProjectionRepairFailed = errors.New("digital human memory projection repair failed")
	// ErrTaskNotFound indicates that a scoped DigitalHuman task does not exist.
	ErrTaskNotFound = errors.New("digital human task not found")
	// ErrTaskConflict indicates a stale revision or invalid task transition.
	ErrTaskConflict = errors.New("digital human task conflict")
	// ErrTaskPolicyRejected indicates that the invocation cannot manage tasks.
	ErrTaskPolicyRejected = errors.New("digital human task policy rejected")
	// ErrTaskVisibilityForbidden indicates that only a task center manager may
	// change who can see a task. It is a task policy rejection that is
	// reported as missing permission rather than as an unsupported surface.
	ErrTaskVisibilityForbidden = fmt.Errorf("%w: task visibility requires a task center manager", ErrTaskPolicyRejected)
	// ErrChannelInstallationNotFound indicates a missing scoped channel installation.
	ErrChannelInstallationNotFound = errors.New("digital human channel installation not found")
	// ErrChannelCredentialNotFound indicates that an installation does not own
	// an embedded credential and should be resolved through Connection Center.
	ErrChannelCredentialNotFound = errors.New("digital human channel credential not found")
	// ErrChannelBindingNotFound indicates a missing scoped channel binding.
	ErrChannelBindingNotFound = errors.New("digital human channel binding not found")
	// ErrChannelConflict indicates a duplicate channel identity or stale revision.
	ErrChannelConflict = errors.New("digital human channel conflict")
	// ErrChannelNotReady indicates an invalid or incomplete provider connection.
	ErrChannelNotReady = errors.New("digital human channel is not ready")
	// ErrChannelTransportUnsupported indicates that a Connector is not backed by
	// a registered receiver for the selected channel transport.
	ErrChannelTransportUnsupported = errors.New("digital human channel transport is unsupported")
	// ErrChannelCredentialManaged indicates that a connection-backed channel
	// must be rotated through Connector maintenance rather than embedded input.
	ErrChannelCredentialManaged = errors.New("digital human channel credential is managed by Connector")
	// ErrCredentialStorageUnavailable indicates that the deployment cannot
	// securely persist DigitalHuman-owned channel or direct Skill credentials.
	ErrCredentialStorageUnavailable = errors.New("digital human credential storage is unavailable")
	// ErrChannelDeliveryNotFound indicates a missing scoped delivery record.
	ErrChannelDeliveryNotFound = errors.New("digital human channel delivery not found")
	// ErrChannelDeliveryConflict indicates an unsafe delivery state transition.
	ErrChannelDeliveryConflict = errors.New("digital human channel delivery conflict")
)

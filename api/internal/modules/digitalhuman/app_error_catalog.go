package digitalhuman

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

var (
	// AppCodeRequestInvalid identifies invalid DigitalHuman input.
	AppCodeRequestInvalid = apperror.MustCode("digital_human.request.invalid")
	// AppCodeNotFound identifies a missing scoped DigitalHuman.
	AppCodeNotFound = apperror.MustCode("digital_human.definition.not_found")
	// AppCodeVersionNotFound identifies a missing DigitalHuman version.
	AppCodeVersionNotFound = apperror.MustCode("digital_human.version.not_found")
	// AppCodeNameConflict identifies a duplicate workspace-scoped name.
	AppCodeNameConflict = apperror.MustCode("digital_human.name.conflict")
	// AppCodeRevisionConflict identifies an optimistic concurrency conflict.
	AppCodeRevisionConflict = apperror.MustCode("digital_human.revision.conflict")
	// AppCodePublishedImmutable identifies a forbidden published-version mutation.
	AppCodePublishedImmutable = apperror.MustCode("digital_human.version.immutable")
	// AppCodePublishInvalid identifies a draft that is not ready for publication.
	AppCodePublishInvalid = apperror.MustCode("digital_human.publish.invalid")
	// AppCodeVersionRestoreImpactChanged identifies a stale version restore preview.
	AppCodeVersionRestoreImpactChanged = apperror.MustCode("digital_human.version.restore_impact_changed")
	// AppCodeDeleteBlocked identifies deletion prevented by durable references.
	AppCodeDeleteBlocked                = apperror.MustCode("digital_human.delete.blocked")
	AppCodePersonaTemplateNotFound      = apperror.MustCode("digital_human.persona_template.not_found")
	AppCodePersonaTemplateNameConflict  = apperror.MustCode("digital_human.persona_template.name.conflict")
	AppCodeResourceBindingInvalid       = apperror.MustCode("digital_human.resource_binding.invalid")
	AppCodeRunNotFound                  = apperror.MustCode("digital_human.run.not_found")
	AppCodeRunConflict                  = apperror.MustCode("digital_human.run.conflict")
	AppCodeEndUserSuspended             = apperror.MustCode("digital_human.end_user.suspended")
	AppCodeConversationNotFound         = apperror.MustCode("digital_human.conversation.not_found")
	AppCodeConsoleFileNotAccessible     = apperror.MustCode("digital_human.file.not_found")
	AppCodeWebAppFileRequiresLogin      = apperror.MustCode("digital_human.file.login_required")
	AppCodeMemoryNotFound               = apperror.MustCode("digital_human.memory.not_found")
	AppCodeMemoryConflict               = apperror.MustCode("digital_human.memory.conflict")
	AppCodeMemoryPolicyRejected         = apperror.MustCode("digital_human.memory.policy_rejected")
	AppCodeMemoryUndoUnavailable        = apperror.MustCode("digital_human.memory.undo_unavailable")
	AppCodeMemoryProjectionUnavailable  = apperror.MustCode("digital_human.memory.projection_unavailable")
	AppCodeMemoryRepairFailed           = apperror.MustCode("digital_human.memory.repair_failed")
	AppCodeTaskNotFound                 = apperror.MustCode("digital_human.task.not_found")
	AppCodeTaskInvalidRequest           = apperror.MustCode("digital_human.task.invalid_request")
	AppCodeTaskConflict                 = apperror.MustCode("digital_human.task.conflict")
	AppCodeTaskPolicyRejected           = apperror.MustCode("digital_human.task.policy_rejected")
	AppCodeChannelInstallationNotFound  = apperror.MustCode("digital_human.channel.installation.not_found")
	AppCodeChannelConflict              = apperror.MustCode("digital_human.channel.conflict")
	AppCodeChannelNotReady              = apperror.MustCode("digital_human.channel.not_ready")
	AppCodeChannelTransportUnsupported  = apperror.MustCode("digital_human.channel.transport.unsupported")
	AppCodeChannelCredentialManaged     = apperror.MustCode("digital_human.channel.credential.managed")
	AppCodeCredentialStorageUnavailable = apperror.MustCode("digital_human.credential_storage.unavailable")
	AppCodeChannelDeliveryNotFound      = apperror.MustCode("digital_human.channel.delivery.not_found")
	AppCodeChannelDeliveryConflict      = apperror.MustCode("digital_human.channel.delivery.conflict")
)

// CatalogDefinitions returns public application errors owned by DigitalHuman.
func CatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		digitalHumanDefinition(AppCodeRequestInvalid, appcatalog.CategoryValidation, 400,
			"The DigitalHuman request is invalid. Check the fields and try again.",
			"数字人请求内容不正确，请检查字段后重试。"),
		digitalHumanDefinition(AppCodeNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman was not found or is no longer available.",
			"未找到该数字人，它可能已被删除。"),
		digitalHumanDefinition(AppCodeVersionNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman version was not found.",
			"未找到该数字人版本。"),
		digitalHumanDefinition(AppCodeNameConflict, appcatalog.CategoryConflict, 409,
			"A DigitalHuman with this name already exists in the workspace.",
			"当前工作空间中已存在同名数字人。"),
		digitalHumanDefinition(AppCodeRevisionConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman has changed. Refresh and try again.",
			"数字人配置已发生变化，请刷新后重试。"),
		digitalHumanDefinition(AppCodePublishedImmutable, appcatalog.CategoryConflict, 409,
			"Published DigitalHuman versions cannot be changed.",
			"已发布的数字人版本不可修改。"),
		digitalHumanDefinition(AppCodePublishInvalid, appcatalog.CategoryValidation, 400,
			"Complete the conversation name, persona, and default model before publishing.",
			"请先完善对话名称、人设和默认模型，再发布数字人。"),
		digitalHumanDefinition(AppCodeVersionRestoreImpactChanged, appcatalog.CategoryConflict, 409,
			"The draft or restore impact changed. Review the latest impact and try again.",
			"草稿或恢复影响已发生变化，请确认最新影响后重试。"),
		digitalHumanDefinition(AppCodeDeleteBlocked, appcatalog.CategoryConflict, 409,
			"This DigitalHuman has published or referenced configuration and cannot be deleted.",
			"该数字人存在已发布或被引用的配置，暂时无法删除。"),
		digitalHumanDefinition(AppCodePersonaTemplateNotFound, appcatalog.CategoryNotFound, 404,
			"The persona template was not found or is no longer available.", "未找到该人设模板，它可能已被删除。"),
		digitalHumanDefinition(AppCodePersonaTemplateNameConflict, appcatalog.CategoryConflict, 409,
			"A persona template with this name already exists in the workspace.", "当前工作空间中已存在同名人设模板。"),
		digitalHumanDefinition(AppCodeResourceBindingInvalid, appcatalog.CategoryValidation, 400,
			"The resource binding is incomplete or contains unsafe credential material.", "资源绑定不完整或包含不安全的凭据内容。"),
		digitalHumanDefinition(AppCodeRunNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman run was not found.", "未找到该数字人运行记录。"),
		digitalHumanDefinition(AppCodeRunConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman run cannot transition from its current state.", "该数字人运行无法从当前状态继续转换。"),
		digitalHumanDefinition(AppCodeEndUserSuspended, appcatalog.CategoryAuthorization, 403,
			"This DigitalHuman is not available to this user right now.", "当前用户暂时无法使用该数字人。"),
		digitalHumanDefinition(AppCodeConversationNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman conversation was not found.", "未找到该数字人的会话。"),
		digitalHumanDefinition(AppCodeConsoleFileNotAccessible, appcatalog.CategoryNotFound, 404,
			"The file was not found or is not available for this conversation.", "未找到该文件，或该文件无法用于当前会话。"),
		digitalHumanDefinition(AppCodeWebAppFileRequiresLogin, appcatalog.CategoryAuthorization, 403,
			"Sign in to send files. You can keep chatting without attachments.", "发送文件需要先登录；不带附件仍可继续对话。"),
		digitalHumanDefinition(AppCodeMemoryNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman memory was not found.", "未找到该数字人的记忆。"),
		digitalHumanDefinition(AppCodeMemoryConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman memory changed or its execution fence is stale. Refresh and try again.", "数字人记忆或执行围栏已变化，请刷新后重试。"),
		digitalHumanDefinition(AppCodeMemoryPolicyRejected, appcatalog.CategoryValidation, 400,
			"The memory proposal was rejected by evidence, privacy, or sensitivity policy.", "该记忆提议未通过证据、隐私或敏感信息策略。"),
		digitalHumanDefinition(AppCodeMemoryUndoUnavailable, appcatalog.CategoryConflict, 409,
			"This memory change can no longer be undone.", "该记忆变更已无法撤销。"),
		digitalHumanDefinition(AppCodeMemoryProjectionUnavailable, appcatalog.CategoryInternal, 503,
			"Long-term memory repair is not available in this deployment.", "当前部署暂不支持长期记忆修复。"),
		digitalHumanDefinition(AppCodeMemoryRepairFailed, appcatalog.CategoryInternal, 500,
			"Long-term memory could not be rebuilt. Check the embedding model and vector store configuration.",
			"长期记忆重建失败，请检查嵌入模型与向量库配置。"),
		digitalHumanDefinition(AppCodeTaskNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman task was not found or is not available to this user.", "未找到该任务，或该任务不属于当前用户。"),
		digitalHumanDefinition(AppCodeTaskInvalidRequest, appcatalog.CategoryValidation, 400,
			"The DigitalHuman task request is invalid. Check the fields and try again.", "数字人任务请求内容不正确，请检查字段后重试。"),
		digitalHumanDefinition(AppCodeTaskConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman task changed or cannot transition from its current state. Refresh and try again.", "数字人任务已发生变化或无法从当前状态转换，请刷新后重试。"),
		digitalHumanDefinition(AppCodeTaskPolicyRejected, appcatalog.CategoryValidation, 400,
			"This DigitalHuman surface cannot create or manage tasks.", "当前数字人入口不允许创建或管理任务。"),
		digitalHumanDefinition(AppCodeChannelInstallationNotFound, appcatalog.CategoryNotFound, 404,
			"The channel installation was not found.", "未找到该渠道安装。"),
		digitalHumanDefinition(AppCodeChannelConflict, appcatalog.CategoryConflict, 409,
			"The channel configuration changed or is already in use.", "渠道配置已变化或已被占用。"),
		digitalHumanDefinition(AppCodeChannelNotReady, appcatalog.CategoryValidation, 400,
			"The channel connection, webhook security, or bot identity could not be verified.", "无法验证渠道连接、Webhook 安全配置或机器人身份。"),
		digitalHumanDefinition(AppCodeChannelTransportUnsupported, appcatalog.CategoryValidation, 400,
			"This Connector does not support the selected channel transport.", "该连接器暂不支持当前渠道传输方式。"),
		digitalHumanDefinition(AppCodeChannelCredentialManaged, appcatalog.CategoryConflict, 409,
			"This channel uses a Connector connection. Manage its credential in Connector settings.", "该渠道使用连接器连接，请在连接器设置中管理凭证。"),
		digitalHumanDefinition(AppCodeCredentialStorageUnavailable, appcatalog.CategoryInternal, 503,
			"Secure credential storage is not configured. Ask an administrator to configure it before saving channel or web-search credentials.",
			"安全凭证存储尚未配置，请联系管理员完成配置后，再保存渠道或网页搜索凭证。"),
		digitalHumanDefinition(AppCodeChannelDeliveryNotFound, appcatalog.CategoryNotFound, 404,
			"The channel delivery was not found.", "未找到该渠道投递记录。"),
		digitalHumanDefinition(AppCodeChannelDeliveryConflict, appcatalog.CategoryConflict, 409,
			"The channel delivery cannot be retried from its current state.", "该渠道投递无法从当前状态重试。"),
	}
}

func digitalHumanDefinition(code apperror.Code, category appcatalog.Category, status int, english, chinese string) appcatalog.Definition {
	return appcatalog.Definition{
		Code:       code,
		Category:   category,
		HTTPStatus: status,
		Retryable:  false,
		Messages: map[appcatalog.Locale]string{
			appcatalog.LocaleEnglishUS:         english,
			appcatalog.LocaleChineseSimplified: chinese,
		},
	}
}

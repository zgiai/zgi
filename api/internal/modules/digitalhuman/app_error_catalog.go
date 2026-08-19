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
	// AppCodeDeleteBlocked identifies deletion prevented by durable references.
	AppCodeDeleteBlocked          = apperror.MustCode("digital_human.delete.blocked")
	AppCodeRoleTemplateNotFound   = apperror.MustCode("digital_human.role_template.not_found")
	AppCodeRoleVersionNotFound    = apperror.MustCode("digital_human.role_template_version.not_found")
	AppCodeRoleNameConflict       = apperror.MustCode("digital_human.role_template.name.conflict")
	AppCodeRolePublishInvalid     = apperror.MustCode("digital_human.role_template.publish.invalid")
	AppCodeResourceBindingInvalid = apperror.MustCode("digital_human.resource_binding.invalid")
	AppCodeRunNotFound            = apperror.MustCode("digital_human.run.not_found")
	AppCodeRunConflict            = apperror.MustCode("digital_human.run.conflict")
	AppCodeConversationNotFound   = apperror.MustCode("digital_human.conversation.not_found")
	AppCodeMemoryNotFound         = apperror.MustCode("digital_human.memory.not_found")
	AppCodeMemoryConflict         = apperror.MustCode("digital_human.memory.conflict")
	AppCodeMemoryPolicyRejected   = apperror.MustCode("digital_human.memory.policy_rejected")
	AppCodeMemoryUndoUnavailable  = apperror.MustCode("digital_human.memory.undo_unavailable")
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
			"Complete the persona, rules, and default model before publishing.",
			"请先完善人设、规则和默认模型，再发布数字人。"),
		digitalHumanDefinition(AppCodeDeleteBlocked, appcatalog.CategoryConflict, 409,
			"This DigitalHuman has published or referenced configuration and cannot be deleted.",
			"该数字人存在已发布或被引用的配置，暂时无法删除。"),
		digitalHumanDefinition(AppCodeRoleTemplateNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman role template was not found.", "未找到该数字人岗位模板。"),
		digitalHumanDefinition(AppCodeRoleVersionNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman role template version was not found.", "未找到该数字人岗位模板版本。"),
		digitalHumanDefinition(AppCodeRoleNameConflict, appcatalog.CategoryConflict, 409,
			"A role template with this name already exists in the workspace.", "当前工作空间中已存在同名岗位模板。"),
		digitalHumanDefinition(AppCodeRolePublishInvalid, appcatalog.CategoryValidation, 400,
			"Complete the role goals and responsibilities before publishing.", "请先完善岗位目标和职责，再发布岗位模板。"),
		digitalHumanDefinition(AppCodeResourceBindingInvalid, appcatalog.CategoryValidation, 400,
			"The resource binding is incomplete or contains unsafe credential material.", "资源绑定不完整或包含不安全的凭据内容。"),
		digitalHumanDefinition(AppCodeRunNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman run was not found.", "未找到该数字人运行记录。"),
		digitalHumanDefinition(AppCodeRunConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman run cannot transition from its current state.", "该数字人运行无法从当前状态继续转换。"),
		digitalHumanDefinition(AppCodeConversationNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman conversation was not found.", "未找到该数字人的会话。"),
		digitalHumanDefinition(AppCodeMemoryNotFound, appcatalog.CategoryNotFound, 404,
			"The DigitalHuman memory was not found.", "未找到该数字人的记忆。"),
		digitalHumanDefinition(AppCodeMemoryConflict, appcatalog.CategoryConflict, 409,
			"The DigitalHuman memory changed or its execution fence is stale. Refresh and try again.", "数字人记忆或执行围栏已变化，请刷新后重试。"),
		digitalHumanDefinition(AppCodeMemoryPolicyRejected, appcatalog.CategoryValidation, 400,
			"The memory proposal was rejected by evidence, privacy, or sensitivity policy.", "该记忆提议未通过证据、隐私或敏感信息策略。"),
		digitalHumanDefinition(AppCodeMemoryUndoUnavailable, appcatalog.CategoryConflict, 409,
			"This memory change can no longer be undone.", "该记忆变更已无法撤销。"),
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

package core

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

const (
	ErrorCodeWeixinAlreadyBound           ErrorCode = "connector_weixin_already_bound"
	ErrorCodeWeixinRegionUnsupported      ErrorCode = "connector_weixin_region_unsupported"
	ErrorCodeVerificationWaiting          ErrorCode = "connector_verification_waiting"
	ErrorCodeProviderResponseUnrecognized ErrorCode = "connector_provider_response_unrecognized"
	ErrorCodeVerificationUncertain        ErrorCode = "connector_verification_uncertain"
	ErrorCodeMailAuthenticationRejected   ErrorCode = "connector_mail_authentication_rejected"
	ErrorCodeMailAppPasswordRequired      ErrorCode = "connector_mail_app_password_required"
	ErrorCodeMailTLSRequired              ErrorCode = "connector_mail_tls_required"
	ErrorCodeMailPolicyRestricted         ErrorCode = "connector_mail_policy_restricted"
	ErrorCodeMailReferenceExpired         ErrorCode = "connector_mail_reference_expired"
	ErrorCodeMailSizeExceeded             ErrorCode = "connector_mail_size_exceeded"
	ErrorCodeMailRecipientRejected        ErrorCode = "connector_mail_recipient_rejected"
	ErrorCodeMailSubmissionRejected       ErrorCode = "connector_mail_submission_rejected"
	ErrorCodeMailClientIdentityRequired   ErrorCode = "connector_mail_client_identity_required"
	ErrorCodeMailAuthMechanismUnsupported ErrorCode = "connector_mail_auth_mechanism_unsupported"
	ErrorCodeMailUidStateUnavailable      ErrorCode = "connector_mail_uid_state_unavailable"
)

type authErrorDefinition struct {
	legacy   ErrorCode
	code     apperror.Code
	category appcatalog.Category
	status   int
	en, zh   string
}

var authErrorDefinitions = []authErrorDefinition{
	{ErrorCodeWeixinAlreadyBound, apperror.MustCode("connector.weixin.already_bound"), appcatalog.CategoryConflict, 409, "WeChat reports an existing binding but returned no new credentials. Review the original connection or start a new login.", "微信提示已有绑定，但未返回新凭据。请查看原连接，或重新发起登录。"},
	{ErrorCodeWeixinRegionUnsupported, apperror.MustCode("connector.weixin.region_unsupported"), appcatalog.CategoryValidation, 422, "This WeChat service region is not supported by this deployment.", "当前部署暂不支持此微信服务区域。"},
	{ErrorCodeVerificationWaiting, apperror.MustCode("connector.verification.waiting"), appcatalog.CategoryConflict, 409, "Verification is still waiting for the provider. Check its status before retrying.", "仍在等待平台验证结果，请先查看状态。"},
	{ErrorCodeProviderResponseUnrecognized, apperror.MustCode("connector.provider.response_unrecognized"), appcatalog.CategoryUpstream, 502, "The provider returned an unrecognized response. The connection has not been activated.", "无法识别平台返回的数据，连接尚未启用。"},
	{ErrorCodeVerificationUncertain, apperror.MustCode("connector.verification.uncertain"), appcatalog.CategoryConflict, 409, "The verification result could not be confirmed. Start a new authorization to continue safely.", "无法确认本次验证结果，请重新授权后继续。"},
	{ErrorCodeMailAuthenticationRejected, apperror.MustCode("connector.mail.authentication_rejected"), appcatalog.CategoryValidation, 422, "Email authentication failed. Check the login address, client password or authorization code, and enabled mail services.", "邮箱认证失败，请检查登录邮箱、客户端专用密码或授权码，以及邮件服务是否已开启。"},
	{ErrorCodeMailAppPasswordRequired, apperror.MustCode("connector.mail.app_password_required"), appcatalog.CategoryValidation, 422, "Use a Google application password instead of your account password.", "请使用 Google 应用专用密码，而非账号登录密码。"},
	{ErrorCodeMailTLSRequired, apperror.MustCode("connector.mail.tls_required"), appcatalog.CategoryUpstream, 502, "A secure connection to the mail server could not be established. Check your network and try again.", "无法与邮件服务器建立安全连接，请检查网络后重试。"},
	{ErrorCodeMailPolicyRestricted, apperror.MustCode("connector.mail.policy_restricted"), appcatalog.CategoryAuthorization, 403, "Your account policy does not allow this mail connection. Check the account or organization settings.", "账号策略不允许此邮箱连接，请检查账号或组织设置。"},
	{ErrorCodeMailReferenceExpired, apperror.MustCode("connector.mail.reference_expired"), appcatalog.CategoryConflict, 409, "This message reference has expired. List the messages again before opening one.", "邮件引用已失效，请刷新邮件列表后重新打开。"},
	{ErrorCodeMailSizeExceeded, apperror.MustCode("connector.mail.size_exceeded"), appcatalog.CategoryValidation, 422, "The mail content exceeds the supported size limit.", "邮件内容超过当前支持的大小限制。"},
	{ErrorCodeMailRecipientRejected, apperror.MustCode("connector.mail.recipient_rejected"), appcatalog.CategoryValidation, 422, "The mail server rejected a recipient. No message was submitted; check the recipients before trying again.", "邮件服务器拒绝了收件人，邮件未提交，请检查收件人后重试。"},
	{ErrorCodeMailSubmissionRejected, apperror.MustCode("connector.mail.submission_rejected"), appcatalog.CategoryUpstream, 502, "The mail server rejected this submission.", "邮件服务器拒绝了本次提交。"},
	{ErrorCodeMailClientIdentityRequired, apperror.MustCode("connector.mail.client_identity_required"), appcatalog.CategoryUpstream, 502, "The mail server rejected the client identity or access policy. Check the client compatibility; do not disable account security.", "邮箱服务器拒绝了客户端身份或访问策略，请检查客户端兼容性，无需关闭账号安全设置。"},
	{ErrorCodeMailAuthMechanismUnsupported, apperror.MustCode("connector.mail.auth_mechanism_unsupported"), appcatalog.CategoryUpstream, 502, "The mail server does not offer a supported authentication method.", "邮箱服务器未提供当前支持的认证方式。"},
	{ErrorCodeMailUidStateUnavailable, apperror.MustCode("connector.mail.uid_state_unavailable"), appcatalog.CategoryUpstream, 502, "The mail server did not report this mailbox's UID range, so messages cannot be listed yet. Try again later or check the mailbox IMAP settings.", "邮箱服务器未返回该文件夹的 UID 范围，暂时无法列出邮件。请稍后重试，或检查该邮箱的 IMAP 设置。"},
}

func authCatalogDefinitions() []appcatalog.Definition {
	result := make([]appcatalog.Definition, 0, len(authErrorDefinitions))
	for _, item := range authErrorDefinitions {
		result = append(result, appDefinition(item.code, item.category, item.status, item.en, item.zh))
	}
	return result
}

func authApplicationError(code ErrorCode) (error, bool) {
	for _, item := range authErrorDefinitions {
		if item.legacy == code {
			return apperror.New(item.code), true
		}
	}
	return nil, false
}

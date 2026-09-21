package llmerrors

import (
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// Billing-owned application error identities, composed once during bootstrap.
var (
	AppCodeBillingOrganizationBalanceInsufficient   = apperror.MustCode("billing.organization.balance_insufficient")
	AppCodeBillingPrivateChannelBalanceInsufficient = apperror.MustCode("billing.private_channel.balance_insufficient")
	AppCodeBillingWorkspaceQuotaInsufficient        = apperror.MustCode("billing.workspace.quota_insufficient")
	AppCodeBillingModelPricingNotConfigured         = apperror.MustCode("billing.model.pricing_not_configured")
	AppCodeBillingExchangeRateUnavailable           = apperror.MustCode("billing.exchange_rate.unavailable")
	AppCodeBillingPredeductFailed                   = apperror.MustCode("billing.prededuct.failed")
	AppCodeBillingSettleFailed                      = apperror.MustCode("billing.settle.failed")
	AppCodeBillingRequestInvalid                    = apperror.MustCode("billing.request.invalid")
	AppCodeBillingDeductionNotFound                 = apperror.MustCode("billing.deduction.not_found")
	AppCodeBillingDeductionMismatch                 = apperror.MustCode("billing.deduction.mismatch")
	AppCodeBillingDeductionExpired                  = apperror.MustCode("billing.deduction.expired")
	AppCodeBillingActualExceedsReserve              = apperror.MustCode("billing.actual_exceeds_reserve")
)

// BillingCatalogDefinitions returns the billing-owned definitions for the process catalog.
func BillingCatalogDefinitions() []appcatalog.Definition {
	return []appcatalog.Definition{
		llmDefinition(AppCodeBillingOrganizationBalanceInsufficient, appcatalog.CategoryQuota, 402, false, "Your organization balance is insufficient. Add funds or contact an administrator.", "组织账户余额不足，请充值或联系管理员。", "app.workflow:207011", "app.workflow:207001"),
		llmDefinition(AppCodeBillingPrivateChannelBalanceInsufficient, appcatalog.CategoryQuota, 402, false, "The private channel balance is insufficient. Recharge the channel or choose another model.", "私有渠道余额不足，请充值渠道或更换模型。", "app.workflow:207013"),
		llmDefinition(AppCodeBillingWorkspaceQuotaInsufficient, appcatalog.CategoryQuota, 402, false, "This workspace has reached its quota. Review the workspace quota or contact an administrator.", "工作空间额度已用完，请检查额度或联系管理员。", "app.workflow:207012"),
		llmDefinition(AppCodeBillingModelPricingNotConfigured, appcatalog.CategoryConfiguration, 402, false, "Pricing is not configured for this model. An administrator must configure it in model management or billing policy.", "模型未配置价格，请管理员在模型管理或计费策略中配置价格。", "app.workflow:207014"),
		llmDefinition(AppCodeBillingExchangeRateUnavailable, appcatalog.CategoryConfiguration, 503, false, "The exchange rate is unavailable, so this request cannot be billed. Contact an administrator.", "汇率不可用，无法完成本次计费，请联系管理员。"),
		llmDefinition(AppCodeBillingPredeductFailed, appcatalog.CategoryInternal, 500, true, "The request could not be reserved for billing. Try again; if it continues, contact an administrator.", "请求预扣费失败，请重试；如持续发生，请联系管理员。"),
		llmDefinition(AppCodeBillingSettleFailed, appcatalog.CategoryInternal, 500, true, "The request completed but billing settlement failed. It will be reconciled automatically; contact an administrator if it persists.", "请求已完成但计费结算失败，系统将自动对账；如持续发生，请联系管理员。"),
		llmDefinition(AppCodeBillingRequestInvalid, appcatalog.CategoryValidation, 400, false, "The billing request is invalid.", "计费请求无效。"),
		llmDefinition(AppCodeBillingDeductionNotFound, appcatalog.CategoryNotFound, 404, false, "The billing reservation was not found.", "未找到计费预扣记录。"),
		llmDefinition(AppCodeBillingDeductionMismatch, appcatalog.CategoryConflict, 409, false, "The billing reservation does not match this request.", "计费预扣与当前请求不匹配。"),
		llmDefinition(AppCodeBillingDeductionExpired, appcatalog.CategoryConflict, 409, false, "The billing reservation expired. Retry the request to create a new reservation.", "计费预扣已过期，请重试以创建新的预扣。"),
		llmDefinition(AppCodeBillingActualExceedsReserve, appcatalog.CategoryConflict, 409, false, "The actual usage exceeded the reserved amount.", "实际用量超过预留额度。"),
	}
}

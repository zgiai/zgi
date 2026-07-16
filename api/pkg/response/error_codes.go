package response

type ErrorCode struct {
	Code        int    `json:"code"`
	Message     string `json:"message"`
	UserVisible bool   `json:"user_visible"`
}

/*
Error Code Design Specification: [A][BB][CCC]
A (1 digit) - Error Level:
  1 - Parameter validation error
  2 - Business logic error
  3 - System error
  4 - Permission/Authentication error
  5 - Third-party service error

BB (2 digits) - Module Number:
  01 - User module
  02 - Dataset module
  03 - Document module
  04 - Application module
  05 - Enterprise/Tenant module
  06 - Message/Conversation module
  07 - Billing module
  08 - Model provider module
  09 - Plugin module
  10 - File module
  11 - Segment module
  12 - System settings module
  99 - General module

CCC (3 digits) - Specific error number: 001, 002, 003...
*/

// ============================================================================
// ============================================================================

var (
	ErrInvalidParam  = ErrorCode{199001, "Invalid parameter", true}
	ErrInvalidParams = ErrorCode{199002, "Invalid request parameters", true}
)

var (
	ErrPhoneFormat   = ErrorCode{101001, "Invalid phone number format", true}
	ErrEmailFormat   = ErrorCode{101002, "Invalid email format", true}
	ErrPasswordWeak  = ErrorCode{101003, "Password is too weak, must contain letters, numbers and special characters", true}
	ErrNameRequired  = ErrorCode{101004, "Username cannot be empty", true}
	ErrInvalidCode   = ErrorCode{101005, "Invalid verification code", true}
	ErrTokenRequired = ErrorCode{101006, "Refresh token cannot be empty", true}
)

var (
	ErrDatasetName            = ErrorCode{102001, "Dataset name cannot be empty", true}
	ErrDatasetNameLong        = ErrorCode{102002, "Dataset name cannot exceed 40 characters", true}
	ErrDatasetDescriptionLong = ErrorCode{102003, "Dataset description cannot exceed 400 characters", true}
	ErrDatasetIdRequired      = ErrorCode{102004, "Dataset ID cannot be empty", true}
)

var (
	ErrDocumentFile       = ErrorCode{103001, "Please select a document file to upload", true}
	ErrDocumentSize       = ErrorCode{103002, "Document file size cannot exceed 10MB", true}
	ErrDocumentFormat     = ErrorCode{103003, "Unsupported document format", true}
	ErrDocumentIdRequired = ErrorCode{103004, "Document ID cannot be empty", true}
)

var (
	ErrInvalidAppId           = ErrorCode{104001, "Invalid application ID format", true}
	ErrAppIdRequired          = ErrorCode{104002, "Application ID cannot be empty", true}
	ErrConversationIdRequired = ErrorCode{104003, "Conversation ID cannot be empty", true}
	ErrGroupIdRequired        = ErrorCode{104004, "Group ID cannot be empty", true}
	ErrInvalidUuid            = ErrorCode{104005, "Invalid UUID format", true}
)

var (
	ErrFileIdRequired      = ErrorCode{110001, "File ID cannot be empty", true}
	ErrFileTooLarge        = ErrorCode{110002, "File size exceeds limit", true}
	ErrUnsupportedFileType = ErrorCode{110003, "Unsupported file type", true}
	ErrNoFileUploaded      = ErrorCode{110004, "Please upload a file", true}
	ErrTooManyFiles        = ErrorCode{110005, "Only one file upload is allowed", true}
	ErrFilenameRequired    = ErrorCode{110006, "Please upload a file with a filename", true}
)

// Quota module errors (14)
var (
	ErrQuotaExceeded                   = ErrorCode{114009, "Quota exceeded", true}
	ErrQuotaSeatsExceeded              = ErrorCode{114009, "Seats quota exceeded. Current: %d, Limit: %d, Attempt: +%d", true}
	ErrQuotaStorageExceeded            = ErrorCode{114009, "Storage quota exceeded. Current: %s, Limit: %s, Attempt: +%s", true}
	ErrQuotaDBRowsExceeded             = ErrorCode{114009, "Database rows quota exceeded. Current: %d, Limit: %d, Attempt: +%d", true}
	ErrQuotaKnowledgeBasesExceeded     = ErrorCode{114009, "Knowledge bases quota exceeded. Current: %d, Limit: %d", true}
	ErrQuotaAIAgentsExceeded           = ErrorCode{114009, "AI agents quota exceeded. Current: %d, Limit: %d", true}
	ErrQuotaWorkflowsExceeded          = ErrorCode{114009, "Workflows quota exceeded. Current: %d, Limit: %d", true}
	ErrQuotaWorkflowExecutionsExceeded = ErrorCode{114009, "Workflow executions quota exceeded. Current: %d, Limit: %d", true}
	ErrQuotaOCRPagesExceeded           = ErrorCode{114009, "OCR pages quota exceeded. Current: %d, Limit: %d, Attempt: +%d", true}
	ErrSubscriptionNotFound            = ErrorCode{114010, "Active subscription not found for this organization", true}
	ErrSubscriptionExpired             = ErrorCode{114011, "Subscription has expired", true}
)

// ============================================================================
// ============================================================================

var (
	ErrUserExists            = ErrorCode{201001, "User already exists", true}
	ErrUserNotFound          = ErrorCode{201002, "User not found", true}
	ErrUserDisabled          = ErrorCode{201003, "Account has been disabled, please contact administrator", true}
	ErrUserQuota             = ErrorCode{114009, "User quota insufficient", true}
	ErrPasswordMismatch      = ErrorCode{201005, "Password mismatch", true}
	ErrAccountDeleteFailed   = ErrorCode{201006, "Failed to delete account", true}
	ErrIntegrationNotFound   = ErrorCode{201007, "Integration not found", true}
	ErrRateLimitExceeded     = ErrorCode{201008, "Too many requests, please try again later", true}
	ErrAccountActivateFailed = ErrorCode{201009, "Failed to activate account", true}
	ErrAccountCheckFailed    = ErrorCode{201010, "Failed to check account", true}
	ErrAccountFrozen         = ErrorCode{201011, "Account has been frozen", true}
	ErrAccountNotFound       = ErrorCode{201012, "Account not found", true}
	ErrAccountUpdate         = ErrorCode{201013, "Failed to update account", true}
	ErrInvalidStatus         = ErrorCode{201014, "Invalid status value", true}
	ErrInvalidRoleType       = ErrorCode{201015, "Invalid role type", true}
	ErrAccountBanned         = ErrorCode{201016, "Account has been banned", true}
	ErrEmailPasswordMismatch = ErrorCode{201017, "Invalid email or password", true}
	ErrLoginErrorRateLimit   = ErrorCode{201018, "Too many login attempts, please try again later", true}
	ErrEmailInvalid          = ErrorCode{201019, "Invalid email address", true}
	ErrGetUserInfoFailed     = ErrorCode{201020, "Failed to get user information", true}
)

var (
	ErrDatasetNotFound         = ErrorCode{202001, "Dataset not found", true}
	ErrDatasetExists           = ErrorCode{202002, "Dataset name already exists, please use a different name", true}
	ErrDatasetProcessing       = ErrorCode{202003, "Dataset is being processed, please try again later", true}
	ErrDatasetQuota            = ErrorCode{202004, "Dataset limit reached, please delete unused datasets", true}
	ErrDatasetCreateFailed     = ErrorCode{202005, "Failed to create dataset", true}
	ErrDatasetUpdateFailed     = ErrorCode{202006, "Failed to update dataset", true}
	ErrDatasetDeleteFailed     = ErrorCode{202007, "Failed to delete dataset", true}
	ErrDatasetGetFailed        = ErrorCode{202008, "Failed to get dataset", true}
	ErrDatasetPermissionDenied = ErrorCode{202009, "Permission denied to access this dataset", true}
)

var (
	ErrDocumentNotFound          = ErrorCode{203001, "Document not found", true}
	ErrDocumentProcessing        = ErrorCode{203002, "Document is being processed, please try again later", true}
	ErrDocumentCreateFailed      = ErrorCode{203003, "Failed to create document", true}
	ErrDocumentUpdateFailed      = ErrorCode{203004, "Failed to update document", true}
	ErrDocumentDeleteFailed      = ErrorCode{203005, "Failed to delete document", true}
	ErrDocumentGetFailed         = ErrorCode{203006, "Failed to get document", true}
	ErrDocumentListFailed        = ErrorCode{203007, "Failed to get document list", true}
	ErrProcessRuleGetFailed      = ErrorCode{203008, "Failed to get processing rules", true}
	ErrBatchIndexingStatusFailed = ErrorCode{203009, "Failed to get batch indexing status", true}
	ErrExplanationNotFound       = ErrorCode{203010, "Explanation not found", true}
	ErrExplanationCreateFailed   = ErrorCode{203011, "Failed to create explanation", true}
	ErrExplanationGenerateFailed = ErrorCode{203012, "Failed to generate explanation", true}
	ErrExplanationTimeout        = ErrorCode{203013, "Explanation generation timeout, please try shorter text or fewer modes", true}
	ErrExplanationGetFailed      = ErrorCode{203014, "Failed to get explanation", true}
	ErrExplanationHistoryFailed  = ErrorCode{203015, "Failed to get explanation history", true}
	ErrExplanationTagsFailed     = ErrorCode{203016, "Failed to get explanation tags", true}
	ErrPopularTagsFailed         = ErrorCode{203017, "Failed to get popular tags", true}
	ErrTagExplanationsFailed     = ErrorCode{203018, "Failed to get tag-related explanations", true}
	ErrInvalidExplanationId      = ErrorCode{203019, "Invalid explanation ID", true}
	ErrInvalidTagId              = ErrorCode{203020, "Invalid tag ID", true}
	ErrInvalidModes              = ErrorCode{203021, "No valid modes", true}
	ErrDocumentDiagnosisFailed   = ErrorCode{203022, "Document diagnosis failed", true}
	ErrDocumentRetryFailed       = ErrorCode{203023, "Failed to retry document", true}
)

var (
	ErrAppNotFound             = ErrorCode{204001, "Application not found", true}
	ErrAppModelNotFound        = ErrorCode{204002, "Application model not found", true}
	ErrInvalidAppModel         = ErrorCode{204003, "Invalid application model", true}
	ErrChannelModeNotSupported = ErrorCode{204004, "CHANNEL mode is not supported", true}
	ErrAppModeNotSupported     = ErrorCode{204005, "Application mode is not in the supported list", true}
	ErrInvalidUserType         = ErrorCode{204006, "Invalid user ID type", true}
	ErrSystemAppCountInvalid   = ErrorCode{204007, "There must be exactly one system application", true}
	ErrWebAppOffline           = ErrorCode{204008, "This web app is offline", true}
	ErrWebAppNotPublished      = ErrorCode{204009, "Agent web app is not published", true}
	ErrAgentPromptTooLong      = ErrorCode{204010, "Agent system prompt is too long", true}
)

var (
	ErrOrganizationNotFound         = ErrorCode{205001, "Enterprise group not found", true}
	ErrEnterpriseExists             = ErrorCode{205002, "Enterprise group name already exists", true}
	ErrOrganizationExists           = ErrorCode{205003, "Enterprise group name already exists, please use a different name", true}
	ErrWorkspaceNotFound            = ErrorCode{205004, "Workspace not found", true}
	ErrWorkspaceExists              = ErrorCode{205005, "Workspace name already exists, please use a different name", true}
	ErrCannotOperateSelf            = ErrorCode{205006, "Cannot perform this operation on yourself", true}
	ErrMemberNotInWorkspace         = ErrorCode{205007, "Member is not in this tenant", true}
	ErrMemberNotFound               = ErrorCode{205008, "Member not found", true}
	ErrEmailExists                  = ErrorCode{205009, "Email already exists", true}
	ErrInvalidRole                  = ErrorCode{205010, "Invalid role", true}
	ErrInvalidPermission            = ErrorCode{205011, "Invalid permission value", true}
	ErrInvalidGender                = ErrorCode{205012, "Invalid gender value", true}
	ErrMemberAlreadyExists          = ErrorCode{205013, "Member already exists in this tenant", true}
	ErrRoleAlreadyAssigned          = ErrorCode{205014, "Role already assigned", true}
	ErrInvalidTenantId              = ErrorCode{205015, "Failed to get tenant ID", true}
	ErrWorkspaceJoinedNotFound      = ErrorCode{205016, "Workspace not found, please contact administrator to invite you to a workspace", true}
	ErrWorkspaceNotInOrganization   = ErrorCode{205017, "Tenant is not in this group", true}
	ErrCannotDeleteShadowWorkspace  = ErrorCode{205018, "Cannot delete shadow tenant", true}
	ErrCannotDeleteTenantWithAssets = ErrorCode{205019, "Cannot delete tenant with remaining assets", true}
	ErrMemberAlreadyInOrganization  = ErrorCode{205020, "User is already a member of this organization", true}
	ErrJoinRequestPending           = ErrorCode{205021, "Join request is pending approval", true}
)

var (
	ErrMessageNotFound           = ErrorCode{206001, "Message not found", true}
	ErrInvalidMessageId          = ErrorCode{206002, "Invalid message ID", true}
	ErrCreateMessageFailed       = ErrorCode{206003, "Failed to create message", true}
	ErrUpdateMessageFailed       = ErrorCode{206004, "Failed to update message", true}
	ErrDeleteMessageFailed       = ErrorCode{206005, "Failed to delete message", true}
	ErrFetchMessagesFailed       = ErrorCode{206006, "Failed to fetch messages", true}
	ErrInvalidConversationId     = ErrorCode{206007, "Invalid conversation ID", true}
	ErrConversationNotFound      = ErrorCode{206008, "Conversation not found", true}
	ErrGroupNotFound             = ErrorCode{206009, "Group not found", true}
	ErrConversationGroupNotFound = ErrorCode{206010, "Conversation group not found", true}
	ErrConversationNotInGroup    = ErrorCode{206011, "Conversation is not in this group", true}
	ErrMessageDeleteFailed       = ErrorCode{206012, "Failed to delete message", true}
)

var (
	ErrInsufficientBalance                       = ErrorCode{207001, "Insufficient account balance, please recharge", true}
	ErrPlanExpired                               = ErrorCode{207002, "Plan has expired, please renew", true}
	ErrTTSGenerateFailed                         = ErrorCode{207003, "Failed to generate speech", true}
	ErrTTSAudioNotFound                          = ErrorCode{207004, "Audio file not found", true}
	ErrTTSHistoryGetFailed                       = ErrorCode{207005, "Failed to get speech history", true}
	ErrTTSHistoryDeleteFailed                    = ErrorCode{207006, "Failed to delete speech history", true}
	ErrTTSTranslateFailed                        = ErrorCode{207007, "Failed to translate speech", true}
	ErrWorkflowOrganizationBalanceLow            = ErrorCode{207008, "Workflow organization balance is low", true}
	ErrWorkflowWorkspaceQuotaLow                 = ErrorCode{207009, "Workflow workspace quota is low", true}
	ErrWorkflowPrivateChannelBalanceLow          = ErrorCode{207010, "Workflow private channel balance is low", true}
	ErrWorkflowOrganizationBalanceInsufficient   = ErrorCode{207011, "Workflow organization balance is insufficient", true}
	ErrWorkflowWorkspaceQuotaInsufficient        = ErrorCode{207012, "Workflow workspace quota is insufficient", true}
	ErrWorkflowPrivateChannelBalanceInsufficient = ErrorCode{207013, "Workflow private channel balance is insufficient", true}
	ErrWorkflowModelPricingNotConfigured         = ErrorCode{207014, "模型未配置价格，请先在模型管理或计费策略中配置价格。", true}
	ErrWorkflowPrivateChannelUpstreamUnavailable = ErrorCode{207015, "Private channel upstream credential is unavailable", true}
	ErrWorkflowPlatformChannelUnavailable        = ErrorCode{207016, "当前模型服务暂时不可用，请稍后重试或选择其他模型。", true}
)

var (
	ErrProviderListFailed                  = ErrorCode{208001, "Failed to get model provider list", true}
	ErrProviderCredentialsFailed           = ErrorCode{208002, "Failed to get model provider credentials", true}
	ErrInvalidProviderParam                = ErrorCode{208003, "Invalid provider parameter", true}
	ErrProviderCredentialsValidationFailed = ErrorCode{208004, "Model provider credentials validation failed", true}
	ErrProviderCredentialsSaveFailed       = ErrorCode{208005, "Failed to save model provider credentials", true}
	ErrProviderCredentialsRemoveFailed     = ErrorCode{208006, "Failed to remove model provider credentials", true}
	ErrProviderEnableFailed                = ErrorCode{208007, "Failed to enable model provider", true}
	ErrProviderDisableFailed               = ErrorCode{208008, "Failed to disable model provider", true}
	ErrProviderUpdateFailed                = ErrorCode{208009, "Failed to update model provider", true}
	ErrProviderIconFailed                  = ErrorCode{208010, "Failed to get model provider icon", true}
	ErrCheckoutUrlFailed                   = ErrorCode{208011, "Failed to get checkout URL", true}
	ErrModelListFailed                     = ErrorCode{208012, "Failed to get model list", true}
	ErrModelCredentialsFailed              = ErrorCode{208013, "Failed to get model credentials", true}
	ErrModelCredentialsSaveFailed          = ErrorCode{208014, "Failed to save model credentials", true}
	ErrModelCredentialsRemoveFailed        = ErrorCode{208015, "Failed to remove model credentials", true}
	ErrModelValidationFailed               = ErrorCode{208016, "Model validation failed", true}
	ErrModelEnableFailed                   = ErrorCode{208017, "Failed to enable model", true}
	ErrModelDisableFailed                  = ErrorCode{208018, "Failed to disable model", true}
	ErrModelParameterRulesFailed           = ErrorCode{208019, "Failed to get model parameter rules", true}
	ErrAvailableModelsFailed               = ErrorCode{208020, "Failed to get available models", true}
	ErrDefaultModelFailed                  = ErrorCode{208021, "Failed to get default model", true}
	ErrDefaultModelUpdateFailed            = ErrorCode{208022, "Failed to update default model", true}
	ErrModelSquareFailed                   = ErrorCode{208023, "Failed to get model marketplace", true}
	ErrModelDetailFailed                   = ErrorCode{208024, "Failed to get model details", true}
	ErrModelTypeRequired                   = ErrorCode{208025, "Model type cannot be empty", true}
	ErrModelRequired                       = ErrorCode{208026, "Model parameter cannot be empty", true}
	ErrModelNameRequired                   = ErrorCode{208027, "Model name cannot be empty", true}
	ErrModelSettingsRequired               = ErrorCode{208028, "Model settings cannot be empty", true}
	ErrInvalidContextLengthRange           = ErrorCode{208029, "Invalid context length range, must be one of: 16k_under, 16k_64k, 64k_128k, 128k_above", true}
)

var (
	ErrPluginListFailed            = ErrorCode{209001, "Failed to get plugin list", true}
	ErrModelPluginListFailed       = ErrorCode{209002, "Failed to get model plugin list", true}
	ErrInvalidPluginIdentifier     = ErrorCode{209003, "Invalid plugin identifier", true}
	ErrPluginInstallFailed         = ErrorCode{209004, "Failed to install plugin", true}
	ErrPluginUninstallFailed       = ErrorCode{209005, "Failed to uninstall plugin", true}
	ErrPluginPermissionQueryFailed = ErrorCode{209006, "Permission query failed", true}
	ErrInstallPermissionDenied     = ErrorCode{209007, "Install permission denied", true}
	ErrDebugPermissionDenied       = ErrorCode{209008, "Debug permission denied", true}
)

var (
	ErrFileNotFound         = ErrorCode{210001, "File not found", true}
	ErrFileUploadFailed     = ErrorCode{210002, "Failed to upload file", true}
	ErrFileReadFailed       = ErrorCode{210003, "Failed to read file", true}
	ErrFilePreviewFailed    = ErrorCode{210004, "Failed to get file preview", true}
	ErrFileDownloadFailed   = ErrorCode{210005, "Failed to download file", true}
	ErrFileInUse            = ErrorCode{210006, "File is in use and cannot be deleted", true}
	ErrToolListFailed       = ErrorCode{210007, "Failed to get tool list", true}
	ErrToolProviderNotFound = ErrorCode{210008, "Tool provider not found", true}
	ErrToolInvokeFailed     = ErrorCode{210009, "Failed to invoke tool", true}
	ErrToolParameterInvalid = ErrorCode{210010, "Invalid tool parameter", true}
	ErrFileFolderExists     = ErrorCode{210011, "A folder with this name already exists in the same directory", true}
)

var (
	ErrSegmentNotFound         = ErrorCode{211001, "Segment not found", true}
	ErrSegmentGetFailed        = ErrorCode{211002, "Failed to get segment", true}
	ErrSegmentCreateFailed     = ErrorCode{211003, "Failed to create segment", true}
	ErrSegmentUpdateFailed     = ErrorCode{211004, "Failed to update segment", true}
	ErrSegmentDeleteFailed     = ErrorCode{211005, "Failed to delete segment", true}
	ErrSegmentIdRequired       = ErrorCode{211006, "Segment ID cannot be empty", true}
	ErrNoSegmentIds            = ErrorCode{211007, "No segment IDs provided", true}
	ErrSegmentPermissionDenied = ErrorCode{211008, "Permission denied to access this segment", true}
	ErrChildChunkCreateFailed  = ErrorCode{211009, "Failed to create child chunk", true}
	ErrChildChunkUpdateFailed  = ErrorCode{211010, "Failed to update child chunk", true}
	ErrChildChunkNotFound      = ErrorCode{211011, "Child chunk not found", true}
)

var (
	ErrSystemFeatureGetFailed      = ErrorCode{212001, "Failed to get system features", true}
	ErrSetupStatusGetFailed        = ErrorCode{212002, "Failed to get setup status", true}
	ErrSelfHostedOnly              = ErrorCode{212003, "This feature is only available for self-hosted version", true}
	ErrSetupInProgress             = ErrorCode{212004, "System setup is in progress, please try again later", true}
	ErrAlreadySetup                = ErrorCode{212005, "System has already been set up", true}
	ErrTenantCountGetFailed        = ErrorCode{212006, "Failed to get tenant count", true}
	ErrInitValidateStatusGetFailed = ErrorCode{212007, "Failed to get initialization validation status", true}
	ErrNotInitValidated            = ErrorCode{212008, "System has not completed initialization validation", true}
	ErrSetupRequestInvalid         = ErrorCode{212009, "Invalid setup request parameters", true}
	ErrPasswordValidationFailed    = ErrorCode{212010, "Password validation failed", true}
	ErrSystemSetupFailed           = ErrorCode{212011, "System setup failed", true}
	ErrAuthHeaderRequired          = ErrorCode{212012, "Authentication header is missing", true}
	ErrInvalidAuthFormat           = ErrorCode{212013, "Invalid authentication format", true}
	ErrAccountNotInitialized       = ErrorCode{212014, "Account has not been initialized, please complete account initialization first", true}
	ErrConfigError                 = ErrorCode{212015, "System configuration not initialized", true}
	ErrSystemNotSetup              = ErrorCode{212016, "System has not been initialized and installed", true}
)

// ============================================================================
// ============================================================================

var (
	ErrSystemError           = ErrorCode{399001, "Internal server error", false}
	ErrServiceUnavailable    = ErrorCode{399002, "Service unavailable", false}
	ErrAccountListFailed     = ErrorCode{399003, "Failed to get account list", false}
	ErrUserRoleCheckFailed   = ErrorCode{399004, "User role check failed", false}
	ErrGroupOwnerCheckFailed = ErrorCode{399005, "Group owner check failed", false}
	ErrAccountGetFailed      = ErrorCode{399006, "Failed to get account information", false}
	ErrTokenGenerateFailed   = ErrorCode{399007, "Failed to generate login token", false}
	ErrDataTypeError         = ErrorCode{399008, "Data type error", false}
)

var (
	ErrDatabaseError   = ErrorCode{301001, "Database connection failed", false}
	ErrDatabaseTimeout = ErrorCode{301002, "Database query timeout", false}
)

var (
	ErrCacheError   = ErrorCode{302001, "Cache connection failed", false}
	ErrCacheTimeout = ErrorCode{302002, "Cache operation timeout", false}
)

var (
	ErrFileSystemError = ErrorCode{310001, "File system error", false}
	ErrFileWriteFailed = ErrorCode{310002, "Failed to write file", false}
)

// ============================================================================
// ============================================================================

var (
	ErrUnauthorized  = ErrorCode{401001, "Please login first", true}
	ErrTokenInvalid  = ErrorCode{401002, "Invalid login status, please login again", true}
	ErrTokenExpired  = ErrorCode{401003, "Login has expired, please login again", true}
	ErrLoginRequired = ErrorCode{401004, "Login required", true}
)

var (
	ErrPermissionDenied   = ErrorCode{403001, "Insufficient permissions", true}
	ErrResourceDenied     = ErrorCode{403002, "Cannot access this resource", true}
	ErrActionNotAllowed   = ErrorCode{403003, "This action is not allowed", true}
	ErrWorkspaceDenied    = ErrorCode{403004, "Cannot access this workspace", true}
	ErrSuperAdminRequired = ErrorCode{403005, "Only super administrators can perform this operation", true}
	ErrGroupOwnerRequired = ErrorCode{403006, "Only group owners or system administrators can perform this operation", true}
	ErrRegisterNotAllowed = ErrorCode{403007, "Registration is not allowed for private deployment", true}
)

var (
	ErrNotFound = ErrorCode{404001, "Requested resource not found", true}
)

// ============================================================================
// ============================================================================

var (
	ErrThirdPartyService = ErrorCode{599001, "Third-party service error", false}
)

var (
	ErrOpenAIError   = ErrorCode{501001, "AI service is temporarily unavailable, please try again later", false}
	ErrOpenAIQuota   = ErrorCode{501002, "AI service quota exhausted, please contact administrator", false}
	ErrOpenAITimeout = ErrorCode{501003, "AI service response timeout, please try again later", false}
)

var (
	ErrVectorDBError   = ErrorCode{502001, "Vector database connection failed", false}
	ErrVectorDBTimeout = ErrorCode{502002, "Vector database operation timeout", false}
)

var (
	ErrEmailError      = ErrorCode{503001, "Email service is temporarily unavailable, please try again later", false}
	ErrEmailSendFailed = ErrorCode{503002, "Failed to send email", false}
)

var (
	ErrPaymentError  = ErrorCode{504001, "Payment service is temporarily unavailable, please try again later", false}
	ErrPaymentFailed = ErrorCode{504002, "Payment failed, please try again later", false}
)

var (
	ErrPluginDebuggingKeyFailed     = ErrorCode{509001, "Failed to get debugging key", false}
	ErrPluginIconFailed             = ErrorCode{509002, "Failed to get plugin icon", false}
	ErrPluginUploadFailed           = ErrorCode{509003, "Failed to upload plugin", false}
	ErrPluginUpgradeFailed          = ErrorCode{509004, "Failed to upgrade plugin", false}
	ErrPluginFetchManifestFailed    = ErrorCode{509005, "Failed to fetch plugin manifest", false}
	ErrPluginFetchTasksFailed       = ErrorCode{509006, "Failed to fetch installation task list", false}
	ErrPluginFetchTaskFailed        = ErrorCode{509007, "Failed to fetch installation task", false}
	ErrPluginDeleteTaskFailed       = ErrorCode{509008, "Failed to delete installation task", false}
	ErrPluginDeleteAllTasksFailed   = ErrorCode{509009, "Failed to delete all installation tasks", false}
	ErrPluginDeleteTaskItemFailed   = ErrorCode{509010, "Failed to delete installation task item", false}
	ErrPluginChangePermissionFailed = ErrorCode{509011, "Failed to change plugin permission", false}
	ErrPluginFetchPermissionFailed  = ErrorCode{509012, "Failed to fetch plugin permission", false}
)

// API Key module errors (13)
var (
	ErrAPIKeyRequired     = ErrorCode{113001, "API Key cannot be empty", true}
	ErrAPIKeyInvalid      = ErrorCode{113002, "Invalid API Key format", true}
	ErrAPIKeyExpired      = ErrorCode{113003, "API Key has expired", true}
	ErrAPIKeyRevoked      = ErrorCode{113004, "API Key has been revoked", true}
	ErrAPIKeyNotFound     = ErrorCode{113005, "API Key not found", true}
	ErrAPIKeyCreateFailed = ErrorCode{113006, "Failed to create API Key", true}
	ErrAPIKeyUpdateFailed = ErrorCode{113007, "Failed to update API Key", true}
	ErrAPIKeyDeleteFailed = ErrorCode{113008, "Failed to delete API Key", true}
	ErrAPIKeyListFailed   = ErrorCode{113009, "Failed to get API Key list", true}
	ErrAPIKeyGetFailed    = ErrorCode{113010, "Failed to get API Key", true}
	ErrAPIKeyRevokeFailed = ErrorCode{113011, "Failed to revoke API Key", true}
)

// API Key business logic errors (213)
var (
	ErrAPIKeyLimitExceeded    = ErrorCode{213001, "API Key limit reached", true}
	ErrAPIKeyPermissionDenied = ErrorCode{213002, "Permission denied to access this API Key", true}
	ErrAPIKeyAlreadyExists    = ErrorCode{213003, "API Key already exists", true}
	ErrAPIKeyInvalidAgent     = ErrorCode{213004, "Invalid Agent ID", true}
	ErrAPIKeyInvalidTenant    = ErrorCode{213005, "Invalid Tenant ID", true}
)

// API Key authentication errors (413)
var (
	ErrAPIKeyAuthFailed   = ErrorCode{413001, "API Key authentication failed", true}
	ErrAPIKeyUnauthorized = ErrorCode{413002, "API Key unauthorized", true}
	ErrAPIKeyForbidden    = ErrorCode{413003, "API Key insufficient permissions", true}
)

// Virtual user authentication errors (114)
var (
	ErrInvalidVirtualUserID     = ErrorCode{114001, "Invalid virtual user ID format", true}
	ErrMigrationHeadersRequired = ErrorCode{114002, "Migration requires both Authorization and X-User-Account-Id headers", true}
	ErrSameAccountMigration     = ErrorCode{114003, "Cannot migrate user to the same account", true}
)

// User migration system errors (314)
var (
	ErrUserMigrationFailed = ErrorCode{314001, "User migration failed", false}
)

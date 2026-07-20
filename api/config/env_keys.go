// Package config loads application settings from the project's .env file.
package config

// Server and process runtime keys.
const (
	// Network listeners.
	// envServerPort sets the HTTP server port. Default: 2670.
	envServerPort = "SERVER_PORT"
	// envGRPCEnabled controls whether the gRPC server starts. Default: true.
	envGRPCEnabled = "GRPC_ENABLED"
	// envGRPCPort sets the gRPC server port. Default: 50051.
	envGRPCPort = "GRPC_PORT"

	// HTTP request limits.
	// envServerReadTimeout sets the HTTP server read timeout in seconds. Default: 60.
	envServerReadTimeout = "SERVER_READ_TIMEOUT"
	// envServerWriteTimeout sets the HTTP server write timeout in seconds. Default: 60.
	envServerWriteTimeout = "SERVER_WRITE_TIMEOUT"
	// envServerMaxHeaderBytes sets the maximum allowed size of HTTP request headers in bytes. Default: 1048576.
	envServerMaxHeaderBytes = "SERVER_MAX_HEADER_BYTES"

	// Runtime labels.
	// envServerMode sets the Gin server mode. Default: debug.
	envServerMode = "SERVER_MODE"
	// envEnv sets the runtime environment label used for development checks and Sentry reporting. Default: local.
	envEnv = "ENV"

	// Browser access policy.
	// envWebAPICORSAllowOrigins sets the allowed CORS origins for the web API. Default: empty list.
	envWebAPICORSAllowOrigins = "WEB_API_CORS_ALLOW_ORIGINS"

	// envChatRuntimeModelIdleTimeoutSeconds stops one model call after this many
	// seconds without any upstream response. Default: 300.
	envChatRuntimeModelIdleTimeoutSeconds = "CHAT_RUNTIME_MODEL_IDLE_TIMEOUT_SECONDS"
)

// Database and cache keys.
const (
	// Primary database connection.
	// envDatabaseURL sets the full database connection URL when a single DSN is used. Default: empty.
	envDatabaseURL = "DATABASE_URL"
	// envDBDriver selects the database driver. Default: postgres.
	envDBDriver = "DB_DRIVER"
	// envDBHost sets the database host. Default: localhost.
	envDBHost = "DB_HOST"
	// envDBPort sets the database port. Default: 5432.
	envDBPort = "DB_PORT"
	// envDBUsername sets the database username. Default: postgres.
	envDBUsername = "DB_USERNAME"
	// envDBPassword sets the database password. Default: empty.
	envDBPassword = "DB_PASSWORD"
	// envDBName sets the database name. Default: zgi_test.
	envDBName = "DB_NAME"
	// envDBSSLMode sets the database SSL mode. Default: disable.
	envDBSSLMode = "DB_SSLMODE"
	// envDBTimezone sets the database timezone. Default: Asia/Shanghai.
	envDBTimezone = "DB_TIMEZONE"

	// Database pool tuning.
	// envDBMaxIdleConns sets the maximum number of idle database connections. Default: 10.
	envDBMaxIdleConns = "DB_MAX_IDLE_CONNS"
	// envDBMaxOpenConns sets the maximum number of open database connections. Default: 100.
	envDBMaxOpenConns = "DB_MAX_OPEN_CONNS"
	// envDBConnMaxLifetime sets the maximum lifetime of a database connection in seconds. Default: 3600.
	envDBConnMaxLifetime = "DB_CONN_MAX_LIFETIME"
	// envDBDebugSQL controls verbose SQL logging for database diagnostics. Default: false.
	envDBDebugSQL = "DB_DEBUG_SQL"

	// Redis connection and pool.
	// envRedisHost sets the Redis host. Default: localhost.
	envRedisHost = "REDIS_HOST"
	// envRedisPort sets the Redis port. Default: 6379.
	envRedisPort = "REDIS_PORT"
	// envRedisPassword sets the Redis password. Default: empty.
	envRedisPassword = "REDIS_PASSWORD"
	// envRedisDB selects the Redis database index. Default: 0.
	envRedisDB = "REDIS_DB"
	// envRedisPoolSize sets the Redis connection pool size. Default: 10.
	envRedisPoolSize = "REDIS_POOL_SIZE"
	// envRedisMinIdleConns sets the minimum number of idle Redis connections. Default: 5.
	envRedisMinIdleConns = "REDIS_MIN_IDLE_CONNS"
)

// Core auth and platform identity keys.
const (
	// JWT signing and lifetime.
	// envAccessTokenExpireMinutes sets the access token lifetime in minutes. Default: 60.
	envAccessTokenExpireMinutes = "ACCESS_TOKEN_EXPIRE_MINUTES"
	// envSecretKey sets the shared secret used for JWT signing and related security flows. Default: empty.
	envSecretKey = "SECRET_KEY"

	// Platform run mode selects cloud or self-hosted platform behavior. Default: SELF_HOSTED.
	envZGIRunMode = "ZGI_RUN_MODE"
	// envZGIOrgInviteDefaultPassword sets the fallback password for self-hosted organization member invites. Default: ZGI@Welcome1.
	envZGIOrgInviteDefaultPassword = "ZGI_ORG_INVITE_DEFAULT_PASSWORD"
)

// Logging and diagnostics keys.
const (
	// Log output identity.
	// envLogLevel sets the application log level. Default: info in production/release, debug otherwise.
	envLogLevel = "LOG_LEVEL"
	// envLogFilename sets the log output file path. Default: logs/app.log.
	envLogFilename = "LOG_FILENAME"

	// Log rotation policy.
	// envLogMaxSize sets the maximum size of a single log file in megabytes before rotation. Default: 100.
	envLogMaxSize = "LOG_MAX_SIZE"
	// envLogMaxAge sets how many days rotated log files are kept. Default: 15.
	envLogMaxAge = "LOG_MAX_AGE"
	// envLogMaxBackups sets how many rotated log files are kept. Default: 7.
	envLogMaxBackups = "LOG_MAX_BACKUPS"
	// envLogCompress controls whether rotated log files are compressed. Default: true.
	envLogCompress = "LOG_COMPRESS"
)

// Mail and application URL keys.
const (
	// Email backend selection.
	// envEmailMailType selects the email delivery backend. Default: resend.
	envEmailMailType = "EMAIL_MAIL_TYPE"
	// envMailType keeps the legacy email backend key working during migration. Default: resend.
	envMailType = "MAIL_TYPE"

	// Resend delivery settings.
	// envEmailResendAPIKey sets the Resend API key. Default: empty.
	envEmailResendAPIKey = "EMAIL_RESEND_API_KEY"
	// envEmailResendAPIURL sets the Resend API base URL. Default: https://api.resend.com.
	envEmailResendAPIURL = "EMAIL_RESEND_API_URL"

	// SMTP delivery settings.
	// envEmailSMTPServer sets the SMTP server host. Default: smtp.gmail.com.
	envEmailSMTPServer = "EMAIL_SMTP_SERVER"
	// envEmailPort sets the SMTP port for email delivery. Default: 587.
	envEmailPort = "EMAIL_PORT"
	// envEmailSMTPUsername sets the SMTP username. Default: empty.
	envEmailSMTPUsername = "EMAIL_SMTP_USERNAME"
	// envEmailSMTPPassword sets the SMTP password. Default: empty.
	envEmailSMTPPassword = "EMAIL_SMTP_PASSWORD"
	// envEmailSMTPUseTLS controls whether SMTP connections require TLS from the start. Default: false.
	envEmailSMTPUseTLS = "EMAIL_SMTP_USE_TLS"
	// envEmailSMTPOpportunisticTLS controls whether SMTP upgrades to TLS opportunistically. Default: false.
	envEmailSMTPOpportunisticTLS = "EMAIL_SMTP_OPPORTUNISTIC_TLS"

	// Email branding and links.
	// envEmailMailDefaultSendFrom sets the default sender address for outgoing emails. Default: noreply@example.com.
	envEmailMailDefaultSendFrom = "EMAIL_MAIL_DEFAULT_SEND_FROM"
	// envEmailMailTemplateLogoURL sets the logo URL used in email templates. Default: empty.
	envEmailMailTemplateLogoURL = "EMAIL_MAIL_TEMPLATE_LOGO_URL"
	// envEmailMailTemplateBrandName sets the brand name shown in email templates. Default: ZGI.
	envEmailMailTemplateBrandName = "EMAIL_MAIL_TEMPLATE_BRAND_NAME"
	// envEmailConsoleWebURL sets the console web URL inserted into email templates. Default: http://localhost:3000.
	envEmailConsoleWebURL = "EMAIL_CONSOLE_WEB_URL"

	// Application identity and file access.
	// envAppName sets the application name. Default: ZGI-GinKit.
	envAppName = "APP_NAME"
	// envFilesAccessTimeout sets how long generated file access links remain valid in seconds. Default: 3600.
	envFilesAccessTimeout = "FILES_ACCESS_TIMEOUT"
	// envAppVersion sets the application version string. Default: 1.0.0.
	envAppVersion = "APP_VERSION"
	// envFilesURL sets the public base URL for file access. Default: http://localhost:2679.
	envFilesURL = "FILES_URL"
	// envInternalFilesURL sets the internal base URL used for file access inside the deployment. Default: empty.
	envInternalFilesURL = "INTERNAL_FILES_URL"
)

// Console routing and product feature keys.
const (
	// Console endpoints and service-to-service credentials.
	// envConsoleAPIURL sets the public base URL of the console API. Default: http://127.0.0.1:2679.
	envConsoleAPIURL = "CONSOLE_API_URL"
	// envConsoleAPIGRPCAddr sets the console gRPC endpoint address. Default: empty.
	envConsoleAPIGRPCAddr = "CONSOLE_API_GRPC_ADDR"
	// envConsoleInternalAPIKey sets the internal API key shared with the console service. Default: empty.
	envConsoleInternalAPIKey = "CONSOLE_INTERNAL_API_KEY"

	// Bootstrap credentials.
	// envZGIAdminPass sets the static admin token for self-hosted platform identity calls. Default: empty.
	envZGIAdminPass = "ZGI_ADMIN_PASS"
	// envCloudBootstrapAdminEmail sets the first local admin email used by cloud startup bootstrap. Default: empty.
	envCloudBootstrapAdminEmail = "ZGI_CLOUD_BOOTSTRAP_ADMIN_EMAIL"
	// envCloudBootstrapAdminName sets the first local admin display name used by cloud startup bootstrap. Default: empty.
	envCloudBootstrapAdminName = "ZGI_CLOUD_BOOTSTRAP_ADMIN_NAME"
	// envCloudBootstrapAdminPassword sets the first local admin password used by cloud startup bootstrap. Default: empty.
	envCloudBootstrapAdminPassword = "ZGI_CLOUD_BOOTSTRAP_ADMIN_PASSWORD"

	// Product feature exposure.
	// envEnterpriseEnabled exposes enterprise-only system features in self-hosted mode. Default: false.
	envEnterpriseEnabled = "ENTERPRISE_ENABLED"
	// envMarketplaceEnabled exposes marketplace features to the frontend. Default: true.
	envMarketplaceEnabled = "MARKETPLACE_ENABLED"
	// envPluginMaxPackageSize overrides the plugin package size returned by the system feature API. Default: 0.
	envPluginMaxPackageSize = "PLUGIN_MAX_PACKAGE_SIZE"

	// LLM policy prompt injection. Disabled by default for open-source deployments.
	envLLMPolicyPromptEnabled = "LLM_POLICY_PROMPT_ENABLED"
	// envLLMPolicyPromptFile points to a local file containing the policy prompt.
	envLLMPolicyPromptFile = "LLM_POLICY_PROMPT_FILE"
	// envLLMPolicyPromptText carries the policy prompt directly and takes precedence over the file.
	envLLMPolicyPromptText = "LLM_POLICY_PROMPT_TEXT"

	// Public access and signup flow.
	// envPublicDeploymentEnabled creates a personal organization when public signup activates an account. Default: false.
	envPublicDeploymentEnabled = "PUBLIC_DEPLOYMENT_ENABLED"
	// envAllowRegister allows users to create accounts without an administrator invite. Default: false.
	envAllowRegister = "ALLOW_REGISTER"
	// envAllowCreateWorkspace allows users to create workspaces from the product UI. Default: false.
	envAllowCreateWorkspace = "ALLOW_CREATE_WORKSPACE"

	// Login entry points.
	// envEnableEmailCodeLogin controls whether users can log in with an email verification code. Default: false.
	envEnableEmailCodeLogin = "ENABLE_EMAIL_CODE_LOGIN"
	// envEnableEmailPasswordLogin controls whether users can log in with email and password. Default: true.
	envEnableEmailPasswordLogin = "ENABLE_EMAIL_PASSWORD_LOGIN"
	// envEnablePhoneLogin controls whether the phone login entry is available. Default: false.
	envEnablePhoneLogin = "ENABLE_PHONE_LOGIN"
	// envEnableSocialOAuthLogin controls whether social OAuth login is enabled. Default: false.
	envEnableSocialOAuthLogin = "ENABLE_SOCIAL_OAUTH_LOGIN"
)

// Background workers and retrieval infrastructure keys.
const (
	// Plugin runner integration.
	// envPluginRunnerEnabled controls whether the plugin runner integration is enabled. Default: false.
	envPluginRunnerEnabled = "PLUGIN_RUNNER_ENABLED"
	// envPluginRunnerURL sets the plugin runner base URL. Default: http://localhost:2665.
	envPluginRunnerURL = "PLUGIN_RUNNER_URL"
	// envPluginRunnerAPIKey sets the plugin runner API key. Default: empty.
	envPluginRunnerAPIKey = "PLUGIN_RUNNER_API_KEY"
	// envPluginRunnerTimeout sets the plugin runner request timeout in seconds. Default: 30.
	envPluginRunnerTimeout = "PLUGIN_RUNNER_TIMEOUT"

	// Task queue backend and worker policy.
	// The task queue reuses the primary Redis host, port, and password.
	// envTaskQueueRedisDB selects the Redis database index used by the task queue. Default: 0.
	envTaskQueueRedisDB = "TASK_QUEUE_REDIS_DB"
	// envTaskQueueConcurrency sets how many task queue workers run in parallel. Default: 4.
	envTaskQueueConcurrency = "TASK_QUEUE_CONCURRENCY"
	// envTaskQueueRetention sets how long completed task metadata is retained. Default: 24h.
	envTaskQueueRetention = "TASK_QUEUE_RETENTION"
	// envTaskQueueEnvPrefix sets the environment prefix used to isolate task queue keys. Default: empty.
	envTaskQueueEnvPrefix = "TASK_QUEUE_ENV_PREFIX"
	// envWorkflowTestTaskBackend selects how workflow test AI tasks are executed. Values: local, asynq. Default: local.
	envWorkflowTestTaskBackend = "WORKFLOW_TEST_TASK_BACKEND"

	// Vector store backend selection.
	// envVectorStore selects the vector store backend. Default: weaviate.
	envVectorStore = "VECTOR_STORE"

	// Weaviate connection.
	// envWeaviateEndpoint sets the Weaviate endpoint. Default: empty.
	envWeaviateEndpoint = "WEAVIATE_ENDPOINT"
	// envWeaviateAPIKey sets the Weaviate API key. Default: empty.
	envWeaviateAPIKey = "WEAVIATE_API_KEY"
	// envWeaviateGRPCEnabled controls whether the Weaviate gRPC client is enabled. Default: false.
	envWeaviateGRPCEnabled = "WEAVIATE_GRPC_ENABLED"

	// Indexing throughput controls.
	// envIndexingBatchSize sets the indexing batch size used for embedding and vector writes. Default: 4.
	envIndexingBatchSize = "INDEXING_BATCH_SIZE"
	// envIndexingMaxSegmentationTokensLength sets the token limit for each indexing segment. Default: 4000.
	envIndexingMaxSegmentationTokensLength = "INDEXING_MAX_SEGMENTATION_TOKENS_LENGTH"
	// envIndexingMaxWorkers sets how many indexing workers run in parallel. Default: 4.
	envIndexingMaxWorkers = "INDEXING_MAX_WORKERS"
	// envIndexingTimeoutMinutes sets the indexing timeout in minutes. Default: 60.
	envIndexingTimeoutMinutes = "INDEXING_TIMEOUT_MINUTES"

	// Keyword retrieval backend.
	// envKeywordDataSourceType selects the keyword data source backend. Default: database.
	envKeywordDataSourceType = "KEYWORD_DATA_SOURCE_TYPE"
)

// Upload, ETL, and code execution keys.
const (
	// Upload limits.
	// envUploadFileSizeLimit sets the maximum upload size for general files in megabytes. Default: 15.
	envUploadFileSizeLimit = "UPLOAD_FILE_SIZE_LIMIT"
	// envUploadFileBatchLimit sets how many files can be uploaded in one batch. Default: 5.
	envUploadFileBatchLimit = "UPLOAD_FILE_BATCH_LIMIT"
	// envUploadImageFileSizeLimit sets the maximum upload size for image files in megabytes. Default: 10.
	envUploadImageFileSizeLimit = "UPLOAD_IMAGE_FILE_SIZE_LIMIT"
	// envUploadVideoFileSizeLimit sets the maximum upload size for video files in megabytes. Default: 100.
	envUploadVideoFileSizeLimit = "UPLOAD_VIDEO_FILE_SIZE_LIMIT"
	// envUploadAudioFileSizeLimit sets the maximum upload size for audio files in megabytes. Default: 50.
	envUploadAudioFileSizeLimit = "UPLOAD_AUDIO_FILE_SIZE_LIMIT"
	// envBatchUploadLimit sets the maximum number of batch upload jobs accepted at once. Default: 10.
	envBatchUploadLimit = "BATCH_UPLOAD_LIMIT"
	// envWorkflowFileUploadLimit sets how many files a workflow request can upload. Default: 10.
	envWorkflowFileUploadLimit = "WORKFLOW_FILE_UPLOAD_LIMIT"

	// Default tenant quota.
	// envEnterpriseStorageQuotaGB sets the total storage quota reported for each organization in gigabytes. Default: 20.
	envEnterpriseStorageQuotaGB = "ENTERPRISE_STORAGE_QUOTA_GB"

	// ETL backend selection.
	// envETLType selects the ETL backend. Default: zgi.
	envETLType = "ETL_TYPE"

	// ETL provider credentials.
	// envUnstructuredAPIURL sets the Unstructured API base URL. Default: empty.
	envUnstructuredAPIURL = "UNSTRUCTURED_API_URL"
	// envUnstructuredAPIKey sets the Unstructured API key. Default: empty.
	envUnstructuredAPIKey = "UNSTRUCTURED_API_KEY"
	// envLandingAIAPIKey sets the LandingAI API key. Default: empty.
	envLandingAIAPIKey = "LANDINGAI_API_KEY"
	// envReductoAPIKey sets the Reducto API key. Default: empty.
	envReductoAPIKey = "REDUCTO_API_KEY"
	// envETLHyperparseEnabled toggles the Hyperparse native PDF extractor used by Mixed/Hyperparse modes. Default: true.
	envETLHyperparseEnabled = "ETL_HYPERPARSE_ENABLED"
	// envETLHyperparseBackend selects Hyperparse SDK engine (local|mineru|reducto). Default: mineru.
	// Legacy direct vlm remains accepted for compatibility, but product VLM routing uses the system default vision model.
	envETLHyperparseBackend = "ETL_HYPERPARSE_BACKEND"
	// envHyperparseBackend is the upstream SDK-compatible backend env key kept for legacy deployments.
	envHyperparseBackend = "HYPERPARSE_BACKEND"
	// envETLHyperparseMode selects the Hyperparse parser mode (relaxed|strict). Default: relaxed.
	envETLHyperparseMode = "ETL_HYPERPARSE_MODE"

	// Remote code execution service.
	// envCodeExecutionEndpoint sets the remote code execution service endpoint. Default: empty.
	envCodeExecutionEndpoint = "CODE_EXECUTION_ENDPOINT"
	// envCodeExecutionAPIKey sets the API key for the remote code execution service. Default: empty.
	envCodeExecutionAPIKey = "CODE_EXECUTION_API_KEY"
	// envCodeExecutionConnectTimeout sets the sandbox adapter connect timeout in seconds. Default: 5.
	envCodeExecutionConnectTimeout = "CODE_EXECUTION_CONNECT_TIMEOUT_SECONDS"
	// envCodeExecutionCreateTimeout sets the sandbox creation request timeout in seconds. Default: 10.
	envCodeExecutionCreateTimeout = "CODE_EXECUTION_CREATE_TIMEOUT_SECONDS"
	// envCodeExecutionUploadTimeout sets the sandbox archive upload request timeout in seconds. Default: 30.
	envCodeExecutionUploadTimeout = "CODE_EXECUTION_UPLOAD_TIMEOUT_SECONDS"
	// envCodeExecutionCommandTimeoutPadding sets extra HTTP time allowed beyond the skill command timeout in seconds. Default: 15.
	envCodeExecutionCommandTimeoutPadding = "CODE_EXECUTION_COMMAND_TIMEOUT_PADDING_SECONDS"
	// envCodeExecutionArtifactTimeout sets artifact list and download request timeout in seconds. Default: 10.
	envCodeExecutionArtifactTimeout = "CODE_EXECUTION_ARTIFACT_TIMEOUT_SECONDS"
	// envCodeExecutionCleanupTimeout sets sandbox cleanup request timeout in seconds. Default: 5.
	envCodeExecutionCleanupTimeout = "CODE_EXECUTION_CLEANUP_TIMEOUT_SECONDS"
	// envCodeExecutionEnableNetwork toggles network access for the legacy /v1/sandbox/run workflow code path. Default: false.
	envCodeExecutionEnableNetwork = "CODE_EXECUTION_ENABLE_NETWORK"
	// envCodeExecutionSystemOfficeProfile sets the managed dependency profile used by system Office/PDF/PPTX file tools. Default: skill-office.
	envCodeExecutionSystemOfficeProfile = "CODE_EXECUTION_SYSTEM_OFFICE_PROFILE"

	// Code execution safety limits.
	// envCodeMaxNumber sets the largest numeric value allowed in code execution. Default: 9223372036854775807.
	envCodeMaxNumber = "CODE_MAX_NUMBER"
	// envCodeMinNumber sets the smallest numeric value allowed in code execution. Default: -9223372036854775808.
	envCodeMinNumber = "CODE_MIN_NUMBER"
	// envCodeMaxStringLength sets the maximum string length allowed in code execution. Default: 80000.
	envCodeMaxStringLength = "CODE_MAX_STRING_LENGTH"
)

// Workflow runtime, file handling, and encryption keys.
const (
	// Workflow runtime deadlines.
	// envWorkflowExecutionTimeout sets the workflow execution timeout in seconds. Default: 300.
	envWorkflowExecutionTimeout = "WORKFLOW_EXECUTION_TIMEOUT"
	// envWorkflowLLMTimeout sets the LLM call timeout inside workflows in seconds. Default: 120.
	envWorkflowLLMTimeout = "WORKFLOW_LLM_TIMEOUT"
	// envWorkflowHeartbeatInterval sets the workflow heartbeat interval in seconds. Default: 5.
	envWorkflowHeartbeatInterval = "WORKFLOW_HEARTBEAT_INTERVAL"
	// envWorkflowCleanupTimeout sets the workflow cleanup timeout in seconds. Default: 30.
	envWorkflowCleanupTimeout = "WORKFLOW_CLEANUP_TIMEOUT"
	// envWorkflowImageInputURLMode controls URLs passed to LLM vision for workflow image inputs.
	// Accepted values: zgi_proxy | public_storage_url. Default: zgi_proxy.
	envWorkflowImageInputURLMode = "WORKFLOW_IMAGE_INPUT_URL_MODE"
	// envWorkflowImageInputPublicBaseURL sets the optional public storage/CDN base URL for workflow image inputs.
	envWorkflowImageInputPublicBaseURL = "WORKFLOW_IMAGE_INPUT_PUBLIC_BASE_URL"

	// Workflow file extraction behavior.
	// envWorkflowFileExtractionEnabled controls whether workflow file extraction is enabled. Default: true.
	envWorkflowFileExtractionEnabled = "WORKFLOW_FILE_EXTRACTION_ENABLED"
	// envWorkflowFileExtractionMaxContentSize sets the maximum extracted workflow file content size in bytes. Default: 1048576.
	envWorkflowFileExtractionMaxContentSize = "WORKFLOW_FILE_EXTRACTION_MAX_CONTENT_SIZE"
	// envWorkflowFileExtractionTimeout sets the workflow file extraction timeout in seconds. Default: 120.
	envWorkflowFileExtractionTimeout = "WORKFLOW_FILE_EXTRACTION_TIMEOUT"
	// envWorkflowFileExtractionCacheEnabled controls whether workflow file extraction results are cached. Default: true.
	envWorkflowFileExtractionCacheEnabled = "WORKFLOW_FILE_EXTRACTION_CACHE_ENABLED"
	// envWorkflowFileExtractionStrategy sets the extraction strategy for workflow file uploads.
	// Accepted values: mineru | local | reducto | unstructured | landingai | "" (auto).
	// When empty the strategy is inherited from ETL_HYPERPARSE_BACKEND when ETL_HYPERPARSE_ENABLED=true,
	// falling back to the built-in default extractor if Hyperparse is disabled.
	envWorkflowFileExtractionStrategy = "WORKFLOW_FILE_EXTRACTION_STRATEGY"

	// Content parse shadow integration behavior.
	// envContentParseShadowDatasetIndexingEnabled controls whether dataset indexing
	// should run a best-effort content parse shadow pass for comparison only.
	envContentParseShadowDatasetIndexingEnabled = "CONTENT_PARSE_SHADOW_DATASET_INDEXING_ENABLED"

	// Answer node streaming behavior.
	// envAnswerNodeStreamingChunkSize sets the chunk size used by answer node streaming. Default: 20.
	envAnswerNodeStreamingChunkSize = "ANSWER_NODE_STREAMING_CHUNK_SIZE"

	// Stored secret protection.
	// envAPIKeyEncryptionKey sets the 32-byte AES key used for stored API keys outside the LLM module. Default: empty.
	envAPIKeyEncryptionKey = "API_KEY_ENCRYPTION_KEY"
	// envLLMCredentialSecretKey keeps the legacy LLM credential secret key working as a fallback. Default: empty.
	envLLMCredentialSecretKey = "LLM_CREDENTIAL_SECRET_KEY"
)

// Observability and dependency service keys.
const (
	// Observability providers.
	// envZGIReporters selects comma-separated ZGI Reporter adapters.
	// Supported built-ins: sentry, otel. Empty auto-detects configured providers;
	// "none" disables all reporters.
	envZGIReporters = "ZGI_REPORTERS"
	// envOTELEnabled controls whether OpenTelemetry tracing is enabled. Default: false.
	envOTELEnabled = "OTEL_ENABLED"
	// envOTELServiceName sets the OpenTelemetry service name. Default: zgi-api.
	envOTELServiceName = "OTEL_SERVICE_NAME"
	// envOTELExporterOTLPEndpoint sets the OTLP collector endpoint. Default: empty.
	envOTELExporterOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	// envOTELExporterOTLPTracesEndpoint sets the signal-specific OTLP trace endpoint.
	envOTELExporterOTLPTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	// envOTELExporterOTLPProtocol sets the OTLP protocol. Only http/protobuf is supported by this service.
	envOTELExporterOTLPProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"
	// envOTELTracesSampleRate controls how many traces are sampled and exported. Default: 1.0.
	envOTELTracesSampleRate = "OTEL_TRACES_SAMPLE_RATE"
	// envOTELExporterOTLPHeaders sets OTLP HTTP headers as comma-separated key=value pairs.
	envOTELExporterOTLPHeaders = "OTEL_EXPORTER_OTLP_HEADERS"
	// envOTELExporterOTLPTracesHeaders sets trace-specific OTLP HTTP headers.
	envOTELExporterOTLPTracesHeaders = "OTEL_EXPORTER_OTLP_TRACES_HEADERS"
	// envOTELTracesExporter follows the standard selector. Value "none" disables tracing.
	envOTELTracesExporter = "OTEL_TRACES_EXPORTER"
	// envOTELInstrumentHTTPClient controls outbound HTTP client tracing. Default: true.
	envOTELInstrumentHTTPClient = "OTEL_INSTRUMENT_HTTP_CLIENT"
	// envOTELInstrumentWorkflow controls workflow execution and node tracing. Default: true.
	envOTELInstrumentWorkflow = "OTEL_INSTRUMENT_WORKFLOW"
	// envOTELInstrumentDB controls database tracing. Default: false.
	envOTELInstrumentDB = "OTEL_INSTRUMENT_DB"
	// envOTELInstrumentRedis controls Redis tracing. Default: false.
	envOTELInstrumentRedis = "OTEL_INSTRUMENT_REDIS"
	// envOTELInstrumentGRPC controls gRPC client/server tracing. Default: false.
	envOTELInstrumentGRPC = "OTEL_INSTRUMENT_GRPC"
	// envOTELLLMLangfuseAttributes controls Langfuse-specific LLM trace attributes. Default: true.
	envOTELLLMLangfuseAttributes = "OTEL_LLM_LANGFUSE_ATTRIBUTES"
	// envOTELLLMCaptureContent controls LLM input/output capture: none, summary, or full. Default: summary.
	envOTELLLMCaptureContent = "OTEL_LLM_CAPTURE_CONTENT"
	// envOTELLLMCaptureMaxChars caps serialized LLM input/output attributes. Default: 65536.
	envOTELLLMCaptureMaxChars = "OTEL_LLM_CAPTURE_MAX_CHARS"
	// envLangfuseEnabled switches the OpenTelemetry exporter to Langfuse direct ingest when keys are present.
	envLangfuseEnabled = "LANGFUSE_ENABLED"
	// envLangfusePublicKey sets the Langfuse project public key.
	envLangfusePublicKey = "LANGFUSE_PUBLIC_KEY"
	// envLangfuseSecretKey sets the Langfuse project secret key.
	envLangfuseSecretKey = "LANGFUSE_SECRET_KEY"
	// envLangfuseBaseURL sets the Langfuse base URL, for example https://cloud.langfuse.com.
	envLangfuseBaseURL = "LANGFUSE_BASE_URL"
	// envLangfuseHost is the Langfuse SDK-compatible base URL alias.
	envLangfuseHost = "LANGFUSE_HOST"
	// envLangfuseOTELEndpoint sets the Langfuse OTLP base endpoint.
	envLangfuseOTELEndpoint = "LANGFUSE_OTEL_ENDPOINT"
	// envLangfuseAuthString sets base64("<public_key>:<secret_key>").
	envLangfuseAuthString = "LANGFUSE_AUTH_STRING"
	// envSentryDSN sets the Sentry DSN. Default: empty.
	envSentryDSN = "SENTRY_DSN"
	// envSentryEnvironment overrides the Sentry environment label. Default: ENV.
	envSentryEnvironment = "SENTRY_ENVIRONMENT"

	// Dependency services.
	// envModelMetaAPIURL sets the ModelMeta-compatible API base URL. Default: https://models.zgi.ai.
	envModelMetaAPIURL = "MODELMETA_API_URL"
	// envNeo4jURI sets the Neo4j connection URI. Default: empty, which disables GraphFlow Neo4j integration.
	envNeo4jURI = "NEO4J_URI"
	// envNeo4jUsername sets the Neo4j username. Default: neo4j.
	envNeo4jUsername = "NEO4J_USERNAME"
	// envNeo4jPassword sets the Neo4j password. Default: empty.
	envNeo4jPassword = "NEO4J_PASSWORD"
	// envNeo4jDatabase sets the Neo4j database name used by GraphFlow. Default: neo4j.
	envNeo4jDatabase = "NEO4J_DATABASE"
)

// File storage, marketplace, and SQL access keys.
const (
	// Storage backend selection.
	// envStorageType selects the file storage backend. Default: local.
	envStorageType = "STORAGE_TYPE"
	// envOpenDALBasePath sets the local base path used by the OpenDAL storage backend. Default: ./storage/opendal.
	envOpenDALBasePath = "OPENDAL_BASE_PATH"

	// Aliyun OSS backend.
	// envAliyunOSSEndpoint sets the Aliyun OSS endpoint. Default: empty.
	envAliyunOSSEndpoint = "ALIYUN_OSS_ENDPOINT"
	// envAliyunOSSBucketName sets the Aliyun OSS bucket name. Default: empty.
	envAliyunOSSBucketName = "ALIYUN_OSS_BUCKET_NAME"
	// envAliyunOSSPath sets the path prefix used inside the Aliyun OSS bucket. Default: empty.
	envAliyunOSSPath = "ALIYUN_OSS_PATH"
	// envAliyunOSSAccessKey sets the Aliyun OSS access key. Default: empty.
	envAliyunOSSAccessKey = "ALIYUN_OSS_ACCESS_KEY"
	// envAliyunOSSSecretKey sets the Aliyun OSS secret key. Default: empty.
	envAliyunOSSSecretKey = "ALIYUN_OSS_SECRET_KEY"
	// envAliyunOSSAuthVersion sets the Aliyun OSS authentication version. Default: empty.
	envAliyunOSSAuthVersion = "ALIYUN_OSS_AUTH_VERSION"
	// envAliyunOSSRegion sets the Aliyun OSS region. Default: empty.
	envAliyunOSSRegion = "ALIYUN_OSS_REGION"

	// Qiniu backend.
	// envQiniuAccessKey sets the Qiniu access key. Default: empty.
	envQiniuAccessKey = "QINIU_ACCESS_KEY"
	// envQiniuSecretKey sets the Qiniu secret key. Default: empty.
	envQiniuSecretKey = "QINIU_SECRET_KEY"
	// envQiniuBucketName sets the Qiniu bucket name. Default: empty.
	envQiniuBucketName = "QINIU_BUCKET_NAME"
	// envQiniuDomain sets the public Qiniu domain. Default: empty.
	envQiniuDomain = "QINIU_DOMAIN"
	// envQiniuZone sets the Qiniu storage zone. Default: empty.
	envQiniuZone = "QINIU_ZONE"
	// envQiniuUseHTTPS controls whether Qiniu URLs use HTTPS. Default: false.
	envQiniuUseHTTPS = "QINIU_USE_HTTPS"
	// envQiniuPath sets the path prefix used inside Qiniu storage. Default: empty.
	envQiniuPath = "QINIU_PATH"

	// S3-compatible backend.
	// envS3AccessKey sets the S3 access key. Default: empty.
	envS3AccessKey = "S3_ACCESS_KEY"
	// envS3SecretKey sets the S3 secret key. Default: empty.
	envS3SecretKey = "S3_SECRET_KEY"
	// envS3Region sets the S3 region. Default: empty.
	envS3Region = "S3_REGION"
	// envS3BucketName sets the S3 bucket name. Default: empty.
	envS3BucketName = "S3_BUCKET_NAME"
	// envS3Endpoint sets the S3 endpoint. Default: empty.
	envS3Endpoint = "S3_ENDPOINT"
	// envS3ForcePathStyle controls whether the S3 client uses path-style addressing. Default: false.
	envS3ForcePathStyle = "S3_FORCE_PATH_STYLE"
	// envS3DisableSSL controls whether SSL is disabled for S3 requests. Default: false.
	envS3DisableSSL = "S3_DISABLE_SSL"
	// envS3Path sets the path prefix used inside the S3 bucket. Default: empty.
	envS3Path = "S3_PATH"

	// Marketplace endpoints.
	// envMarketplaceAPIURL sets the marketplace API base URL. Default: https://market.zgiai.com.
	envMarketplaceAPIURL = "MARKETPLACE_API_URL"
	// envMarketplaceSource sets the marketplace source identifier. Default: zgi.
	envMarketplaceSource = "MARKETPLACE_SOURCE"
	// envMarketplaceBaseURL sets the marketplace web base URL. Default: http://localhost:8025.
	envMarketplaceBaseURL = "MARKETPLACE_BASE_URL"

	// Internal SQL base service.
	// envSQLBaseType selects which SQL base backend implementation is used. Default: internal.
	envSQLBaseType = "SQL_BASE_TYPE"
	// Internal SQL base reuses the primary database host, port, user, and password.
	// envSQLBaseInternalDB sets the database name used by the internal SQL base service. Default: empty.
	envSQLBaseInternalDB = "SQL_BASE_INTERNAL_DB"

	// External SQL base service.
	// envSQLBaseExternalURL sets the external SQL base service URL. Default: empty.
	envSQLBaseExternalURL = "SQL_BASE_EXTERNAL_URL"
	// envSQLBaseExternalAPIKey sets the external SQL base service API key. Default: empty.
	envSQLBaseExternalAPIKey = "SQL_BASE_EXTERNAL_API_KEY"

	// Postgres metadata service.
	// envPostgresMetaBaseURL sets the Postgres metadata service base URL. Default: empty.
	envPostgresMetaBaseURL = "POSTGRES_META_BASE_URL"
)

// Identity provider and bootstrap keys.
const (
	// Bootstrap secrets.
	// envMasterVerificationCode sets the fallback verification code for privileged account flows. Default: empty.
	envMasterVerificationCode = "MASTER_VERIFICATION_CODE"

	// Casdoor OAuth integration.
	// envCasdoorHost sets the Casdoor host. Default: empty.
	envCasdoorHost = "CASDOOR_HOST"
	// envCasdoorClientID sets the Casdoor client ID. Default: empty.
	envCasdoorClientID = "CASDOOR_CLIENT_ID"
	// envCasdoorClientSecret sets the Casdoor client secret. Default: empty.
	envCasdoorClientSecret = "CASDOOR_CLIENT_SECRET"
	// envCasdoorRedirectURI sets the Casdoor OAuth redirect URI. Default: empty.
	envCasdoorRedirectURI = "CASDOOR_REDIRECT_URI"
	// envCasdoorScopes sets the OAuth scopes requested from Casdoor. Default: openid, profile, email.
	envCasdoorScopes = "CASDOOR_SCOPES"

	// Frontend callback routing.
	// envSSOFrontendCallbackURL sets the frontend callback URL used after SSO login. Default: empty.
	envSSOFrontendCallbackURL = "SSO_FRONTEND_CALLBACK_URL"
	// envSSOFrontendCallbackURLPrefix sets named frontend callback URLs for SSO login.
	envSSOFrontendCallbackURLPrefix = "SSO_FRONTEND_CALLBACK_URL_"
)

// Domain behavior and toolchain keys.
const (
	// Knowledge retrieval limit switch.
	// envKnowledgeRateLimitEnabled controls whether knowledge retrieval rate limiting is enabled. Default: false.
	envKnowledgeRateLimitEnabled = "KNOWLEDGE_RATE_LIMIT_ENABLED"

	// Knowledge API throttling.
	// envKnowledgeRateLimitWindowMS sets the knowledge API rate-limit window in milliseconds. Default: 60000.
	envKnowledgeRateLimitWindowMS = "KNOWLEDGE_RATE_LIMIT_WINDOW_MS"
	// envKnowledgeRateLimitMax sets the maximum number of knowledge API requests allowed per window. Default: 60.
	envKnowledgeRateLimitMax = "KNOWLEDGE_RATE_LIMIT_MAX"

	// GraphFlow sync throughput.
	// envGraphFlowVectorSyncBatchSize sets the GraphFlow vector sync batch size. Default: 50.
	envGraphFlowVectorSyncBatchSize = "GRAPHFLOW_VECTOR_SYNC_BATCH_SIZE"
	// envGraphFlowVectorSyncConcurrency sets how many GraphFlow vector sync jobs run in parallel. Default: 10.
	envGraphFlowVectorSyncConcurrency = "GRAPHFLOW_VECTOR_SYNC_CONCURRENCY"

	// LLM platform controls.
	// envOfficialModelSyncStrictMode rejects empty or suspiciously shrinking official model snapshots. Default: false.
	envOfficialModelSyncStrictMode = "OFFICIAL_MODEL_SYNC_STRICT_MODE"
	// envLLMEncryptionKey sets the AES key used for LLM provider credentials. Default: empty.
	envLLMEncryptionKey = "LLM_ENCRYPTION_KEY"
	// envLLMGuardOutboundURL controls literal outbound URL safety checks. Default: true.
	envLLMGuardOutboundURL = "LLM_GUARD_OUTBOUND_URL"
	// envLLMGuardOutboundDNS controls DNS-resolved outbound address checks. Default: false.
	envLLMGuardOutboundDNS = "LLM_GUARD_OUTBOUND_DNS"
	// envLLMAllowPrivateBaseURL allows Ollama to target private or localhost base URLs. Default: false.
	envLLMAllowPrivateBaseURL = "LLM_ALLOW_PRIVATE_BASE_URL"
	// envLLMUpstreamBalancePollingEnabled controls private credential balance polling. Default: false.
	envLLMUpstreamBalancePollingEnabled = "LLM_UPSTREAM_BALANCE_POLLING_ENABLED"
	// envLLMUpstreamGuardMode controls private credential route protection. Default: off.
	envLLMUpstreamGuardMode = "LLM_UPSTREAM_GUARD_MODE"
	// envLLMUpstreamGuardPercentage controls deterministic organization rollout. Default: 0.
	envLLMUpstreamGuardPercentage = "LLM_UPSTREAM_GUARD_PERCENTAGE"

	// Automation dispatch behavior.
	// envAutomationDispatchEnabled controls whether this API instance registers automation due-task dispatch. Default: true.
	envAutomationDispatchEnabled = "AUTOMATION_DISPATCH_ENABLED"

	// Command-line tool behavior.
	// envDryRun controls whether startup commands run in dry-run mode. Default: false.
	envDryRun = "DRY_RUN"
)

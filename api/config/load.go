package config

import (
	"encoding/base64"
	"fmt"
	"runtime"
	"strings"
	"time"
)

type configLoader struct {
	name string
	load func(*Config, *envSource) error
}

func Load() (*Config, error) {
	source, err := newDefaultEnvSource()
	if err != nil {
		return nil, err
	}
	return loadFromSource(source)
}

func LoadFromFile(path string) (*Config, error) {
	source, err := newEnvSource(path)
	if err != nil {
		return nil, err
	}
	return loadFromSource(source)
}

func loadFromSource(source *envSource) (*Config, error) {
	cfg := &Config{source: source}

	// Load the baseline runtime first because most packages depend on these fields.
	if err := loadCoreRuntimeConfig(cfg, source); err != nil {
		return nil, err
	}

	// Load infrastructure adapters and external service endpoints next.
	if err := loadInfrastructureConfig(cfg, source); err != nil {
		return nil, err
	}

	// Load workflow and security controls before higher-level product behavior.
	if err := loadExecutionAndSecurityConfig(cfg, source); err != nil {
		return nil, err
	}

	// Load product-facing integrations and domain switches last.
	if err := loadDomainConfig(cfg, source); err != nil {
		return nil, err
	}

	// Validate the assembled view once all config sections are present.
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	GlobalConfig = cfg
	return cfg, nil
}

func loadCoreRuntimeConfig(cfg *Config, source *envSource) error {
	return runConfigLoaders(cfg, source,
		requiredConfigLoader("server", loadServerConfig),
		requiredConfigLoader("database", loadDatabaseConfig),
		requiredConfigLoader("redis", loadRedisConfig),
		requiredConfigLoader("jwt", loadJWTConfig),
		requiredConfigLoader("log", loadLogConfig),

		optionalConfigLoader("console", loadConsoleConfig),
		optionalConfigLoader("platform", loadPlatformConfig),
		optionalConfigLoader("feature", loadFeatureConfig),
	)
}

func loadInfrastructureConfig(cfg *Config, source *envSource) error {
	return runConfigLoaders(cfg, source,
		requiredConfigLoader("email", loadEmailConfig),
		requiredConfigLoader("app", loadAppConfig),

		requiredConfigLoader("plugin runner", loadPluginRunnerConfig),
		requiredConfigLoader("task queue", loadTaskQueueConfig),
		requiredConfigLoader("vector store", loadVectorStoreConfig),
		requiredConfigLoader("upload", loadUploadConfig),

		requiredConfigLoader("etl", loadETLConfig),
		requiredConfigLoader("code execution", loadCodeExecConfig),
		requiredConfigLoader("storage", loadStorageConfig),
		requiredConfigLoader("sql base", loadSQLBaseConfig),
	)
}

func loadExecutionAndSecurityConfig(cfg *Config, source *envSource) error {
	return runConfigLoaders(cfg, source,
		requiredConfigLoader("workflow", loadWorkflowConfig),
		requiredConfigLoader("workflow file extraction", loadWorkflowFileExtractionConfig),
		requiredConfigLoader("answer node streaming", loadAnswerNodeStreamingConfig),
		requiredConfigLoader("encryption", loadEncryptionConfig),
		requiredConfigLoader("llm policy prompt", loadLLMPolicyPromptConfig),

		optionalConfigLoader("auth", loadAuthConfig),
		optionalConfigLoader("opentelemetry", loadOpenTelemetryConfig),
		optionalConfigLoader("sentry", loadSentryConfig),
	)
}

func loadDomainConfig(cfg *Config, source *envSource) error {
	return runConfigLoaders(cfg, source,
		optionalConfigLoader("model meta", loadModelMetaConfig),
		optionalConfigLoader("neo4j", loadNeo4jConfig),
		optionalConfigLoader("marketplace", loadMarketplaceConfig),

		optionalConfigLoader("knowledge", loadKnowledgeConfig),
		optionalConfigLoader("graph flow", loadGraphFlowConfig),
		optionalConfigLoader("content parse", loadContentParseConfig),
		optionalConfigLoader("llm", loadLLMConfig),
		optionalConfigLoader("automation", loadAutomationConfig),
		optionalConfigLoader("tooling", loadToolingConfig),
	)
}

func runConfigLoaders(cfg *Config, source *envSource, loaders ...configLoader) error {
	for _, loader := range loaders {
		if err := loader.load(cfg, source); err != nil {
			return fmt.Errorf("load %s config: %w", loader.name, err)
		}
	}
	return nil
}

func requiredConfigLoader(name string, load func(*Config, *envSource) error) configLoader {
	return configLoader{name: name, load: load}
}

func optionalConfigLoader(name string, load func(*Config, *envSource)) configLoader {
	return configLoader{
		name: name,
		load: func(cfg *Config, source *envSource) error {
			load(cfg, source)
			return nil
		},
	}
}

func loadServerConfig(cfg *Config, source *envSource) error {
	port, err := source.int(defaultServerPort, envServerPort)
	if err != nil {
		return err
	}

	grpcEnabled, err := source.bool(true, envGRPCEnabled)
	if err != nil {
		return err
	}

	grpcPort := defaultGRPCPort
	if grpcEnabled {
		grpcPort, err = source.int(defaultGRPCPort, envGRPCPort)
		if err != nil {
			return err
		}
	}

	readTimeout, err := source.int(60, envServerReadTimeout)
	if err != nil {
		return err
	}
	writeTimeout, err := source.int(60, envServerWriteTimeout)
	if err != nil {
		return err
	}
	maxHeaderBytes, err := source.int(1048576, envServerMaxHeaderBytes)
	if err != nil {
		return err
	}

	cfg.Server = ServerConfig{
		Port:             port,
		GRPCPort:         grpcPort,
		GRPCEnabled:      grpcEnabled,
		Mode:             source.string("debug", envServerMode),
		Environment:      source.string("local", envEnv),
		ReadTimeout:      readTimeout,
		WriteTimeout:     writeTimeout,
		MaxHeaderBytes:   maxHeaderBytes,
		CORSAllowOrigins: source.csv(nil, envWebAPICORSAllowOrigins),
	}
	return nil
}

func loadDatabaseConfig(cfg *Config, source *envSource) error {
	port, err := source.int(5432, envDBPort)
	if err != nil {
		return err
	}
	maxIdleConns, err := source.int(10, envDBMaxIdleConns)
	if err != nil {
		return err
	}
	maxOpenConns, err := source.int(100, envDBMaxOpenConns)
	if err != nil {
		return err
	}
	connMaxLifetime, err := source.int(3600, envDBConnMaxLifetime)
	if err != nil {
		return err
	}
	debugSQL, err := source.bool(false, envDBDebugSQL)
	if err != nil {
		return err
	}

	cfg.Database = DatabaseConfig{
		URL:             source.string("", envDatabaseURL),
		Driver:          source.string("postgres", envDBDriver),
		Host:            source.string("localhost", envDBHost),
		Port:            port,
		Username:        source.string("postgres", envDBUsername),
		Password:        source.string("", envDBPassword),
		DBName:          source.string("zgi_test", envDBName),
		SSLMode:         source.string("disable", envDBSSLMode),
		Timezone:        source.string("Asia/Shanghai", envDBTimezone),
		MaxIdleConns:    maxIdleConns,
		MaxOpenConns:    maxOpenConns,
		ConnMaxLifetime: connMaxLifetime,
		DebugSQL:        debugSQL,
	}
	return nil
}

func loadRedisConfig(cfg *Config, source *envSource) error {
	port, err := source.int(6379, envRedisPort)
	if err != nil {
		return err
	}
	db, err := source.int(0, envRedisDB)
	if err != nil {
		return err
	}
	poolSize, err := source.int(10, envRedisPoolSize)
	if err != nil {
		return err
	}
	minIdleConns, err := source.int(5, envRedisMinIdleConns)
	if err != nil {
		return err
	}

	cfg.Redis = RedisConfig{
		Host:         source.string("localhost", envRedisHost),
		Port:         port,
		Password:     source.string("", envRedisPassword),
		DB:           db,
		PoolSize:     poolSize,
		MinIdleConns: minIdleConns,
	}
	return nil
}

func loadJWTConfig(cfg *Config, source *envSource) error {
	expireMinutes, err := source.int(60, envAccessTokenExpireMinutes)
	if err != nil {
		return err
	}

	cfg.JWT = JWTConfig{
		Secret:         source.string("", envSecretKey),
		ExpireDays:     expireMinutes / (24 * 60),
		ExpireDuration: time.Duration(expireMinutes) * time.Minute,
		JWTExpire:      time.Duration(expireMinutes) * time.Minute,
		Issuer:         platformRunMode(source),
	}
	return nil
}

func loadLogConfig(cfg *Config, source *envSource) error {
	maxSize, err := source.int(100, envLogMaxSize)
	if err != nil {
		return err
	}
	maxAge, err := source.int(15, envLogMaxAge)
	if err != nil {
		return err
	}
	maxBackups, err := source.int(7, envLogMaxBackups)
	if err != nil {
		return err
	}
	compress, err := source.bool(true, envLogCompress)
	if err != nil {
		return err
	}

	cfg.Log = LogConfig{
		Level:      source.string(defaultLogLevel(cfg), envLogLevel),
		Filename:   source.string("logs/app.log", envLogFilename),
		MaxSize:    maxSize,
		MaxAge:     maxAge,
		MaxBackups: maxBackups,
		Compress:   compress,
	}
	return nil
}

func defaultLogLevel(cfg *Config) string {
	if strings.EqualFold(cfg.Server.Environment, "production") || strings.EqualFold(cfg.Server.Mode, "release") {
		return "info"
	}
	return "debug"
}

func loadEmailConfig(cfg *Config, source *envSource) error {
	port, err := source.int(587, envEmailPort)
	if err != nil {
		return err
	}
	smtpUseTLS, err := source.bool(false, envEmailSMTPUseTLS)
	if err != nil {
		return err
	}
	smtpOpportunisticTLS, err := source.bool(false, envEmailSMTPOpportunisticTLS)
	if err != nil {
		return err
	}

	cfg.Email = EmailConfig{
		MailType:              source.string("resend", envEmailMailType, envMailType),
		MailDefaultSendFrom:   source.string("noreply@example.com", envEmailMailDefaultSendFrom),
		ResendAPIKey:          source.string("", envEmailResendAPIKey),
		ResendAPIURL:          source.string("https://api.resend.com", envEmailResendAPIURL),
		MailTemplateLogoUrl:   source.string("", envEmailMailTemplateLogoURL),
		MailTemplateBrandName: source.string("ZGI", envEmailMailTemplateBrandName),
		ConsoleWebURL:         source.string("http://localhost:3000", envEmailConsoleWebURL),
		SMTPServer:            source.string("smtp.gmail.com", envEmailSMTPServer),
		SMTPPort:              port,
		SMTPUsername:          source.string("", envEmailSMTPUsername),
		SMTPPassword:          source.string("", envEmailSMTPPassword),
		SMTPUseTLS:            smtpUseTLS,
		SMTPOpportunisticTLS:  smtpOpportunisticTLS,
	}
	return nil
}

func loadAppConfig(cfg *Config, source *envSource) error {
	filesAccessTimeout, err := source.int(3600, envFilesAccessTimeout)
	if err != nil {
		return err
	}

	cfg.App = AppConfig{
		Name:               source.string("ZGI-GinKit", envAppName),
		SecretKey:          source.string("", envSecretKey),
		FilesURL:           source.string("http://localhost:2679", envFilesURL),
		InternalFilesURL:   source.string("", envInternalFilesURL),
		FilesAccessTimeout: filesAccessTimeout,
	}
	return nil
}

func loadConsoleConfig(cfg *Config, source *envSource) {
	cfg.Console = ConsoleConfig{
		APIURL:         source.string("http://127.0.0.1:2679", envConsoleAPIURL),
		GRPCAddr:       source.string("", envConsoleAPIGRPCAddr),
		InternalAPIKey: source.string("", envConsoleInternalAPIKey),
		WebURL:         source.string(cfg.Email.ConsoleWebURL, envEmailConsoleWebURL),
	}
}

func loadPlatformConfig(cfg *Config, source *envSource) {
	cfg.Platform = PlatformConfig{
		Edition:                  platformRunMode(source),
		AdminPass:                source.string("", envZGIAdminPass),
		OrgInviteDefaultPassword: source.string("", envZGIOrgInviteDefaultPassword),
		CloudBootstrap: CloudBootstrapConfig{
			AdminEmail:    source.string("", envCloudBootstrapAdminEmail),
			AdminName:     source.string("", envCloudBootstrapAdminName),
			AdminPassword: source.string("", envCloudBootstrapAdminPassword),
		},
	}
}

func platformRunMode(source *envSource) string {
	mode := source.string("SELF_HOSTED", envZGIRunMode)
	return normalizePlatformRunMode(mode)
}

func normalizePlatformRunMode(mode string) string {
	normalized := strings.ToUpper(strings.TrimSpace(mode))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return "SELF_HOSTED"
	}
	return normalized
}

func loadFeatureConfig(cfg *Config, source *envSource) {
	pluginMaxPackageSize, _ := source.int(0, envPluginMaxPackageSize)

	cfg.Feature = FeatureConfig{
		PublicDeploymentEnabled:  mustBool(source.bool(false, envPublicDeploymentEnabled)),
		EnableEmailCodeLogin:     mustBool(source.bool(false, envEnableEmailCodeLogin)),
		EnableEmailPasswordLogin: mustBool(source.bool(true, envEnableEmailPasswordLogin)),
		EnableSocialOAuthLogin:   mustBool(source.bool(false, envEnableSocialOAuthLogin)),
		AllowRegister:            mustBool(source.bool(false, envAllowRegister)),
		AllowCreateWorkspace:     mustBool(source.bool(false, envAllowCreateWorkspace)),
		EnterpriseEnabled:        mustBool(source.bool(false, envEnterpriseEnabled)),
		MarketplaceEnabled:       mustBool(source.bool(true, envMarketplaceEnabled)),
		PluginMaxPackageSize:     pluginMaxPackageSize,
	}
}

func loadPluginRunnerConfig(cfg *Config, source *envSource) error {
	timeout, err := source.int(30, envPluginRunnerTimeout)
	if err != nil {
		return err
	}

	enabled, err := source.bool(false, envPluginRunnerEnabled)
	if err != nil {
		return err
	}

	cfg.PluginRunner = PluginRunnerConfig{
		Enabled: enabled,
		BaseURL: source.string("http://localhost:2665", envPluginRunnerURL),
		APIKey:  source.string("", envPluginRunnerAPIKey),
		Timeout: timeout,
	}
	return nil
}

func loadTaskQueueConfig(cfg *Config, source *envSource) error {
	redisDB, err := source.int(0, envTaskQueueRedisDB)
	if err != nil {
		return err
	}
	concurrency, err := source.int(4, envTaskQueueConcurrency)
	if err != nil {
		return err
	}
	retention, err := source.duration(24*time.Hour, envTaskQueueRetention)
	if err != nil {
		return err
	}

	cfg.TaskQueue = TaskQueueConfig{
		RedisDB:                 redisDB,
		Concurrency:             concurrency,
		Retention:               retention,
		EnvPrefix:               source.string("", envTaskQueueEnvPrefix),
		WorkflowTestTaskBackend: source.string("local", envWorkflowTestTaskBackend),
	}
	return nil
}

func loadVectorStoreConfig(cfg *Config, source *envSource) error {
	indexingBatchSize, err := source.int(4, envIndexingBatchSize)
	if err != nil {
		return err
	}
	if indexingBatchSize < 1 {
		indexingBatchSize = 4
	}
	indexingMaxTokens, err := source.int(4000, envIndexingMaxSegmentationTokensLength)
	if err != nil {
		return err
	}
	indexingMaxWorkers, err := source.int(4, envIndexingMaxWorkers)
	if err != nil {
		return err
	}
	if indexingMaxWorkers < 1 {
		indexingMaxWorkers = 4
	}

	maxAllowedWorkers := 20
	if runtime.NumCPU()*2 < maxAllowedWorkers {
		maxAllowedWorkers = runtime.NumCPU() * 2
	}
	if indexingMaxWorkers > maxAllowedWorkers {
		indexingMaxWorkers = maxAllowedWorkers
	}

	indexingTimeout, err := source.int(60, envIndexingTimeoutMinutes)
	if err != nil {
		return err
	}
	if indexingTimeout < 5 {
		indexingTimeout = 5
	}
	weaviateGRPC, err := source.bool(false, envWeaviateGRPCEnabled)
	if err != nil {
		return err
	}

	cfg.VectorStore = VectorStoreConfig{
		Type:               source.string("weaviate", envVectorStore),
		WeaviateEndpoint:   source.string("", envWeaviateEndpoint),
		WeaviateAPIKey:     source.string("", envWeaviateAPIKey),
		WeaviateGRPC:       weaviateGRPC,
		IndexingBatchSize:  indexingBatchSize,
		IndexingMaxTokens:  indexingMaxTokens,
		IndexingMaxWorkers: indexingMaxWorkers,
		IndexingTimeout:    indexingTimeout,
		KeywordDataSource:  source.string("database", envKeywordDataSourceType),
	}
	return nil
}

func loadUploadConfig(cfg *Config, source *envSource) error {
	fileSizeLimit, err := source.int(15, envUploadFileSizeLimit)
	if err != nil {
		return err
	}
	fileBatchLimit, err := source.int(5, envUploadFileBatchLimit)
	if err != nil {
		return err
	}
	imageSizeLimit, err := source.int(10, envUploadImageFileSizeLimit)
	if err != nil {
		return err
	}
	videoSizeLimit, err := source.int(100, envUploadVideoFileSizeLimit)
	if err != nil {
		return err
	}
	audioSizeLimit, err := source.int(50, envUploadAudioFileSizeLimit)
	if err != nil {
		return err
	}
	batchUploadLimit, err := source.int(10, envBatchUploadLimit)
	if err != nil {
		return err
	}
	workflowFileLimit, err := source.int(10, envWorkflowFileUploadLimit)
	if err != nil {
		return err
	}
	enterpriseStorageQuotaGB, err := source.int(20, envEnterpriseStorageQuotaGB)
	if err != nil {
		return err
	}

	cfg.Upload = UploadConfig{
		FileSizeLimit:          fileSizeLimit,
		FileBatchLimit:         fileBatchLimit,
		ImageSizeLimit:         imageSizeLimit,
		VideoSizeLimit:         videoSizeLimit,
		AudioSizeLimit:         audioSizeLimit,
		BatchUploadLimit:       batchUploadLimit,
		WorkflowFileLimit:      workflowFileLimit,
		EnterpriseStorageQuota: int64(enterpriseStorageQuotaGB) * 1024 * 1024 * 1024,
	}
	return nil
}

func loadETLConfig(cfg *Config, source *envSource) error {
	etlType := source.string("zgi", envETLType)
	switch etlType {
	case "Unstructured", "LandingAI", "Reducto", "Mixed", "zgi", "Hyperparse":
	default:
		etlType = "zgi"
	}

	hyperparseEnabled, err := source.bool(true, envETLHyperparseEnabled)
	if err != nil {
		return err
	}
	hyperparseMode := strings.ToLower(strings.TrimSpace(source.string("relaxed", envETLHyperparseMode)))
	if hyperparseMode != "strict" {
		hyperparseMode = "relaxed"
	}
	hyperparseBackend := strings.ToLower(strings.TrimSpace(source.string("", envETLHyperparseBackend)))
	if hyperparseBackend == "" {
		hyperparseBackend = strings.ToLower(strings.TrimSpace(source.string("", envHyperparseBackend)))
	}
	// Backward compatibility: if old mode env is set to an engine value, use it as backend.
	if hyperparseBackend == "" {
		if legacy, ok := normalizeHyperparseBackend(hyperparseMode); ok {
			hyperparseBackend = legacy
		}
	}
	if normalized, ok := normalizeHyperparseBackend(hyperparseBackend); ok {
		hyperparseBackend = normalized
	} else {
		hyperparseBackend = "mineru"
	}

	cfg.ETL = ETLConfig{
		Type:               etlType,
		UnstructuredAPIURL: source.string("", envUnstructuredAPIURL),
		UnstructuredAPIKey: source.string("", envUnstructuredAPIKey),
		LandingAIAPIKey:    source.string("", envLandingAIAPIKey),
		ReductoAPIKey:      source.string("", envReductoAPIKey),
		HyperparseEnabled:  hyperparseEnabled,
		HyperparseMode:     hyperparseMode,
		HyperparseBackend:  hyperparseBackend,
	}
	return nil
}

func normalizeHyperparseBackend(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "local":
		return "local", true
	case "mineru":
		return "mineru", true
	case "reducto":
		return "reducto", true
	case "vlm":
		return "vlm", true
	default:
		return "", false
	}
}

func loadCodeExecConfig(cfg *Config, source *envSource) error {
	connectTimeout, err := source.int(5, envCodeExecutionConnectTimeout)
	if err != nil {
		return err
	}
	createTimeout, err := source.int(10, envCodeExecutionCreateTimeout)
	if err != nil {
		return err
	}
	uploadTimeout, err := source.int(30, envCodeExecutionUploadTimeout)
	if err != nil {
		return err
	}
	commandTimeoutPadding, err := source.int(15, envCodeExecutionCommandTimeoutPadding)
	if err != nil {
		return err
	}
	artifactTimeout, err := source.int(10, envCodeExecutionArtifactTimeout)
	if err != nil {
		return err
	}
	cleanupTimeout, err := source.int(5, envCodeExecutionCleanupTimeout)
	if err != nil {
		return err
	}
	enableNetwork, err := source.bool(false, envCodeExecutionEnableNetwork)
	if err != nil {
		return err
	}
	maxNumber, err := source.int64(9223372036854775807, envCodeMaxNumber)
	if err != nil {
		return err
	}
	minNumber, err := source.int64(-9223372036854775808, envCodeMinNumber)
	if err != nil {
		return err
	}
	maxStringLength, err := source.int(80000, envCodeMaxStringLength)
	if err != nil {
		return err
	}

	cfg.CodeExec = CodeExecConfig{
		Endpoint:                     source.string("", envCodeExecutionEndpoint),
		APIKey:                       source.string("", envCodeExecutionAPIKey),
		ConnectTimeoutSeconds:        connectTimeout,
		CreateTimeoutSeconds:         createTimeout,
		UploadTimeoutSeconds:         uploadTimeout,
		CommandTimeoutPaddingSeconds: commandTimeoutPadding,
		ArtifactTimeoutSeconds:       artifactTimeout,
		CleanupTimeoutSeconds:        cleanupTimeout,
		EnableNetwork:                enableNetwork,
		SystemOfficeProfile:          source.string("skill-office", envCodeExecutionSystemOfficeProfile),
		MaxNumber:                    maxNumber,
		MinNumber:                    minNumber,
		MaxStringLength:              maxStringLength,
	}
	return nil
}

func loadWorkflowConfig(cfg *Config, source *envSource) error {
	executionTimeout, err := source.int(300, envWorkflowExecutionTimeout)
	if err != nil {
		return err
	}
	llmTimeout, err := source.int(120, envWorkflowLLMTimeout)
	if err != nil {
		return err
	}
	heartbeatInterval, err := source.int(5, envWorkflowHeartbeatInterval)
	if err != nil {
		return err
	}
	cleanupTimeout, err := source.int(30, envWorkflowCleanupTimeout)
	if err != nil {
		return err
	}
	imageInputURLMode := strings.ToLower(strings.TrimSpace(source.string(WorkflowImageInputURLModeZGIProxy, envWorkflowImageInputURLMode)))
	if imageInputURLMode == "" {
		imageInputURLMode = WorkflowImageInputURLModeZGIProxy
	}

	cfg.Workflow = WorkflowConfig{
		ExecutionTimeout:        executionTimeout,
		LLMTimeout:              llmTimeout,
		HeartbeatInterval:       heartbeatInterval,
		CleanupTimeout:          cleanupTimeout,
		ImageInputURLMode:       imageInputURLMode,
		ImageInputPublicBaseURL: strings.TrimSpace(source.string("", envWorkflowImageInputPublicBaseURL)),
	}
	return nil
}

func loadWorkflowFileExtractionConfig(cfg *Config, source *envSource) error {
	enabled, err := source.bool(true, envWorkflowFileExtractionEnabled)
	if err != nil {
		return err
	}
	maxContentSize, err := source.int(1048576, envWorkflowFileExtractionMaxContentSize)
	if err != nil {
		return err
	}
	extractionTimeout, err := source.int(120, envWorkflowFileExtractionTimeout)
	if err != nil {
		return err
	}
	cacheEnabled, err := source.bool(true, envWorkflowFileExtractionCacheEnabled)
	if err != nil {
		return err
	}

	// Resolve extraction strategy: explicit env var takes precedence; if unset,
	// inherit the Hyperparse backend configured for the knowledge-base pipeline so
	// both paths use the same parser without extra configuration.
	strategy := strings.ToLower(strings.TrimSpace(source.string("", envWorkflowFileExtractionStrategy)))
	if strategy == "" && cfg.ETL.HyperparseEnabled {
		strategy = cfg.ETL.HyperparseBackend
	}

	cfg.WorkflowFileExtraction = WorkflowFileExtractionConfig{
		Enabled:           enabled,
		MaxContentSize:    maxContentSize,
		ExtractionTimeout: extractionTimeout,
		CacheEnabled:      cacheEnabled,
		Strategy:          strategy,
	}
	return nil
}

func loadContentParseConfig(cfg *Config, source *envSource) {
	cfg.ContentParse = ContentParseConfig{
		ShadowDatasetIndexingEnabled: mustBool(source.bool(false, envContentParseShadowDatasetIndexingEnabled)),
	}
}

func loadAnswerNodeStreamingConfig(cfg *Config, source *envSource) error {
	chunkSize, err := source.int(20, envAnswerNodeStreamingChunkSize)
	if err != nil {
		return err
	}

	cfg.AnswerNodeStreaming = AnswerNodeStreamingConfig{
		ChunkSize: chunkSize,
	}
	return nil
}

func loadEncryptionConfig(cfg *Config, source *envSource) error {
	apiKeyEncryptionKey := source.string("", envAPIKeyEncryptionKey)
	if apiKeyEncryptionKey != "" && len(apiKeyEncryptionKey) != 32 {
		return fmt.Errorf("%s must be exactly 32 bytes long, got %d bytes", envAPIKeyEncryptionKey, len(apiKeyEncryptionKey))
	}

	llmCredentialSecretKey := source.string("", envLLMCredentialSecretKey)
	if llmCredentialSecretKey == "" {
		llmCredentialSecretKey = apiKeyEncryptionKey
	}
	if llmCredentialSecretKey != "" && len(llmCredentialSecretKey) != 32 {
		return fmt.Errorf("%s must be exactly 32 bytes long, got %d bytes", envLLMCredentialSecretKey, len(llmCredentialSecretKey))
	}

	cfg.Encryption = EncryptionConfig{
		APIKeyEncryptionKey:    apiKeyEncryptionKey,
		LLMCredentialSecretKey: llmCredentialSecretKey,
	}
	return nil
}

func loadOpenTelemetryConfig(cfg *Config, source *envSource) {
	enabled := mustBool(source.bool(false, envOTELEnabled))
	if source.string("", envOTELTracesExporter) == "none" {
		enabled = false
	}

	endpoint := source.string("", envOTELExporterOTLPEndpoint)
	tracesEndpoint := source.string("", envOTELExporterOTLPTracesEndpoint)
	headers := loadOTELHeaders(source)
	if langfuseEndpoint, langfuseHeaders := loadLangfuseOTELConfig(source, endpoint, tracesEndpoint); len(langfuseHeaders) > 0 {
		if langfuseEndpoint != "" {
			endpoint = langfuseEndpoint
			tracesEndpoint = ""
		}
		for key, value := range langfuseHeaders {
			headers[key] = value
		}
	}

	cfg.OpenTelemetry = OpenTelemetryConfig{
		Enabled:               enabled,
		ServiceName:           source.string("zgi-api", envOTELServiceName),
		Endpoint:              endpoint,
		TracesEndpoint:        tracesEndpoint,
		Protocol:              source.string("http/protobuf", envOTELExporterOTLPProtocol),
		TraceSampleRate:       mustFloat64(source.float64(1.0, envOTELTracesSampleRate)),
		Headers:               headers,
		InstrumentHTTPClient:  mustBool(source.bool(true, envOTELInstrumentHTTPClient)),
		InstrumentWorkflow:    mustBool(source.bool(true, envOTELInstrumentWorkflow)),
		InstrumentDB:          mustBool(source.bool(false, envOTELInstrumentDB)),
		InstrumentRedis:       mustBool(source.bool(false, envOTELInstrumentRedis)),
		InstrumentGRPC:        mustBool(source.bool(false, envOTELInstrumentGRPC)),
		LLMLangfuseAttributes: mustBool(source.bool(true, envOTELLLMLangfuseAttributes)),
		LLMCaptureContent:     source.string("summary", envOTELLLMCaptureContent),
		LLMCaptureMaxChars:    mustInt(source.int(65536, envOTELLLMCaptureMaxChars)),
	}
}

func loadOTELHeaders(source *envSource) map[string]string {
	headers := parseOTELHeaders(source.string("", envOTELExporterOTLPHeaders))
	if traceHeaders := parseOTELHeaders(source.string("", envOTELExporterOTLPTracesHeaders)); len(traceHeaders) > 0 {
		headers = traceHeaders
	}
	return headers
}

func parseOTELHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		headers[key] = value
	}
	return headers
}

func loadLangfuseOTELConfig(source *envSource, currentEndpoint string, currentTracesEndpoint string) (string, map[string]string) {
	langfuseEnabled := false
	if value, ok := source.lookup(envLangfuseEnabled); ok && value != "" {
		langfuseEnabled = mustBool(source.bool(false, envLangfuseEnabled))
		if !langfuseEnabled {
			return "", nil
		}
	}

	publicKey := strings.TrimSpace(source.string("", envLangfusePublicKey))
	secretKey := strings.TrimSpace(source.string("", envLangfuseSecretKey))
	hasKeyPair := publicKey != "" && secretKey != ""

	authString := strings.TrimSpace(source.string("", envLangfuseAuthString))
	if authString == "" && hasKeyPair {
		authString = base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
	}
	if authString == "" {
		return "", nil
	}
	if !langfuseEnabled && !shouldAutoEnableLangfuseDirectExport(currentEndpoint, currentTracesEndpoint, hasKeyPair) {
		return "", nil
	}

	return langfuseOTELEndpoint(source), map[string]string{
		"Authorization":                "Basic " + authString,
		"x-langfuse-ingestion-version": "4",
	}
}

func langfuseOTELEndpoint(source *envSource) string {
	if endpoint := strings.TrimSpace(source.string("", envLangfuseOTELEndpoint)); endpoint != "" {
		return endpoint
	}

	baseURL := strings.TrimSpace(source.string("", envLangfuseBaseURL, envLangfuseHost))
	if baseURL == "" {
		baseURL = "https://cloud.langfuse.com"
	}
	return strings.TrimRight(baseURL, "/") + "/api/public/otel"
}

func shouldAutoEnableLangfuseDirectExport(endpoint string, tracesEndpoint string, hasKeyPair bool) bool {
	if !hasKeyPair {
		return false
	}

	return strings.TrimSpace(endpoint) == "" && strings.TrimSpace(tracesEndpoint) == ""
}

func loadModelMetaConfig(cfg *Config, source *envSource) {
	cfg.ModelMeta = ModelMetaConfig{
		APIURL: source.string("https://models.zgi.ai", envModelMetaAPIURL),
	}
}

func loadNeo4jConfig(cfg *Config, source *envSource) {
	cfg.Neo4j = Neo4jConfig{
		URI:      source.string("", envNeo4jURI),
		Username: source.string("neo4j", envNeo4jUsername),
		Password: source.string("", envNeo4jPassword),
		Database: source.string("neo4j", envNeo4jDatabase),
	}
}

func loadSentryConfig(cfg *Config, source *envSource) {
	environment := source.string("", envSentryEnvironment)
	if environment == "" {
		environment = cfg.Server.Environment
	}

	cfg.Sentry = SentryConfig{
		DSN:         source.string("", envSentryDSN),
		Environment: environment,
		Release:     source.string("1.0.0", envAppVersion),
	}
}

func loadStorageConfig(cfg *Config, source *envSource) error {
	s3ForcePathStyle, err := source.bool(false, envS3ForcePathStyle)
	if err != nil {
		return err
	}
	s3DisableSSL, err := source.bool(false, envS3DisableSSL)
	if err != nil {
		return err
	}
	qiniuUseHTTPS, err := source.bool(false, envQiniuUseHTTPS)
	if err != nil {
		return err
	}

	cfg.Storage = StorageConfig{
		Type:            source.string("local", envStorageType),
		OpenDALBasePath: source.string("./storage/opendal", envOpenDALBasePath),
		AliyunOSS: AliyunOSSStorageConfig{
			Endpoint:        source.string("", envAliyunOSSEndpoint),
			BucketName:      source.string("", envAliyunOSSBucketName),
			Folder:          source.string("", envAliyunOSSPath),
			AccessKeyID:     source.string("", envAliyunOSSAccessKey),
			AccessKeySecret: source.string("", envAliyunOSSSecretKey),
			AuthVersion:     source.string("", envAliyunOSSAuthVersion),
			Region:          source.string("", envAliyunOSSRegion),
		},
		Qiniu: QiniuStorageConfig{
			AccessKey: source.string("", envQiniuAccessKey),
			SecretKey: source.string("", envQiniuSecretKey),
			Bucket:    source.string("", envQiniuBucketName),
			Domain:    source.string("", envQiniuDomain),
			Zone:      source.string("", envQiniuZone),
			UseHTTPS:  qiniuUseHTTPS,
			Folder:    source.string("", envQiniuPath),
		},
		S3: S3StorageConfig{
			AccessKey:        source.string("", envS3AccessKey),
			SecretKey:        source.string("", envS3SecretKey),
			Region:           source.string("", envS3Region),
			BucketName:       source.string("", envS3BucketName),
			Endpoint:         source.string("", envS3Endpoint),
			S3ForcePathStyle: s3ForcePathStyle,
			DisableSSL:       s3DisableSSL,
			Folder:           source.string("", envS3Path),
		},
	}
	return nil
}

func loadMarketplaceConfig(cfg *Config, source *envSource) {
	cfg.Marketplace = MarketplaceConfig{
		APIURL:  source.string("https://market.zgiai.com", envMarketplaceAPIURL),
		Source:  source.string("zgi", envMarketplaceSource),
		BaseURL: source.string("http://localhost:8025", envMarketplaceBaseURL),
	}
}

func loadSQLBaseConfig(cfg *Config, source *envSource) error {
	cfg.SQLBase = SQLBaseConfig{
		Type:                source.string("internal", envSQLBaseType),
		InternalDB:          source.string("", envSQLBaseInternalDB),
		ExternalURL:         source.string("", envSQLBaseExternalURL),
		ExternalAPIKey:      source.string("", envSQLBaseExternalAPIKey),
		PostgresMetaBaseURL: source.string("", envPostgresMetaBaseURL),
	}
	return nil
}

func loadAuthConfig(cfg *Config, source *envSource) {
	cfg.Auth = AuthConfig{
		MasterVerificationCode: source.string("", envMasterVerificationCode),
		Casdoor: CasdoorConfig{
			Host:         source.string("", envCasdoorHost),
			ClientID:     source.string("", envCasdoorClientID),
			ClientSecret: source.string("", envCasdoorClientSecret),
			RedirectURI:  source.string("", envCasdoorRedirectURI),
			Scopes:       source.scopeList([]string{"openid", "profile", "email"}, envCasdoorScopes),
		},
		SSO: SSOConfig{
			FrontendCallbackURL:  source.string("", envSSOFrontendCallbackURL),
			FrontendCallbackURLs: source.prefixedStrings(envSSOFrontendCallbackURLPrefix),
		},
	}
}

func loadKnowledgeConfig(cfg *Config, source *envSource) {
	rateLimitEnabled, _ := source.bool(false, envKnowledgeRateLimitEnabled)
	cfg.Knowledge = KnowledgeConfig{
		RateLimitEnabled:  rateLimitEnabled,
		RateLimitWindowMS: mustInt64(source.int64(60000, envKnowledgeRateLimitWindowMS)),
		RateLimitMax:      mustInt64(source.int64(60, envKnowledgeRateLimitMax)),
	}
}

func loadGraphFlowConfig(cfg *Config, source *envSource) {
	cfg.GraphFlow = GraphFlowConfig{
		VectorSyncBatchSize:   mustInt(source.int(50, envGraphFlowVectorSyncBatchSize)),
		VectorSyncConcurrency: mustInt(source.int(10, envGraphFlowVectorSyncConcurrency)),
	}
}

func loadLLMConfig(cfg *Config, source *envSource) {
	officialModelStrictSync, _ := source.bool(false, envOfficialModelSyncStrictMode)
	guardOutboundURL, _ := source.bool(true, envLLMGuardOutboundURL)
	_, guardOutboundURLSet := source.lookup(envLLMGuardOutboundURL)
	guardOutboundDNS, _ := source.bool(false, envLLMGuardOutboundDNS)
	allowPrivateBaseURL, _ := source.bool(false, envLLMAllowPrivateBaseURL)
	cfg.LLM = LLMConfig{
		EncryptionKey:           source.string("", envLLMEncryptionKey),
		OfficialModelStrictSync: officialModelStrictSync,
		GuardOutboundURL:        guardOutboundURL,
		GuardOutboundDNS:        guardOutboundDNS,
		AllowPrivateBaseURL:     allowPrivateBaseURL,
		guardOutboundURLSet:     guardOutboundURLSet,
	}
}

func loadAutomationConfig(cfg *Config, source *envSource) {
	cfg.Automation = AutomationConfig{
		DispatchEnabled: mustBool(source.bool(true, envAutomationDispatchEnabled)),
	}
}

func loadToolingConfig(cfg *Config, source *envSource) {
	cfg.Tooling = ToolingConfig{
		DryRun: mustBool(source.bool(false, envDryRun)),
	}
}

func mustBool(value bool, err error) bool {
	if err != nil {
		return false
	}
	return value
}

func mustInt(value int, err error) int {
	if err != nil {
		return 0
	}
	return value
}

func mustInt64(value int64, err error) int64 {
	if err != nil {
		return 0
	}
	return value
}

func mustFloat64(value float64, err error) float64 {
	if err != nil {
		return 0
	}
	return value
}

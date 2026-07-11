package config

import "time"

// GlobalConfig stores the loaded application configuration.
var GlobalConfig *Config

const (
	MinChunkSize     = 1
	MaxChunkSize     = 1000
	TempFileTenantID = "00000000-0000-0000-0000-000000000000"

	// DefaultOrgInviteDefaultPassword is the self-hosted fallback password used
	// when ZGI_ORG_INVITE_DEFAULT_PASSWORD is not configured.
	DefaultOrgInviteDefaultPassword = "ZGI@Welcome1"

	defaultServerPort = 2670
	defaultGRPCPort   = 50051
)

type Config struct {
	Server                 ServerConfig
	Database               DatabaseConfig
	Redis                  RedisConfig
	JWT                    JWTConfig
	Log                    LogConfig
	Email                  EmailConfig
	App                    AppConfig
	PluginRunner           PluginRunnerConfig
	TaskQueue              TaskQueueConfig
	VectorStore            VectorStoreConfig
	Upload                 UploadConfig
	ETL                    ETLConfig
	CodeExec               CodeExecConfig
	Console                ConsoleConfig
	Feature                FeatureConfig
	Workflow               WorkflowConfig
	WorkflowFileExtraction WorkflowFileExtractionConfig
	AnswerNodeStreaming    AnswerNodeStreamingConfig
	Encryption             EncryptionConfig
	OpenTelemetry          OpenTelemetryConfig
	ModelMeta              ModelMetaConfig
	Neo4j                  Neo4jConfig
	Sentry                 SentryConfig
	Platform               PlatformConfig
	Storage                StorageConfig
	Marketplace            MarketplaceConfig
	SQLBase                SQLBaseConfig
	Auth                   AuthConfig
	Knowledge              KnowledgeConfig
	GraphFlow              GraphFlowConfig
	ContentParse           ContentParseConfig
	LLM                    LLMConfig
	LLMPolicyPrompt        LLMPolicyPromptConfig
	Automation             AutomationConfig
	Tooling                ToolingConfig

	source *envSource
}

type ServerConfig struct {
	Port             int      `json:"port"`
	GRPCPort         int      `json:"grpc_port"`
	GRPCEnabled      bool     `json:"grpc_enabled"`
	Mode             string   `json:"mode"`
	Environment      string   `json:"environment"`
	ReadTimeout      int      `json:"read_timeout"`
	WriteTimeout     int      `json:"write_timeout"`
	MaxHeaderBytes   int      `json:"max_header_bytes"`
	CORSAllowOrigins []string `json:"cors_allow_origins"`
}

type DatabaseConfig struct {
	URL             string `json:"-"`
	Driver          string `json:"driver"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"-"`
	DBName          string `json:"dbname"`
	SSLMode         string `json:"sslmode"`
	Timezone        string `json:"timezone"`
	MaxIdleConns    int    `json:"max_idle_conns"`
	MaxOpenConns    int    `json:"max_open_conns"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
	DebugSQL        bool   `json:"debug_sql"`
}

type RedisConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Password     string `json:"-"`
	DB           int    `json:"db"`
	PoolSize     int    `json:"pool_size"`
	MinIdleConns int    `json:"min_idle_conns"`
}

type JWTConfig struct {
	Secret         string        `json:"-"`
	ExpireDays     int           `json:"expire_days"`
	ExpireDuration time.Duration `json:"-"`
	JWTExpire      time.Duration `json:"jwt_expire"`
	Issuer         string        `json:"issuer"`
}

type LogConfig struct {
	Level      string `json:"level"`
	Filename   string `json:"filename"`
	MaxSize    int    `json:"max_size"`
	MaxAge     int    `json:"max_age"`
	MaxBackups int    `json:"max_backups"`
	Compress   bool   `json:"compress"`
}

type EmailConfig struct {
	MailType              string `json:"mail_type,omitempty"`
	MailDefaultSendFrom   string `json:"mail_default_send_from,omitempty"`
	ResendAPIKey          string `json:"resend_api_key,omitempty"`
	ResendAPIURL          string `json:"resend_api_url,omitempty"`
	MailTemplateLogoUrl   string `json:"mail_template_logo_url,omitempty"`
	MailTemplateBrandName string `json:"mail_template_brand_name,omitempty"`
	ConsoleWebURL         string `json:"console_web_url,omitempty"`
	SMTPServer            string `json:"smtp_server,omitempty"`
	SMTPPort              int    `json:"smtp_port,omitempty"`
	SMTPUsername          string `json:"smtp_username,omitempty"`
	SMTPPassword          string `json:"-"`
	SMTPUseTLS            bool   `json:"smtp_use_tls,omitempty"`
	SMTPOpportunisticTLS  bool   `json:"smtp_opportunistic_tls,omitempty"`
}

type AppConfig struct {
	Name               string `json:"name"`
	SecretKey          string `json:"-"`
	FilesURL           string `json:"files_url"`
	InternalFilesURL   string `json:"internal_files_url"`
	FilesAccessTimeout int    `json:"files_access_timeout"`
}

type ConsoleConfig struct {
	APIURL         string `json:"api_url"`
	GRPCAddr       string `json:"grpc_addr"`
	InternalAPIKey string `json:"-"`
	WebURL         string `json:"web_url"`
}

type LLMPolicyPromptConfig struct {
	Enabled bool   `json:"enabled"`
	File    string `json:"file,omitempty"`
	Prompt  string `json:"-"`
}

type FeatureConfig struct {
	PublicDeploymentEnabled  bool `json:"public_deployment_enabled"`
	EnableEmailCodeLogin     bool `json:"enable_email_code_login"`
	EnableEmailPasswordLogin bool `json:"enable_email_password_login"`
	EnableSocialOAuthLogin   bool `json:"enable_social_oauth_login"`
	AllowRegister            bool `json:"allow_register"`
	AllowCreateWorkspace     bool `json:"allow_create_workspace"`
	EnterpriseEnabled        bool `json:"enterprise_enabled"`
	MarketplaceEnabled       bool `json:"marketplace_enabled"`
	PluginMaxPackageSize     int  `json:"plugin_max_package_size"`
}

type PluginRunnerConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"-"`
	Timeout int    `json:"timeout"`
}

type TaskQueueConfig struct {
	RedisDB                 int           `json:"redis_db"`
	Concurrency             int           `json:"concurrency"`
	Retention               time.Duration `json:"-"`
	EnvPrefix               string        `json:"env_prefix"`
	WorkflowTestTaskBackend string        `json:"workflow_test_task_backend"`
}

type VectorStoreConfig struct {
	Type               string `json:"type"`
	WeaviateEndpoint   string `json:"weaviate_endpoint"`
	WeaviateAPIKey     string `json:"-"`
	WeaviateGRPC       bool   `json:"weaviate_grpc_enabled"`
	IndexingBatchSize  int    `json:"indexing_batch_size"`
	IndexingMaxTokens  int    `json:"indexing_max_segmentation_tokens_length"`
	IndexingMaxWorkers int    `json:"indexing_max_workers"`
	IndexingTimeout    int    `json:"indexing_timeout_minutes"`
	KeywordDataSource  string `json:"keyword_data_source_type"`
}

type UploadConfig struct {
	FileSizeLimit          int   `json:"file_size_limit"`
	FileBatchLimit         int   `json:"file_batch_limit"`
	ImageSizeLimit         int   `json:"image_size_limit"`
	VideoSizeLimit         int   `json:"video_size_limit"`
	AudioSizeLimit         int   `json:"audio_size_limit"`
	BatchUploadLimit       int   `json:"batch_upload_limit"`
	WorkflowFileLimit      int   `json:"workflow_file_limit"`
	EnterpriseStorageQuota int64 `json:"enterprise_storage_quota"`
}

type ETLConfig struct {
	Type               string `json:"type"`
	UnstructuredAPIURL string `json:"unstructured_api_url"`
	UnstructuredAPIKey string `json:"-"`
	LandingAIAPIKey    string `json:"-"`
	ReductoAPIKey      string `json:"-"`
	HyperparseEnabled  bool   `json:"hyperparse_enabled"`
	HyperparseMode     string `json:"hyperparse_mode"`
	HyperparseBackend  string `json:"hyperparse_backend"`
}

type CodeExecConfig struct {
	Endpoint                     string `json:"endpoint"`
	APIKey                       string `json:"-"`
	ConnectTimeoutSeconds        int    `json:"connect_timeout_seconds"`
	CreateTimeoutSeconds         int    `json:"create_timeout_seconds"`
	UploadTimeoutSeconds         int    `json:"upload_timeout_seconds"`
	CommandTimeoutPaddingSeconds int    `json:"command_timeout_padding_seconds"`
	ArtifactTimeoutSeconds       int    `json:"artifact_timeout_seconds"`
	CleanupTimeoutSeconds        int    `json:"cleanup_timeout_seconds"`
	EnableNetwork                bool   `json:"enable_network"`
	SystemOfficeProfile          string `json:"system_office_profile"`
	MaxNumber                    int64  `json:"max_number"`
	MinNumber                    int64  `json:"min_number"`
	MaxStringLength              int    `json:"max_string_length"`
}

const (
	WorkflowImageInputURLModeZGIProxy         = "zgi_proxy"
	WorkflowImageInputURLModePublicStorageURL = "public_storage_url"
)

type WorkflowConfig struct {
	ExecutionTimeout        int    `json:"execution_timeout"`
	LLMTimeout              int    `json:"llm_timeout"`
	HeartbeatInterval       int    `json:"heartbeat_interval"`
	CleanupTimeout          int    `json:"cleanup_timeout"`
	ImageInputURLMode       string `json:"image_input_url_mode"`
	ImageInputPublicBaseURL string `json:"image_input_public_base_url"`
}

type WorkflowFileExtractionConfig struct {
	Enabled           bool   `json:"enabled"`
	MaxContentSize    int    `json:"max_content_size"`
	ExtractionTimeout int    `json:"extraction_timeout"`
	CacheEnabled      bool   `json:"cache_enabled"`
	Strategy          string `json:"strategy"` // mineru|local|reducto|unstructured|landingai|""
}

type AnswerNodeStreamingConfig struct {
	ChunkSize int `json:"chunk_size"`
}

type EncryptionConfig struct {
	APIKeyEncryptionKey    string `json:"-"`
	LLMCredentialSecretKey string `json:"-"`
}

type OpenTelemetryConfig struct {
	Enabled               bool              `json:"enabled"`
	ServiceName           string            `json:"service_name"`
	Endpoint              string            `json:"endpoint"`
	TracesEndpoint        string            `json:"traces_endpoint"`
	Protocol              string            `json:"protocol"`
	TraceSampleRate       float64           `json:"trace_sample_rate"`
	Headers               map[string]string `json:"-"`
	InstrumentHTTPClient  bool              `json:"instrument_http_client"`
	InstrumentWorkflow    bool              `json:"instrument_workflow"`
	InstrumentDB          bool              `json:"instrument_db"`
	InstrumentRedis       bool              `json:"instrument_redis"`
	InstrumentGRPC        bool              `json:"instrument_grpc"`
	LLMLangfuseAttributes bool              `json:"llm_langfuse_attributes"`
	LLMCaptureContent     string            `json:"llm_capture_content"`
	LLMCaptureMaxChars    int               `json:"llm_capture_max_chars"`
}

type ModelMetaConfig struct {
	APIURL string `json:"api_url"`
}

type Neo4jConfig struct {
	URI      string `json:"uri"`
	Username string `json:"username"`
	Password string `json:"-"`
	Database string `json:"database"`
}

type SentryConfig struct {
	DSN         string `json:"-"`
	Environment string `json:"environment"`
	Release     string `json:"release"`
}

type CloudBootstrapConfig struct {
	AdminEmail    string `json:"admin_email"`
	AdminName     string `json:"admin_name"`
	AdminPassword string `json:"-"`
}

type PlatformConfig struct {
	Edition                  string               `json:"edition"`
	AdminPass                string               `json:"-"`
	OrgInviteDefaultPassword string               `json:"-"`
	CloudBootstrap           CloudBootstrapConfig `json:"cloud_bootstrap"`
}

type StorageConfig struct {
	Type            string                 `json:"type"`
	OpenDALBasePath string                 `json:"opendal_base_path"`
	AliyunOSS       AliyunOSSStorageConfig `json:"aliyun_oss"`
	Qiniu           QiniuStorageConfig     `json:"qiniu"`
	S3              S3StorageConfig        `json:"s3"`
}

type AliyunOSSStorageConfig struct {
	Endpoint        string `json:"endpoint"`
	BucketName      string `json:"bucket_name"`
	Folder          string `json:"folder"`
	AccessKeyID     string `json:"-"`
	AccessKeySecret string `json:"-"`
	AuthVersion     string `json:"auth_version"`
	Region          string `json:"region"`
}

type QiniuStorageConfig struct {
	AccessKey string `json:"-"`
	SecretKey string `json:"-"`
	Bucket    string `json:"bucket"`
	Domain    string `json:"domain"`
	Zone      string `json:"zone"`
	UseHTTPS  bool   `json:"use_https"`
	Folder    string `json:"folder"`
}

type S3StorageConfig struct {
	AccessKey        string `json:"-"`
	SecretKey        string `json:"-"`
	Region           string `json:"region"`
	BucketName       string `json:"bucket_name"`
	Endpoint         string `json:"endpoint"`
	S3ForcePathStyle bool   `json:"s3_force_path_style"`
	DisableSSL       bool   `json:"disable_ssl"`
	Folder           string `json:"folder"`
}

type MarketplaceConfig struct {
	APIURL  string `json:"api_url"`
	Source  string `json:"source"`
	BaseURL string `json:"base_url"`
}

type SQLBaseConfig struct {
	Type                string `json:"type"`
	InternalDB          string `json:"internal_db"`
	ExternalURL         string `json:"external_url"`
	ExternalAPIKey      string `json:"-"`
	PostgresMetaBaseURL string `json:"postgres_meta_base_url"`
}

type AuthConfig struct {
	MasterVerificationCode string        `json:"-"`
	Casdoor                CasdoorConfig `json:"casdoor"`
	SSO                    SSOConfig     `json:"sso"`
}

type CasdoorConfig struct {
	Host         string   `json:"host"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"-"`
	RedirectURI  string   `json:"redirect_uri"`
	Scopes       []string `json:"scopes"`
}

type SSOConfig struct {
	FrontendCallbackURL  string            `json:"frontend_callback_url"`
	FrontendCallbackURLs map[string]string `json:"frontend_callback_urls"`
}

type KnowledgeConfig struct {
	RateLimitEnabled  bool  `json:"rate_limit_enabled"`
	RateLimitWindowMS int64 `json:"rate_limit_window_ms"`
	RateLimitMax      int64 `json:"rate_limit_max"`
}

type GraphFlowConfig struct {
	VectorSyncBatchSize   int `json:"vector_sync_batch_size"`
	VectorSyncConcurrency int `json:"vector_sync_concurrency"`
}

type ContentParseConfig struct {
	ShadowDatasetIndexingEnabled bool `json:"shadow_dataset_indexing_enabled"`
}

type LLMConfig struct {
	EncryptionKey           string `json:"-"`
	OfficialModelStrictSync bool   `json:"official_model_strict_sync"`
	GuardOutboundURL        bool   `json:"guard_outbound_url"`
	GuardOutboundDNS        bool   `json:"guard_outbound_dns"`
	AllowPrivateBaseURL     bool   `json:"allow_private_base_url"`
	UpstreamBalancePolling  bool   `json:"upstream_balance_polling"`
	UpstreamGuardMode       string `json:"upstream_guard_mode"`
	UpstreamGuardPercentage int    `json:"upstream_guard_percentage"`

	guardOutboundURLSet bool
}

func (c LLMConfig) OutboundURLGuardEnabled() bool {
	if !c.guardOutboundURLSet && !c.GuardOutboundURL {
		return true
	}
	return c.GuardOutboundURL
}

type AutomationConfig struct {
	DispatchEnabled bool `json:"dispatch_enabled"`
}

type ToolingConfig struct {
	DryRun bool `json:"dry_run"`
}

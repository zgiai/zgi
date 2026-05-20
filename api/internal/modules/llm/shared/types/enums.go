package types

// Scope defines the ownership scope of a resource
type Scope string

const (
	ScopeSystem Scope = "system" // Platform-provided resource
	ScopeTenant Scope = "tenant" // Tenant-created resource
)

// ProviderType defines the type of provider
type ProviderType string

const (
	ProviderTypeVendor ProviderType = "vendor" // Third-party vendor (OpenAI, Anthropic, etc.)
	ProviderTypeCustom ProviderType = "custom" // Custom/self-hosted provider
)

// ModelType defines the type of model
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeEmbedding ModelType = "text-embedding"
	ModelTypeImage     ModelType = "image"
	ModelTypeAudio     ModelType = "audio"
	ModelTypeVideo     ModelType = "video"
	ModelTypeRerank    ModelType = "rerank"
)

// AccessScope defines who can access a resource
type AccessScope string

const (
	AccessScopeAll   AccessScope = "all"   // Everyone in tenant
	AccessScopeGroup AccessScope = "group" // Specific groups
	AccessScopeUser  AccessScope = "user"  // Specific users
)

// RouteType defines the type of route
type RouteType string

const (
	RouteTypeZGICloud RouteType = "ZGI_CLOUD" // ZGI official cloud service channel
	RouteTypePrivate  RouteType = "PRIVATE"   // User's private channel
)

// RouteStatus defines the status of a route
type RouteStatus string

const (
	RouteStatusActive      RouteStatus = "active"
	RouteStatusDisabled    RouteStatus = "disabled"
	RouteStatusBanned      RouteStatus = "banned"
	RouteStatusMaintenance RouteStatus = "maintenance"
)

// LoadBalanceStrategy defines the load balancing strategy
type LoadBalanceStrategy string

const (
	LoadBalanceRoundRobin LoadBalanceStrategy = "round_robin"
	LoadBalanceRandom     LoadBalanceStrategy = "random"
	LoadBalanceWeighted   LoadBalanceStrategy = "weighted"
)

// APIKeyStatus defines the status of an API key
type APIKeyStatus string

const (
	APIKeyStatusActive   APIKeyStatus = "active"
	APIKeyStatusDisabled APIKeyStatus = "disabled"
	APIKeyStatusExpired  APIKeyStatus = "expired"
)

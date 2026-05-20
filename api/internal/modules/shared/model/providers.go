package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/shopspring/decimal"
)

// ModelType identifies a supported model family.
type ModelType string

const (
	ModelTypeLLM           ModelType = "llm"
	ModelTypeEmbedding     ModelType = "text-embedding"
	ModelTypeTextEmbedding ModelType = "text-embedding"
	ModelTypeRerank        ModelType = "rerank"
	ModelTypeModeration    ModelType = "moderation"
	ModelTypeSpeech2Text   ModelType = "speech2text"
	ModelTypeTTS           ModelType = "tts"
)

// ToOriginModelType converts the internal model type to the upstream plugin/runtime value.
func (m ModelType) ToOriginModelType() string {
	switch m {
	case ModelTypeLLM:
		return "text-generation"
	case ModelTypeEmbedding:
		return "embeddings"
	case ModelTypeRerank:
		return "reranking"
	case ModelTypeSpeech2Text:
		return "speech2text"
	case ModelTypeTTS:
		return "tts"
	case ModelTypeModeration:
		return "moderation"
	default:
		return string(m)
	}
}

// FromOriginModelType converts an upstream model type into the internal representation.
func FromOriginModelType(originModelType string) (ModelType, error) {
	switch originModelType {
	case "text-generation", string(ModelTypeLLM):
		return ModelTypeLLM, nil
	case "embeddings", string(ModelTypeEmbedding):
		return ModelTypeEmbedding, nil
	case "reranking", string(ModelTypeRerank):
		return ModelTypeRerank, nil
	case string(ModelTypeSpeech2Text):
		return ModelTypeSpeech2Text, nil
	case string(ModelTypeTTS):
		return ModelTypeTTS, nil
	case string(ModelTypeModeration):
		return ModelTypeModeration, nil
	default:
		return "", fmt.Errorf("invalid origin model type %s", originModelType)
	}
}

// Value implements database/sql/driver.Valuer.
func (m ModelType) Value() (driver.Value, error) {
	return string(m), nil
}

// Scan implements sql.Scanner.
func (m *ModelType) Scan(value interface{}) error {
	if value == nil {
		*m = ""
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*m = ModelType(v)
	case string:
		*m = ModelType(v)
	default:
		return fmt.Errorf("unsupported type for ModelType: %T", value)
	}
	return nil
}

// I18nObject stores localized strings used by plugin declarations and model schemas.
type I18nObject struct {
	ZhHans string `json:"zh_Hans"`
	EnUS   string `json:"en_US"`
}

// UnmarshalJSON normalizes missing zh_Hans values to keep old plugin payloads compatible.
func (i *I18nObject) UnmarshalJSON(data []byte) error {
	type alias I18nObject
	aux := &struct {
		*alias
	}{
		alias: (*alias)(i),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if i.ZhHans == "" {
		i.ZhHans = i.EnUS
	}

	return nil
}

type FetchFrom string

const (
	FetchFromPredefinedModel   FetchFrom = "predefined-model"
	FetchFromCustomizableModel FetchFrom = "customizable-model"
)

type ModelFeature string

const (
	ModelFeatureToolCall         ModelFeature = "tool-call"
	ModelFeatureMultiToolCall    ModelFeature = "multi-tool-call"
	ModelFeatureAgentThought     ModelFeature = "agent-thought"
	ModelFeatureVision           ModelFeature = "vision"
	ModelFeatureStreamToolCall   ModelFeature = "stream-tool-call"
	ModelFeatureDocument         ModelFeature = "document"
	ModelFeatureVideo            ModelFeature = "video"
	ModelFeatureAudio            ModelFeature = "audio"
	ModelFeatureStructuredOutput ModelFeature = "structured-output"
)

// Value implements database/sql/driver.Valuer.
func (f ModelFeature) Value() (driver.Value, error) {
	return string(f), nil
}

// Scan implements sql.Scanner.
func (f *ModelFeature) Scan(value interface{}) error {
	if value == nil {
		*f = ""
		return nil
	}
	switch v := value.(type) {
	case []byte:
		*f = ModelFeature(v)
	case string:
		*f = ModelFeature(v)
	default:
		return fmt.Errorf("unsupported type for ModelFeature: %T", value)
	}
	return nil
}

type DefaultParameterName string

const (
	DefaultParameterNameTemperature      DefaultParameterName = "temperature"
	DefaultParameterNameTopP             DefaultParameterName = "top_p"
	DefaultParameterNameTopK             DefaultParameterName = "top_k"
	DefaultParameterNamePresencePenalty  DefaultParameterName = "presence_penalty"
	DefaultParameterNameFrequencyPenalty DefaultParameterName = "frequency_penalty"
	DefaultParameterNameMaxTokens        DefaultParameterName = "max_tokens"
	DefaultParameterNameResponseFormat   DefaultParameterName = "response_format"
	DefaultParameterNameJSONSchema       DefaultParameterName = "json_schema"
)

type ParameterType string

const (
	ParameterTypeFloat   ParameterType = "float"
	ParameterTypeInt     ParameterType = "int"
	ParameterTypeString  ParameterType = "string"
	ParameterTypeBoolean ParameterType = "boolean"
	ParameterTypeText    ParameterType = "text"
)

type ModelPropertyKey string

const (
	ModelPropertyKeyMode                    ModelPropertyKey = "mode"
	ModelPropertyKeyContextSize             ModelPropertyKey = "context_size"
	ModelPropertyKeyMaxChunks               ModelPropertyKey = "max_chunks"
	ModelPropertyKeyFileUploadLimit         ModelPropertyKey = "file_upload_limit"
	ModelPropertyKeySupportedFileExtensions ModelPropertyKey = "supported_file_extensions"
	ModelPropertyKeyMaxCharactersPerChunk   ModelPropertyKey = "max_characters_per_chunk"
	ModelPropertyKeyDefaultVoice            ModelPropertyKey = "default_voice"
	ModelPropertyKeyVoices                  ModelPropertyKey = "voices"
	ModelPropertyKeyWordLimit               ModelPropertyKey = "word_limit"
	ModelPropertyKeyAudioType               ModelPropertyKey = "audio_type"
	ModelPropertyKeyMaxWorkers              ModelPropertyKey = "max_workers"
)

// RuntimeProviderModel is the shared immutable description of a model declaration.
type RuntimeProviderModel struct {
	Model           string                           `json:"model"`
	Label           I18nObject                       `json:"label"`
	ModelType       ModelType                        `json:"model_type"`
	Features        []ModelFeature                   `json:"features,omitempty"`
	FetchFrom       FetchFrom                        `json:"fetch_from"`
	ModelProperties map[ModelPropertyKey]interface{} `json:"model_properties"`
	Deprecated      bool                             `json:"deprecated,omitempty"`
}

type ParameterRule struct {
	Name        string        `json:"name"`
	UseTemplate *string       `json:"use_template,omitempty"`
	Label       I18nObject    `json:"label"`
	Type        ParameterType `json:"type"`
	Help        *I18nObject   `json:"help,omitempty"`
	Required    bool          `json:"required"`
	Default     interface{}   `json:"default,omitempty"`
	Min         *float64      `json:"min,omitempty"`
	Max         *float64      `json:"max,omitempty"`
	Precision   *int          `json:"precision,omitempty"`
	Options     []string      `json:"options,omitempty"`
}

type PriceConfig struct {
	Input    decimal.Decimal  `json:"input"`
	Output   *decimal.Decimal `json:"output,omitempty"`
	Unit     decimal.Decimal  `json:"unit"`
	Currency string           `json:"currency"`
}

// AIModelEntity is the provider-declared schema returned by plugins and reused by workflow code.
type AIModelEntity struct {
	RuntimeProviderModel
	ParameterRules []ParameterRule `json:"parameter_rules,omitempty"`
	Pricing        *PriceConfig    `json:"pricing,omitempty"`
}

// Validate backfills the structured-output feature for schemas that expose json_schema.
func (a *AIModelEntity) Validate() error {
	for _, rule := range a.ParameterRules {
		if rule.Name != string(DefaultParameterNameJSONSchema) {
			continue
		}

		for _, feature := range a.Features {
			if feature == ModelFeatureStructuredOutput {
				return nil
			}
		}

		a.Features = append(a.Features, ModelFeatureStructuredOutput)
		return nil
	}

	return nil
}

type ConfigurateMethod string

const (
	ConfigurateMethodPredefinedModel   ConfigurateMethod = "predefined-model"
	ConfigurateMethodCustomizableModel ConfigurateMethod = "customizable-model"
)

type FormType string

const (
	FormTypeTextInput   FormType = "text-input"
	FormTypeSecretInput FormType = "secret-input"
	FormTypeSelect      FormType = "select"
	FormTypeRadio       FormType = "radio"
	FormTypeSwitch      FormType = "switch"
)

type ProviderQuotaType string

const (
	ProviderQuotaTypePaid  ProviderQuotaType = "paid"
	ProviderQuotaTypeFree  ProviderQuotaType = "free"
	ProviderQuotaTypeTrial ProviderQuotaType = "trial"
)

func ProviderQuotaTypeValueOf(value string) (ProviderQuotaType, error) {
	switch value {
	case string(ProviderQuotaTypePaid):
		return ProviderQuotaTypePaid, nil
	case string(ProviderQuotaTypeFree):
		return ProviderQuotaTypeFree, nil
	case string(ProviderQuotaTypeTrial):
		return ProviderQuotaTypeTrial, nil
	default:
		return "", fmt.Errorf("no matching enum found for value %q", value)
	}
}

type QuotaUnit string

const (
	QuotaUnitTimes   QuotaUnit = "times"
	QuotaUnitTokens  QuotaUnit = "tokens"
	QuotaUnitCredits QuotaUnit = "credits"
)

type FormShowOnObject struct {
	Variable string `json:"variable"`
	Value    string `json:"value"`
}

type FormOption struct {
	Label  I18nObject         `json:"label"`
	Value  string             `json:"value"`
	ShowOn []FormShowOnObject `json:"show_on,omitempty"`
}

type CredentialFormSchema struct {
	Variable    string             `json:"variable"`
	Label       I18nObject         `json:"label"`
	Type        FormType           `json:"type"`
	Required    bool               `json:"required,omitempty"`
	Default     *string            `json:"default,omitempty"`
	Options     []FormOption       `json:"options,omitempty"`
	Placeholder *I18nObject        `json:"placeholder,omitempty"`
	MaxLength   int                `json:"max_length,omitempty"`
	ShowOn      []FormShowOnObject `json:"show_on,omitempty"`
}

type ProviderCredentialSchema struct {
	CredentialFormSchemas []CredentialFormSchema `json:"credential_form_schemas"`
}

type FieldModelSchema struct {
	Label       I18nObject  `json:"label"`
	Placeholder *I18nObject `json:"placeholder,omitempty"`
}

type ModelCredentialSchema struct {
	Model                 FieldModelSchema       `json:"model"`
	CredentialFormSchemas []CredentialFormSchema `json:"credential_form_schemas"`
}

type ProviderHelpEntity struct {
	Title I18nObject `json:"title"`
	URL   I18nObject `json:"url"`
}

type ProviderEntity struct {
	Provider                 string                    `json:"provider"`
	Label                    I18nObject                `json:"label"`
	Description              *I18nObject               `json:"description,omitempty"`
	IconSmall                *I18nObject               `json:"icon_small,omitempty"`
	IconLarge                *I18nObject               `json:"icon_large,omitempty"`
	Background               string                    `json:"background,omitempty"`
	Help                     *ProviderHelpEntity       `json:"help,omitempty"`
	SupportedModelTypes      []ModelType               `json:"supported_model_types"`
	ConfigurateMethods       []ConfigurateMethod       `json:"configurate_methods"`
	Models                   []AIModelEntity           `json:"models,omitempty"`
	ProviderCredentialSchema *ProviderCredentialSchema `json:"provider_credential_schema,omitempty"`
	ModelCredentialSchema    *ModelCredentialSchema    `json:"model_credential_schema,omitempty"`
	Position                 map[string][]string       `json:"position,omitempty"`
}

type SimpleProviderEntity struct {
	Provider            string          `json:"provider"`
	Label               I18nObject      `json:"label"`
	IconSmall           *I18nObject     `json:"icon_small,omitempty"`
	IconLarge           *I18nObject     `json:"icon_large,omitempty"`
	SupportedModelTypes []ModelType     `json:"supported_model_types"`
	Models              []AIModelEntity `json:"models,omitempty"`
}

func (p *ProviderEntity) ToSimpleProvider() *SimpleProviderEntity {
	return &SimpleProviderEntity{
		Provider:            p.Provider,
		Label:               p.Label,
		IconSmall:           p.IconSmall,
		IconLarge:           p.IconLarge,
		SupportedModelTypes: p.SupportedModelTypes,
		Models:              p.Models,
	}
}

type ModelStatus string

const (
	ModelStatusActive        ModelStatus = "active"
	ModelStatusNoConfigure   ModelStatus = "no-configure"
	ModelStatusQuotaExceeded ModelStatus = "quota-exceeded"
	ModelStatusNoPermission  ModelStatus = "no-permission"
	ModelStatusDisabled      ModelStatus = "disabled"
)

type ProviderModelWithStatusEntity struct {
	RuntimeProviderModel
	Status               ModelStatus `json:"status"`
	LoadBalancingEnabled bool        `json:"load_balancing_enabled"`
}

type ProviderType string

const (
	ProviderTypeSystem ProviderType = "system"
	ProviderTypeCustom ProviderType = "custom"
)

type RestrictModel struct {
	Model         string    `json:"model"`
	BaseModelName *string   `json:"base_model_name,omitempty"`
	ModelType     ModelType `json:"model_type"`
}

type QuotaConfiguration struct {
	QuotaType      ProviderQuotaType `json:"quota_type"`
	QuotaUnit      QuotaUnit         `json:"quota_unit"`
	QuotaLimit     int               `json:"quota_limit"`
	QuotaUsed      int               `json:"quota_used"`
	IsValid        bool              `json:"is_valid"`
	RestrictModels []RestrictModel   `json:"restrict_models,omitempty"`
}

type SystemConfiguration struct {
	Enabled             bool                   `json:"enabled"`
	CurrentQuotaType    *ProviderQuotaType     `json:"current_quota_type,omitempty"`
	QuotaConfigurations []QuotaConfiguration   `json:"quota_configurations,omitempty"`
	Credentials         map[string]interface{} `json:"credentials,omitempty"`
}

type CustomProviderConfiguration struct {
	Credentials map[string]interface{} `json:"credentials"`
}

type CustomModelConfiguration struct {
	Model       string                 `json:"model"`
	ModelType   ModelType              `json:"model_type"`
	Credentials map[string]interface{} `json:"credentials"`
}

type CustomConfiguration struct {
	Provider *CustomProviderConfiguration `json:"provider,omitempty"`
	Models   []CustomModelConfiguration   `json:"models,omitempty"`
}

type ModelLoadBalancingConfiguration struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Credentials map[string]interface{} `json:"credentials"`
}

type ModelSettings struct {
	Model                string                            `json:"model"`
	ModelType            ModelType                         `json:"model_type"`
	Enabled              bool                              `json:"enabled,omitempty"`
	LoadBalancingConfigs []ModelLoadBalancingConfiguration `json:"load_balancing_configs,omitempty"`
}

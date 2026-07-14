export type ModelType =
  | 'llm'
  | 'text-embedding'
  | 'rerank'
  | 'moderation'
  | 'speech2text'
  | 'tts'
  | 'text2video'
  | 'text2img';

export type ModelUseCase =
  | 'text-chat'
  | 'vision'
  | 'image-gen'
  | 'embedding'
  | 'rerank'
  | 'speech-to-text'
  | 'text-to-speech'
  | 'realtime-audio'
  | 'video-gen'
  | 'moderation'
  | 'reasoning'
  | 'function-calling'
  | 'agent';

export type DefaultModelUseCase = ModelUseCase;
export type DefaultModelSource = 'explicit' | 'auto' | 'none';

export interface DefaultModelValue {
  provider: string;
  model: string;
  params: Record<string, number | string | boolean>;
}

export interface ResolvedDefaultModelItem extends DefaultModelValue {
  use_case: DefaultModelUseCase;
  source: DefaultModelSource;
}

export interface ResolvedDefaultModelList {
  items: ResolvedDefaultModelItem[];
  total: number;
}

export interface UpsertDefaultModelRequest {
  provider: string;
  model: string;
  params?: Record<string, number | string | boolean> | null;
}

export interface DefaultModelRecord extends DefaultModelValue {
  id: string;
  organization_id: string;
  use_case: DefaultModelUseCase;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface GetModelsParams {
  // Filter by input modalities (comma separated)
  input_modalities?: string;
  // Items per page
  page_size?: number;
  // Filter by output modalities (comma separated)
  output_modalities?: string;
  // Page number
  page?: number;
  // Filter by provider
  provider?: string;
  // Search keyword
  search?: string;
  // Filter by model status
  is_enabled?: boolean; // Renamed from is_active
  // Filter by model type
  type?: string;
  // Filter by model capabilities
  capabilities?: string;
  // Filter by model use case
  use_case?: ModelUseCase;
  // Filter by model lifecycle status
  status?: 'active' | 'deprecated';
}

// Available models fetch params (enabled models only)
export interface GetAvailableModelsParams {
  // Filter by model use case
  use_case?: ModelUseCase;
  // Filter by provider
  provider?: string;
}

// Dynamic model capability mappings - keys may be extended by API
export type ModelEndpoints = Record<string, boolean>;
export type ModelFeatures = Record<string, boolean>;
export type ModelTools = Record<string, boolean>;

export interface ModelParameters {
  supports_temperature: boolean;
  supports_top_p: boolean;
  supports_presence_penalty: boolean;
  supports_frequency_penalty: boolean;
  supports_logit_bias: boolean;
  supports_seed: boolean;
  supports_stop: boolean;
  max_stop_sequences: number;
}

// Detailed model metadata components
export interface ModelArchitecture {
  tokenizer?: string;
  instruct_type?: string;
  modality?: string;
}

export interface ModelTrainingData {
  cutoff_date?: string;
  data_sources?: string[];
}

export interface ModelPerformance {
  mmlu_score?: number;
  hellaswag_score?: number;
  humaneval_score?: number;
  [key: string]: number | undefined;
}

export interface ModelUsageGuidelines {
  recommended_use_cases?: string[];
  limitations?: string[];
  safety_considerations?: string[];
}

export interface ModelItem {
  id: string;
  provider: string; // Renamed from name
  model: string; // Renamed from name (used in API)
  model_name: string; // Renamed from display_name
  slug?: string; // App-internal
  family: string;
  family_name?: string; // NEW
  family_slug?: string; // App-internal
  status: string; // active, deprecated, beta, archived
  tagline: string;
  is_flagship: boolean;
  is_recommended: boolean;
  is_featured: boolean;
  is_new: boolean;
  access_type: string; // open, closed, limited
  open_weights?: boolean;
  currency: string;
  input_price: number;
  output_price: number;
  input_price_configured?: boolean;
  output_price_configured?: boolean;
  cached_input_price?: number;
  context_window: number;
  max_output_tokens: number;
  max_input_tokens?: number;
  endpoints: ModelEndpoints;
  features: ModelFeatures;
  tools: ModelTools;
  parameters?: ModelParameters; // Optional, might be removed in later cleanup
  use_cases: ModelUseCase[] | null;
  input_modalities: string[];
  output_modalities: string[];
  is_enabled: boolean; // Renamed from is_active
  is_available: boolean; // Read-only: has active channels
  is_configured: boolean; // Frontend state: Supported by channel
  zgi_official_available?: boolean;
  callable: boolean;
  tier: string;
  created_at: number; // Unix timestamp (seconds)
  updated_at: number; // Unix timestamp (seconds)

  // Depth metadata (from detail view)
  architecture?: ModelArchitecture;
  training_data?: ModelTrainingData;
  performance?: ModelPerformance;
  usage_guidelines?: ModelUsageGuidelines;
  deprecation_date?: string | null;
  deprecation_reason?: string | null;
  replacement_provider?: string | null;
  replacement_model?: string | null;
  unselectable_reason_code?: 'deprecated';
}

export interface ModelList {
  items: ModelItem[];
  total: number;
  page: number;
  limit: number;
  page_size?: number; // Aliased from limit for doc compatibility
  total_pages?: number; // NEW
  has_more: boolean;
}

export type ModelDetail = ModelItem;

export interface ToggleModelRequest {
  model_name: string;
  is_enabled: boolean; // Renamed from is_active
}

export interface ToggleModelResponse {
  is_enabled: boolean; // Renamed from is_active
  message: string;
  model_name: string;
}

export interface ConfigureModelRequest {
  model_id: string;
  is_enabled?: boolean;
  custom_display_name?: string;
  input_price_override?: string;
  output_price_override?: string;
  access_scope?: string;
  visible_groups?: string[];
  visible_users?: string[];
  sort_order?: number;
}

export interface GetModelParametersParams {
  model: string;
  provider: string;
}

export type ParameterValueType = 'int' | 'string' | 'float' | 'boolean' | 'text';
export type ParameterScalarValue = number | string | boolean | null;

export interface ModelConfigParameter {
  name: string;
  template_key: string;
  type: ParameterValueType;
  required: boolean;
  default?: ParameterScalarValue;
  min?: ParameterScalarValue;
  max?: ParameterScalarValue;
  precision?: number | null;
}

export interface CreateCustomModelRequest {
  provider: string; // Slug
  model: string; // Identifier
  model_name: string;
  use_cases: ModelUseCase[];
  context_window?: number;
  max_output_tokens?: number;
  input_price?: string;
  output_price?: string;
  knowledge_cutoff?: string;
  description?: string;
  endpoints?: ModelEndpoints;
  features?: ModelFeatures;
  tools?: ModelTools;
  parameters?: ModelParameters;
  config_parameters?: ModelConfigParameter[];
  is_active?: boolean;
}

export type UpdateCustomModelRequest = Partial<
  Omit<CreateCustomModelRequest, 'provider' | 'model'>
>;

export interface GetCustomModelsParams {
  page?: number;
  page_size?: number;
  provider_id?: string;
  type?: string;
  is_active?: boolean;
}

export interface CustomModelListResponse {
  list: ModelItem[];
  page: number;
  page_size: number;
  total: number;
}

// Localized text for labels and help
export interface LocalizedText {
  zh_Hans?: string;
  en_US?: string;
}

// Parameter rule item returned by the API
export interface ParameterRuleItem extends ModelConfigParameter {
  use_template?: string | null;
  label?: LocalizedText;
  help?: LocalizedText;
  options?: string[];
}

// Batch toggle models request (with specific models array)
export interface BatchToggleModelsRequest {
  provider: string;
  models: string[];
  is_enabled: boolean; // Renamed from is_active
}

// Toggle all provider models request (without models array)
export interface ToggleProviderModelsRequest {
  provider: string;
  is_enabled: boolean; // Renamed from is_active
}

// Response for batch/provider toggle operations
export interface BatchToggleModelsResponse {
  [key: string]: unknown;
}

export type PricingFallbackOperation = 'chat' | 'embedding' | 'rerank' | 'image_generation';
export type PricingFallbackMeter = 'input_token' | 'output_token' | 'image';
export type PricingFallbackSource =
  | 'upstream_model_price'
  | 'admin_fallback'
  | 'code_default_fallback';

export interface PricingFallbackRule {
  id: string;
  enabled?: boolean;
  operation: PricingFallbackOperation;
  meter: PricingFallbackMeter;
  provider?: string;
  model?: string;
  size?: string;
  quality?: string;
  style?: string;
  unit?: string;
  price_usd_per_1m_tokens?: string;
  credits?: number;
  pricing_source?: PricingFallbackSource;
}

export interface PricingFallbackConfig {
  enabled: boolean;
  default_rules: PricingFallbackRule[];
  override_rules: PricingFallbackRule[];
  effective_rules: PricingFallbackRule[];
}

export interface UpdatePricingFallbackConfigRequest {
  enabled: boolean;
  override_rules: PricingFallbackRule[];
}

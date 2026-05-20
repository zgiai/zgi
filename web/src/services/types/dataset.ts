import type { IconType } from '@/utils/icon-helpers';
export type { IconType };

// Reranking weights configuration
export interface RerankingWeights {
  keyword_setting?: { keyword_weight: number };
  vector_setting?: {
    vector_weight: number;
    embedding_model_name: string;
    embedding_provider_name: string;
  };
  /** legacy fields */
  vector_search_weight?: number;
  text_search_weight?: number;
}

// Owner information interface
export interface OwnerInfo {
  id: string;
  name: string;
  email?: string;
  avatar?: string;
}

// Document Status Enums
export enum DocumentDisplayStatus {
  WAITING = 'waiting',
  QUEUING = 'queuing',
  INDEXING = 'indexing',
  PAUSED = 'paused',
  ERROR = 'error',
  AVAILABLE = 'available',
  ENABLED = 'enabled',
  DISABLED = 'disabled',
  ARCHIVED = 'archived',
}

export enum DocumentIndexingStatus {
  WAITING = 'waiting',
  PARSING = 'parsing',
  CLEANING = 'cleaning',
  SPLITTING = 'splitting',
  INDEXING = 'indexing',
  PAUSED = 'paused',
  ERROR = 'error',
  COMPLETED = 'completed',
  // GraphFlow-specific statuses
  EXTRACTING = 'extracting',
  ALIGNMENT = 'alignment',
  INGESTING = 'ingesting',
}

export type DocumentExtractionStrategy =
  | 'mineru'
  | 'reducto'
  | 'local'
  | 'unstructured'
  | 'landingai';

export interface DocumentExtractionStrategiesResponse {
  strategies: DocumentExtractionStrategy[];
  recommended_strategy?: DocumentExtractionStrategy;
  items?: DocumentExtractionStrategyStatus[];
}

export interface DocumentExtractionStrategyStatus {
  strategy: DocumentExtractionStrategy;
  available: boolean;
  configured: boolean;
  recommended?: boolean;
  reason?: string;
}

export interface DocumentExtractionAttempt {
  strategy: DocumentExtractionStrategy;
  etl_type?: string;
  success: boolean;
  error?: string;
}

export interface DocumentExtractionMetadata {
  requested_strategy?: DocumentExtractionStrategy;
  actual_strategy?: DocumentExtractionStrategy;
  fallback_used?: boolean;
  attempts?: DocumentExtractionAttempt[];
}

export interface DocumentDocMetadata {
  extraction?: DocumentExtractionMetadata;
  [key: string]: unknown;
}

// Document list query parameters interface
export interface DocumentListParams {
  /** Index status */
  indexing_status?: DocumentIndexingStatus;
  /** Search keyword */
  keyword?: string;
  /** Items per page (max: 100) */
  limit?: string;
  /** Page number */
  page?: string;
  /** Sort field, prefix "-" means descending order, e.g. -created_at */
  sort?: string;
  [property: string]: unknown;
}

// Retrieval search method type
export type SearchMethod = 'graph_search' | 'semantic_search';

export enum ProcessStatus {
  WAITING = 'waiting',
  PROCESSING = 'processing',
  COMPLETED = 'completed',
  ERROR = 'error',
}

export type SegmentEnabledFilter = boolean | 'all';

export type SegmentStatus = 'enabled' | 'disabled' | 'processing' | 'error';

// Document Detail Interface
export interface DocumentDetail {
  id: string;
  name: string;
  doc_language: string;
  display_status: DocumentDisplayStatus;
  indexing_status: DocumentIndexingStatus;
  graph_indexing_status?: string;
  progress?: number;
  enabled: boolean;
  archived: boolean;
  word_count: number;
  hit_count: number;
  segment_count: number;
  data_source_info: {
    upload_file: {
      id: string;
      name: string;
      extension: string;
      size: number;
      mime_type: string;
      created_at: number;
      created_by: string;
    };
    provider?: string;
    job_id: string;
    url: string;
  };
  document_process_rule: {
    rules: {
      pre_processing_rules: Array<{ id: string; enabled: boolean }>;
    };
  };
  created_at: number;
  updated_at: number;
  can_edit: boolean;
}

// Document Metadata Interface
export interface DocumentMetadataDetail {
  doc_type?: string | null | 'others';
  doc_metadata?: DocumentDocMetadata | null;
}

// Preview data types
export interface PreviewChunk {
  content: string;
  child_chunks?: string[];
}

export interface QAPreviewChunk {
  content: string;
  answer?: string;
}

// Union type for preview results
export type PreviewResult = PreviewChunk | QAPreviewChunk;

// Question Interface
export interface Question {
  created_at?: number;
  created_by?: string;
  dataset_id?: string;
  document_id?: string;
  id?: string;
  question?: string;
  segment_id?: string;
  workspace_id?: string;
  updated_at?: number;
}

/**
 * SegmentQuestion
 * Strict question model used in UI state for segment-bound questions.
 */
export interface SegmentQuestion {
  id: string;
  question: string;
}

export interface QuestionsResponse {
  data: Question[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

// Random questions response for dataset-level API
export interface RandomQuestionsResponse {
  data: Array<{
    id: string;
    question: string;
  }>;
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

// Segment Interface
export interface ChildChunkDetail {
  id: string;
  position: number;
  segment_id: string;
  content: string;
  word_count: number;
  created_at: number;
  updated_at: number;
  type: 'automatic' | 'customized';
}

export interface SegmentDetail {
  id: string;
  position: number;
  document_id: string;
  content: string;
  sign_content: string;
  word_count: number;
  tokens: number;
  keywords: string[];
  index_node_id: string;
  index_node_hash: string;
  hit_count: number;
  enabled: boolean;
  disabled_at: number;
  disabled_by: string;
  status: SegmentStatus;
  created_by: string;
  created_at: number;
  indexing_at: number;
  completed_at: number;
  error: string | null;
  stopped_at: number;
  answer?: string;
  child_chunks?: ChildChunkDetail[];
  updated_at: number;
}

export interface SegmentsResponse {
  data: SegmentDetail[];
  has_more: boolean;
  limit: number;
  total: number;
  total_pages: number;
  page: number;
}

// Dataset Operations
export interface CreateSegmentRequest {
  content: string;
  answer?: string;
  keywords?: string[];
  regenerate_child_chunks?: boolean;
}

export interface CreateDatasetRequest {
  name: string;
  description?: string;
  provider: string;
  workspace_id?: string;
  data_source_type: string;
  indexing_technique: string;
  embedding_model_provider: string;
  embedding_model: string;
  entity_model_provider?: string;
  entity_model?: string;
  icon_type: IconType;
  icon: string;
  icon_background: string;
  enable_graph_flow?: boolean;
  folder_id?: string;
  retrieval_config?: {
    search_method: SearchMethod;
    top_k: number;
    score_threshold: number;
    score_threshold_enabled: boolean;
    reranking_enable: boolean;
    reranking_model?: {
      reranking_provider_name: string;
      reranking_model_name: string;
    };
  };
}

export interface UpdateDatasetRequest {
  name?: string;
  description?: string;
  embedding_model?: string;
  embedding_model_provider?: string;
  entity_model?: string;
  entity_model_provider?: string;
  icon?: string;
  icon_type?: IconType;
  icon_background?: string;
  workspace_id?: string;
  enable_graph_flow?: boolean;
  retrieval_config?: {
    search_method?: SearchMethod;
    top_k?: number;
    score_threshold_enabled?: boolean;
    score_threshold?: number;
    reranking_enable?: boolean;
    reranking_model?: {
      reranking_provider_name: string;
      reranking_model_name: string;
    };
  };
}

export interface UpdateSegmentRequest {
  content: string;
  answer?: string;
  keywords?: string[];
  regenerate_child_chunks?: boolean;
}

export interface CreateSegmentResponse {
  data: SegmentDetail;
}

// Child Segments (for hierarchical mode)
export interface ChildSegmentsResponse {
  data: ChildChunkDetail[];
  total: number;
  total_pages: number;
  page: number;
  limit: number;
}

export interface CreateChildSegmentRequest {
  content: string;
}

export interface UpdateChildSegmentRequest {
  content: string;
}

// Batch Import Interface
export interface BatchImportResponse {
  job_id: string;
  job_status: ProcessStatus;
}

export interface BatchImportStatusResponse {
  job_id: string;
  job_status: ProcessStatus;
}

export interface Dataset {
  id: string;
  name: string;
  description: string;
  provider: string;
  data_source_type: string;
  indexing_technique: string;
  word_count: number;
  created_by: string;
  created_at: string;
  updated_by: string | null;
  updated_at: string;
  embedding_model: string;
  embedding_model_provider: string;
  entity_model?: string;
  entity_model_provider?: string;
  embedding_available: boolean;
  retrieval_config: InternalRetrievalConfig;
  tags: string[] | null;
  doc_form?: string;
  external_knowledge_info?: {
    external_knowledge_id: string | null;
    external_knowledge_api_id: string | null;
    external_knowledge_api_name: string | null;
    external_knowledge_api_endpoint: string | null;
  };
  external_retrieval_model?: {
    top_k: number;
    score_threshold: number;
    score_threshold_enabled: boolean;
  };
  enable_graph_flow?: boolean;
  extraction_strategy?: string;
  icon: string;
  icon_type: IconType;
  icon_background: string;
  icon_url?: string;
  app_count: number;
  document_count: number;
  available_document_count: number;
  available_segment_count: number;
  collection_binding_id: string | null;
  owner: OwnerInfo | null;
  owner_account: {
    name: string;
  } | null;
  workspace_id?: string;
  workspace?: {
    id: string;
    name: string;
  };
  is_editor: boolean;
  can_edit: boolean;
}

export interface DatasetList {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: Dataset[];
}

// Document status types
export type DocumentStatus =
  | 'processing'
  | 'completed'
  | 'error'
  | 'pending'
  | 'indexing'
  | 'disabled'
  | 'failed'
  | 'auto_disabled';

// Document processing result
export interface DocumentProcessingResult {
  total_segments: number;
  total_tokens: number;
  total_word_count: number;
  process_time?: number;
}

// Document metadata
export interface DocumentMetadata {
  file_size?: number;
  file_type?: string;
  original_filename?: string;
  mime_type?: string;
  page_count?: number;
  language?: string;
  author?: string;
  creation_date?: string;
  modification_date?: string;
  [key: string]: unknown;
}

// Enhanced document interface
export interface Document {
  id: string;
  dataset_id: string;
  name: string;
  content?: string;
  created_at: number;
  updated_at: number;
  /**
   * Legacy overall processing status. Newer API prefers `display_status` and `indexing_status`.
   */
  status?: DocumentStatus;
  error_message?: string;
  processing_result?: DocumentProcessingResult;
  metadata?: DocumentMetadata;
  created_by?: string;
  updated_by?: string;
  enabled?: boolean;
  archived?: boolean;
  /**
   * Document display status as provided by backend API (e.g. "completed", "queuing", "error").
   */
  display_status?: DocumentDisplayStatus;

  /**
   * Indexing status indicating the current processing step (e.g. "indexing", "completed").
   */
  indexing_status?: DocumentIndexingStatus;

  /**
   * Graph indexing status for GraphFlow-enabled datasets.
   */
  graph_indexing_status?: string;

  /**
   * Overall processing progress (0-100) provided by backend API.
   */
  progress?: number;

  /**
   * Index position of the document inside the dataset list.
   */
  position?: number;

  /**
   * Data source type, currently only "upload_file" is supported by API.
   */
  data_source_type?: 'upload_file' | string;

  /**
   * Data source information object. The structure may vary depending on the data_source_type.
   */
  data_source_info?: {
    upload_file_id: string;
    /** Allow additional dynamic keys returned by the API */
    [key: string]: unknown;
  };

  /**
   * Dataset process rule identifier associated with this document.
   */
  dataset_process_rule_id?: string;

  /**
   * Full process rule object returned by the API.
   */
  dataset_process_rule?: {
    id: string;
    mode: string;
    rules: {
      pre_processing_rules: Array<{ id: string; enabled: boolean }>;
      segmentation?: {
        separator: string;
        max_tokens: number;
        chunk_overlap: number;
      };
      subchunk_segmentation?: {
        separator: string;
        max_tokens: number;
      };
    };
    createdAt: number;
    createdBy: string;
  };

  /**
   * Chunking model type (e.g. "text_model", "hierarchical_model", "qa_model").
   */
  doc_form?: string;

  /**
   * Source of creation such as "web" or "api".
   */
  created_from?: string;

  /**
   * Token count for the document. It can be null when not yet processed.
   */
  tokens?: number | null;

  /**
   * Document processing completion timestamp.
   */
  completed_at?: number;

  /**
   * Indexing latency in milliseconds or null.
   */
  indexing_latency?: number | null;

  /**
   * Error message returned from backend.
   */
  error?: string | null;

  /**
   * Timestamp when the document was disabled. Null if not disabled.
   */
  disabled_at?: number | null;

  /**
   * User ID of the operator who disabled the document.
   */
  disabled_by?: string | null;

  /**
   * Segment statistics for the document.
   */
  segment_count?: number;
  average_segment_length?: number;
  hit_count?: number;
  word_count?: number;

  /**
   * Original language code or name, e.g. "English".
   */
  doc_language?: string;

  /**
   * Document-level metadata returned by backend.
   */
  doc_metadata?: DocumentDocMetadata | null;
}

// Document list with pagination
export interface DocumentList {
  page: number;
  limit: number;
  total: number;
  data: Document[];
}

// Application linked to a dataset
export interface RelatedApp {
  id: string;
  name: string;
  // Allow extra fields from API without using `any`
  [key: string]: unknown;
}

export interface RelatedAppList {
  total: number;
  data: RelatedApp[];
}

// Hit testing related types
export interface HitTestingQuery {
  content: string;
  tsne_position?: TsnePosition;
}

export interface TsnePosition {
  x: number;
  y: number;
}

// Real API segment structure
export interface HitTestingSegment {
  id: string;
  position: number;
  document_id: string;
  content: string;
  sign_content: string;
  answer: string | null;
  word_count: number;
  tokens: number;
  keywords: string[] | null;
  index_node_id: string;
  index_node_hash: string;
  hit_count: number;
  enabled: boolean;
  disabled_at: number | null;
  disabled_by: string | null;
  status: string;
  created_by: string;
  created_at: number;
  indexing_at: number;
  completed_at: number;
  error: string | null;
  stopped_at: number | null;
  document: {
    id: string;
    data_source_type: string;
    name: string;
    doc_type: string | null;
    doc_metadata: Record<string, unknown> | null;
  };
}

export interface HitTestingChildChunk {
  id: string;
  content: string;
  position: number;
  score: number;
}

export interface HitTestingResult {
  segment: HitTestingSegment;
  child_chunks: HitTestingChildChunk[];
  score: number;
  tsne_position: TsnePosition | null;
  match_type?: 'original' | 'graph_knowledge';
  retrieval_source?: {
    method: string;
    reason: string;
    matched_entities?: string[];
  };
}

export interface HitTestingResponse {
  query: HitTestingQuery;
  records: HitTestingResult[];
  retrieval_time?: number;
  total_segments?: number;
  elapsed_time?: number;
  graph_execution?: {
    entities: string[];
    triples: any[];
    steps: Array<{
      step: number;
      action: string;
      description: string;
      result: string;
    }>;
    summary: string;
    debug_info: {
      chunks_count: number;
      entities_count: number;
      hop_depth: number;
      seeds: string[];
      triples_count: number;
    };
  };
}

// External Dataset Hit Testing Types
export interface ExternalDatasetHitTestingResponse {
  query: { content: string };
  records: Array<{
    content: string;
    title: string;
    score: number;
    metadata: {
      'x-amz-bedrock-kb-source-uri': string;
      'x-amz-bedrock-kb-data-source-id': string;
    };
  }>;
}

// Unified retrieval configuration for internal dataset
export interface InternalRetrievalConfig {
  search_method: SearchMethod;
  reranking_enable: boolean;
  reranking_model?: {
    reranking_provider_name: string;
    reranking_model_name: string;
  };
  top_k: number;
  score_threshold_enabled: boolean;
  score_threshold: number;
}

// Simplified config for API requests (matches backend expectations)
export interface HitTestingConfig {
  search_method: SearchMethod;
  top_k: number;
  score_threshold_enabled: boolean;
  score_threshold: number;
  reranking_enable: boolean;
}

// External retrieval configuration
export interface ExternalRetrievalConfig {
  search_method: SearchMethod;
  top_k: number;
  score_threshold_enabled: boolean;
  score_threshold: number;
  reranking_enable: boolean;
}

// Hit testing history record
export interface HitTestingHistoryRecord {
  /** Unique identifier of the query record */
  id: string;

  /** The query text content returned by backend. */
  content: string;

  /** Source of the query record, e.g. "hit_testing" or "app". */
  source: string;

  /** The related app id when source = "app". Nullable otherwise. */
  source_app_id: string | null;

  /** Role of the creator (e.g. "account"). */
  created_by_role: string;

  /** Creator user ID. */
  created_by: string;

  /** ISO-8601 timestamp when the query was created. */
  created_at: string;

  /** Elapsed time of the query. */
  elapsed_time: number;
}

// Hit testing history response
export interface HitTestingHistoryResponse {
  data: HitTestingHistoryRecord[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

// API request interfaces
export interface HitTestingRequest {
  query: string;
  retrieval_model: InternalRetrievalConfig;
}

export interface ExternalHitTestingRequest {
  query: string;
  external_retrieval_model: ExternalRetrievalConfig;
}

// Batch hit testing request interface
export interface BatchHitTestingRequest {
  dataset_ids: string[];
  queries: string[];
  retrieval_model?: InternalRetrievalConfig;
}

export interface BatchHitTestingResponse {
  task_id: string;
}

// Legacy interface for backward compatibility
export interface HitTestingHistoryItem {
  id: string;
  query: string;
  config: HitTestingConfig;
  results_count: number;
  timestamp: number;
}

// Enhanced RelatedApp interface with more complete typing
export interface RelatedAppComplete {
  id: string;
  name: string;
  description?: string;
  icon?: string;
  icon_background?: string;
  created_at: number;
  updated_at: number;
  mode: 'chat' | 'agent' | 'workflow' | 'completion';
  status: 'active' | 'draft' | 'archived';
  model_config?: {
    provider: string;
    model: string;
    temperature?: number;
    max_tokens?: number;
  };
  usage_stats?: {
    total_conversations: number;
    total_messages: number;
    active_users: number;
    last_used_at?: number;
  };
  owner?: OwnerInfo;
  permissions?: string[];
}

// External dataset types
export interface ExternalDataset {
  id: string;
  name: string;
  description?: string;
  external_knowledge_api_id: string;
  external_knowledge_api_name: string;
  external_knowledge_api_endpoint: string;
  created_at: number;
  updated_at: number;
  status: 'active' | 'inactive' | 'error';
  last_sync_at?: number;
  config?: Record<string, unknown>;
}

export interface BulkOperation {
  ids: string[];
}
export interface BulkOperationResult {
  result: 'success' | 'fail';
  ids: string[];
}
export interface IndexingEstimate {
  tokens: number;
  total_price: number;
  currency: string;
}

export interface IndexingProgress {
  total_segments: number;
  completed_segments: number;
  status: string;
}
export interface DatasetSettings {
  name: string;
  description?: string;
  retrieval_config: InternalRetrievalConfig;
}
export interface ExternalAPI {
  id: string;
  name: string;
  endpoint: string;
}

export interface ProcessRuleResponse {
  mode: string;
  rules: {
    pre_processing_rules: Array<{ id: string; enabled: boolean }>;
  };
  limits?: {
    indexing_max_segmentation_tokens_length: number;
  };
  [key: string]: any;
}

export interface DocumentIndexingStatusResponse {
  indexing_status: DocumentIndexingStatus;
  display_status: DocumentDisplayStatus;
  total_segments: number;
  completed_segments: number;
  processing_started_at?: number;
  parsing_completed_at?: number;
  cleaning_completed_at?: number;
  splitting_completed_at?: number;
  completed_at?: number;
  paused_at?: number;
  stopped_at?: number;
  error?: string;
  graph_indexing_status?: string;
  progress?: number;
}

export interface ErrorDocsResponse {
  total: number;
  data: any[];
}

/**
 * UploadedFile
 * Simplified model for files that have been successfully uploaded to cloud.
 */
export interface UploadedFile {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  hash?: string;
  created_by?: string;
  created_at?: number | string;
  url?: string;
  [key: string]: unknown;
}

/**
 * ProcessConfiguration
 * Configuration used during dataset creation/document upload.
 */
export interface ProcessConfiguration {
  clean_mode: 'automatic' | 'manual';
  pre_processing_rules: Array<{ id: string; enabled: boolean }>;
  rules: Record<string, unknown>;
}

/**
 * Form data structure for dataset creation wizard.
 */
export interface DatasetUploadFormData {
  files: UploadedFile[];
  notionPages?: any[];
  crawlResults?: any[];
  processConfig: ProcessConfiguration | null;
  docLanguage?: string;
  indexType?: string;
  embeddingModel?: string | null;
  embeddingModelProvider?: string | null;
  enableGraphFlow?: boolean;
  entityModel?: string | null;
  entityModelProvider?: string | null;
  retrievalConfig: InternalRetrievalConfig | null;
}

/* -------------------------------------------------------------------------- */
/* Graph Related Types                                                        */
/* -------------------------------------------------------------------------- */

export interface GraphNodeSource {
  doc: {
    id: string;
    title: string;
  };
  weight: number;
}

export interface GraphNode {
  id: string;
  label: string;
  category: string;
  data: {
    description: string;
    sources: GraphNodeSource[];
  };
  [key: string]: any;
}

export interface GraphEdge {
  source: string;
  target: string;
  label: string;
}

export interface GraphCategory {
  id: string;
  label: Record<string, string>;
}

export interface DatasetGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
  categories: GraphCategory[];
}

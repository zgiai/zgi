/**
 * File management service types
 */
import type { Dataset } from './dataset';
import type { ParseProviderKey } from './content-parse';

export type FileUploadProcessingMode = 'store_only' | 'process_now';
export type FileParseProviderKey = ParseProviderKey;

export type FileAssetProductStatus =
  | 'stored_only'
  | 'parsing'
  | 'confirming'
  | 'generating'
  | 'parse_failed'
  | 'ready';

export type FileAssetProcessingStage =
  | 'upload'
  | 'parse'
  | 'review'
  | 'chunk'
  | 'vectorize'
  | 'sync';

export type FileAssetVectorStatus = 'none' | 'indexing' | 'ready' | 'failed';

export type FileProcessingRequestMode = 'parse_now' | 'reparse' | 'generate_after_confirm';

export type FileProcessingTargetLevel = 'parse' | 'split' | 'vectorize' | 'full';

export type FileProcessingRequestStatus =
  | 'planned'
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled';

export type FileParseConfirmationStatus = 'pending' | 'kept' | 'edited' | 'ignored';

export type FileParseConfirmationAction = 'keep' | 'edit' | 'ignore';

export type FileDocumentChunkType = 'parent' | 'child' | 'manual' | 'auto';

export type FileDocumentChunkStatus = 'ready' | 'reindexing' | 'error' | 'deleted';

export type JsonObject = Record<string, unknown>;

export interface FileItem {
  content_text: null | string;
  created_at: string;
  created_by: string;
  created_by_role: string;
  extension: string;
  hash: string;
  id: string;
  is_favorite?: boolean;
  key: string;
  mime_type: string;
  name: string;
  related_count: number;
  related_dataset_count: number;
  size: number;
  source_url: string;
  storage_type: string;
  workspace_id: string;
  used: boolean;
  used_at: null;
  used_by: null;
  asset_id?: string;
  processing_status?: FileAssetProductStatus | string;
  processing_stage?: FileAssetProcessingStage | string;
  processing_progress?: number;
  processing_request_id?: string;
  processing_run_id?: string;
  generation_no?: number;
  pending_confirmation_count?: number;
  chunk_count?: number;
  embedding_count?: number;
  vector_status?: FileAssetVectorStatus | string;
  last_error_code?: string;
  last_error_message?: string;
}

export interface AllFilesResponse {
  data: FileItem[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface FileMetadataResponse {
  data: FileItem[];
}

export interface GetAllFilesRequest {
  page: string;
  limit: string;
  keyword?: string;
  sort?: string;
  folder_id?: string;
  extension?: string;
  workspace_id?: string;
}

export interface RelatedResource {
  datasets: {
    count: number;
    items: Dataset[];
  };
}

export interface RelatedResourceItem {
  id: string;
  name: string;
  type: 'dataset' | 'agent' | 'workflow';
}

export interface RelatedResourcesResponse {
  data: RelatedResource;
}

export interface FileOriginalPreviewUrlResponse {
  url: string;
  file_id: string;
  name: string;
  extension: string;
  mime_type: string;
}

export interface FileSourcePreviewPagesResponse {
  engine: string;
  page_count: number;
  pages: string[];
}

export interface StorageUsage {
  used: number; // GB
  total: number; // GB
}

/**
 * File folder item
 */
export interface FileFolder {
  id: string;
  name: string;
  description: string | null;
  icon: string | null;
  icon_background: string | null;
  icon_type: string | null;
  parent_id: string | null;
  position: number;
  workspace_id: string;
  created_at: string;
  created_by: string;
  updated_at: string;
  updated_by: string | null;
}

/**
 * File folders response
 */
export interface FileFoldersResponse {
  data: FileFolder[];
}

/**
 * Upload file request
 */
export interface UploadFileRequest {
  file: File;
  folder_id?: string;
  workspace_id?: string;
  processing_mode?: FileUploadProcessingMode;
  parse_provider?: FileParseProviderKey;
}

/**
 * Upload file response
 */
export interface UploadFileResponse {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  created_at: string;
  created_by: string;
  processing_mode?: FileUploadProcessingMode;
  asset_id?: string;
  processing_status?: FileAssetProductStatus | string;
  processing_request_id?: string;
  processing_run_id?: string;
  generation_no?: number;
}

export interface ReplaceDocumentRequest {
  file: File;
  processing_mode?: FileUploadProcessingMode;
  parse_provider?: FileParseProviderKey;
}

export interface ReplaceDocumentResponse {
  file: UploadFileResponse;
  asset?: unknown;
  processing_request?: unknown;
  processing_run_id?: string;
  generation_no?: number;
  processing_mode: FileUploadProcessingMode;
}

/**
 * Create folder request
 */
export interface CreateFolderRequest {
  name: string;
  parent_id?: string;
  workspace_id?: string;
}

/**
 * Create folder response
 */
export interface CreateFolderResponse {
  id: string;
  name: string;
  parent_id: string | null;
  created_at: string;
  created_by: string;
}

/**
 * Update folder request
 */
export interface UpdateFolderRequest {
  name: string;
  parent_id: string;
}

/**
 * Update folder response
 */
export interface UpdateFolderResponse {
  id: string;
  name: string;
  parent_id: string | null;
  updated_at: string;
  updated_by: string;
}

/**
 * Create text file request
 */
export interface CreateTextFileRequest {
  filename: string;
  content: string;
  folder_id?: string;
  workspace_id?: string;
}

/**
 * Create text file response
 */
export interface CreateTextFileResponse {
  id: string;
  name: string;
  size: number;
  extension: string;
  mime_type: string;
  created_at: string;
  created_by: string;
}

export interface FileDocumentAsset {
  id: string;
  organization_id: string;
  workspace_id?: string;
  title: string;
  source_file_id: string;
  current_version_id?: string;
  content_hash?: string;
  status: string;
  processing_level: string;
  product_status: FileAssetProductStatus | string;
  processing_stage?: FileAssetProcessingStage | string;
  processing_progress: number;
  active_processing_request_id?: string;
  processing_run_id?: string;
  generation_no: number;
  parse_artifact_id?: string;
  chunk_artifact_set_id?: string;
  chunk_count: number;
  embedding_provider?: string;
  embedding_model?: string;
  embedding_dimension?: number;
  vector_status: FileAssetVectorStatus | string;
  last_error_code?: string;
  last_error_message?: string;
  quality_score?: number;
  metadata_json?: JsonObject;
  permission_policy?: JsonObject;
  created_by?: string;
  created_at: string;
  updated_at: string;
}

export interface FileProcessingRequestPlan {
  asset_id: string;
  target_level: FileProcessingTargetLevel | string;
  will_parse: boolean;
  will_split: boolean;
  will_vectorize: boolean;
  will_extract_full: boolean;
}

export interface FileProcessingRequestView {
  id: string;
  organization_id: string;
  workspace_id?: string;
  asset_id: string;
  target_level: FileProcessingTargetLevel | string;
  status: FileProcessingRequestStatus | string;
  requested_by?: string;
  force: boolean;
  plan?: FileProcessingRequestPlan | null;
  request_metadata?: JsonObject;
  execution_metadata?: JsonObject;
  executor_key?: string;
  error_code?: string;
  error_message?: string;
  attempt_count: number;
  queued_at?: string;
  started_at?: string;
  completed_at?: string;
  failed_at?: string;
  cancelled_at?: string;
  created_at: string;
  updated_at: string;
}

export interface FileProcessingSummary {
  asset_id?: string;
  product_status?: FileAssetProductStatus | string;
  processing_stage?: FileAssetProcessingStage | string;
  processing_progress?: number;
  pending_confirmation_count?: number;
  chunk_count?: number;
  embedding_count?: number;
  embedding_provider?: string;
  embedding_model?: string;
  embedding_dimension?: number;
  vector_status?: FileAssetVectorStatus | string;
  processing_request_id?: string;
  processing_run_id?: string;
  generation_no?: number;
  last_error_code?: string;
  last_error_message?: string;
}

export interface FileAssetArtifactState {
  has_parse_artifact?: boolean;
  has_chunks?: boolean;
  has_embeddings?: boolean;
  parse_artifact_id?: string;
  chunk_artifact_set_id?: string;
  chunk_count?: number;
  embedding_provider?: string;
  embedding_model?: string;
  embedding_dimension?: number;
  vector_status?: FileAssetVectorStatus | string;
}

export interface FileAssetProcessingError {
  code?: string;
  message?: string;
}

export interface FileDetailProcessing {
  latest_request?: FileProcessingRequestView;
  summary: FileProcessingSummary;
  pending_confirmation_count: number;
  chunk_count: number;
  embedding_count: number;
}

export interface FileDetailResponse {
  file: FileItem;
  asset?: FileDocumentAsset;
  processing?: FileDetailProcessing;
  artifact_state?: FileAssetArtifactState;
  error?: FileAssetProcessingError;
}

export interface CreateFileProcessingRequest {
  target_level?: FileProcessingTargetLevel;
  mode?: FileProcessingRequestMode;
  force?: boolean;
  parse_provider?: FileParseProviderKey;
}

export interface CreateFileProcessingResponse {
  asset: FileDocumentAsset;
  processing_request: FileProcessingRequestView;
  processing_run_id?: string;
  generation_no: number;
  file_id: string;
  target_level: FileProcessingTargetLevel | string;
  mode: FileProcessingRequestMode | string;
  request_queue_status: FileProcessingRequestStatus | string;
}

export interface FileParseBoundingBox {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export interface FileParsePreviewConfirmation {
  id: string;
  artifact_element_id?: string;
  element_index?: number;
  item_type: string;
  status: FileParseConfirmationStatus | string;
  original_content: string;
  suggested_content?: string;
  final_content?: string;
  confidence?: number;
  review_reason?: string;
  source_locator?: JsonObject;
}

export interface FileParsePreviewElement {
  id?: string;
  type: string;
  subtype?: string;
  page: number;
  content?: string;
  bbox?: FileParseBoundingBox;
  ordinal: number;
  precision?: string;
  confidence?: number;
  metadata?: JsonObject;
  confirmation?: FileParsePreviewConfirmation;
}

export interface FileParsePreviewResponse {
  asset_id: string;
  file_id: string;
  product_status: FileAssetProductStatus | string;
  processing_run_id?: string;
  generation_no: number;
  parse_artifact_id: string;
  artifact_status: string;
  artifact_quality_level: string;
  engine_used?: string;
  text?: string;
  markdown?: string;
  elements: FileParsePreviewElement[];
  confirmation_items: FileParsePreviewConfirmation[];
  total_confirmation_count: number;
  pending_confirmation_count: number;
}

export interface FileParseConfirmationItem {
  id: string;
  organization_id: string;
  workspace_id?: string;
  asset_id: string;
  processing_run_id: string;
  generation_no: number;
  item_type: string;
  status: FileParseConfirmationStatus | string;
  source_locator_json?: JsonObject;
  original_content: string;
  suggested_content?: string;
  final_content?: string;
  confidence?: number;
  review_reason?: string;
  created_by?: string;
  updated_by?: string;
  resolved_at?: string;
  created_at: string;
  updated_at: string;
}

export interface FileParseConfirmationListResponse {
  asset_id: string;
  file_id: string;
  product_status: FileAssetProductStatus | string;
  processing_run_id?: string;
  generation_no: number;
  items: FileParseConfirmationItem[];
  total: number;
  pending_count: number;
}

export interface ResolveFileParseConfirmationRequest {
  action: FileParseConfirmationAction;
  final_content?: string;
}

export interface ResolveFileParseConfirmationResponse {
  item: FileParseConfirmationItem;
  pending_count: number;
  should_generate: boolean;
  generation_request?: CreateFileProcessingResponse | null;
}

export interface BatchIgnoreFileParseConfirmationsRequest {
  item_ids?: string[];
}

export interface BatchIgnoreFileParseConfirmationsResponse {
  items: FileParseConfirmationItem[];
  resolved_count: number;
  pending_count: number;
  should_generate: boolean;
  generation_request?: CreateFileProcessingResponse | null;
}

export interface FileDocumentChunk {
  id: string;
  organization_id: string;
  workspace_id?: string;
  asset_id: string;
  processing_run_id: string;
  generation_no: number;
  chunk_artifact_set_id?: string;
  parent_chunk_id?: string;
  position: number;
  chunk_type: FileDocumentChunkType | string;
  content: string;
  content_hash: string;
  source_locator_json?: JsonObject;
  enabled: boolean;
  status: FileDocumentChunkStatus | string;
  metadata_json?: JsonObject;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
  children?: FileDocumentChunk[];
}

export interface ListFileChunksRequest {
  page?: number;
  limit?: number;
  search?: string;
  status?: FileDocumentChunkStatus | string;
  chunk_type?: Array<FileDocumentChunkType | string>;
  enabled?: boolean;
  parent_chunk_id?: string;
  include_tree?: boolean;
}

export interface ListFileChunksResponse {
  asset: FileDocumentAsset;
  items: FileDocumentChunk[];
  tree?: FileDocumentChunk[];
  total: number;
  primary_chunk_count?: number;
  secondary_chunk_count?: number;
  embedding_count?: number;
  limit: number;
  page: number;
  has_more: boolean;
  generation_no: number;
}

export interface UpdateFileChunkRequest {
  content?: string;
  enabled?: boolean;
}

export interface FileDocumentChunkEmbedding {
  id: string;
  organization_id: string;
  workspace_id?: string;
  asset_id: string;
  chunk_id: string;
  processing_run_id: string;
  generation_no: number;
  embedding_provider: string;
  embedding_model: string;
  embedding_dimension: number;
  embedding_vector?: number[];
  content_hash: string;
  status: string;
  metadata_json?: JsonObject;
  created_at: string;
  updated_at: string;
}

export interface UpdateFileChunkResponse {
  asset: FileDocumentAsset;
  chunk: FileDocumentChunk;
  embedding?: FileDocumentChunkEmbedding;
  embedding_ready: boolean;
}

export interface AskFileQuestionRequest {
  question: string;
  top_k?: number;
}

export interface FileQuestionAnswerChildSource {
  chunk_id: string;
  position: number;
  content: string;
  snippet: string;
  score?: number;
  distance?: number;
}

export interface FileQuestionAnswerSource {
  primary_chunk_id: string;
  position: number;
  content: string;
  snippet: string;
  score?: number;
  distance?: number;
  children: FileQuestionAnswerChildSource[];
}

export interface FileQuestionAnswerRetrieval {
  top_k: number;
  hit_count: number;
  primary_hit_count: number;
  embedding_provider?: string;
  embedding_model?: string;
  answer_model?: string;
}

export interface AskFileQuestionResponse {
  answer: string;
  sources: FileQuestionAnswerSource[];
  retrieval: FileQuestionAnswerRetrieval;
}

export interface FileQuestionStreamEvent {
  type: 'retrieval' | 'delta' | 'done' | 'error';
  delta?: string;
  answer?: string;
  sources?: FileQuestionAnswerSource[];
  retrieval?: FileQuestionAnswerRetrieval;
  error?: string;
}

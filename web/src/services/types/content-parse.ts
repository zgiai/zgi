export type ParseProviderKey = 'auto' | 'local' | 'mineru' | 'reducto' | 'vlm' | 'hyperparse_api';

export type ParseOCREngine = 'auto' | 'tesseract' | 'paddleocr';

export type ParseProfile =
  | 'auto'
  | 'high_quality'
  | 'fast'
  | 'local_first'
  | 'default'
  | 'fast_preview'
  | 'layout_first'
  | 'text_first'
  | 'dataset_index';

export type ParseIntent = 'preview' | 'dataset_index' | 'chat_context';

export interface ParseBoundingBox {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export interface ParsedElement {
  id?: string;
  type: string;
  subtype?: string;
  page: number;
  content?: string;
  bbox?: ParseBoundingBox;
  ordinal: number;
  precision?: string;
  confidence?: number;
  metadata?: Record<string, unknown>;
}

export interface ParseArtifact {
  artifact_id?: string;
  source_type: string;
  source_ref?: string;
  file_name?: string;
  intent: ParseIntent;
  profile?: ParseProfile;
  status: 'succeeded' | 'degraded' | 'failed';
  quality_level: 'high' | 'standard' | 'degraded' | 'failed';
  engine_used?: string;
  fallback_used?: boolean;
  text?: string;
  markdown?: string;
  elements?: ParsedElement[];
  metadata?: Record<string, unknown>;
  diagnostics?: Record<string, unknown>;
}

export interface RouteCandidate {
  provider_key: string;
  adapter_name: string;
  engine_name?: string;
  priority?: number;
  fallback_only?: boolean;
  reason?: Record<string, unknown>;
}

export interface RoutePlan {
  mode: ParseProfile;
  requested_engine?: string;
  primary?: RouteCandidate;
  fallback_candidates?: RouteCandidate[];
  metadata?: Record<string, unknown>;
}

export interface ChunkSourceDocument {
  document_id?: string;
  dataset_id?: string;
  file_id?: string;
  source?: string;
  title?: string;
  language?: string;
  elements?: ParsedElement[];
  metadata?: Record<string, unknown>;
  diagnostics?: Record<string, unknown>;
}

export interface ChunkPlan {
  use_case: string;
  parent_mode?: string;
  segmentation?: string;
  target_kinds?: string[];
  preserve_order: boolean;
  metadata?: Record<string, unknown>;
}

export interface ContentParseQualitySummary {
  status: string;
  quality_level: string;
  engine_used?: string;
  fallback_used: boolean;
  duration_ms: number;
  text_length: number;
  markdown_length: number;
  element_count: number;
  bbox_count: number;
  reliable_bbox_count: number;
  unreliable_bbox_count: number;
  bbox_ratio: number;
  reliable_bbox_ratio: number;
  page_count: number;
  avg_confidence?: number;
  ocr_engine?: string;
  ocr_strategy?: string;
}

export interface ContentParsePlaygroundFile {
  name: string;
  size: number;
  sha256: string;
}

export interface ContentParsePlaygroundParseResponse {
  file: ContentParsePlaygroundFile;
  route_plan?: RoutePlan;
  artifact?: ParseArtifact;
  chunk_source?: ChunkSourceDocument;
  chunk_plan?: ChunkPlan;
  quality_summary: ContentParseQualitySummary;
}

export interface ContentParsePlaygroundSavedRun {
  id: string;
  workspace_id?: string;
  account_id?: string;
  file_name: string;
  file_size: number;
  source_content_hash: string;
  source_storage_type?: string;
  source_mime_type?: string;
  source_file_ext?: string;
  requested_provider_key: string;
  final_provider_key?: string;
  adapter_name?: string;
  engine_name?: string;
  profile: ParseProfile | string;
  ocr_engine?: string;
  status: string;
  quality_level: string;
  fallback_used: boolean;
  duration_ms?: number;
  artifact_json?: ParseArtifact;
  route_plan_json?: RoutePlan;
  chunk_source_json?: ChunkSourceDocument;
  chunk_plan_json?: ChunkPlan;
  quality_summary_json?: ContentParseQualitySummary;
  summary_json?: Record<string, unknown>;
  share_token: string;
  is_share_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ContentParsePlaygroundSaveResponse {
  run: ContentParsePlaygroundSavedRun;
  parse_result: ContentParsePlaygroundParseResponse;
}

export interface ContentParsePlaygroundRunsResponse {
  items: ContentParsePlaygroundSavedRun[];
}

export interface ContentParsePlaygroundCompareResponse {
  source_content_hash: string;
  items: ContentParsePlaygroundSavedRun[];
}

export interface ContentParsePlaygroundProviderSummary {
  provider_key: string;
  adapter_name?: string;
  engine_name?: string;
  run_count: number;
  success_count: number;
  degraded_count: number;
  failed_count: number;
  fallback_count: number;
  avg_duration_ms: number;
  avg_text_length: number;
  avg_element_count: number;
  estimated_cost: number;
  cost_currency: string;
  last_run_at?: string;
}

export interface ContentParsePlaygroundProviderSummaryResponse {
  items: ContentParsePlaygroundProviderSummary[];
}

export interface ContentParsePlaygroundPDFRenderResponse {
  engine: string;
  page_count: number;
  pages: string[];
}

export type ContentParseProviderStatusValue =
  | 'available'
  | 'fallback'
  | 'not_configured'
  | 'unavailable'
  | 'unknown';

export interface ContentParsePlaygroundProviderStatus {
  key: ParseProviderKey;
  display_name: string;
  type: string;
  adapter_name?: string;
  engine_name?: string;
  enabled: boolean;
  configured: boolean;
  available: boolean;
  selectable: boolean;
  fallback_only: boolean;
  priority?: number;
  route_rank?: number;
  status: ContentParseProviderStatusValue;
  reason?: string;
}

export type ContentParseFileRouteProviderStatus = ContentParsePlaygroundProviderStatus;

export interface ContentParsePlaygroundOCREngineStatus {
  key: ParseOCREngine;
  provider?: string;
  available: boolean;
  default?: boolean;
  path?: string;
  reason?: string;
}

export interface ContentParsePlaygroundProvidersResponse {
  source: string;
  providers: ContentParsePlaygroundProviderStatus[];
  ocr_engines?: ContentParsePlaygroundOCREngineStatus[];
}

export interface ContentParseFileRouteProvidersResponse {
  source: string;
  file_ext: string;
  providers: ContentParseFileRouteProviderStatus[];
}

export interface ContentParsePlaygroundParseRequest {
  file: File;
  provider?: ParseProviderKey;
  profile?: ParseProfile;
  intent?: ParseIntent;
  fresh?: boolean;
  ocrEngine?: ParseOCREngine;
  parseResult?: ContentParsePlaygroundParseResponse;
}

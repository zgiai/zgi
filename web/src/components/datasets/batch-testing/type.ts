import type { SearchMethod } from '@/services/types/dataset';

export interface Response {
  code: string;
  data: BatchTestData;
  message: string;
}

export interface BatchTestData {
  completed: number;
  created_at: number;
  failed: number;
  progress: number;
  results: ResultElement[];
  started_at: number;
  status: string;
  task_id: string;
  total: number;
}

export interface ResultElement {
  finished_at: number;
  query: string;
  result: ResultResult;
  started_at: number;
  status: string;
}

export interface ResultResult {
  elapsed_time: number;
  query: Query;
  records: Record[];
}

export interface Query {
  content: string;
}

export interface ChildChunk {
  content: string;
  created_at: 0;
  id: string;
  score: number;
  segment_id: string;
  type: string;
  position: string;
}
export interface Record {
  match_type: string;
  score: number;
  segment: Segment;
  child_chunks: ChildChunk[];
  tsne_position: TsnePosition;
}

export interface Segment {
  answer: string;
  completed_at: number;
  content: string;
  created_at: number;
  created_by: string;
  dataset_process_rule: DatasetProcessRule;
  disabled_at: null;
  disabled_by: null;
  document: Document;
  document_id: string;
  enabled: boolean;
  error: null;
  hit_count: number;
  id: string;
  index_node_hash: string;
  index_node_id: string;
  indexing_at: number;
  keywords: null;
  position: number;
  sign_content: string;
  status: string;
  stopped_at: null;
  tokens: number;
  word_count: number;
}

export interface DatasetProcessRule {
  createdAt: number;
  createdBy: string;
  id: string;
  mode: string;
  rules: DatasetProcessRuleRules;
}

export interface DatasetProcessRuleRules {
  mode: string;
  rules: RulesRules;
}

export interface RulesRules {
  pre_processing_rules: string[];
  segmentation: Segmentation;
}

export interface Segmentation {
  chunk_overlap: number;
  max_tokens: number;
  separator: string;
}

export interface Document {
  data_source_type: string;
  doc_metadata: null;
  doc_type: string;
  id: string;
  name: string;
}

export interface TsnePosition {
  x: number;
  y: number;
}

// API request/response types for batch hit testing (moved from services/types/dataset.ts)
export interface BatchHitTestingRequest {
  queries: string[];
  retrieval_model: {
    search_method: SearchMethod;
    reranking_enable: boolean;
    reranking_model?: {
      reranking_provider_name: string;
      reranking_model_name: string;
    };
    weights?: {
      keyword_setting?: { keyword_weight: number };
      vector_setting?: {
        vector_weight: number;
        embedding_model_name: string;
        embedding_provider_name: string;
      };
      vector_search_weight?: number;
      text_search_weight?: number;
    } | null;
    top_k: number;
    score_threshold_enabled: boolean;
    score_threshold: number;
    return_full_doc?: boolean | null;
    pre_qa_extension?: boolean | null;
  };
}

export interface BatchHitTestingResponse {
  task_id: string;
  status: string;
  message?: string;
}

export interface BatchHitTestingStatusResponse {
  results: Array<{
    query: string;
    status: 'pending' | 'processing' | 'completed' | 'failed' | 'error';
    started_at?: number;
    finished_at?: number;
    result?: ResultResult;
  }>;
}

export interface BatchHitTestingReportResponse {
  average_response_time: number;
  question_match_rate: number;
  retrieval_success_rate: number;
  task_id: string;
  total_queries: number;
}

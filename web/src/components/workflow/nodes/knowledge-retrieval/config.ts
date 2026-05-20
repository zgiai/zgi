import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export interface KnowledgeRetrievalNodeData {
  type: 'knowledge-retrieval';
  title: string;
  desc: string;
  query_variable_selector: string[];
  dataset_ids: string[];
  retrieval_mode: 'single' | 'multiple';
  multiple_retrieval_config: {
    top_k: number;
    reranking_enable: boolean;
    score_threshold?: number;
    reranking_mode?: 'weighted_score' | 'reranking_model';
    reranking_model?: { provider: string; model: string };
    weights?: {
      vector_setting: {
        vector_weight: number;
        embedding_provider_name: string;
        embedding_model_name: string;
      };
      keyword_setting: { keyword_weight: number };
    };
  };
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_KNOWLEDGE_RETRIEVAL_NODE: KnowledgeRetrievalNodeData = {
  type: 'knowledge-retrieval',
  title: 'knowledge-retrieval',
  desc: '',
  query_variable_selector: [],
  dataset_ids: [],
  retrieval_mode: 'multiple',
  multiple_retrieval_config: {
    top_k: 4,
    reranking_enable: false,
  },
  isInLoop: false,
  isInIteration: false,
};

export const checkValid = (
  nodeData: KnowledgeRetrievalNodeData,
  ctx?: ValidationCtx
): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!Array.isArray(nodeData.dataset_ids) || nodeData.dataset_ids.length === 0) {
    errors.push({ code: 'knowledgeRetrieval.validation.mustSelectDataset' });
  }

  if (
    !Array.isArray(nodeData.query_variable_selector) ||
    nodeData.query_variable_selector.length < 2 ||
    typeof nodeData.query_variable_selector[0] !== 'string' ||
    typeof nodeData.query_variable_selector[1] !== 'string'
  ) {
    errors.push({ code: 'knowledgeRetrieval.validation.mustBindQueryVar' });
  }
  if (
    Array.isArray(nodeData.query_variable_selector) &&
    nodeData.query_variable_selector.length >= 2
  ) {
    const [sourceId] = nodeData.query_variable_selector;
    const allowed = new Set<string>(['sys', 'conversation', 'environment']);
    const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
    if (!allowed.has(sourceId) && !hasNode) {
      warnings.push({ code: 'validation.invalidUpstream' });
    }
  }

  if (
    nodeData.retrieval_mode === 'multiple' &&
    nodeData.multiple_retrieval_config?.reranking_enable &&
    nodeData.multiple_retrieval_config?.reranking_mode === 'reranking_model'
  ) {
    const provider = nodeData.multiple_retrieval_config?.reranking_model?.provider;
    const model = nodeData.multiple_retrieval_config?.reranking_model?.model;
    if (!provider || !model) {
      errors.push({ code: 'knowledgeRetrieval.validation.mustSelectRerankModel' });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};

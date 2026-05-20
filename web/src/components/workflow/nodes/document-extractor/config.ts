import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

export interface DocumentExtractorNodeData {
  type: 'document-extractor';
  title: string;
  desc: string;
  /** Variable selector path: [sourceId, key] or [sourceId, key, subField, ...] */
  variable_selector: string[];
  is_array_file: boolean;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_DOCUMENT_EXTRACTOR_NODE_DATA: DocumentExtractorNodeData = {
  type: 'document-extractor',
  title: 'Document Extractor',
  desc: '',
  variable_selector: [],
  is_array_file: false,
  isInLoop: false,
  isInIteration: false,
};

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const checkValid = (
  nodeData: DocumentExtractorNodeData,
  ctx?: ValidationCtx
): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (
    !Array.isArray(nodeData.variable_selector) ||
    nodeData.variable_selector.length < 2 ||
    typeof nodeData.variable_selector[0] !== 'string' ||
    typeof nodeData.variable_selector[1] !== 'string' ||
    !nodeData.variable_selector[0] ||
    !nodeData.variable_selector[1]
  ) {
    errors.push({ code: 'documentExtractor.validation.mustSelectFileVar' });
  }
  if (Array.isArray(nodeData.variable_selector) && nodeData.variable_selector.length >= 2) {
    const [sourceId] = nodeData.variable_selector;
    const allowed = new Set<string>(['sys', 'conversation', 'environment']);
    const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
    if (!allowed.has(sourceId) && !hasNode) {
      warnings.push({ code: 'validation.invalidUpstream' });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};

// Shared validation types for workflow nodes
import type { WorkflowNodeData } from '../../store/type';

import type { NodesSuffix } from '@/i18n';

export interface ValidationError {
  code: NodesSuffix;
  params?: Record<string, string | number>;
}

export interface ValidationResult {
  isValid: boolean;
  errors: ValidationError[];
  warnings: ValidationError[];
}

export type NodeValidator<T extends WorkflowNodeData> = (data: T) => ValidationResult;

// Optional: specific validators can be re-exported from each node's config

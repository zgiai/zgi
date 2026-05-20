import type { WorkflowVariable } from '../../store/type';
export interface AnswerNodeData {
  type: 'answer';
  title: string;
  desc: string;
  variables: WorkflowVariable[];
  answer: string;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_ANSWER_NODE_DATA: AnswerNodeData = {
  type: 'answer',
  title: 'Answer',
  desc: '',
  variables: [],
  answer: '',
  isInLoop: false,
  isInIteration: false,
};

import type { ValidationResult, ValidationError } from '../common/validation';

export const checkValid = (nodeData: AnswerNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const hasAnswer = typeof nodeData.answer === 'string' && nodeData.answer.trim() !== '';
  const variables = Array.isArray(nodeData.variables) ? nodeData.variables : [];
  const hasVars = variables.length > 0;

  if (!hasAnswer && !hasVars) {
    errors.push({ code: 'answer.validation.contentOrVarRequired' });
  }

  // If variables are set, ensure names/selectors are valid if structure expects it
  variables.forEach((v, idx) => {
    if (!v.name || v.name.trim() === '') {
      errors.push({
        code: 'answer.validation.variableNameRequired',
        params: { index: idx + 1 },
      });
    }
  });

  return { isValid: errors.length === 0, errors, warnings };
};

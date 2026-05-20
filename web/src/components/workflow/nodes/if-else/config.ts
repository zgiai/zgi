import type { ValidationResult, ValidationError } from '../common/validation';
import type { IfElseNodeData, CaseItem, Condition } from './types';
import { operatorNeedsValue } from './utils';

export const DEFAULT_IF_ELSE_NODE_DATA: IfElseNodeData = {
  type: 'if-else',
  title: 'If-Else',
  desc: '',
  cases: [],
  targetBranches: [],
  isInLoop: false,
  isInIteration: false,
};

export const checkValid = (nodeData: IfElseNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const cases: CaseItem[] = Array.isArray(nodeData.cases) ? nodeData.cases : [];
  if (cases.length === 0) {
    errors.push({ code: 'ifElse.validation.ifRequired' });
    return { isValid: false, errors, warnings };
  }

  for (let caseIdx = 0; caseIdx < cases.length; caseIdx++) {
    const ci = cases[caseIdx];
    const conditions: Condition[] = Array.isArray(ci.conditions) ? ci.conditions : [];

    if (conditions.length === 0) {
      errors.push({ code: 'ifElse.validation.caseNeedsCondition' });
      continue;
    }

    for (let condIdx = 0; condIdx < conditions.length; condIdx++) {
      const cond = conditions[condIdx];
      const hasVar = Array.isArray(cond.variable_selector) && cond.variable_selector.length >= 2;
      const hasOp = Boolean(cond.comparison_operator);
      const hasSub = Boolean(cond.sub_variable_condition);

      // If condition is completely empty (no variable and no sub-condition), single error
      if (!hasVar && !hasSub && !hasOp) {
        errors.push({ code: 'ifElse.validation.conditionIncomplete' });
        continue;
      }

      // Check specific missing fields
      if (!hasVar && !hasSub) {
        errors.push({ code: 'ifElse.validation.variableRequired' });
      }

      if (!hasOp) {
        errors.push({ code: 'ifElse.validation.operatorRequired' });
      }

      // Value check - only if operator needs a value
      if (hasOp && operatorNeedsValue(cond.comparison_operator)) {
        const varType = cond.varType;
        const allowUndef = varType === 'boolean' || varType === 'array[boolean]';
        if (cond.value === undefined && !allowUndef) {
          errors.push({ code: 'ifElse.validation.valueRequired' });
        }
      }

      // Validate sub-condition recursively
      if (cond.sub_variable_condition) {
        const sub = cond.sub_variable_condition;
        const subConds = Array.isArray(sub.conditions) ? sub.conditions : [];
        if (subConds.length === 0) {
          warnings.push({ code: 'ifElse.validation.subConditionEmpty' });
        }
      }
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};

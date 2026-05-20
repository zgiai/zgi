import type { BranchRef, CaseItem, IfElseNodeData } from './types';
import { DEFAULT_BRANCHES } from './types';
import { generateClientId } from '@/utils/client-id';

// Re-export common utils
export {
  getOperators,
  operatorNeedsValue,
  isTypeCompatible,
  newCondition,
  operatorI18nKey,
} from '../../common/condition-builder/utils';

export function branchNameCorrect(cases: CaseItem[], _current: BranchRef[]): BranchRef[] {
  // Always include ELSE at end; re-label IF/CASE n
  const heads = cases.map((c, idx) => ({
    id: c.case_id,
    name: idx === 0 ? 'IF' : `CASE ${idx + 1}`,
  }));
  const withElse = [...heads, { id: 'false', name: 'ELSE' }];
  return withElse;
}

export function newCase(): CaseItem {
  const id = generateClientId('case');
  return { case_id: id, logical_operator: 'and', conditions: [], id };
}

export function normalizeBranches(data: IfElseNodeData): BranchRef[] {
  if (!Array.isArray(data.targetBranches) || data.targetBranches.length < 2) {
    return DEFAULT_BRANCHES;
  }
  return data.targetBranches;
}

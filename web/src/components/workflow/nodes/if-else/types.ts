import { ComparisonOperator, type Condition, type VarType } from '../../common/condition-builder/types';

export { ComparisonOperator, type VarType, type Condition };

export interface CaseItem {
  id?: string; // UI only id
  case_id: string; // Handle id to route edges
  logical_operator: 'and' | 'or';
  conditions: Condition[];
}

export interface BranchRef {
  id: string; // same as case_id for branches, 'false' reserved for ELSE
  name: string; // display name: IF / CASE n / ELSE
}

export interface IfElseNodeData {
  type: 'if-else';
  title: string;
  desc?: string;
  // Primary: case list, first is IF, intermediates are ELIF/CASE, ELSE is implicit via targetBranches 'false'
  cases: CaseItem[];
  // Backward compatibility (root level); renderer uses cases primarily
  logical_operator?: 'and' | 'or';
  conditions?: Condition[];
  // Context hints
  isInIteration?: boolean;
  isInLoop?: boolean;
  // Dynamic branch outputs for wiring (includes at least 'true' and 'false')
  targetBranches: BranchRef[];
}

export const DEFAULT_BRANCHES: BranchRef[] = [
  { id: 'true', name: 'IF' },
  { id: 'false', name: 'ELSE' },
];

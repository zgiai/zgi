import type { ValidationResult, ValidationError } from '../common/validation';
import { isValidIdentifier } from '@/utils/validation';
import type { WorkflowVariable } from '../../store/type';

// Primitive type alias aligned with workflow variable types
export type PrimitiveType = WorkflowVariable['type'];

export type VarSelector = string[];

export interface VariableAggregatorGroup {
  groupId: string;
  group_name: string; // downstream variable name (validated by isValidIdentifier)
  output_type?: PrimitiveType; // decided by the first selected variable of this group
  variables: VarSelector[]; // [sourceNodeId, variableKey]
}

export interface VariableAggregatorAdvancedSettings {
  group_enabled: boolean;
  groups: VariableAggregatorGroup[];
}

export interface VariableAggregatorNodeData {
  type: 'variable-aggregator';
  title: string;
  desc: string;
  // Normal mode fields
  output_type?: PrimitiveType; // decided when the first variable is selected
  variables: VarSelector[];
  // Group mode fields
  advanced_settings?: VariableAggregatorAdvancedSettings;
  // UI flags
  selected?: boolean;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_VARIABLE_AGGREGATOR_NODE_DATA: VariableAggregatorNodeData = {
  type: 'variable-aggregator',
  title: 'Variable Aggregator',
  desc: '',
  variables: [],
  output_type: undefined,
  advanced_settings: { group_enabled: false, groups: [] },
  isInLoop: false,
  isInIteration: false,
};

/**
 * Validate Variable Aggregator configuration
 * - Normal mode: requires at least one variable; first variable cannot be a system variable (sourceId !== 'sys')
 * - Group mode: requires at least one group; each group requires a valid name and at least one variable; first
 *   variable per group cannot be a system variable.
 */
export const checkValid = (data: VariableAggregatorNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const groupEnabled = Boolean(data.advanced_settings?.group_enabled);

  if (groupEnabled) {
    const groups = Array.isArray(data.advanced_settings?.groups)
      ? (data.advanced_settings?.groups as VariableAggregatorGroup[])
      : [];
    if (groups.length === 0) {
      errors.push({ code: 'variableAggregator.validation.groupsRequired' });
      return { isValid: false, errors, warnings };
    }

    // group name uniqueness
    const nameSet = new Set<string>();
    for (let i = 0; i < groups.length; i += 1) {
      const g = groups[i];
      const name = (g.group_name || '').trim();
      if (!name) {
        errors.push({
          code: 'variableAggregator.validation.groupNameRequired',
          params: { index: i + 1 },
        });
      } else if (!isValidIdentifier(name)) {
        errors.push({
          code: 'variableAggregator.validation.groupNameInvalid',
          params: { index: i + 1 },
        });
      } else if (nameSet.has(name)) {
        errors.push({
          code: 'variableAggregator.validation.groupNameDuplicate',
          params: { name },
        });
      }
      if (name) nameSet.add(name);

      const vars = Array.isArray(g.variables) ? g.variables : [];
      if (vars.length === 0) {
        errors.push({
          code: 'variableAggregator.validation.groupVariablesRequired',
          params: { index: i + 1 },
        });
      } else {
        // disallow any system variable in group
        const hasSys = vars.some(p => p && p[0] === 'sys');
        if (hasSys) {
          errors.push({ code: 'variableAggregator.validation.systemVarNotAllowed' });
        }
        // duplicate check inside a group
        const pairSet = new Set<string>();
        for (const p of vars) {
          if (!Array.isArray(p)) continue;
          const key = p.join('::');
          if (pairSet.has(key)) {
            errors.push({
              code: 'variableAggregator.validation.duplicateVariableInGroup',
              params: { name },
            });
            break;
          }
          pairSet.add(key);
        }
      }
    }
  } else {
    const vars = Array.isArray(data.variables) ? data.variables : [];
    if (vars.length === 0) {
      errors.push({ code: 'variableAggregator.validation.variablesRequired' });
    } else {
      // disallow any system variable
      const hasSys = vars.some(p => p && p[0] === 'sys');
      if (hasSys) {
        errors.push({ code: 'variableAggregator.validation.systemVarNotAllowed' });
      }
      const pairSet = new Set<string>();
      for (const p of vars) {
        if (!Array.isArray(p)) continue;
        const key = p.join('::');
        if (pairSet.has(key)) {
          errors.push({ code: 'variableAggregator.validation.duplicateVariable' });
          break;
        }
        pairSet.add(key);
      }
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};

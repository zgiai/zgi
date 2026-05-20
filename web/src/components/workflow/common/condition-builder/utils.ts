import { generateClientId } from '@/utils/client-id';
import { ComparisonOperator, type Condition, type VarType } from './types';

// Operators by var type
export function getOperators(varType: VarType, isFileKey?: string | boolean): ComparisonOperator[] {
  // Special case: file 'type' attribute comparison (often has string/enum type)
  if (isFileKey === 'type') {
    return [ComparisonOperator.in, ComparisonOperator.notIn];
  }

  switch (varType) {
    case 'string':
      return [
        ComparisonOperator.is,
        ComparisonOperator.isNot,
        ComparisonOperator.contains,
        ComparisonOperator.notContains,
        ComparisonOperator.startWith,
        ComparisonOperator.endWith,
        ComparisonOperator.empty,
        ComparisonOperator.notEmpty,
      ];
    case 'number':
      return [
        ComparisonOperator.equal,
        ComparisonOperator.notEqual,
        ComparisonOperator.largerThan,
        ComparisonOperator.lessThan,
        ComparisonOperator.largerThanOrEqual,
        ComparisonOperator.lessThanOrEqual,
        ComparisonOperator.isNull,
        ComparisonOperator.isNotNull,
      ];
    case 'boolean':
      return [
        ComparisonOperator.is,
        ComparisonOperator.isNot,
        ComparisonOperator.isNull,
        ComparisonOperator.isNotNull,
      ];
    case 'array':
    case 'array[string]':
    case 'array[number]':
    case 'array[boolean]':
    case 'array[object]':
      return [
        ComparisonOperator.contains,
        ComparisonOperator.notContains,
        ComparisonOperator.empty,
        ComparisonOperator.notEmpty,
        ComparisonOperator.allOf,
      ];
    case 'array[file]':
      // Files list: allow contains/empty and nested sub conditions
      return [
        ComparisonOperator.contains,
        ComparisonOperator.empty,
        ComparisonOperator.notEmpty,
        ComparisonOperator.exists,
        ComparisonOperator.notExists,
      ];
    case 'file':
    case 'object':
      // For file/object attribute checks via sub conditions
      if (isFileKey) {
        return [
          ComparisonOperator.is,
          ComparisonOperator.isNot,
          ComparisonOperator.contains,
          ComparisonOperator.notContains,
          ComparisonOperator.empty,
          ComparisonOperator.notEmpty,
        ];
      }
      return [ComparisonOperator.exists, ComparisonOperator.notExists];
    default:
      return [ComparisonOperator.is, ComparisonOperator.isNot];
  }
}

export function operatorNeedsValue(op: ComparisonOperator): boolean {
  return ![
    ComparisonOperator.empty,
    ComparisonOperator.notEmpty,
    ComparisonOperator.isNull,
    ComparisonOperator.isNotNull,
    ComparisonOperator.exists,
    ComparisonOperator.notExists,
  ].includes(op);
}

/**
 * Check if a target variable type is compatible with a base type for comparison.
 * Used to filter right-side variable selection in if-else conditions.
 */
export function isTypeCompatible(baseType: VarType, targetType: string): boolean {
  // Exact match is always compatible
  if (baseType === targetType) return true;

  // Normalize to handle generic array matching
  const normalize = (t: string): string => {
    if (t.startsWith('array[')) return 'array';
    return t;
  };

  const baseNorm = normalize(baseType);
  const targetNorm = normalize(targetType);

  // Generic array can match specific array types
  if (baseNorm === 'array' && targetNorm === 'array') return true;

  // Number comparison: only numbers with numbers
  if (baseType === 'number' && targetType === 'number') return true;

  // String comparison: only strings with strings
  if (baseType === 'string' && targetType === 'string') return true;

  // Boolean comparison: only booleans with booleans
  if (baseType === 'boolean' && targetType === 'boolean') return true;

  return false;
}

export function newCondition(varType: VarType, isFileKey?: boolean): Condition {
  const ops = getOperators(varType, isFileKey);
  const op = ops[0] ?? ComparisonOperator.is;
  const valueDefault = varType === 'boolean' ? false : '';
  return {
    id: generateClientId('condition'),
    varType,
    comparison_operator: op,
    value: operatorNeedsValue(op) ? (valueDefault as string | boolean) : undefined,
  } as Condition;
}

// Map operator value to i18n key segment
export function operatorI18nKey(op: ComparisonOperator): string {
  switch (op) {
    case ComparisonOperator.equal:
      return 'equal';
    case ComparisonOperator.notEqual:
      return 'notEqual';
    case ComparisonOperator.largerThan:
      return 'largerThan';
    case ComparisonOperator.lessThan:
      return 'lessThan';
    case ComparisonOperator.largerThanOrEqual:
      return 'largerThanOrEqual';
    case ComparisonOperator.lessThanOrEqual:
      return 'lessThanOrEqual';
    case ComparisonOperator.contains:
      return 'contains';
    case ComparisonOperator.notContains:
      return 'notContains';
    case ComparisonOperator.startWith:
      return 'startWith';
    case ComparisonOperator.endWith:
      return 'endWith';
    case ComparisonOperator.is:
      return 'is';
    case ComparisonOperator.isNot:
      return 'isNot';
    case ComparisonOperator.empty:
      return 'empty';
    case ComparisonOperator.notEmpty:
      return 'notEmpty';
    case ComparisonOperator.isNull:
      return 'isNull';
    case ComparisonOperator.isNotNull:
      return 'isNotNull';
    case ComparisonOperator.in:
      return 'in';
    case ComparisonOperator.notIn:
      return 'notIn';
    case ComparisonOperator.allOf:
      return 'allOf';
    case ComparisonOperator.exists:
      return 'exists';
    case ComparisonOperator.notExists:
      return 'notExists';
    default:
      return String(op);
  }
}

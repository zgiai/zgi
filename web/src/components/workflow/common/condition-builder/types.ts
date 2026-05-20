export enum ComparisonOperator {
  contains = 'contains',
  notContains = 'not contains',
  startWith = 'start with',
  endWith = 'end with',
  is = 'is',
  isNot = 'is not',
  empty = 'empty',
  notEmpty = 'not empty',
  equal = '=',
  notEqual = '≠',
  largerThan = '>',
  lessThan = '<',
  largerThanOrEqual = '≥',
  lessThanOrEqual = '≤',
  isNull = 'is null',
  isNotNull = 'is not null',
  in = 'in',
  notIn = 'not in',
  allOf = 'all of',
  exists = 'exists',
  notExists = 'not exists',
}

export type VarType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'object'
  | 'file'
  | 'array'
  | 'array[string]'
  | 'array[number]'
  | 'array[boolean]'
  | 'array[object]'
  | 'array[file]';

export interface SubConditionGroup {
  logical_operator: 'and' | 'or';
  conditions: Condition[];
}

export interface Condition {
  id: string;
  varType: VarType;
  // Selector to upstream variable and sub-key path
  variable_selector?: string[];
  // When comparing file attributes, indicates sub-key like name/size/mime_type/extension/type
  key?: string;
  comparison_operator: ComparisonOperator;
  value?: string | string[] | boolean;
  // For number with subtype (e.g., tool/number)
  numberVarType?: 'variable' | 'number';
  // Nested condition group for sub-variable (e.g., file attribute conditions)
  sub_variable_condition?: SubConditionGroup;
}

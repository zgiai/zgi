import type { Condition } from '../../common/condition-builder/types';

export type LoopVarType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'object'
  | 'array[string]'
  | 'array[number]'
  | 'array[boolean]'
  | 'array[object]';

export interface LoopVariable {
  label: string;
  var_type: LoopVarType;
  value_type: 'constant' | 'variable';
  value: string | number | boolean | object | string[] | number[] | boolean[] | object[];
}

export interface LoopNodeData {
  type: 'loop';
  title: string;
  desc: string;
  start_node_id: string;
  loop_count: number;
  break_conditions: Condition[];
  logical_operator: 'and' | 'or';
  loop_variables: LoopVariable[];
  isInLoop?: boolean;
  isInIteration?: boolean;
  _children: string[];
}

export const DEFAULT_LOOP_NODE: LoopNodeData = {
  type: 'loop',
  title: '循环',
  desc: '',
  start_node_id: '',
  loop_count: 10,
  break_conditions: [],
  logical_operator: 'and',
  loop_variables: [],
  isInLoop: false,
  isInIteration: false,
  _children: [],
};

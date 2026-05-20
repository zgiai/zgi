import React, { useCallback, useMemo } from 'react';

import { useT } from '@/i18n';
import { ensureUniqueIdentifier, sanitizeIdentifier } from '@/utils/validation';

import { useLocalNodeData } from '../../../hooks';
import {
  selectorsEqual,
  VariableBindingEditor,
  type VariableBindingEditorAdapter,
} from '../../../common/variable-binding-editor';
import type { OutputVariable } from '../config';

type EndOutputAdapter = VariableBindingEditorAdapter<OutputVariable>;
type EndNormalizeArgs =
  NonNullable<EndOutputAdapter['normalizeRowOnBlur']> extends (args: infer TArgs) => OutputVariable
    ? TArgs
    : never;
type EndSelectorArgs = Parameters<EndOutputAdapter['applySelectorChange']>[0];

const OUTPUT_TYPES: Array<OutputVariable['type']> = [
  'string',
  'number',
  'boolean',
  'object',
  'file',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
  'array[file]',
];

function isOutputVariableType(
  value: EndSelectorArgs['payload']['type']
): value is OutputVariable['type'] {
  return OUTPUT_TYPES.includes(value as OutputVariable['type']);
}

function normalizeEndOutputRow({ row, rows, index }: EndNormalizeArgs): OutputVariable {
  const normalized = ensureUniqueIdentifier(
    sanitizeIdentifier(row.variable || ''),
    rows.filter((_, rowIndex) => rowIndex !== index).map(item => item.variable || '')
  );

  if (normalized === row.variable) {
    return row;
  }

  return { ...row, variable: normalized };
}

function applyEndSelectorChange({ row, rows, index, payload }: EndSelectorArgs): OutputVariable {
  const nextType: OutputVariable['type'] = isOutputVariableType(payload.type)
    ? payload.type
    : 'string';
  const currentName = (row.variable || '').trim();
  const mappedDefaultName = payload.key === 'body' ? 'result' : payload.key;
  const candidate = sanitizeIdentifier(currentName.length === 0 ? mappedDefaultName : currentName);
  const uniqueCandidate = ensureUniqueIdentifier(
    candidate,
    rows.filter((_, rowIndex) => rowIndex !== index).map(item => item.variable || '')
  );

  return {
    ...row,
    variable: uniqueCandidate,
    type: nextType,
    value_selector: payload.valuePath,
  };
}

const END_OUTPUT_ADAPTER: EndOutputAdapter = {
  createRow: () => ({ variable: '', type: 'string', value_selector: [] }),
  isRowEqual: (left, right) =>
    left.variable === right.variable &&
    left.type === right.type &&
    selectorsEqual(left.value_selector, right.value_selector),
  getName: row => row.variable || '',
  setName: (row, name) => (row.variable === name ? row : { ...row, variable: name }),
  getSelector: row =>
    Array.isArray(row.value_selector) &&
    row.value_selector.length >= 2 &&
    typeof row.value_selector[0] === 'string' &&
    typeof row.value_selector[1] === 'string'
      ? row.value_selector
      : undefined,
  normalizeRowOnBlur: normalizeEndOutputRow,
  applySelectorChange: applyEndSelectorChange,
};

interface OutputManagerProps {
  id: string;
  readOnly?: boolean;
}

/**
 * @component OutputManager
 * @category Feature
 * @status Stable
 * @description End-node output editor built on the shared variable-binding editor shell.
 * @usage Use in the node panel when configuring workflow end outputs.
 * @example
 * <OutputManager id={nodeId} readOnly={false} />
 */
const OutputManager: React.FC<OutputManagerProps> = ({ id, readOnly = false }) => {
  const t = useT('nodes');

  const { localData: outputsRaw, setLocalData: setOutputs } = useLocalNodeData<OutputVariable[]>(
    id,
    {
      path: 'outputs',
      delay: 400,
      debugLabel: `end:${id}:outputs`,
    }
  );

  const outputs = useMemo<OutputVariable[]>(() => outputsRaw || [], [outputsRaw]);
  const handleOutputsChange = useCallback(
    (nextRows: OutputVariable[]) => {
      setOutputs(nextRows);
    },
    [setOutputs]
  );

  return (
    <VariableBindingEditor
      rows={outputs}
      onChange={handleOutputsChange}
      labels={{
        title: t('end.outputs.title'),
        addLabel: t('end.outputs.addVariable'),
        emptyText: t('end.outputs.noOutputs'),
        namePlaceholder: t('end.outputs.types.text-input'),
        selectorPlaceholder: t('end.outputs.types.text-input'),
        removeLabel: () => t('common.remove'),
      }}
      adapter={END_OUTPUT_ADAPTER}
      nodeId={id}
      readOnly={readOnly}
      debugLabel={`end:${id}:outputs`}
    />
  );
};

export default React.memo(OutputManager);

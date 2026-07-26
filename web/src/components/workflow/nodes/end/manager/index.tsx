'use client';

import React, { useCallback, useMemo } from 'react';
import { Trash2 } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tabs } from '@/components/ui/tabs';
import {
  WorkflowCompactTabsList,
  WorkflowCompactTabsTrigger,
} from '@/components/workflow/common/compact-form';
import SortableListSection from '@/components/workflow/common/sortable-list/sortable-list-section';
import { useStableSortableList } from '@/components/workflow/common/sortable-list/use-stable-sortable-list';
import ValueSourceEditor from '@/components/workflow/common/value-source-editor';
import DefaultValueEditor from '@/components/workflow/ui/conversation-variables-panel/default-value-editor';
import type { ConversationVariable } from '@/components/workflow/store/type';
import { useT } from '@/i18n';
import { ensureUniqueIdentifier, sanitizeIdentifier } from '@/utils/validation';

import { useLocalNodeData } from '../../../hooks';
import { WorkflowIdentifierInput } from '../../../common/variable-binding-editor/identifier-input';
import type { OutputVariable } from '../config';
import ComplexOutputValueDialog from './complex-output-value-dialog';

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

const CONSTANT_OUTPUT_TYPES = OUTPUT_TYPES.filter(
  type => type !== 'file' && type !== 'array[file]'
) as Array<ConversationVariable['type']>;

function outputMode(row: OutputVariable): 'constant' | 'variable' {
  return row.value_type === 'constant' ? 'constant' : 'variable';
}

function constantType(type: OutputVariable['type']): ConversationVariable['type'] {
  return CONSTANT_OUTPUT_TYPES.includes(type as ConversationVariable['type'])
    ? (type as ConversationVariable['type'])
    : 'string';
}

function defaultConstantValue(type: ConversationVariable['type']): unknown {
  if (type === 'number') return '';
  if (type === 'boolean') return false;
  if (type === 'object') return {};
  if (type.startsWith('array')) return [];
  return '';
}

function isComplexConstantType(
  type: ConversationVariable['type']
): type is Extract<
  ConversationVariable['type'],
  'object' | 'array[string]' | 'array[number]' | 'array[boolean]' | 'array[object]'
> {
  return type === 'object' || type.startsWith('array');
}

function rowsEqual(left: OutputVariable, right: OutputVariable): boolean {
  return (
    left.variable === right.variable &&
    left.type === right.type &&
    outputMode(left) === outputMode(right) &&
    JSON.stringify(left.value_selector ?? []) === JSON.stringify(right.value_selector ?? []) &&
    JSON.stringify(left.value) === JSON.stringify(right.value)
  );
}

interface OutputManagerProps {
  id: string;
  readOnly?: boolean;
}

interface BooleanConstantEditorProps {
  value: unknown;
  onChange: (value: boolean) => void;
  readOnly: boolean;
}

function BooleanConstantEditor({ value, onChange, readOnly }: BooleanConstantEditorProps) {
  const tCommon = useT('common');
  const selected = value === true || value === 'true' ? 'true' : 'false';

  return (
    <Tabs value={selected} onValueChange={next => onChange(next === 'true')}>
      <WorkflowCompactTabsList className="w-[112px]">
        <WorkflowCompactTabsTrigger className="min-w-0 flex-1" value="true" disabled={readOnly}>
          {tCommon('yes')}
        </WorkflowCompactTabsTrigger>
        <WorkflowCompactTabsTrigger className="min-w-0 flex-1" value="false" disabled={readOnly}>
          {tCommon('no')}
        </WorkflowCompactTabsTrigger>
      </WorkflowCompactTabsList>
    </Tabs>
  );
}

/** End-node output editor supporting upstream variables and fixed values. */
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
  const { rows, items, sensors, handleDragEnd, append, removeAt, updateAt } =
    useStableSortableList<OutputVariable>({
      derive: () => outputs,
      deps: [outputs],
      isRowEqual: rowsEqual,
      debugLabel: `end:${id}:outputs`,
      serialize: nextRows => {
        if (!readOnly) setOutputs(nextRows);
      },
    });

  const handleAdd = useCallback(() => {
    append({
      variable: '',
      type: 'string',
      value_type: 'variable',
      value_selector: [],
    });
  }, [append]);

  const handleNameBlur = useCallback(
    (index: number) => {
      updateAt(index, row => {
        const normalized = ensureUniqueIdentifier(
          sanitizeIdentifier(row.variable || ''),
          rows.filter((_, rowIndex) => rowIndex !== index).map(item => item.variable || '')
        );
        return normalized === row.variable ? row : { ...row, variable: normalized };
      });
    },
    [rows, updateAt]
  );

  return (
    <SortableListSection
      title={t('end.outputs.title')}
      addLabel={t('end.outputs.addVariable')}
      emptyText={t('end.outputs.noOutputs')}
      isReadOnly={readOnly}
      items={items}
      sensors={sensors}
      onDragEnd={handleDragEnd}
      onAdd={handleAdd}
      renderRow={index => {
        const row = rows[index];
        const mode = outputMode(row);
        const fixedType = constantType(row.type);
        const constantEditor =
          fixedType === 'boolean' ? (
            <BooleanConstantEditor
              value={row.value}
              onChange={value => updateAt(index, current => ({ ...current, value }))}
              readOnly={readOnly}
            />
          ) : isComplexConstantType(fixedType) ? (
            <ComplexOutputValueDialog
              variableName={row.variable}
              type={fixedType}
              value={row.value}
              onChange={value => updateAt(index, current => ({ ...current, value }))}
              readOnly={readOnly}
            />
          ) : (
            <DefaultValueEditor
              type={fixedType}
              value={row.value}
              onChange={value => updateAt(index, current => ({ ...current, value }))}
              density="compact"
              readOnly={readOnly}
            />
          );

        return (
          <div className="rounded-lg border bg-card p-2">
            <div className="mb-1.5 flex items-center gap-2">
              <div className="min-w-0 flex-1">
                <WorkflowIdentifierInput
                  initial={row.variable}
                  onCommit={variable => updateAt(index, current => ({ ...current, variable }))}
                  onBlurNormalize={() => handleNameBlur(index)}
                  placeholder={t('end.outputs.variablePlaceholder')}
                  invalid={!row.variable.trim()}
                  disabled={readOnly}
                  className="h-8 px-2.5 text-xs"
                  debugLabel={`end:${id}:output-${index}:name`}
                />
              </div>
              {mode === 'constant' ? (
                <Select
                  value={fixedType}
                  onValueChange={value =>
                    updateAt(index, current => {
                      const type = value as ConversationVariable['type'];
                      return {
                        ...current,
                        type,
                        value: defaultConstantValue(type),
                      };
                    })
                  }
                  disabled={readOnly}
                >
                  <SelectTrigger className="h-8 w-[118px] text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {CONSTANT_OUTPUT_TYPES.map(type => (
                      <SelectItem key={type} value={type}>
                        {t(`types.${type}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : null}
              <Button
                variant="ghost"
                isIcon
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                onClick={() => removeAt(index)}
                disabled={readOnly}
                aria-label={t('common.remove')}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>

            <ValueSourceEditor
              nodeId={id}
              mode={mode}
              onModeChange={nextMode =>
                updateAt(index, current => {
                  if (nextMode === 'constant') {
                    const type = constantType(current.type);
                    return {
                      ...current,
                      type,
                      value_type: 'constant',
                      value_selector: undefined,
                      value: defaultConstantValue(type),
                    };
                  }
                  return {
                    ...current,
                    value_type: 'variable',
                    value_selector: [],
                    value: undefined,
                  };
                })
              }
              constantEditor={constantEditor}
              variableValue={row.value_selector}
              onVariableChange={payload =>
                updateAt(index, current => {
                  const existingName = current.variable.trim();
                  const suggestedName = payload.key === 'body' ? 'result' : payload.key;
                  const variable = existingName
                    ? current.variable
                    : ensureUniqueIdentifier(
                        sanitizeIdentifier(suggestedName),
                        rows
                          .filter((_, rowIndex) => rowIndex !== index)
                          .map(item => item.variable || '')
                      );
                  return {
                    ...current,
                    variable,
                    type: payload.type as OutputVariable['type'],
                    value_selector: payload.valuePath,
                  };
                })
              }
              variablePlaceholder={t('end.outputs.selectorPlaceholder')}
              disabled={readOnly}
              density="compact"
              layout="inline"
              className="space-y-1.5"
              constantLabel={t('end.outputs.constant')}
              variableLabel={t('end.outputs.variable')}
            />
          </div>
        );
      }}
    />
  );
};

export default React.memo(OutputManager);

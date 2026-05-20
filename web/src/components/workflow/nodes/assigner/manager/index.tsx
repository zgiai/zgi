'use client';

import React, { useCallback, useEffect, useMemo } from 'react';
import type { AssignerNodeData, AssignerNodeOperation } from '../../../store/type';
import { Button } from '@/components/ui/button';
import NodeValueSelector from '../../../common/node-value-selector';
import { Select, SelectContent, SelectItem, SelectValue } from '@/components/ui/select';
import SortableListSection from '../../../common/sortable-list/sortable-list-section';
import { useStableSortableList } from '../../../common/sortable-list/use-stable-sortable-list';
import { Info, Trash2 } from 'lucide-react';
import { useWorkflowStore } from '../../../store';
import DefaultValueEditor from '../../../ui/conversation-variables-panel/default-value-editor';
import { useT, type NodesKey } from '@/i18n';
import { useNodeData, useNodeDataUpdate, useResolvedVariableReference } from '../../../hooks';
import { AgentType } from '@/services/types/agent';
import { CompactNote } from '../../../common/compact-note';
import {
  WorkflowCompactFieldLabel,
  WorkflowCompactSelectTrigger,
} from '../../../common/compact-form';
import ValueSourceEditor from '../../../common/value-source-editor';

interface AssignerManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const MATH_OPERATIONS: AssignerNodeData['items'][number]['operation'][] = ['+=', '-=', '*=', '/='];
const BASE_OPERATIONS: AssignerNodeData['items'][number]['operation'][] = [
  'set',
  'over-write',
  'clear',
];
const ALL_OPERATIONS: AssignerNodeData['items'][number]['operation'][] = [
  ...BASE_OPERATIONS,
  ...MATH_OPERATIONS,
];
const isMathOp = (op: AssignerNodeData['items'][number]['operation']) =>
  MATH_OPERATIONS.includes(op);

interface AssignerPreviewTextProps {
  nodeId: string;
  row: AssignerNodeOperation;
  formatLiteralValue: (value: unknown) => string;
}

const AssignerPreviewText: React.FC<AssignerPreviewTextProps> = ({
  nodeId,
  row,
  formatLiteralValue,
}) => {
  const t = useT();
  const target = useResolvedVariableReference({
    selector: row.variable_selector,
    currentNodeId: nodeId,
  });
  const value = useResolvedVariableReference({
    selector: Array.isArray(row.value) ? (row.value as string[]) : undefined,
    currentNodeId: nodeId,
  });

  const previewActionKey = (op: AssignerNodeOperation['operation']) => {
    switch (op) {
      case '+=':
        return 'add';
      case '-=':
        return 'subtract';
      case '*=':
        return 'multiply';
      case '/=':
        return 'divide';
      default:
        return 'set';
    }
  };

  if (!target) {
    return <>{t('nodes.assigner.preview.selectTargetFirst')}</>;
  }

  if (row.operation === 'clear') {
    return <>{t('nodes.assigner.preview.clear', { target: target.displayText })}</>;
  }

  const variableValue = value?.displayText ?? t('nodes.assigner.preview.pendingValue');
  const constantValue = formatLiteralValue(row.value);

  if (row.operation === 'set') {
    return (
      <>
        {t('nodes.assigner.preview.setConstant', {
          target: target.displayText,
          value: constantValue,
        })}
      </>
    );
  }

  if (row.operation === 'over-write') {
    return (
      <>
        {t('nodes.assigner.preview.setVariable', {
          target: target.displayText,
          value: variableValue,
        })}
      </>
    );
  }

  return (
    <>
      {t('nodes.assigner.preview.math', {
        target: target.displayText,
        action: t(`nodes.assigner.preview.actions.${previewActionKey(row.operation)}` as NodesKey),
        value: row.input_type === 'variable' ? variableValue : constantValue,
      })}
    </>
  );
};

const AssignerManager: React.FC<AssignerManagerProps> = ({ id, readOnly = false }) => {
  type Row = AssignerNodeData['items'][number];
  const t = useT();
  const agentType = useWorkflowStore.use.agentType();

  const nodeData = useNodeData<AssignerNodeData>(id);
  const updateNodeData = useNodeDataUpdate<AssignerNodeData>(id);

  const { rows, items, sensors, handleDragEnd, append, removeAt, updateAt } =
    useStableSortableList<Row>({
      derive: () => (nodeData?.items && Array.isArray(nodeData.items) ? nodeData.items : []),
      deps: [nodeData?.items],
      isRowEqual: (a: Row, b: Row) =>
        a.operation === b.operation &&
        a.input_type === b.input_type &&
        JSON.stringify(a.variable_selector) === JSON.stringify(b.variable_selector) &&
        JSON.stringify(a.value) === JSON.stringify(b.value),
      serialize: (next: Row[]) => updateNodeData({ items: next }),
    });

  const handleAdd = useCallback(() => {
    append({
      variable_selector: ['', ''],
      input_type: 'constant',
      operation: 'set',
      value: '',
    });
  }, [append]);

  const handleChangeTarget = useCallback(
    (
      index: number,
      payload: { sourceId: string; key: string; valuePath: string[]; type?: string }
    ) => {
      updateAt(index, cur => {
        const nextTargetType = payload.type;
        if (isMathOp(cur.operation) && nextTargetType !== 'number') {
          return {
            ...cur,
            variable_selector: payload.valuePath,
            operation: 'set',
            input_type: 'constant',
            value: '',
          };
        }
        return { ...cur, variable_selector: payload.valuePath };
      });
    },
    [updateAt]
  );

  const handleChangeOp = useCallback(
    (index: number, val: Row['operation']) =>
      updateAt(index, cur => {
        if (val === 'over-write') {
          return { ...cur, operation: val, input_type: 'variable', value: [] } as Row;
        }
        if (val === 'clear') {
          return { ...cur, operation: val, value: '' } as Row;
        }
        if (val === 'set') {
          return { ...cur, operation: val, input_type: 'constant' } as Row;
        }
        return { ...cur, operation: val } as Row;
      }),
    [updateAt]
  );

  const handleChangeInputType = useCallback(
    (index: number, val: Row['input_type']) =>
      updateAt(index, cur => ({ ...cur, input_type: val, value: val === 'variable' ? [] : '' })),
    [updateAt]
  );

  const getUpstreamVariables = useWorkflowStore.use.getUpstreamVariables();
  const upstreamGroups = useMemo(() => getUpstreamVariables(id) || [], [getUpstreamVariables, id]);
  const upstreamTypeMap = useMemo(() => {
    const map = new Map<string, string>();
    upstreamGroups.forEach(group =>
      (group.variables || []).forEach(variable =>
        map.set(`${group.nodeId}::${variable.key}`, variable.type)
      )
    );
    return map;
  }, [upstreamGroups]);

  const getTargetType = useCallback(
    (selector: string[] | undefined): string | undefined => {
      if (!Array.isArray(selector) || selector.length < 2) return undefined;
      return upstreamTypeMap.get(`${selector[0]}::${selector[1]}`);
    },
    [upstreamTypeMap]
  );

  const sanitizeRow = useCallback(
    (row: Row): Row => {
      const targetType = getTargetType(row.variable_selector);

      if (isMathOp(row.operation)) {
        if (targetType !== 'number') {
          return {
            ...row,
            operation: 'set',
            input_type: 'constant',
            value: '',
          };
        }

        if (row.input_type === 'variable') {
          return {
            ...row,
            value: Array.isArray(row.value) ? row.value : [],
          };
        }

        return {
          ...row,
          input_type: 'constant',
          value: Array.isArray(row.value) ? '' : row.value,
        };
      }

      if (row.operation === 'over-write') {
        return {
          ...row,
          input_type: 'variable',
          value: Array.isArray(row.value) ? row.value : [],
        };
      }

      if (row.operation === 'set') {
        return {
          ...row,
          input_type: 'constant',
          value: Array.isArray(row.value) ? '' : row.value,
        };
      }

      if (row.operation === 'clear') {
        return {
          ...row,
          input_type: 'constant',
          value: '',
        };
      }

      return row;
    },
    [getTargetType]
  );

  useEffect(() => {
    const sanitized = rows.map(sanitizeRow);
    const changed = sanitized.some((row, index) => {
      const current = rows[index];
      return (
        row.operation !== current.operation ||
        row.input_type !== current.input_type ||
        JSON.stringify(row.value) !== JSON.stringify(current.value)
      );
    });
    if (changed) {
      updateNodeData({ items: sanitized });
    }
  }, [rows, sanitizeRow, updateNodeData]);

  const handleChangeValueVar = useCallback(
    (
      index: number,
      payload: { sourceId: string; key: string; valuePath: string[]; type?: string }
    ) =>
      updateAt(index, cur => {
        if (cur.operation === 'over-write') {
          const targetType = getTargetType(cur.variable_selector as [string, string] | undefined);
          if (targetType && payload.type && payload.type !== targetType) {
            return cur;
          }
        }
        return { ...cur, value: payload.valuePath } as Row;
      }),
    [updateAt, getTargetType]
  );

  const isTargetSelected = (selector?: string[]) =>
    Array.isArray(selector) && selector.length >= 2 && !!selector[1];

  const opLabelKey = (op: Row['operation']): string => {
    switch (op) {
      case 'set':
        return 'set';
      case 'over-write':
        return 'over-write';
      case 'clear':
        return 'clear';
      case '+=':
        return 'add';
      case '-=':
        return 'subtract';
      case '*=':
        return 'multiply';
      case '/=':
        return 'divide';
      default:
        return String(op);
    }
  };

  const toCvType = (
    valueType?: string
  ):
    | 'string'
    | 'number'
    | 'boolean'
    | 'object'
    | 'array[string]'
    | 'array[number]'
    | 'array[boolean]'
    | 'array[object]' => {
    switch (valueType) {
      case 'string':
      case 'number':
      case 'boolean':
      case 'object':
        return valueType;
      case 'array[string]':
      case 'array[number]':
      case 'array[boolean]':
      case 'array[object]':
        return valueType;
      case 'array':
        return 'array[object]';
      default:
        return 'string';
    }
  };

  const formatLiteralValue = useCallback(
    (value: unknown) => {
      if (value === undefined || value === null || value === '') {
        return t('nodes.assigner.preview.pendingValue');
      }
      if (typeof value === 'string') return `"${value}"`;
      if (typeof value === 'number' || typeof value === 'boolean') return String(value);
      try {
        const text = JSON.stringify(value);
        return text.length > 60 ? `${text.slice(0, 57)}...` : text;
      } catch {
        return String(value);
      }
    },
    [t]
  );

  const getOperationValueLabel = useCallback(
    (row: Row) => {
      if (row.operation === 'set') return t('nodes.assigner.fields.fixedValue');
      if (row.operation === 'over-write') return t('nodes.assigner.fields.sourceVariable');
      if (isMathOp(row.operation) && row.input_type === 'variable') {
        return t('nodes.assigner.fields.deltaSource');
      }
      if (isMathOp(row.operation)) return t('nodes.assigner.fields.deltaValue');
      return t('nodes.assigner.fields.value');
    },
    [t]
  );

  const getValueHint = useCallback(
    (row: Row, targetType?: string) => {
      if (!isTargetSelected(row.variable_selector)) {
        return t('nodes.assigner.hints.selectTargetFirst');
      }
      if (row.operation === 'over-write') return t('nodes.assigner.hints.sameTypeOnly');
      if (isMathOp(row.operation) && row.input_type === 'variable') {
        return t('nodes.assigner.hints.numericSourceOnly');
      }
      if (isMathOp(row.operation)) return t('nodes.assigner.hints.inputNumber');
      switch (toCvType(targetType)) {
        case 'number':
          return t('nodes.assigner.hints.inputNumber');
        case 'boolean':
          return t('nodes.assigner.hints.toggleBoolean');
        case 'object':
        case 'array[string]':
        case 'array[number]':
        case 'array[boolean]':
        case 'array[object]':
          return t('nodes.assigner.hints.inputJson');
        default:
          return t('nodes.assigner.hints.inputText');
      }
    },
    [t]
  );

  const writableCategories = useMemo(() => {
    const categories = [
      t('nodes.assigner.info.loopVariables'),
      t('nodes.assigner.info.iterationVariables'),
    ];
    if (agentType === AgentType.CONVERSATIONAL_AGENT) {
      return [t('nodes.assigner.info.conversationVariables'), ...categories];
    }
    return categories;
  }, [agentType, t]);

  const writableCategoriesText = writableCategories.join(' / ');

  return (
    <div className="space-y-4">
      <CompactNote
        icon={<Info className="h-3.5 w-3.5 text-primary" />}
        title={t('nodes.assigner.info.title')}
      >
        <p>
          {t('nodes.assigner.info.description')} {writableCategoriesText}
        </p>
        <p>{t('nodes.assigner.info.notWritable')}</p>
      </CompactNote>

      <SortableListSection
        title={t('nodes.assigner.listTitle')}
        addLabel={t('nodes.assigner.actions.addOperation')}
        emptyText={t('nodes.assigner.empty')}
        isReadOnly={readOnly}
        items={items}
        sensors={sensors}
        onDragEnd={handleDragEnd}
        onAdd={handleAdd}
        renderRow={(index: number) => {
          const row = rows[index];
          const targetType = getTargetType(row.variable_selector);
          const hasTarget = isTargetSelected(row.variable_selector);
          const availableOperations = targetType === 'number' ? ALL_OPERATIONS : BASE_OPERATIONS;

          return (
            <div className="relative rounded-lg border bg-card px-3 py-2.5">
              <div className="space-y-2.5">
                <div className="pr-8">
                  <div className="text-[11px] font-medium leading-4 text-muted-foreground">
                    {t('nodes.assigner.itemTitle', { index: index + 1 })}
                  </div>
                  <p className="mt-1 text-xs leading-5 text-foreground/80">
                    <AssignerPreviewText
                      nodeId={id}
                      row={row}
                      formatLiteralValue={formatLiteralValue}
                    />
                  </p>
                </div>

                <div className="flex items-start gap-3">
                  <div className="min-w-0 flex-1 space-y-1">
                    <NodeValueSelector
                      nodeId={id}
                      value={row.variable_selector}
                      onChange={p => handleChangeTarget(index, p)}
                      writableOnly
                      label={t('nodes.assigner.fields.targetVariable')}
                      density="compact"
                      className="space-y-1"
                      placeholder={t('nodes.assigner.placeholders.selectTargetVariable')}
                      disabled={readOnly}
                    />
                  </div>
                  <div className="w-40 shrink-0 space-y-1">
                    <WorkflowCompactFieldLabel>
                      {t('nodes.assigner.fields.operation')}
                    </WorkflowCompactFieldLabel>
                    <Select
                      value={row.operation}
                      onValueChange={v => handleChangeOp(index, v as Row['operation'])}
                      disabled={readOnly}
                    >
                      <WorkflowCompactSelectTrigger>
                        <SelectValue
                          placeholder={t('nodes.assigner.placeholders.selectOperation')}
                        />
                      </WorkflowCompactSelectTrigger>
                      <SelectContent>
                        {availableOperations.map(op => (
                          <SelectItem key={op} value={op}>
                            {t(`nodes.assigner.operations.${opLabelKey(op)}` as NodesKey)}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {row.operation === 'clear' ? null : (
                  <div className="space-y-2">
                    {!hasTarget ? (
                      <div className="rounded-md border border-dashed bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
                        {t('nodes.assigner.hints.selectTargetFirst')}
                      </div>
                    ) : (
                      <div className="space-y-2">
                        <ValueSourceEditor
                          nodeId={id}
                          mode={row.operation === 'over-write' ? 'variable' : row.input_type}
                          onModeChange={
                            isMathOp(row.operation)
                              ? mode => handleChangeInputType(index, mode as Row['input_type'])
                              : undefined
                          }
                          showModeSwitcher={isMathOp(row.operation)}
                          label={getOperationValueLabel(row)}
                          hint={getValueHint(row, targetType)}
                          variableValue={
                            Array.isArray(row.value) ? (row.value as string[]) : undefined
                          }
                          onVariableChange={p => handleChangeValueVar(index, p)}
                          variableTypeFilter={
                            row.operation === 'over-write'
                              ? type => type === getTargetType(row.variable_selector)
                              : isMathOp(row.operation)
                                ? type => type === 'number'
                                : undefined
                          }
                          variablePlaceholder={
                            row.operation === 'over-write'
                              ? targetType
                                ? t('nodes.assigner.placeholders.selectSameTypeVar')
                                : t('nodes.assigner.placeholders.selectTargetFirst')
                              : isMathOp(row.operation)
                                ? t('nodes.assigner.placeholders.selectNumericSourceVariable')
                                : undefined
                          }
                          disabled={
                            row.operation === 'over-write' ? !targetType || readOnly : readOnly
                          }
                          density="compact"
                          constantEditor={
                            <div className={readOnly ? 'pointer-events-none opacity-60' : ''}>
                              <DefaultValueEditor
                                type={isMathOp(row.operation) ? 'number' : toCvType(targetType)}
                                value={row.value}
                                density="compact"
                                readOnly={readOnly}
                                onChange={value => updateAt(index, cur => ({ ...cur, value }))}
                              />
                            </div>
                          }
                          constantLabel={t('nodes.assigner.inputTypes.constantShort')}
                          variableLabel={t('nodes.assigner.inputTypes.variableShort')}
                        />
                      </div>
                    )}
                  </div>
                )}
              </div>

              <Button
                variant="ghost"
                size="xs"
                isIcon
                className="absolute right-2 top-2 h-7 w-7 rounded-md"
                onClick={() => removeAt(index)}
                aria-label={t('nodes.assigner.aria.delete')}
                disabled={readOnly}
              >
                <Trash2 size={15} />
              </Button>
            </div>
          );
        }}
      />
    </div>
  );
};

export default AssignerManager;

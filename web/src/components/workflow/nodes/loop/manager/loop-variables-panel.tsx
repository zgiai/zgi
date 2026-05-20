'use client';

import React, { useCallback } from 'react';
import { useT } from '@/i18n';

import { Button } from '@/components/ui/button';
import { Trash2 } from 'lucide-react';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { LoopVariable, LoopVarType } from '../config';
import { useStableSortableList } from '@/components/workflow/common/sortable-list/use-stable-sortable-list';
import { type ConversationVariable } from '@/components/workflow/store';
import SortableListSection from '@/components/workflow/common/sortable-list/sortable-list-section';
import DefaultValueEditor from '@/components/workflow/ui/conversation-variables-panel/default-value-editor';
import ValueSourceEditor from '@/components/workflow/common/value-source-editor';

interface LoopVariablesPanelProps {
  nodeId: string;
  variables: LoopVariable[];
  onChange: (variables: LoopVariable[]) => void;
  readOnly?: boolean;
}

const VAR_TYPES: LoopVarType[] = [
  'string',
  'number',
  'boolean',
  'object',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
];

const LoopVariablesPanel: React.FC<LoopVariablesPanelProps> = ({
  nodeId,
  variables,
  onChange,
  readOnly,
}) => {
  const t = useT();

  const { rows, items, sensors, handleDragEnd, append, removeAt, updateAt } =
    useStableSortableList<LoopVariable>({
      derive: () => variables || [],
      deps: [variables],
      isRowEqual: (a, b) =>
        a.label === b.label &&
        a.var_type === b.var_type &&
        a.value_type === b.value_type &&
        JSON.stringify(a.value) === JSON.stringify(b.value),
      serialize: onChange,
    });

  const handleAdd = useCallback(() => {
    append({
      label: '',
      var_type: 'string',
      value_type: 'constant',
      value: '',
    });
  }, [append]);

  const handleChangeValueVar = useCallback(
    (index: number, payload: { valuePath: string[] }) => {
      updateAt(index, cur => ({ ...cur, value: payload.valuePath }));
    },
    [updateAt]
  );

  const toCvType = (t: LoopVarType): ConversationVariable['type'] => {
    // Map LoopVarType to DefaultValueEditor type
    if (t === 'array[string]') return 'array[string]';
    if (t === 'array[number]') return 'array[number]';
    if (t === 'array[boolean]') return 'array[boolean]';
    if (t === 'array[object]') return 'array[object]';
    return t as 'string' | 'number' | 'boolean' | 'object';
  };

  return (
    <SortableListSection
      title={t('nodes.loop.fields.loopVariables')}
      addLabel={t('nodes.common.add')}
      emptyText={t('nodes.loop.empty.noVariables')}
      isReadOnly={readOnly || false}
      items={items}
      sensors={sensors}
      onDragEnd={handleDragEnd}
      onAdd={handleAdd}
      renderRow={index => (
        <div className="flex gap-2 items-start bg-muted/30 p-2 rounded-md border border-border/50">
          <div className="w-0 grow space-y-2">
            <div className="flex gap-2">
              <Input
                value={rows[index].label}
                onChange={e => updateAt(index, cur => ({ ...cur, label: e.target.value }))}
                placeholder={t('nodes.loop.placeholders.variableName')}
                className="h-8 text-sm flex-1"
                disabled={readOnly}
              />
              <Select
                value={rows[index].var_type}
                onValueChange={v =>
                  updateAt(index, cur => ({
                    ...cur,
                    var_type: v as LoopVarType,
                    value: cur.value_type === 'constant' ? '' : cur.value, // Reset constant value on type change
                  }))
                }
                disabled={readOnly}
              >
                <SelectTrigger className="w-[140px] h-8 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {VAR_TYPES.map(type => (
                    <SelectItem key={type} value={type}>
                      {t(`nodes.types.${type}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="w-full">
              <ValueSourceEditor
                nodeId={nodeId}
                mode={rows[index].value_type}
                onModeChange={mode =>
                  updateAt(index, cur => ({
                    ...cur,
                    value_type: mode,
                    value: mode === 'variable' ? [] : '',
                  }))
                }
                constantEditor={
                  <div className="flex items-center min-w-0 flex-1">
                    <DefaultValueEditor
                      type={toCvType(rows[index].var_type)}
                      value={rows[index].value}
                      onChange={v =>
                        updateAt(index, cur => ({ ...cur, value: v as LoopVariable['value'] }))
                      }
                      readOnly={readOnly}
                    />
                  </div>
                }
                variableValue={
                  Array.isArray(rows[index].value) ? (rows[index].value as string[]) : undefined
                }
                onVariableChange={p => handleChangeValueVar(index, p)}
                disabled={readOnly}
                density="compact"
                constantLabel={t('nodes.tool.form.constant')}
                variableLabel={t('nodes.tool.form.variable')}
              />
            </div>
          </div>
          <Button
            variant="ghost"
            isIcon
            className="h-8 w-8 text-muted-foreground hover:text-destructive shrink-0 mt-0.5"
            onClick={() => removeAt(index)}
            disabled={readOnly}
          >
            <Trash2 size={16} />
          </Button>
        </div>
      )}
    />
  );
};

export default LoopVariablesPanel;

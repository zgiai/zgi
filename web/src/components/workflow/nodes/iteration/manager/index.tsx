'use client';

import React, { useCallback } from 'react';
import type { IterationNodeData, WorkflowVariable } from '../../../store/type';
import NodeValueSelector from '../../../common/node-value-selector';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Slider } from '@/components/ui/slider';
import OutputVariablesView from '../../../common/output-variables-view';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { mapToArrayType } from '../config';
import {
  useContainerVariableSources,
  useNodeData,
  useNodeDataUpdate,
  useNodeOutputVariables,
} from '../../../hooks';

interface IterationManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const IterationManager: React.FC<IterationManagerProps> = ({ id, className, readOnly = false }) => {
  const t = useT();
  const nodeData = useNodeData<IterationNodeData>(id);
  const updateNodeData = useNodeDataUpdate<IterationNodeData>(id);
  const outputVariables = useNodeOutputVariables(id);
  const childOutputGroups = useContainerVariableSources(id, {
    includeScopedSelf: true,
    includeChildOutputs: true,
    excludeChildNodeTypes: ['iteration-start'],
  });

  const filterArrayType = useCallback((type: WorkflowVariable['type']): boolean => {
    return (
      type === 'array' ||
      type === 'array[string]' ||
      type === 'array[number]' ||
      type === 'array[boolean]' ||
      type === 'array[object]' ||
      type === 'array[file]'
    );
  }, []);

  return (
    <div className={['space-y-4', className].filter(Boolean).join(' ')}>
      <div className="space-y-2">
        <Label>{t('nodes.iteration.fields.input')}</Label>
        <NodeValueSelector
          nodeId={id}
          value={
            Array.isArray(nodeData?.iterator_selector) && nodeData.iterator_selector.length >= 2
              ? nodeData.iterator_selector
              : undefined
          }
          onChange={(payload: {
            sourceId: string;
            key: string;
            valuePath: string[];
            type: WorkflowVariable['type'];
          }) => {
            const nextType = payload.type;
            const typeOk = filterArrayType(nextType);
            updateNodeData({
              iterator_selector: payload.valuePath,
              iterator_input_type: typeOk
                ? nextType
                : ('array[string]' as WorkflowVariable['type']),
            });
          }}
          disabled={readOnly}
          typeFilter={type =>
            type === 'array' ||
            type === 'array[string]' ||
            type === 'array[number]' ||
            type === 'array[boolean]' ||
            type === 'array[object]' ||
            type === 'array[file]'
          }
        />
      </div>

      <div className="space-y-2">
        <Label>{t('nodes.iteration.fields.output')}</Label>
        <NodeValueSelector
          nodeId={id}
          value={
            Array.isArray(nodeData?.output_selector) && nodeData.output_selector.length >= 2
              ? nodeData.output_selector
              : undefined
          }
          onChange={(payload: {
            sourceId: string;
            key: string;
            valuePath: string[];
            type: WorkflowVariable['type'];
          }) => {
            updateNodeData({
              output_selector: payload.valuePath,
              output_type: mapToArrayType(payload.type),
            });
          }}
          disabled={readOnly}
          upstreamsOverride={childOutputGroups}
        />
      </div>

      <div className="flex items-center gap-3">
        <Switch
          checked={nodeData?.is_parallel ?? false}
          onCheckedChange={checked => updateNodeData({ is_parallel: Boolean(checked) })}
          disabled={readOnly}
        />
        <Label>{t('nodes.iteration.fields.parallel')}</Label>
      </div>

      {nodeData?.is_parallel && (
        <div className="space-y-3 pt-1">
          <div className="flex items-center justify-between">
            <Label className="text-xs text-muted-foreground">
              {t('nodes.iteration.fields.parallelNums')}
            </Label>
            <span className="text-xs font-medium w-8 text-right">
              {nodeData.parallel_nums ?? 1}
            </span>
          </div>
          <div className="flex items-center gap-4">
            <Slider
              value={[nodeData.parallel_nums ?? 1]}
              min={1}
              max={10}
              step={1}
              onValueChange={vals => updateNodeData({ parallel_nums: vals[0] })}
              className="flex-1"
              disabled={readOnly}
            />
          </div>
        </div>
      )}

      <div className="space-y-2">
        <Label>{t('nodes.iteration.fields.errorMode')}</Label>
        <Select
          value={nodeData?.error_handle_mode ?? 'terminated'}
          onValueChange={val =>
            updateNodeData({ error_handle_mode: val as IterationNodeData['error_handle_mode'] })
          }
          disabled={readOnly}
        >
          <SelectTrigger>
            <SelectValue placeholder={t('nodes.iteration.fields.errorMode')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="terminated">{t('nodes.iteration.errorModes.terminated')}</SelectItem>
            <SelectItem value="continue">{t('nodes.iteration.errorModes.continue')}</SelectItem>
            <SelectItem value="drop-error-output">
              {t('nodes.iteration.errorModes.dropErrorOutput')}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      <OutputVariablesView variables={outputVariables} />
    </div>
  );
};

export default React.memo(IterationManager);

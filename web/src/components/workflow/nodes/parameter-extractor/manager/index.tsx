'use client';

import React, { useCallback, useMemo, useRef } from 'react';
import type { ParameterExtractorNodeData, ParameterSchemaItem, ParameterType } from '../config';
import { Button } from '@/components/ui/button';
import ParamEditorDialog from './param-editor-dialog';
import NodeValueSelector from '@/components/workflow/common/node-value-selector';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '@/components/workflow/common/workflow-value-editor';
import { useT } from '@/i18n';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { cn } from '@/lib/utils';
import { Trash2, Plus, Pencil } from 'lucide-react';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import type { WorkflowVariable } from '../../../store/type';
import { Badge } from '@/components/ui/badge';

import { useNodeData, useNodeDataUpdate, useLocalNodeData, useNodeOutputVariables } from '../../../hooks';

interface ParameterExtractorManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const PARAM_TYPES: ParameterType[] = [
  'string',
  'number',
  'boolean',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
];

const ParameterExtractorManager: React.FC<ParameterExtractorManagerProps> = ({
  id,
  className,
  readOnly = false,
}) => {
  const t = useT();

  const nodeData = useNodeData<ParameterExtractorNodeData>(id);
  const updateNodeData = useNodeDataUpdate<ParameterExtractorNodeData>(id);

  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: nodeData?.model || {},
    enabled: !readOnly,
    onInitialize: v => {
      updateNodeData({
        model: {
          provider: v.provider,
          name: v.model,
          mode: nodeData?.model?.mode ?? 'chat',
          completion_params: v.params as Record<string, string | number | boolean>,
        },
      });
    },
  });

  const safe = useMemo<ParameterExtractorNodeData>(
    () =>
      ({
        ...nodeData,
        model: {
          provider: nodeData?.model?.provider ?? '',
          name: nodeData?.model?.name ?? '',
          mode: nodeData?.model?.mode ?? 'chat',
          completion_params: ((): Record<string, string | number | boolean> => {
            const src = nodeData?.model?.completion_params as Record<string, unknown> | undefined;
            if (!src) return {};
            const out: Record<string, string | number | boolean> = {};
            for (const k in src) {
              const v = src[k];
              if (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean') {
                out[k] = v;
              }
            }
            return out;
          })(),
        },
        query: Array.isArray(nodeData?.query) ? nodeData.query : [],
        reasoning_mode: nodeData?.reasoning_mode ?? 'prompt',
        vision: {
          enabled: nodeData?.vision?.enabled ?? false,
          configs: {
            detail: nodeData?.vision?.configs?.detail ?? 'high',
            variable_selector: (Array.isArray(nodeData?.vision?.configs?.variable_selector)
              ? nodeData.vision?.configs?.variable_selector
              : []) as [string, string] | [],
          },
        },
        instruction: nodeData?.instruction ?? '',
        parameters: Array.isArray(nodeData?.parameters) ? nodeData.parameters : [],
      }) as ParameterExtractorNodeData,
    [nodeData]
  );

  const { localData: localInstruction, setLocalData: setLocalInstruction } =
    useLocalNodeData<string>(id, {
      path: 'instruction',
      delay: 400,
    });

  const handleVisionToggle = useCallback(
    (enabled: boolean) => {
      updateNodeData({ vision: { ...safe.vision, enabled } });
    },
    [updateNodeData, safe.vision]
  );

  const handleVisionVar = useCallback(
    (payload: {
      sourceId: string;
      key: string;
      valuePath: string[];
      type: WorkflowVariable['type'];
    }) => {
      // Expect file or array[file]
      if (payload.type !== 'file' && payload.type !== 'array[file]') return;
      updateNodeData({
        vision: {
          ...safe.vision,
          configs: { ...safe.vision.configs, variable_selector: payload.valuePath },
        },
      });
    },
    [updateNodeData, safe.vision]
  );

  const [paramModalOpen, setParamModalOpen] = React.useState(false);
  const [editingIndex, setEditingIndex] = React.useState<number | null>(null);
  const [localParam, setLocalParam] = React.useState<ParameterSchemaItem>({
    name: '',
    type: 'string',
    required: false,
    description: '',
    options: [],
  });

  const openAddParam = useCallback(() => {
    setEditingIndex(null);
    setLocalParam({ name: '', type: 'string', required: false, description: '', options: [] });
    setParamModalOpen(true);
  }, []);

  const openEditParam = useCallback(
    (index: number) => {
      const cur = safe.parameters[index];
      if (!cur) return;
      setEditingIndex(index);
      setLocalParam({
        name: cur.name || '',
        type: cur.type,
        required: Boolean(cur.required),
        description: cur.description || '',
        options: Array.isArray(cur.options) ? cur.options : [],
      });
      setParamModalOpen(true);
    },
    [safe.parameters]
  );

  const saveParam = useCallback(
    (finalValue: ParameterSchemaItem) => {
      const nextItem: ParameterSchemaItem = {
        name: finalValue.name,
        type: finalValue.type,
        required: Boolean(finalValue.required),
        description: finalValue.description || '',
        options: Array.isArray(finalValue.options) ? finalValue.options : [],
      };
      if (typeof editingIndex === 'number') {
        const next = safe.parameters.map((p, i) => (i === editingIndex ? nextItem : p));
        updateNodeData({ parameters: next });
      } else {
        updateNodeData({ parameters: [...safe.parameters, nextItem] });
      }
      setParamModalOpen(false);
      setEditingIndex(null);
    },
    [editingIndex, updateNodeData, safe.parameters]
  );

  const removeParamAt = useCallback(
    (index: number) => {
      const next = safe.parameters.filter((_, i) => i !== index);
      updateNodeData({ parameters: next });
    },
    [updateNodeData, safe.parameters]
  );

  const outputs = useNodeOutputVariables(id);

  const editorRef = useRef<WorkflowValueEditorHandle | null>(null);

  return (
    <div className={cn('space-y-4', className)}>
      <section className="space-y-3">
        <h3 className="text-base font-semibold">{t('nodes.parameterExtractor.section.model')}</h3>
        <ModelSelectorParameter
          modelType="text-chat"
          value={{
            provider: safe.model.provider,
            model: safe.model.name,
            params: safe.model.completion_params || {},
          }}
          disabled={readOnly}
          onChange={v => {
            updateNodeData({
              model: {
                provider: v.provider,
                name: v.model,
                mode: safe.model.mode,
                completion_params: v.params,
              },
            });
          }}
        />
      </section>

      {/* Query Selection */}
      <section className="space-y-3">
        <h3 className="text-base font-semibold">{t('nodes.parameterExtractor.section.query')}</h3>
        <NodeValueSelector
          nodeId={id}
          value={safe.query}
          onChange={val => updateNodeData({ query: val.valuePath })}
          disabled={readOnly}
          placeholder={t('nodes.parameterExtractor.placeholders.selectTextVar')}
        />
      </section>

      {/* Instructions */}
      <section className="space-y-3">
        <h3 className="text-base font-semibold">
          {t('nodes.parameterExtractor.section.instruction')}
        </h3>
        <WorkflowValueInserter
          nodeId={id}
          className="w-full"
          disabled={readOnly}
          onInsert={value => {
            editorRef.current?.insertToken(value.sourceId, value.key);
          }}
        />
        <WorkflowValueEditor
          ref={editorRef}
          value={localInstruction}
          onChange={setLocalInstruction}
          nodeId={id}
          editorClassName="min-h-[160px]"
          placeholder={t('nodes.parameterExtractor.placeholders.instruction')}
          readOnly={readOnly}
        />
      </section>

      <div>
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-sm font-medium">
            {t('nodes.parameterExtractor.section.parameters')}
          </h3>
          <Button
            variant="ghost"
            isIcon
            onClick={openAddParam}
            className="w-8 h-8"
            disabled={readOnly}
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>
        {safe.parameters.length === 0 ? (
          <div className="text-xs text-muted-foreground">
            {t('nodes.parameterExtractor.empty.noParameters')}
          </div>
        ) : (
          <div className="space-y-2">
            {safe.parameters.map((p, i) => (
              <div
                key={`${p.name}-${i}`}
                className="border rounded-md p-2 group relative bg-muted overflow-hidden"
              >
                <div className="flex-1 space-y-1">
                  <div className="font-medium leading-none flex justify-between items-center">
                    <div className="grow flex items-center gap-2">
                      <span className="truncate">{p.name}</span>
                      <Badge className="py-0 px-1">{p.type}</Badge>
                    </div>
                    {p.required ? (
                      <span className="text-secondary-foreground text-xs font-mono">
                        {t('nodes.parameterExtractor.labels.required')}
                      </span>
                    ) : null}
                  </div>
                  {p.description ? (
                    <div className="text-xs text-muted-foreground line-clamp-2">
                      {p.description}
                    </div>
                  ) : null}
                </div>
                <div className="hidden h-full px-1 group-hover:flex items-center gap-1 absolute right-0 top-1/2 -translate-y-1/2 bg-gradient-to-r from-transparent to-background">
                  <Button
                    variant="ghost"
                    isIcon
                    onClick={() => openEditParam(i)}
                    aria-label={t('nodes.parameterExtractor.actions.edit')}
                    className="w-7 h-7"
                    disabled={readOnly}
                  >
                    <Pencil className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    isIcon
                    onClick={() => removeParamAt(i)}
                    aria-label={t('nodes.parameterExtractor.actions.remove')}
                    className="w-7 h-7 text-destructive hover:bg-red-100 hover:text-destructive"
                    disabled={readOnly}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div>
        <div className="flex items-center gap-2 mb-2">
          <h3 className="text-sm font-medium">{t('nodes.parameterExtractor.section.vision')}</h3>
          <Switch checked={safe.vision.enabled} onCheckedChange={handleVisionToggle} disabled={readOnly} />
        </div>
        {safe.vision.enabled && (
          <div className="space-y-2">
            <div>
              <div className="text-xs font-medium mb-1">
                {t('nodes.parameterExtractor.labels.detail')}
              </div>
              <Select
                value={safe.vision.configs.detail}
                onValueChange={val =>
                  updateNodeData({
                    vision: {
                      ...safe.vision,
                      configs: { ...safe.vision.configs, detail: val as 'high' | 'low' },
                    },
                  })
                }
                disabled={readOnly}
              >
                <SelectTrigger className="w-[160px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="low">{t('nodes.parameterExtractor.labels.low')}</SelectItem>
                  <SelectItem value="high">{t('nodes.parameterExtractor.labels.high')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <div className="text-xs font-medium mb-1">
                {t('nodes.parameterExtractor.labels.imageVar')}
              </div>
              <NodeValueSelector
                nodeId={id}
                value={
                  Array.isArray(safe.vision.configs.variable_selector) &&
                  safe.vision.configs.variable_selector.length >= 2
                    ? safe.vision.configs.variable_selector
                    : undefined
                }
                onChange={handleVisionVar}
                typeFilter={type => type === 'file' || type === 'array[file]'}
                placeholder={t('nodes.parameterExtractor.placeholders.selectFileVar')}
                disabled={readOnly}
              />
            </div>
          </div>
        )}
      </div>
      <OutputVariablesView variables={outputs} />
      <ParamEditorDialog
        open={paramModalOpen}
        editing={typeof editingIndex === 'number'}
        value={localParam}
        onChange={setLocalParam}
        onOpenChange={setParamModalOpen}
        onSubmit={saveParam}
        existingNames={
          safe.parameters
            .map((p, i) => (i === editingIndex ? '' : p.name))
            .filter(Boolean) as string[]
        }
        typeOptions={PARAM_TYPES}
        labels={{
          titleAdd: t('nodes.parameterExtractor.modal.addTitle'),
          titleEdit: t('nodes.parameterExtractor.modal.editTitle'),
          fieldName: t('nodes.parameterExtractor.fields.name'),
          fieldType: t('nodes.parameterExtractor.fields.type'),
          fieldDesc: t('nodes.parameterExtractor.fields.description'),
          required: t('nodes.parameterExtractor.labels.required'),
          actionCancel: t('nodes.parameterExtractor.actions.cancel'),
          actionSave: t('nodes.parameterExtractor.actions.save'),
          invalidIdentifier: t('nodes.parameterExtractor.modal.invalidIdentifier'),
          duplicateName: t('nodes.parameterExtractor.modal.duplicateName'),
        }}
      />
    </div>
  );
};

export default ParameterExtractorManager;

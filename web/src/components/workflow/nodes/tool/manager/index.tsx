'use client';

import React, { useMemo, useCallback } from 'react';
import { cn } from '@/lib/utils';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import NodeValueSelector from '../../../common/node-value-selector';
import type { ToolNodeData, ToolParameterBinding, ToolParameterBindingType } from '../config';
import type { ToolFormField } from '@/services/types/tool';
import { useBuiltinTools } from '@/hooks/workflow/use-builtin-tools';
import { mapParametersToFormFields, coerceValue } from '@/utils/tool-helpers';
import { useLocale } from '@/hooks/use-locale';
import type { WorkflowVariable } from '@/components/workflow/store';
import { Wrench, Info, Variable } from 'lucide-react';
import { useWorkflowStore } from '../../../store';
import OutputVariablesView from '../../../common/output-variables-view';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '../../../common/workflow-value-editor';
import { useT } from '@/i18n';
import ToolConfigDialog from './tool-config-dialog';
import ValueSourceEditor from '../../../common/value-source-editor';

import { useNodeData, useNodeDataUpdate, useLocalNodeData, useNodeOutputVariables } from '../../../hooks';

interface ToolManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

// Map form field type to primitive type for variable selector filter
const primitiveOfField = (t: ToolFormField['type']): WorkflowVariable['type'] => {
  switch (t) {
    case 'number':
      return 'number';
    case 'checkbox':
      return 'boolean';
    case 'file':
      return 'file';
    case 'select':
    case 'secret':
    case 'text':
    default:
      return 'string';
  }
};

const paramsEqual = (a: any, b: any) => {
  try {
    return JSON.stringify(a) === JSON.stringify(b);
  } catch {
    return a === b;
  }
};

const ToolManager: React.FC<ToolManagerProps> = ({ id: nodeId, className, readOnly = false }) => {
  const { tools, isLoading, isFetching } = useBuiltinTools();
  const { locale } = useLocale();
  const t = useT();
  const [showConfig, setShowConfig] = React.useState(false);

  // Use store-aware useLocalNodeData for 'tool_parameters' with debouncing
  const { localData: toolParameters, setLocalData: setToolParameters } = useLocalNodeData<
    Record<string, ToolParameterBinding>
  >(nodeId, {
    path: 'tool_parameters',
    delay: 400,
    isEqual: paramsEqual,
  });

  // Use fine-grained selector for other data or just useNodeData
  const nodeData = useNodeData<ToolNodeData>(nodeId);
  const updateNodeData = useNodeDataUpdate<ToolNodeData>(nodeId);
  const outputs = useNodeOutputVariables(nodeId);

  // Get provider icon URL from tools list
  const providerIcon = useMemo(() => {
    if (!nodeData?.provider_id) return undefined;
    const providerList = Array.isArray(tools) ? tools : [];
    const provider = providerList.find(
      p => p.id === nodeData.provider_id || p.name === nodeData.provider_id
    );
    return provider?.icon;
  }, [tools, nodeData?.provider_id]);

  // Compute form fields from provider declaration dynamically
  const formFields = useMemo(() => {
    const providerList = Array.isArray(tools) ? tools : [];
    const provider = providerList.find(
      p => p.id === nodeData?.provider_id || p.name === nodeData?.provider_id
    );
    const toolList = provider?.tools ?? [];
    const tool = toolList.find(t => t.name === nodeData?.tool_name);
    return tool ? mapParametersToFormFields(tool.parameters, locale) : [];
  }, [tools, nodeData?.provider_id, nodeData?.tool_name, locale]);

  // Find tool definition for config_parameters
  const toolDefinition = useMemo(() => {
    const providerList = Array.isArray(tools) ? tools : [];
    const provider = providerList.find(
      p => p.id === nodeData?.provider_id || p.name === nodeData?.provider_id
    );
    const toolList = provider?.tools ?? [];
    return toolList.find(t => t.name === nodeData?.tool_name);
  }, [tools, nodeData?.provider_id, nodeData?.tool_name]);

  const configParams = toolDefinition?.config_parameters;
  const hasConfig = !!(configParams && configParams.enable);

  // Create a map for quick field lookup
  const fieldsByName = useMemo(() => {
    const map = new Map<string, ToolFormField>();
    for (const f of formFields) map.set(f.name, f);
    return map;
  }, [formFields]);

  // Read latest tool_parameters from store to avoid reverting sibling edits when updates are batched
  const getLatestParams = useCallback(() => {
    try {
      const nodes = useWorkflowStore.getState().nodes;
      const found = nodes.find(n => n.id === nodeId);
      const params = (found?.data as { tool_parameters?: unknown })?.tool_parameters;
      if (params && typeof params === 'object') {
        return { ...(params as Record<string, ToolParameterBinding>) };
      }
    } catch {
      // fall back to props snapshot
    }
    return { ...(nodeData?.tool_parameters || {}) } as Record<string, ToolParameterBinding>;
  }, [nodeId, nodeData?.tool_parameters]);

  const updateBinding = useCallback(
    (name: string, next: ToolParameterBinding) => {
      setToolParameters(prev => {
        const current = prev?.[name];
        // Skip if binding is identical to avoid redundant local state churn
        if (paramsEqual(current, next)) return prev;
        return { ...prev, [name]: next };
      });
    },
    [setToolParameters]
  );

  const setConstant = useCallback(
    (name: string, raw: unknown) => {
      const field = fieldsByName.get(name);
      const coerced = field ? coerceValue(field, raw) : undefined;
      if (coerced === undefined) {
        updateBinding(name, { type: 'constant', value: undefined });
      } else {
        updateBinding(name, { type: 'constant', value: coerced });
      }
    },
    [fieldsByName, updateBinding]
  );

  const setVariable = useCallback(
    (name: string, value: string[] | undefined) => {
      updateBinding(name, { type: 'variable', value });
    },
    [updateBinding]
  );
  // Move tNodes declaration to top of component (already moved above)

  const memoizedFields = useMemo(() => {
    return formFields.map(field => (
      <ToolParameterField
        key={field.name}
        nodeId={nodeId}
        field={field}
        binding={toolParameters?.[field.name]}
        readOnly={readOnly}
        isLoading={isLoading}
        isFetching={isFetching}
        updateBinding={updateBinding}
        setConstant={setConstant}
        setVariable={setVariable}
        getLatestParams={getLatestParams}
      />
    ));
  }, [
    formFields,
    nodeId,
    toolParameters,
    readOnly,
    isLoading,
    isFetching,
    updateBinding,
    setConstant,
    setVariable,
    getLatestParams,
  ]);

  return (
    <div className={cn('space-y-3', className)}>
      {/* Identity header with provider icon */}
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-full bg-slate-100 flex items-center justify-center overflow-hidden">
            {providerIcon ? (
              // <ImageSafe
              //   src={providerIcon}
              //   className="w-6 h-6 object-contain"
              //   loading="lazy"
              //   aria-hidden="true"
              //   referrerPolicy="no-referrer"
              //   fallbackComponent={
              <div className="p-1.5 rounded-full bg-primary flex items-center justify-center">
                <Wrench size={18} className="text-primary-foreground" />
              </div>
            ) : //   }
            // />
            isLoading ? (
              <Skeleton className="w-6 h-6 rounded" />
            ) : (
              <div className="p-1.5 rounded-full bg-primary flex items-center justify-center">
                <Wrench size={18} className="text-primary-foreground" />
              </div>
            )}
          </div>
          <div className="text-sm">
            {isLoading ? (
              <>
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-3 w-32 mt-1" />
              </>
            ) : (
              <>
                <div className="font-medium">{nodeData?.title}</div>
              </>
            )}
          </div>
        </div>

        {hasConfig && (
          <Button
            size="sm"
            className="h-8 gap-1.5 text-xs"
            onClick={() => setShowConfig(true)}
            disabled={readOnly}
          >
            <Wrench size={14} />
            工具配置
          </Button>
        )}
      </div>

      {/* Parameters editor */}
      <div className="space-y-4">
        {isLoading && formFields.length > 0
          ? formFields.map(f => (
              <div key={f.name} className="space-y-2">
                <Skeleton className="h-4 w-24" />
                <Skeleton className="h-9 w-full" />
              </div>
            ))
          : memoizedFields}
      </div>

      <OutputVariablesView variables={outputs} />

      {hasConfig && configParams && (
        <ToolConfigDialog
          open={showConfig}
          onOpenChange={setShowConfig}
          configParameters={configParams}
          initialValues={nodeData?.tool_configurations || {}}
          onSave={values => {
            updateNodeData({ tool_configurations: values });
          }}
        />
      )}
    </div>
  );
};

interface ToolParameterFieldProps {
  nodeId: string;
  field: ToolFormField;
  binding: ToolParameterBinding | undefined;
  readOnly: boolean;
  isLoading: boolean;
  isFetching: boolean;
  updateBinding: (name: string, next: ToolParameterBinding) => void;
  setConstant: (name: string, raw: unknown) => void;
  setVariable: (name: string, value: string[] | undefined) => void;
  getLatestParams: () => Record<string, ToolParameterBinding>;
}

const ToolParameterField = React.memo<ToolParameterFieldProps>(
  ({
    nodeId,
    field,
    binding,
    readOnly,
    isLoading,
    isFetching,
    updateBinding,
    setConstant,
    setVariable,
    getLatestParams,
  }) => {
    const t = useT();
    const editorRef = React.useRef<WorkflowValueEditorHandle>(null);
    const disabled = readOnly || isLoading || isFetching;
    const primitive = primitiveOfField(field.type);

    // Determine mode based on field type and current binding
    let mode: ToolParameterBindingType = binding?.type || 'constant';

    if (field.type === 'text') {
      mode = 'mixed';
    } else if (field.type === 'file') {
      mode = 'variable';
    } else if (field.type === 'secret' || field.type === 'checkbox') {
      mode = 'constant';
    }

    const reqMark = field.required ? <span className="text-red-500 ml-1">*</span> : null;
    const showModeSwitcher =
      field.type !== 'text' &&
      field.type !== 'file' &&
      field.type !== 'secret' &&
      field.type !== 'checkbox';

    const constantEditor =
      field.type === 'checkbox' ? (
        <div className="flex items-center gap-2">
          <Switch
            checked={Boolean(binding?.value)}
            onCheckedChange={v => setConstant(field.name, v)}
            disabled={disabled}
          />
        </div>
      ) : field.type === 'number' ? (
        <Input
          type="number"
          inputMode="decimal"
          value={typeof binding?.value === 'number' ? String(binding?.value) : ''}
          onChange={e => setConstant(field.name, e.target.value)}
          disabled={disabled}
        />
      ) : field.type === 'select' && field.options && field.options.length > 0 ? (
        <Select
          value={typeof binding?.value === 'string' ? (binding?.value as string) : ''}
          onValueChange={val => setConstant(field.name, val)}
          disabled={disabled}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {field.options.map(opt => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : field.type === 'secret' ? (
        <Input
          type="password"
          value={typeof binding?.value === 'string' ? (binding?.value as string) : ''}
          onChange={e => setConstant(field.name, e.target.value)}
          disabled={disabled}
          autoComplete="new-password"
        />
      ) : (
        <Textarea
          value={typeof binding?.value === 'string' ? (binding?.value as string) : ''}
          onChange={e => setConstant(field.name, e.target.value)}
          disabled={disabled}
          className="min-h-[60px] max-h-[120px]"
          placeholder={field.description}
        />
      );

    return (
      <div className={cn('space-y-2')}>
        <div className="flex items-center justify-between">
          <Label className="text-sm flex items-center gap-1">
            {field.label}
            {reqMark}
            {field.description && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info
                    size={14}
                    className="text-muted-foreground hover:text-foreground cursor-help"
                  />
                </TooltipTrigger>
                <TooltipContent side="top">{field.description}</TooltipContent>
              </Tooltip>
            )}
          </Label>

          <div className="flex items-center gap-2">
            {mode === 'mixed' && !disabled && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="xs"
                    isIcon
                    className="h-6 w-6 text-muted-foreground hover:text-primary hover:bg-primary/5 transition-colors"
                    onClick={() => editorRef.current?.openVariableSelector()}
                  >
                    <Variable size={14} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="top">{t('nodes.common.insertVariable')}</TooltipContent>
              </Tooltip>
            )}
          </div>
        </div>

        {/* Mode-specific input */}
        {mode === 'mixed' ? (
          <WorkflowValueEditor
            ref={editorRef}
            nodeId={nodeId}
            value={typeof binding?.value === 'string' ? binding.value : ''}
            onChange={v => updateBinding(field.name, { type: 'mixed', value: v })}
            placeholder={field.description}
            editorClassName="min-h-[60px] max-h-[160px] text-sm"
            readOnly={disabled}
          />
        ) : showModeSwitcher ? (
          <ValueSourceEditor
            nodeId={nodeId}
            mode={mode}
            onModeChange={nextMode => {
              const latest = getLatestParams()[field.name];
              updateBinding(field.name, {
                type: nextMode,
                value:
                  nextMode === 'variable'
                    ? (Array.isArray(latest?.value) ? latest?.value : undefined)
                    : latest?.value,
              });
            }}
            constantEditor={constantEditor}
            variableValue={(() => {
              const val = binding?.value;
              if (!val) return undefined;
              return Array.isArray(val) && val.length >= 2 ? val : undefined;
            })()}
            onVariableChange={val => setVariable(field.name, val.valuePath)}
            variableTypeFilter={v => v === primitive}
            variablePlaceholder={field.description}
            disabled={disabled}
            density="compact"
            constantLabel={t('nodes.tool.form.constant')}
            variableLabel={t('nodes.tool.form.variable')}
          />
        ) : mode === 'variable' ? (
          <NodeValueSelector
            nodeId={nodeId}
            disabled={disabled}
            typeFilter={v => v === primitive}
            value={(() => {
              const val = binding?.value;
              if (!val) return undefined;
              if (Array.isArray(val) && val.length >= 2) {
                return val;
              }
              return undefined;
            })()}
            onChange={val => setVariable(field.name, val.valuePath)}
          />
        ) : (
          constantEditor
        )}
      </div>
    );
  }
);

export default React.memo(ToolManager);

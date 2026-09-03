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
import {
  integrationConnectionItems,
  useAllAvailableIntegrationConnections,
} from '@/hooks/integrations/use-integrations';

import {
  useNodeData,
  useNodeDataUpdate,
  useLocalNodeData,
  useNodeOutputVariables,
} from '../../../hooks';

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
    case 'json':
      return 'object';
    case 'string-list':
      return 'array[string]';
    case 'date':
    case 'date-time':
    case 'select':
    case 'secret':
    case 'text':
    default:
      return 'string';
  }
};

const variableMatchesField = (
  fieldType: ToolFormField['type'],
  variableType: WorkflowVariable['type']
) => {
  if (fieldType === 'json') {
    return (
      variableType === 'object' || variableType === 'array' || variableType.startsWith('array[')
    );
  }
  if (fieldType === 'string-list') {
    return (
      variableType === 'string' || variableType === 'array' || variableType === 'array[string]'
    );
  }
  return variableType === primitiveOfField(fieldType);
};

const padDateTimePart = (value: number) => String(value).padStart(2, '0');

const toDateTimeLocalValue = (value: unknown) => {
  if (typeof value !== 'string' || value.trim() === '') return '';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return '';
  return `${parsed.getFullYear()}-${padDateTimePart(parsed.getMonth() + 1)}-${padDateTimePart(parsed.getDate())}T${padDateTimePart(parsed.getHours())}:${padDateTimePart(parsed.getMinutes())}:${padDateTimePart(parsed.getSeconds())}`;
};

const fromDateTimeLocalValue = (value: string) => {
  if (!value.trim()) return '';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toISOString();
};

const paramsEqual = (a: unknown, b: unknown) => {
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

  const providerDefinition = useMemo(() => {
    const providerList = Array.isArray(tools) ? tools : [];
    return providerList.find(
      provider => provider.id === nodeData?.provider_id || provider.name === nodeData?.provider_id
    );
  }, [nodeData?.provider_id, tools]);
  const isConnector =
    nodeData?.provider_type === 'connector' || providerDefinition?.type === 'connector';
  const connectionsQuery = useAllAvailableIntegrationConnections(
    { integration_id: isConnector ? nodeData?.provider_id : undefined },
    Boolean(isConnector && nodeData?.provider_id)
  );
  const connectorConnections = useMemo(
    () =>
      integrationConnectionItems(connectionsQuery.data?.data)
        .filter(connection => connection.credential_source !== 'account')
        .filter(connection => connection.status === 'active')
        .sort((left, right) => Number(right.is_default) - Number(left.is_default)),
    [connectionsQuery.data?.data]
  );

  // Get provider icon URL from tools list
  const providerIcon = useMemo(() => {
    if (!nodeData?.provider_id) return undefined;
    return providerDefinition?.icon;
  }, [nodeData?.provider_id, providerDefinition?.icon]);

  // Compute form fields from provider declaration dynamically
  const formFields = useMemo(() => {
    const toolList = providerDefinition?.tools ?? [];
    const tool = toolList.find(t => t.name === nodeData?.tool_name);
    return tool ? mapParametersToFormFields(tool.parameters, locale) : [];
  }, [providerDefinition?.tools, nodeData?.tool_name, locale]);

  // Find tool definition for config_parameters
  const toolDefinition = useMemo(() => {
    const toolList = providerDefinition?.tools ?? [];
    return toolList.find(t => t.name === nodeData?.tool_name);
  }, [providerDefinition?.tools, nodeData?.tool_name]);

  React.useEffect(() => {
    const providerList = Array.isArray(tools) ? tools : [];
    if (providerList.length === 0) return;

    const state = useWorkflowStore.getState();
    for (const node of state.nodes) {
      const data = node.data as Partial<ToolNodeData>;
      if (data.type !== 'tools' || !data.provider_id || !data.tool_name) continue;
      const provider = providerList.find(
        item => item.id === data.provider_id || item.name === data.provider_id
      );
      const definition = provider?.tools?.find(item => item.name === data.tool_name);
      if (!definition) continue;
      const next = definition.workflow_outputs || [];
      if (!paramsEqual(data.workflow_outputs || [], next)) {
        state.updateNodeData(node.id, { workflow_outputs: next });
      }
    }
  }, [tools]);

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
    (name: string, value: string[] | undefined, variableType?: WorkflowVariable['type']) => {
      const field = fieldsByName.get(name);
      updateBinding(name, {
        type: 'variable',
        value,
        coerce:
          field?.type === 'string-list' && variableType === 'string'
            ? 'singleton_array'
            : undefined,
      });
    },
    [fieldsByName, updateBinding]
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

      {isConnector ? (
        <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
          <Label className="text-sm">
            {t('nodes.tool.connection.label')}
            <span className="ml-1 text-red-500">*</span>
          </Label>
          {connectionsQuery.isLoading ? (
            <Skeleton className="h-9 w-full" />
          ) : connectionsQuery.isError ? (
            <p className="text-xs text-destructive">{t('nodes.tool.connection.loadFailed')}</p>
          ) : connectorConnections.length === 0 ? (
            <p className="text-xs text-muted-foreground">{t('nodes.tool.connection.empty')}</p>
          ) : (
            <Select
              value={nodeData?.connection_id || ''}
              onValueChange={connectionId => updateNodeData({ connection_id: connectionId })}
              disabled={readOnly}
            >
              <SelectTrigger>
                <SelectValue placeholder={t('nodes.tool.connection.placeholder')} />
              </SelectTrigger>
              <SelectContent>
                {connectorConnections.map(connection => (
                  <SelectItem key={connection.id} value={connection.id}>
                    {connection.name}
                    {connection.display_name ? ` · ${connection.display_name}` : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <p className="text-xs text-muted-foreground">{t('nodes.tool.connection.hint')}</p>
        </div>
      ) : null}

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
  setVariable: (
    name: string,
    value: string[] | undefined,
    variableType?: WorkflowVariable['type']
  ) => void;
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
    // Determine mode based on field type and current binding
    let mode: ToolParameterBindingType = binding?.type || 'constant';

    if (field.type === 'text') {
      mode = 'mixed';
    } else if (field.type === 'file') {
      mode = 'variable';
    } else if (field.type === 'secret' || field.type === 'checkbox') {
      mode = 'constant';
    } else if (
      binding?.type === 'mixed' &&
      (field.type === 'date' || field.type === 'date-time' || field.type === 'string-list')
    ) {
      // Older workflow drafts stored every string-like field as a mixed
      // binding. Semantic inputs cannot embed template fragments, so migrate
      // their editor mode in place while retaining the persisted value.
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
      ) : field.type === 'json' ? (
        <JSONConstantEditor
          value={binding?.type === 'constant' ? binding.value : undefined}
          onChange={value => updateBinding(field.name, { type: 'constant', value })}
          disabled={disabled}
          invalidMessage={t('nodes.tool.form.invalidJson')}
          placeholder={field.placeholder}
        />
      ) : field.type === 'string-list' ? (
        <Textarea
          value={Array.isArray(binding?.value) ? binding.value.join('\n') : ''}
          onChange={event => setConstant(field.name, event.target.value)}
          disabled={disabled}
          className="min-h-[88px] max-h-[180px]"
          placeholder={field.placeholder || t('nodes.tool.form.stringListPlaceholder')}
        />
      ) : field.type === 'date-time' ? (
        <Input
          type="datetime-local"
          step={1}
          value={toDateTimeLocalValue(binding?.value)}
          onChange={event => setConstant(field.name, fromDateTimeLocalValue(event.target.value))}
          disabled={disabled}
          aria-label={field.label}
        />
      ) : field.type === 'date' ? (
        <Input
          type="date"
          value={typeof binding?.value === 'string' ? binding.value : ''}
          onChange={event => setConstant(field.name, event.target.value)}
          disabled={disabled}
          aria-label={field.label}
        />
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
          placeholder={field.placeholder}
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
            placeholder={field.placeholder}
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
                    ? Array.isArray(latest?.value)
                      ? latest?.value
                      : undefined
                    : latest?.value,
              });
            }}
            constantEditor={constantEditor}
            variableValue={(() => {
              const val = binding?.value;
              if (!val) return undefined;
              return Array.isArray(val) &&
                val.length >= 2 &&
                val.every(item => typeof item === 'string')
                ? (val as string[])
                : undefined;
            })()}
            onVariableChange={val => setVariable(field.name, val.valuePath, val.type)}
            variableTypeFilter={v => variableMatchesField(field.type, v)}
            variablePlaceholder={field.placeholder}
            disabled={disabled}
            density="compact"
            constantLabel={t('nodes.tool.form.constant')}
            variableLabel={t('nodes.tool.form.variable')}
          />
        ) : mode === 'variable' ? (
          <NodeValueSelector
            nodeId={nodeId}
            disabled={disabled}
            typeFilter={v => variableMatchesField(field.type, v)}
            value={(() => {
              const val = binding?.value;
              if (!val) return undefined;
              if (
                Array.isArray(val) &&
                val.length >= 2 &&
                val.every(item => typeof item === 'string')
              ) {
                return val as string[];
              }
              return undefined;
            })()}
            onChange={val => setVariable(field.name, val.valuePath, val.type)}
          />
        ) : (
          constantEditor
        )}
      </div>
    );
  }
);

ToolParameterField.displayName = 'ToolParameterField';

interface JSONConstantEditorProps {
  value: ToolParameterBinding['value'];
  onChange: (value: Record<string, unknown> | unknown[] | undefined) => void;
  disabled: boolean;
  invalidMessage: string;
  placeholder?: string;
}

const JSONConstantEditor: React.FC<JSONConstantEditorProps> = ({
  value,
  onChange,
  disabled,
  invalidMessage,
  placeholder,
}) => {
  const serialize = React.useCallback((input: ToolParameterBinding['value']) => {
    if (!input || typeof input !== 'object') return '';
    try {
      return JSON.stringify(input, null, 2);
    } catch {
      return '';
    }
  }, []);
  const [text, setText] = React.useState(() => serialize(value));
  const [invalid, setInvalid] = React.useState(false);

  React.useEffect(() => {
    setText(serialize(value));
    setInvalid(false);
  }, [serialize, value]);

  return (
    <div className="space-y-1.5">
      <Textarea
        value={text}
        onChange={event => {
          const next = event.target.value;
          setText(next);
          if (next.trim() === '') {
            setInvalid(false);
            onChange(undefined);
            return;
          }
          try {
            const parsed: unknown = JSON.parse(next);
            if (!parsed || typeof parsed !== 'object') {
              setInvalid(true);
              return;
            }
            setInvalid(false);
            onChange(parsed as Record<string, unknown> | unknown[]);
          } catch {
            setInvalid(true);
          }
        }}
        disabled={disabled}
        className="min-h-[120px] font-mono text-xs"
        placeholder={placeholder}
        aria-invalid={invalid}
      />
      {invalid ? <p className="text-xs text-destructive">{invalidMessage}</p> : null}
    </div>
  );
};

export default React.memo(ToolManager);

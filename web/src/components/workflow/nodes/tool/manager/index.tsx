'use client';

import { ConnectorIcon } from '@/components/connectors/connector-icon';

import React, { useMemo, useCallback } from 'react';
import type { ConnectorReferenceCommand } from '@/services/types/connector';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { ConnectorPickerDialog } from '@/components/connectors/picker';
import type {
  ConnectorPickerActionRequest,
  ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { ConnectorCreateContent } from '@/components/connectors/setup/connector-create-dialog';
import { Label } from '@/components/ui/label';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Dialog, DialogContent } from '@/components/ui/dialog';
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
import { ChevronsUpDown, Wrench, Info, Variable } from 'lucide-react';
import { useWorkflowStore } from '../../../store';
import OutputVariablesView from '../../../common/output-variables-view';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '../../../common/workflow-value-editor';
import { useT } from '@/i18n';
import ToolConfigDialog from './tool-config-dialog';
import { workflowConnectorSelection } from './workflow-connector-selection';
import ValueSourceEditor from '../../../common/value-source-editor';
import { personalConnectionItems, usePersonalConnections } from '@/hooks/connectors/use-connectors';
import type { ConnectorCatalogItem } from '@/services/types/connector';

import {
  useNodeData,
  useNodeDataUpdate,
  useLocalNodeData,
  useNodeOutputVariables,
  useWorkflowEditor,
} from '../../../hooks';

interface ToolManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

interface WorkflowConnectorSetupContext {
  connector: ConnectorCatalogItem;
  replacedBinding?: ConnectorPickerBindingRef;
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

function connectorDetailURL(request: ConnectorPickerActionRequest): string {
  const params = new URLSearchParams({
    target_type: 'workflow',
    target_id: request.target?.id ?? '',
  });
  if (request.target?.binding_instance_id) {
    params.set('binding_instance_id', request.target.binding_instance_id);
  }
  if (request.connection?.id) params.set('connection_id', request.connection.id);
  return `/console/connectors/${encodeURIComponent(request.connector.id)}?${params.toString()}`;
}

const ToolManager: React.FC<ToolManagerProps> = ({ id: nodeId, className, readOnly = false }) => {
  const router = useRouter();
  const { workflowId } = useWorkflowEditor();
  const { tools, isLoading, isFetching } = useBuiltinTools();
  const { locale } = useLocale();
  const connectorT = useT('connectors');
  const navigationT = useT('navigation');
  const [showConfig, setShowConfig] = React.useState(false);
  const [showConnectorPicker, setShowConnectorPicker] = React.useState(false);
  const [connectorSetup, setConnectorSetup] = React.useState<WorkflowConnectorSetupContext>();
  const localizedSetupConnector = connectorSetup
    ? localizeConnectorCatalogItem(connectorSetup.connector, locale)
    : undefined;

  React.useEffect(() => {
    setShowConnectorPicker(false);
    setConnectorSetup(undefined);
  }, [nodeId, workflowId, readOnly]);

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
  const connectorID = providerDefinition?.id ?? nodeData?.provider_id ?? '';
  const personalConnectionsQuery = usePersonalConnections(
    {
      connector_id: connectorID || undefined,
      target_type: 'workflow',
      target_id: workflowId || undefined,
    },
    Boolean(isConnector && connectorID && workflowId)
  );
  const personalConnections = personalConnectionItems(personalConnectionsQuery.data?.data);
  const currentConnectionID = nodeData?.connector?.connection_id?.trim() ?? '';
  const selectedPersonalConnection = personalConnections.find(
    connection => connection.id === currentConnectionID
  );
  const { pickerBindings, selectedConnectionLabel } = useMemo(
    () =>
      workflowConnectorSelection({
        locale,
        workflowID: workflowId,
        nodeID: nodeId,
        connectorID,
        currentConnectionID,
        personalConnection: selectedPersonalConnection,
        bindingID: nodeData?.connector?.binding_id,
        revision: nodeData?.connector?.revision,
        icon: providerDefinition?.icon,
        unavailableLabel: connectorT('status.unavailable'),
      }),
    [
      connectorID,
      connectorT,
      currentConnectionID,
      nodeId,
      providerDefinition?.icon,
      nodeData?.connector?.binding_id,
      nodeData?.connector?.revision,
      selectedPersonalConnection,
      workflowId,
      locale,
    ]
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

  const referenceCommands = useWorkflowStore(
    state => state.workflowData?.connector_reference_commands
  );
  const stageReferenceCommand = (command: ConnectorReferenceCommand) => {
    const state = useWorkflowStore.getState();
    if (!state.workflowData || readOnly) return;
    useWorkflowStore.setState({
      workflowData: {
        ...state.workflowData,
        connector_reference_commands: [
          ...(state.workflowData.connector_reference_commands ?? []).filter(
            item => item.reference_id !== command.reference_id
          ),
          command,
        ],
      },
      isDirty: true,
    });
  };
  const setConnectorConnection = (connectionID: string) => {
    const operationID = toolDefinition?.connector_operation_id;
    if (!operationID || !connectorID || readOnly) {
      throw new Error('connector operation is unavailable');
    }
    updateNodeData({
      connector: {
        ...nodeData?.connector,
        connection_id: connectionID,
        connector_id: connectorID,
        operation_id: operationID,
      },
    });
  };

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
            {isConnector ? (
              <ConnectorIcon
                connectorId={connectorID}
                name={connectorID}
                icon={providerIcon}
                classification="connector"
                className="size-8 rounded-full border-0 shadow-none"
              />
            ) : providerIcon ? (
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
            {isConnector ? connectorT('picker.toolConfigAction') : navigationT('settings')}
          </Button>
        )}
      </div>

      {isConnector ? (
        <div className="space-y-2 rounded-lg border bg-muted/20 p-3">
          <Label className="text-sm">
            {connectorT('picker.workflowConnectionLabel')}
            <span className="ml-1 text-red-500">*</span>
          </Label>
          <Button
            type="button"
            variant="outline"
            className="w-full justify-between bg-background font-normal"
            disabled={readOnly || !connectorID || !workflowId}
            onClick={() => setShowConnectorPicker(true)}
          >
            <span
              className={cn(
                'min-w-0 truncate text-left',
                !selectedConnectionLabel && 'text-muted-foreground'
              )}
            >
              {selectedConnectionLabel || connectorT('picker.workflowConnectionPlaceholder')}
            </span>
            <ChevronsUpDown aria-hidden="true" className="size-4 shrink-0 opacity-50" />
          </Button>
          <p className="text-xs text-muted-foreground">
            {connectorT('picker.workflowConnectionHint')}
          </p>
        </div>
      ) : null}

      {isConnector && connectorID && workflowId ? (
        connectorSetup && localizedSetupConnector ? (
          <Dialog
            open={showConnectorPicker}
            onOpenChange={nextOpen => {
              setShowConnectorPicker(nextOpen);
              if (!nextOpen) setConnectorSetup(undefined);
            }}
          >
            <DialogContent size="md" className="overflow-hidden p-0">
              <ConnectorCreateContent
                onBack={() => setConnectorSetup(undefined)}
                description={connectorT('picker.subtitle', {
                  target: nodeData?.title || connectorT('picker.targetTypes.workflow'),
                })}
                connector={localizedSetupConnector}
                submitLabel={connectorT(
                  connectorSetup.replacedBinding
                    ? 'picker.actions.addConnection'
                    : 'picker.actions.connectAndAdd'
                )}
                onReady={connection => {
                  if (connectorSetup.replacedBinding || currentConnectionID) {
                    setConnectorSetup(undefined);
                    toast.success(connectorT('picker.connectionReadyNotAdded'));
                    return;
                  }
                  setConnectorConnection(connection.id);
                  setConnectorSetup(undefined);
                  setShowConnectorPicker(false);
                }}
              />
            </DialogContent>
          </Dialog>
        ) : (
          <ConnectorPickerDialog
            open={showConnectorPicker}
            target={{ type: 'workflow', id: workflowId, binding_instance_id: nodeId }}
            targetName={nodeData?.title || connectorT('picker.targetTypes.workflow')}
            connectorId={connectorID}
            bindings={pickerBindings}
            onReferenceCommand={stageReferenceCommand}
            pendingReferenceCommands={referenceCommands}
            onOpenChange={setShowConnectorPicker}
            onRequestAdd={request => {
              if (request.connector.id !== connectorID) return;
              if (currentConnectionID) {
                throw new Error('connector switch confirmation is required');
              }
              setConnectorConnection(request.connection.id);
              setShowConnectorPicker(false);
            }}
            onRequestSwitch={request => {
              if (request.connector.id !== connectorID) return;
              setConnectorConnection(request.connection.id);
              setShowConnectorPicker(false);
            }}
            onRequestConnectAndAdd={request => {
              if (request.connector.id !== connectorID) return;
              setConnectorSetup({
                connector: request.connector,
                replacedBinding: request.replacedBinding,
              });
            }}
            onRequestUpdateKey={request => {
              setShowConnectorPicker(false);
              router.push(connectorDetailURL(request));
            }}
            onRequestViewReason={request => {
              setShowConnectorPicker(false);
              router.push(connectorDetailURL(request));
            }}
            onRequestDetach={request => {
              if (request.binding.connection_id !== currentConnectionID) return;
              setConnectorConnection('');
              setShowConnectorPicker(false);
            }}
          />
        )
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
          title={isConnector ? connectorT('picker.toolConfigAction') : undefined}
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

'use client';

import React, { useCallback, useMemo } from 'react';
import type {
  JsonParserNodeData,
  JsonParserOutputSchema,
  JsonParserOutputType,
  JsonParserErrorStrategy,
} from '../config';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { Plus, Trash2, Pencil } from 'lucide-react';
import NodeValueSelector from '@/components/workflow/common/node-value-selector';
import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import type { WorkflowVariable } from '../../../store/type';
import { Badge } from '@/components/ui/badge';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';
import OutputSchemaEditorDialog from './output-schema-editor';
import DefaultValueEditor from './default-value-editor';
import type { JsonParserDefaultValueItem } from '../config';

interface JsonParserManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const OUTPUT_TYPES: JsonParserOutputType[] = [
  'string',
  'number',
  'boolean',
  'object',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
];

const ERROR_STRATEGIES: JsonParserErrorStrategy[] = ['none', 'fail-branch', 'default-value'];

const JsonParserManager: React.FC<JsonParserManagerProps> = ({
  id,
  className,
  readOnly = false,
}) => {
  const t = useT('nodes');

  const nodeData = useNodeData<JsonParserNodeData>(id);
  const updateNodeData = useNodeDataUpdate<JsonParserNodeData>(id);

  const safe = useMemo<JsonParserNodeData>(
    () => ({
      type: 'json-parser',
      title: nodeData?.title ?? 'JSON Parser',
      desc: nodeData?.desc ?? '',
      input_selector: (nodeData?.input_selector ?? []) as string[],
      is_flatten_output: nodeData?.is_flatten_output ?? false,
      outputs: nodeData?.outputs ?? { result: { type: 'object', children: {} } },
      error_strategy: nodeData?.error_strategy ?? 'none',
      default_value: nodeData?.default_value ?? [],
      retry_config: nodeData?.retry_config ?? { enable: false, max_times: 3, interval: 1000 },
      isInLoop: nodeData?.isInLoop ?? false,
      isInIteration: nodeData?.isInIteration ?? false,
    }),
    [nodeData]
  );

  // Output schema editor dialog state
  const [schemaDialogOpen, setSchemaDialogOpen] = React.useState(false);
  const [editingKey, setEditingKey] = React.useState<string | null>(null);
  const [localSchema, setLocalSchema] = React.useState<{
    key: string;
    schema: JsonParserOutputSchema;
  }>({ key: '', schema: { type: 'string' } });

  // Handle input selector change
  const handleInputChange = useCallback(
    (payload: { valuePath: string[] }) => {
      updateNodeData({ input_selector: payload.valuePath });
    },
    [updateNodeData]
  );

  // Handle flatten mode toggle
  const handleFlattenToggle = useCallback(
    (checked: boolean) => {
      // If switching to wrap mode (checked=false), ensure only one output exists
      // If multiple outputs exist, keep only the first one or create a default 'result'
      let newOutputs = safe.outputs;
      if (!checked) {
        const keys = Object.keys(safe.outputs);
        if (keys.length > 1) {
          const firstKey = keys[0];
          newOutputs = { [firstKey]: safe.outputs[firstKey] };
        } else if (keys.length === 0) {
          newOutputs = { result: { type: 'object', children: {} } };
        }
      }

      updateNodeData({ is_flatten_output: checked, outputs: newOutputs });
    },
    [safe.outputs, updateNodeData]
  );

  // Handle error strategy change
  const handleErrorStrategyChange = useCallback(
    (value: string) => {
      const strategy = value as JsonParserErrorStrategy;
      if (strategy === 'default-value') {
        // Initialize default_value if not present
        updateNodeData({
          error_strategy: strategy,
          default_value: safe.default_value?.length ? safe.default_value : [],
        });
      } else {
        updateNodeData({ error_strategy: strategy });
      }
    },
    [updateNodeData, safe.default_value]
  );

  // Open add output dialog
  const openAddOutput = useCallback(() => {
    setEditingKey(null);
    setLocalSchema({ key: '', schema: { type: 'string' } });
    setSchemaDialogOpen(true);
  }, []);

  // Open edit output dialog
  const openEditOutput = useCallback(
    (key: string) => {
      const schema = safe.outputs[key];
      if (!schema) return;
      setEditingKey(key);
      setLocalSchema({ key, schema: JSON.parse(JSON.stringify(schema)) });
      setSchemaDialogOpen(true);
    },
    [safe.outputs]
  );

  // Helper to generate default value based on schema
  const generateDefaultValue = useCallback((schema: JsonParserOutputSchema): string => {
    switch (schema.type) {
      case 'string':
        return '';
      case 'number':
        return '0';
      case 'boolean':
        return 'false';
      case 'object': {
        if (schema.children && Object.keys(schema.children).length > 0) {
          const obj: Record<
            string,
            string | number | boolean | object | Array<string | number | boolean | object> | null
          > = {};
          Object.entries(schema.children).forEach(([key, childSchema]) => {
            const childVal = generateDefaultValue(childSchema);
            try {
              // Try to parse child value if it's a JSON string (object/array)
              if (
                childSchema.type === 'object' ||
                childSchema.type.startsWith('array') ||
                childSchema.type === 'boolean' ||
                childSchema.type === 'number'
              ) {
                // Handle empty string for non-string types gracefully
                if (childVal === '' && childSchema.type !== 'string') {
                  obj[key] = null;
                } else {
                  obj[key] = JSON.parse(childVal);
                }
              } else {
                // String type, use directly
                obj[key] = childVal;
              }
            } catch {
              // Fallback for parsing errors or simple strings
              obj[key] = childVal;
            }
          });
          return JSON.stringify(obj, null, 2);
        }
        return '{}';
      }
      case 'array[string]':
      case 'array[number]':
      case 'array[boolean]':
      case 'array[object]':
        return '[]';
      default:
        return '';
    }
  }, []);

  // Save output schema
  const saveOutput = useCallback(
    (key: string, schema: JsonParserOutputSchema) => {
      const newOutputs = { ...safe.outputs };
      let newDefaultValues = [...(safe.default_value || [])];

      // If editing and key changed, remove old key
      if (editingKey && editingKey !== key) {
        delete newOutputs[editingKey];
        // Remove old default value
        newDefaultValues = newDefaultValues.filter(item => item.key !== editingKey);
      }

      newOutputs[key] = schema;

      // Generate a new default value for this key based on the new schema
      // This ensures we always have a default value for new/updated outputs
      const newDefaultVal = generateDefaultValue(schema);

      // Update default value type if key exists, or create new one if not exists
      const dvIndex = newDefaultValues.findIndex(item => item.key === key);
      if (dvIndex > -1) {
        // Update existing default value entry
        // We always update the value to match the new schema structure/type
        newDefaultValues[dvIndex] = {
          ...newDefaultValues[dvIndex],
          type: schema.type,
          value: newDefaultVal,
        };
      } else {
        // Create new default value entry
        newDefaultValues.push({
          key,
          type: schema.type,
          value: newDefaultVal,
        });
      }

      updateNodeData({ outputs: newOutputs, default_value: newDefaultValues });
      setSchemaDialogOpen(false);
      setEditingKey(null);
    },
    [editingKey, safe.outputs, safe.default_value, updateNodeData, generateDefaultValue]
  );

  // Remove output
  const removeOutput = useCallback(
    (key: string) => {
      const newOutputs = { ...safe.outputs };
      delete newOutputs[key];

      // Remove default value
      const newDefaultValues = (safe.default_value || []).filter(item => item.key !== key);

      updateNodeData({ outputs: newOutputs, default_value: newDefaultValues });
    },
    [safe.outputs, safe.default_value, updateNodeData]
  );

  // Handle default value change
  const handleDefaultValuesChange = useCallback(
    (newValues: JsonParserDefaultValueItem[]) => {
      updateNodeData({ default_value: newValues });
    },
    [updateNodeData]
  );

  // Handle auto-generate default values for all outputs
  const handleAutoGenerate = useCallback(() => {
    const newDefaultValues: JsonParserDefaultValueItem[] = [];
    Object.entries(safe.outputs).forEach(([key, schema]) => {
      newDefaultValues.push({
        key,
        type: schema.type,
        value: generateDefaultValue(schema),
      });
    });
    updateNodeData({ default_value: newDefaultValues });
  }, [safe.outputs, updateNodeData, generateDefaultValue]);

  // Get output variables for downstream display
  const outputs = useNodeOutputVariables(id);

  const outputKeys = Object.keys(safe.outputs);

  return (
    <div className={cn('space-y-4', className)}>
      {/* Input Section */}
      <section className="space-y-3">
        <h3 className="text-base font-semibold">{t('jsonParser.section.input')}</h3>
        <NodeValueSelector
          nodeId={id}
          value={safe.input_selector.length >= 2 ? safe.input_selector : undefined}
          onChange={handleInputChange}
          disabled={readOnly}
          placeholder={t('jsonParser.placeholders.selectInput')}
        />
      </section>

      {/* Output Mode Section */}
      <section className="space-y-3">
        <h3 className="text-base font-semibold">{t('jsonParser.section.outputMode')}</h3>
        <div className="flex items-center gap-3">
          <Switch
            id="flatten-mode"
            checked={safe.is_flatten_output}
            onCheckedChange={handleFlattenToggle}
            disabled={readOnly}
          />
          <Label htmlFor="flatten-mode" className="text-sm">
            {t('jsonParser.labels.flattenMode')}
          </Label>
        </div>
        <p className="text-xs text-muted-foreground">
          {safe.is_flatten_output
            ? t('jsonParser.tips.flattenModeEnabled')
            : t('jsonParser.tips.wrapModeEnabled')}
        </p>
      </section>

      {/* Output Schema Section */}
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold">{t('jsonParser.section.outputs')}</h3>
          {!(!safe.is_flatten_output && outputKeys.length >= 1) && (
            <Button
              variant="ghost"
              isIcon
              onClick={openAddOutput}
              disabled={readOnly}
              className="w-8 h-8"
            >
              <Plus className="h-4 w-4" />
            </Button>
          )}
        </div>

        {outputKeys.length === 0 ? (
          <div className="text-xs text-muted-foreground">{t('jsonParser.empty.noOutputs')}</div>
        ) : (
          <div className="space-y-2">
            {outputKeys.map(key => {
              const schema = safe.outputs[key];
              if (!schema) return null;
              return (
                <div
                  key={key}
                  className="border rounded-md p-2 group relative bg-muted overflow-hidden"
                >
                  <div className="flex-1 space-y-1">
                    <div className="font-medium leading-none flex justify-between items-center">
                      <div className="grow flex items-center gap-2">
                        <span className="truncate font-mono text-sm">{key}</span>
                        <Badge className="py-0 px-1">{schema.type}</Badge>
                      </div>
                    </div>
                    {schema.children && Object.keys(schema.children).length > 0 && (
                      <div className="text-xs text-muted-foreground">
                        {t('jsonParser.labels.nestedFields', {
                          count: Object.keys(schema.children).length,
                        })}
                      </div>
                    )}
                  </div>
                  <div className="hidden h-full px-1 group-hover:flex items-center gap-1 absolute right-0 top-1/2 -translate-y-1/2 bg-gradient-to-r from-transparent to-background">
                    <Button
                      variant="ghost"
                      isIcon
                      onClick={() => openEditOutput(key)}
                      disabled={readOnly}
                      aria-label={t('jsonParser.actions.edit')}
                      className="w-7 h-7"
                    >
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      isIcon
                      onClick={() => removeOutput(key)}
                      disabled={readOnly}
                      aria-label={t('jsonParser.actions.remove')}
                      className="w-7 h-7 text-destructive hover:bg-red-100 hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      {/* Error Handling Section */}
      <section className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold">{t('jsonParser.section.errorHandling')}</h3>
          <Select
            value={safe.error_strategy}
            onValueChange={handleErrorStrategyChange}
            disabled={readOnly}
          >
            <SelectTrigger className="w-40 h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ERROR_STRATEGIES.map(strategy => (
                <SelectItem key={strategy} value={strategy}>
                  {t(`jsonParser.errorStrategy.${strategy}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Fail Branch indicator */}
        {safe.error_strategy === 'fail-branch' && (
          <div className="border rounded-lg p-3 bg-destructive/10 text-sm text-destructive">
            {t('jsonParser.tips.failBranchHint')}
          </div>
        )}

        {/* Default Value Editor */}
        {safe.error_strategy === 'default-value' && (
          <DefaultValueEditor
            outputs={safe.outputs}
            defaultValues={safe.default_value || []}
            onChange={handleDefaultValuesChange}
            onAutoGenerate={handleAutoGenerate}
            readOnly={readOnly}
          />
        )}
      </section>

      {/* Output Variables View */}
      <OutputVariablesView variables={outputs} />

      {/* Output Schema Editor Dialog */}
      <OutputSchemaEditorDialog
        open={schemaDialogOpen}
        onOpenChange={setSchemaDialogOpen}
        editing={editingKey !== null}
        initialKey={localSchema.key}
        initialSchema={localSchema.schema}
        existingKeys={outputKeys.filter(k => k !== editingKey)}
        typeOptions={OUTPUT_TYPES}
        onSubmit={saveOutput}
      />
    </div>
  );
};

export default JsonParserManager;

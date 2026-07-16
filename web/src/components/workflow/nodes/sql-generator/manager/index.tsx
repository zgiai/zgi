'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useT } from '@/i18n';
import { Label } from '@/components/ui/label';
import { Database, PlusCircle } from 'lucide-react';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import WorkflowValueEditor from '@/components/workflow/common/workflow-value-editor';
import type { WorkflowValueEditorHandle } from '@/components/workflow/common/workflow-value-editor';
import type {
  SqlGeneratorInnerData,
  SqlGeneratorDataSource,
  SqlGeneratorSourceRef,
  SqlGeneratorTableRef,
} from '../config';
import {
  DEFAULT_SQL_GENERATOR_MODEL,
  DEFAULT_SQL_GENERATOR_PROMPT,
  DEFAULT_SQL_GENERATOR_DATA_SOURCE,
} from '../config';
import type {
  DatabaseExecutionSettings,
  DatabaseSourceRef,
  TableRef,
} from '../../call-database/config';
import { DEFAULT_DATABASE_EXECUTION } from '../../call-database/config';
import PickerDialog from '../../../common/datasource-picker-dialog';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useDbTables } from '@/hooks/db/use-db-tables';
import OutputVariablesView from '../../../common/output-variables-view';
import type { WorkflowVariable } from '../../../store/type';
import {
  useDatabaseNodePermissions,
  useLocalNodeData,
  useNodeOutputVariables,
} from '../../../hooks';

interface SqlGeneratorManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const SqlGeneratorManager: React.FC<SqlGeneratorManagerProps> = ({
  id: nodeId,
  className,
  readOnly = false,
}) => {
  const t = useT();
  const editorRef = useRef<WorkflowValueEditorHandle | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [cachedPickerValue, setCachedPickerValue] = useState<{
    dataSource: DatabaseSourceRef | null;
    tables: TableRef[];
  } | null>(null);
  const [pendingSelection, setPendingSelection] = useState<{
    ds: DatabaseSourceRef;
    tables: TableRef[];
  } | null>(null);
  const { canReadDatabaseBinding } = useDatabaseNodePermissions();
  const canEditDatabaseSource = !readOnly && canReadDatabaseBinding;

  // Use store-aware useLocalNodeData for the 'data' field with debouncing
  const { localData: innerDataRaw, setLocalData: setInnerData } =
    useLocalNodeData<SqlGeneratorInnerData>(nodeId, {
      path: 'data',
      delay: 500,
    });

  const innerData = useMemo<SqlGeneratorInnerData>(() => {
    const inner = innerDataRaw;
    const mergedModel = inner?.model
      ? { ...DEFAULT_SQL_GENERATOR_MODEL, ...inner.model }
      : { ...DEFAULT_SQL_GENERATOR_MODEL };
    const data_source: SqlGeneratorDataSource = inner?.data_source
      ? {
          source: {
            id: inner.data_source.source?.id ?? '',
            name: inner.data_source.source?.name ?? '',
            schema: inner.data_source.source?.schema ?? 'public',
            type: inner.data_source.source?.type ?? 'postgres',
          },
          tables: Array.isArray(inner.data_source.tables) ? inner.data_source.tables : [],
        }
      : { ...DEFAULT_SQL_GENERATOR_DATA_SOURCE };
    const execution: DatabaseExecutionSettings = {
      timeout_seconds:
        inner?.execution?.timeout_seconds ?? DEFAULT_DATABASE_EXECUTION.timeout_seconds,
      max_retries: inner?.execution?.max_retries ?? DEFAULT_DATABASE_EXECUTION.max_retries,
    };
    return {
      model: mergedModel,
      data_source,
      prompt: inner?.prompt ?? DEFAULT_SQL_GENERATOR_PROMPT,
      execution,
    };
  }, [innerDataRaw]);

  const updateInnerData = useCallback(
    (patch: Partial<SqlGeneratorInnerData>) => {
      setInnerData(prev => ({
        ...prev,
        ...patch,
      }));
    },
    [setInnerData]
  );

  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: innerData.model ?? {},
    enabled: !readOnly,
    onInitialize: v => {
      updateInnerData({
        model: {
          provider: v.provider,
          name: v.model,
          mode: innerData.model.mode,
          completion_params: v.params as Record<string, string | number | boolean>,
        },
      });
    },
  });

  // Use fine-grained selector to only get current node instead of entire nodes array
  // This prevents re-render when other nodes change (e.g., during hover)
  const outputs = useNodeOutputVariables(nodeId);

  const dataSource = innerData.data_source;
  const defaultSchema =
    dataSource.source.schema?.trim().length > 0 ? dataSource.source.schema : 'public';

  // Stabilize PickerDialog value to prevent clearing on incidental re-renders
  const pickerValue = useMemo<{ dataSource: DatabaseSourceRef | null; tables: TableRef[] }>(() => {
    const s = dataSource?.source;
    const ds: DatabaseSourceRef | null =
      s && s.id ? { id: s.id, name: s.name, type: s.type, schema_name: s.schema } : null;
    const tables: TableRef[] = Array.isArray(dataSource?.tables)
      ? dataSource.tables.map(t => ({
          schema: t.schema,
          name: t.name,
          label: t.label,
          columns: t.columns,
          id: t.id,
          table_id: typeof t.table_id === 'number' ? t.table_id : 0,
        }))
      : [];
    return { dataSource: ds, tables };
  }, [dataSource.source, dataSource.tables]);

  // Fetch current DB tables to enrich numeric table_id on confirm
  const currentDbId =
    pendingSelection?.ds.id ||
    (pickerOpen && cachedPickerValue?.dataSource?.id) ||
    dataSource.source.id;
  const { tables: dbTables } = useDbTables(currentDbId || '', {
    enabled: Boolean(canReadDatabaseBinding && (pendingSelection || pickerOpen) && currentDbId),
    refetchOnWindowFocus: false,
  });

  // Enrich numeric table_id asynchronously after confirm if needed
  useEffect(() => {
    if (!pendingSelection) return;
    const pid = pendingSelection.ds.id;
    if (!pid || pid !== currentDbId) return;
    if (!Array.isArray(dbTables)) return;

    const idToNumeric = new Map<string, number>();
    dbTables.forEach(tb => {
      if (tb?.id && typeof tb.table_id === 'number') {
        idToNumeric.set(tb.id, tb.table_id);
      }
    });
    const enriched = pendingSelection.tables.map<SqlGeneratorTableRef>(t => ({
      id: t.id || '',
      table_id: t.id && idToNumeric.has(t.id) ? (idToNumeric.get(t.id) as number) : undefined,
      schema: t.schema,
      name: t.name,
      label: t.label,
      columns: t.columns,
    }));
    const nextSource: SqlGeneratorSourceRef = {
      id: pendingSelection.ds.id,
      name: pendingSelection.ds.name,
      type: pendingSelection.ds.type,
      schema: pendingSelection.ds.schema_name || 'public',
    };
    updateInnerData({ data_source: { source: nextSource, tables: enriched } });
    setPendingSelection(null);
  }, [pendingSelection, dbTables, currentDbId, updateInnerData]);

  const handleVariableInsert = useCallback(
    (value: { sourceId: string; key: string; type: WorkflowVariable['type'] }) => {
      editorRef.current?.insertToken(value.sourceId, value.key || '');
    },
    []
  );

  return (
    <div className={className}>
      <div className="space-y-6">
        {/* Model Section */}
        <section className="space-y-3">
          <h3 className="text-lg font-semibold">{t('nodes.sqlGenerator.section.model')}</h3>
          <div className="flex gap-2 items-center">
            <div className="grow">
              <ModelSelectorParameter
                modelType="text-chat"
                value={{
                  provider: innerData.model.provider,
                  model: innerData.model.name,
                  params:
                    (innerData.model.completion_params as Record<
                      string,
                      string | number | boolean
                    >) || {},
                }}
                onChange={v => {
                  updateInnerData({
                    model: {
                      provider: v.provider,
                      name: v.model,
                      mode: innerData.model.mode,
                      completion_params: v.params as Record<string, string | number | boolean>,
                    },
                  });
                }}
                disabled={readOnly}
              />
            </div>
          </div>
        </section>

        {/* Data Source Section */}
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">{t('nodes.sqlGenerator.section.dataSource')}</h3>
          </div>
          <div
            className="rounded-md border p-2 text-sm space-y-2 cursor-pointer hover:bg-muted/60"
            onClick={() => {
              if (!canEditDatabaseSource) return;
              setCachedPickerValue(pickerValue);
              setPickerOpen(true);
            }}
          >
            {!dataSource?.source?.id && dataSource.tables.length === 0 ? (
              <div className="flex items-center gap-3 rounded-md border border-dashed bg-muted/30 p-3">
                <Database className="h-5 w-5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <div className="font-medium truncate">
                    {t('nodes.sqlGenerator.empty.selectorHintTitle')}
                  </div>
                  <div className="text-xs text-muted-foreground truncate">
                    {t('nodes.sqlGenerator.empty.selectorHintDesc')}
                  </div>
                </div>
                <PlusCircle className="h-5 w-5 text-muted-foreground" />
              </div>
            ) : (
              <>
                <div className="flex items-center">
                  <Label className="block">{t('nodes.sqlGenerator.fields.dataSource')}:</Label>
                  <div className="text-muted-foreground flex-1 overflow-hidden truncate">
                    {dataSource?.source?.name ||
                      dataSource?.source?.id ||
                      t('nodes.sqlGenerator.empty.selectorHintTitle')}
                  </div>
                </div>
                <div>
                  <Label className="mb-1 block">{t('nodes.sqlGenerator.fields.tables')}</Label>
                  {dataSource.tables.length === 0 ? (
                    <div className="text-muted-foreground">
                      {t('nodes.sqlGenerator.empty.selectorHintTitle')}
                    </div>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {dataSource.tables.map(item => (
                        <span
                          key={`${item.schema}.${item.name}`}
                          className="rounded bg-muted px-2 py-1 text-xs"
                        >
                          {item.label || item.name}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </>
            )}
          </div>
          <PickerDialog
            open={pickerOpen}
            onOpenChange={setPickerOpen}
            value={pickerOpen && cachedPickerValue ? cachedPickerValue : pickerValue}
            onConfirm={({ dataSource: ds, tables }) => {
              // Immediate update without numeric table_id
              const nextSource: SqlGeneratorSourceRef = {
                id: ds.id,
                name: ds.name,
                type: ds.type,
                schema: ds.schema_name || 'public',
              };
              const idToNumeric = new Map<string, number>();
              if (Array.isArray(dbTables)) {
                dbTables.forEach(tb => {
                  if (tb?.id && typeof tb.table_id === 'number') {
                    idToNumeric.set(tb.id, tb.table_id);
                  }
                });
              }
              const nextTablesShallow: SqlGeneratorTableRef[] = tables.map(t => ({
                id: t.id || '',
                table_id:
                  t.id && idToNumeric.has(t.id) ? (idToNumeric.get(t.id) as number) : undefined,
                schema: t.schema,
                name: t.name,
                label: t.label,
                columns: t.columns,
              }));
              updateInnerData({ data_source: { source: nextSource, tables: nextTablesShallow } });
              // Defer enrichment to effect (fetch numeric table_id for the selected ds)
              setPendingSelection({ ds, tables });
            }}
            initialSchema={defaultSchema}
            readOnly={!canEditDatabaseSource}
          />
        </section>

        {/* Prompt Section */}
        <section className="space-y-3">
          <h3 className="text-lg font-semibold">{t('nodes.sqlGenerator.section.prompt')}</h3>
          <WorkflowValueInserter
            nodeId={nodeId}
            className="w-full"
            onInsert={handleVariableInsert}
            disabled={readOnly}
          />
          <WorkflowValueEditor
            ref={editorRef}
            value={innerData.prompt}
            onChange={prompt => updateInnerData({ prompt })}
            placeholder={t('nodes.sqlGenerator.placeholders.prompt')}
            nodeId={nodeId}
            editorClassName="min-h-[120px]"
            readOnly={readOnly}
          />
        </section>
        <OutputVariablesView variables={outputs} />
      </div>
    </div>
  );
};

export default SqlGeneratorManager;

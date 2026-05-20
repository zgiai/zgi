'use client';

import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Label } from '@/components/ui/label';

import WorkflowValueInserter from '@/components/workflow/common/workflow-value-inserter';
import type {
  CallDatabaseNodeData,
  CallDatabaseNodeInnerData,
  DatabaseExecutionSettings,
} from '../config';
import { DEFAULT_DATABASE_EXECUTION, DEFAULT_DATABASE_SOURCE } from '../config';
import PickerDialog from '../../../common/datasource-picker-dialog';
import SqlMonacoEditor, { type SqlMonacoEditorHandle } from './sql-editor/sql-monaco-editor';
import SqlEditorInsertMenus from './sql-editor/insert-menus';
import { Button } from '@/components/ui/button';
import { Expand, Database, PlusCircle } from 'lucide-react';
import ExpandedSqlEditorDialog from './sql-editor/expanded-dialog';
import OutputVariablesView from '@/components/workflow/common/output-variables-view';
import type { WorkflowVariable } from '../../../store/type';
import { useT } from '@/i18n';
import { useLocalNodeData, useNodeOutputVariables } from '../../../hooks';

interface CallDatabaseManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const CallDatabaseManager: React.FC<CallDatabaseManagerProps> = ({
  id: nodeId,
  className,
  readOnly = false,
}) => {
  const t = useT();
  const editorRef = useRef<SqlMonacoEditorHandle | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [expandedOpen, setExpandedOpen] = useState(false);

  // Use store-aware useLocalNodeData for the 'data' field with debouncing
  const { localData: innerDataRaw, setLocalData: setInnerData } =
    useLocalNodeData<CallDatabaseNodeInnerData>(nodeId, {
      path: 'data',
      delay: 500,
    });

  const innerData = useMemo<CallDatabaseNodeInnerData>(() => {
    const inner = innerDataRaw;
    const mergedSource = inner?.data_source;
    const data_source = mergedSource
      ? { ...DEFAULT_DATABASE_SOURCE, ...mergedSource }
      : { ...DEFAULT_DATABASE_SOURCE };
    const execution: DatabaseExecutionSettings = {
      timeout_seconds:
        inner?.execution?.timeout_seconds ?? DEFAULT_DATABASE_EXECUTION.timeout_seconds,
      max_retries: inner?.execution?.max_retries ?? DEFAULT_DATABASE_EXECUTION.max_retries,
    };
    const table_selection: CallDatabaseNodeInnerData['table_selection'] = Array.isArray(
      inner?.table_selection
    )
      ? [...(inner?.table_selection ?? [])]
      : [];
    return {
      data_source,
      table_selection,
      manual_sql: inner?.manual_sql ?? '',
      execution,
    };
  }, [innerDataRaw]);

  const updateInnerData = useCallback(
    (patch: Partial<CallDatabaseNodeInnerData>) => {
      setInnerData(prev => ({
        ...prev,
        ...patch,
      }));
    },
    [setInnerData]
  );

  const outputs = useNodeOutputVariables(nodeId);

  const dataSource = innerData.data_source;
  const defaultSchema =
    dataSource.schema_name && dataSource.schema_name.trim().length > 0
      ? dataSource.schema_name
      : 'public';

  // Columns are not stored in node data; backend resolves schema at execution time

  const insertSnippet = useCallback(
    (snippet: string) => {
      if (!snippet.trim()) return;
      const editor = editorRef.current;
      if (editor) {
        editor.insertText(snippet);
      } else {
        const existing = innerData.manual_sql ?? '';
        const needsSpace = existing.length > 0 && !existing.endsWith(' ');
        const next = needsSpace ? `${existing} ${snippet}` : `${existing}${snippet}`;
        updateInnerData({ manual_sql: next });
      }
    },
    [innerData.manual_sql, updateInnerData]
  );

  // removed inline insert handlers; insertions are handled by SqlEditorInsertMenus above the editor

  const handleVariableInsert = useCallback(
    (value: { sourceId: string; key: string; type: WorkflowVariable['type'] }) => {
      const token = value.key ? `{{#${value.sourceId}.${value.key}#}}` : `{{#${value.sourceId}#}}`;
      insertSnippet(token);
    },
    [insertSnippet]
  );

  return (
    <div className={className}>
      <div className="space-y-6">
        <section className="space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">{t('nodes.callDatabase.section.dataSource')}</h3>
          </div>
          <div
            className="rounded-md border p-2 text-sm space-y-2 cursor-pointer hover:bg-muted/60"
            onClick={() => {
              if (readOnly) return;
              setPickerOpen(true);
            }}
          >
            {!dataSource?.id && innerData.table_selection.length === 0 ? (
              <div className="flex items-center gap-3 rounded-md border border-dashed bg-muted/30 p-3">
                <Database className="h-5 w-5 text-muted-foreground" />
                <div className="flex-1 min-w-0">
                  <div className="font-medium truncate">
                    {t('nodes.callDatabase.empty.selectorHintTitle')}
                  </div>
                  <div className="text-xs text-muted-foreground truncate">
                    {t('nodes.callDatabase.empty.selectorHintDesc')}
                  </div>
                </div>
                <PlusCircle className="h-5 w-5 text-muted-foreground" />
              </div>
            ) : (
              <>
                <div className="flex items-center">
                  <Label className="block">{t('nodes.callDatabase.fields.dataSource')}</Label>
                  <div className="text-muted-foreground flex-1 overflow-hidden truncate">
                    {dataSource?.name ||
                      dataSource?.id ||
                      t('nodes.callDatabase.empty.selectorHintTitle')}
                  </div>
                </div>
                <div>
                  <Label className="mb-1 block">{t('nodes.callDatabase.fields.tables')}</Label>
                  {innerData.table_selection.length === 0 ? (
                    <div className="text-muted-foreground">
                      {t('nodes.callDatabase.empty.selectorHintTitle')}
                    </div>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {innerData.table_selection.map(item => (
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
            value={{ dataSource, tables: innerData.table_selection }}
            onConfirm={({ dataSource: ds, tables }) => {
              updateInnerData({ data_source: ds, table_selection: tables });
            }}
            initialSchema={defaultSchema}
            readOnly={readOnly}
          />
        </section>

        <section className="space-y-3">
          <h3 className="text-lg font-semibold">{t('nodes.callDatabase.section.sqlEditor')}</h3>

          <WorkflowValueInserter
            nodeId={nodeId}
            className="w-full"
            onInsert={handleVariableInsert}
            disabled={readOnly}
          />
          <div className="flex justify-between items-center">
            <SqlEditorInsertMenus
              dbId={dataSource?.id}
              tables={innerData.table_selection}
              onInsert={insertSnippet}
              disabled={readOnly}
            />
            <Button
              variant="outline"
              isIcon
              className="w-8 h-8"
              onClick={() => setExpandedOpen(true)}
            >
              <Expand size={20} />
            </Button>
          </div>

          <SqlMonacoEditor
            ref={editorRef}
            value={innerData.manual_sql}
            onChange={manual_sql => updateInnerData({ manual_sql })}
            readOnly={readOnly}
          />

          <ExpandedSqlEditorDialog
            open={expandedOpen}
            onOpenChange={setExpandedOpen}
            dbId={dataSource?.id}
            tables={innerData.table_selection}
            sql={innerData.manual_sql}
            onChangeSql={manual_sql => updateInnerData({ manual_sql })}
            nodeId={nodeId}
            readOnly={readOnly}
          />
        </section>
        <OutputVariablesView variables={outputs} />
      </div>
    </div>
  );
};

export default CallDatabaseManager;

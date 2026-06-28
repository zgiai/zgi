'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useT } from '@/i18n/translations';
import Chat, { useChatApi, useChatStore } from '@/components/chat';
import type { ChatAttachment } from '@/components/chat/types';
import { useParams } from 'next/navigation';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { useDb } from '@/hooks/db/use-dbs';
import { useBuiltInWorkflows } from '@/hooks/workflow/use-built-in-workflows';
import { useRunWebAppWorkflowStream } from '@/hooks/webapp/use-run-webapp-workflow-stream';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Separator } from '@/components/ui/separator';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Button } from '@/components/ui/button';
import { ChevronDown, Loader2, AlertCircle } from 'lucide-react';
import { useDbTableColumns } from '@/hooks/db/use-db-table-columns';
import type { DbTable, DbTableColumn } from '@/services/types/db';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { MousePointerClick } from 'lucide-react';
import { unwrap } from '@/utils/webapp/run-mappers';
import ModelSelectorParameter from '@/components/common/model-selector/model-selector-parameter';
import type { ModelSelectorParameterValue } from '@/components/common/model-selector/model-selector-parameter';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import { useCurrentUser } from '@/store/auth-store';
import { getLastSelectedAiModel, saveLastSelectedAiModel } from '@/utils/ui-local';
import { useWebAppPrecheck } from '@/hooks/webapp/use-webapp-precheck';
import { getWorkflowPrecheckWarnings } from '@/utils/workflow/billing';
import { useWorkflowBillingFeedback } from '@/hooks/workflow/use-workflow-billing-feedback';
import { WorkflowPrecheckWarningBanner } from '@/components/workflow/common/workflow-precheck-warning';
import type { WorkflowPrecheckWarning } from '@/services/types/workflow';
import { generateClientId } from '@/utils/client-id';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';

export interface ModelConfig {
  provider: string;
  model: string;
  mode: string;
  completion_params: Record<string, number | string | boolean>;
}

export default function DbSearchPage() {
  const t = useT('webapp');
  const tDb = useT();
  const user = useCurrentUser();
  const { getWorkflowRunErrorText, notifyBillingError } = useWorkflowBillingFeedback('webapp');
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canAiQuery = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.aiQueryRead,
    ...DATABASE_PERMISSION_ACTIONS.aiQueryWrite,
  ]);

  // Route param for current DB
  const params = useParams<{ dbId: string }>();
  const dbId = params?.dbId;

  // Fetch built-in workflows to get bi_chat web_app_id and agent_id
  const { biChatWorkflow, isLoading: isWorkflowLoading } = useBuiltInWorkflows({
    enabled: canAiQuery,
  });
  const webAppId = biChatWorkflow?.web_app_id;
  const biChatAgentId = biChatWorkflow?.agent_id;
  const precheckMutation = useWebAppPrecheck(webAppId ?? '');

  // Render initialization failure state if workflow config is missing after loading
  const isConfigMissing = !isWorkflowLoading && (!webAppId || !biChatAgentId);

  // Tables under current DB
  const { tables, isLoading } = useDbTables(dbId, { enabled: canAiQuery });

  // Conversation state
  const [convId] = useState<string>(() => generateClientId('conversation'));
  const [precheckWarnings, setPrecheckWarnings] = useState<WorkflowPrecheckWarning[]>([]);
  const { onAgentRun } = useChatApi();
  const updateConversation = useChatStore.use.updateConversation();
  const chatConv = useChatStore.use.getConversation()(convId);

  // Refs for SSE handling
  const runnerRef = useRef<ReturnType<typeof onAgentRun> | null>(null);
  const lastMessageIdRef = useRef<string | undefined>(undefined);

  // WebApp workflow stream for bi_chat
  const {
    start: startWebAppStream,
    cancel: cancelStream,
    stop: stopWebAppStream,
    isRunning: isWebAppRunning,
    isStopping,
  } = useRunWebAppWorkflowStream(webAppId ?? '', {
    enabled: Boolean(webAppId),
    agentId: biChatAgentId,
  });

  // Handle stop workflow
  const handleStop = useCallback(() => {
    void stopWebAppStream();
  }, [stopWebAppStream]);

  // Model selector state - initialize from saved preference
  const [modelSelectorValue, setModelSelectorValue] = useState<ModelSelectorParameterValue>(() => {
    if (!user?.id) return { provider: '', model: '', params: {} };
    const saved = getLastSelectedAiModel(user.id, 'biSearch');
    return saved
      ? { provider: saved.provider, model: saved.model, params: {} }
      : { provider: '', model: '', params: {} };
  });

  // Apply default model when loaded (only if no saved preference)
  useInitializeDefaultModelByUseCase({
    useCase: 'text-chat',
    currentModel: modelSelectorValue,
    enabled: Boolean(user?.id && !getLastSelectedAiModel(user.id, 'biSearch')),
    onInitialize: v => {
      setModelSelectorValue({
        provider: v.provider,
        model: v.model,
        params: v.params,
      });
    },
  });

  const handleModelChange = useCallback(
    (value: ModelSelectorParameterValue) => {
      setModelSelectorValue(value);
      // Persist selection for this user
      if (user?.id) {
        saveLastSelectedAiModel(user.id, 'biSearch', {
          provider: value.provider,
          model: value.model,
        });
      }
    },
    [user?.id]
  );

  // Check if model is selected
  const isModelSelected = useMemo(
    () => Boolean(modelSelectorValue.provider && modelSelectorValue.model),
    [modelSelectorValue.provider, modelSelectorValue.model]
  );

  // Convert model selector value to model_config format
  const toModelConfig = useCallback((value: ModelSelectorParameterValue): ModelConfig => {
    return {
      provider: value.provider,
      model: value.model,
      mode: 'chat',
      completion_params: value.params,
    };
  }, []);

  // Sidebar state: table selection and filter
  const [search, setSearch] = useState<string>('');
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set<string>());
  const filteredTables = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return tables;
    return tables.filter(t => {
      const a = (t.name ?? '').toLowerCase();
      const b = (t.table_name ?? '').toLowerCase();
      const c = (t.schema_name ?? '').toLowerCase();
      return a.includes(q) || b.includes(q) || c.includes(q);
    });
  }, [tables, search]);
  const numSelectedInFiltered = useMemo(() => {
    if (filteredTables.length === 0) return 0;
    let n = 0;
    for (const t of filteredTables) if (selectedIds.has(t.id)) n++;
    return n;
  }, [filteredTables, selectedIds]);
  const selectAllState: boolean | 'indeterminate' = useMemo(() => {
    if (filteredTables.length === 0) return false;
    if (numSelectedInFiltered === 0) return false;
    if (numSelectedInFiltered === filteredTables.length) return true;
    return 'indeterminate';
  }, [filteredTables.length, numSelectedInFiltered]);
  const handleSelectAll = useCallback(
    (checked: boolean | 'indeterminate') => {
      setSelectedIds(prev => {
        const next = new Set(prev);
        if (checked === true) {
          filteredTables.forEach(t => next.add(t.id));
        } else {
          filteredTables.forEach(t => next.delete(t.id));
        }
        return next;
      });
    },
    [filteredTables]
  );
  const toggleSelect = useCallback((id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  // Current DB detail -> dynamic data source metadata
  const { data: dbDetail, isLoading: isDbLoading } = useDb(dbId, { enabled: canAiQuery });

  const dataSourceSource = useMemo(() => {
    const db = dbDetail?.data;
    if (!db) return undefined;
    return {
      id: db.id,
      name: db.name,
      schema: db.schema_name ?? 'public',
      type: db.provider ?? 'postgres',
    } as const;
  }, [dbDetail]);
  interface TablePayload {
    id: string;
    label: string;
    name: string;
    schema: string;
    table_id: number;
  }
  const selectedTablesPayload = useMemo<TablePayload[]>(() => {
    return tables
      .filter(t => selectedIds.has(t.id))
      .map(t => ({
        id: t.id,
        label: t.name,
        name: t.table_name,
        schema: t.schema_name ?? 'public',
        table_id: t.table_id,
      }));
  }, [tables, selectedIds]);

  interface TableCardProps {
    dbId?: string;
    table: DbTable;
    selected: boolean;
    onToggle: () => void;
  }

  const TableCard: React.FC<TableCardProps> = ({ dbId, table, selected, onToggle }) => {
    const [expanded, setExpanded] = useState(false);
    const { columns, isLoading: colsLoading } = useDbTableColumns(dbId ?? '', table.id, {
      enabled: Boolean(dbId) && expanded,
      includeSystemFields: true,
      staleTime: 60 * 1000,
      gcTime: 10 * 60 * 1000,
    });
    const updatedAt = useMemo(() => {
      const ts = table.updated_at || table.created_at;
      const d = ts ? new Date(ts) : undefined;
      return d ? d.toLocaleString() : '';
    }, [table.updated_at, table.created_at]);
    const colCount = columns.length;
    const formatType = (colType: DbTableColumn['type']): string => {
      switch (colType) {
        case 'integer':
          return tDb('dbs.biSearch.columnTypes.integer');
        case 'numeric':
          return tDb('dbs.biSearch.columnTypes.numeric');
        case 'boolean':
          return tDb('dbs.biSearch.columnTypes.boolean');
        case 'timestamp':
          return tDb('dbs.biSearch.columnTypes.timestamp');
        case 'text':
        default:
          return tDb('dbs.biSearch.columnTypes.text');
      }
    };
    return (
      <Card
        className={`cursor-pointer transition-colors rounded-xl shadow-sm overflow-hidden ${selected ? 'bg-highlight/10' : 'hover:bg-highlight/5'}`}
        onClick={onToggle}
        data-selected={selected}
      >
        <CardHeader className="p-2.5">
          <div className="flex gap-2 items-center">
            <Checkbox checked={selected} />
            <div className="min-w-0 flex-1">
              <CardTitle className="text-xs font-medium truncate">
                {table.name || table.table_name}
              </CardTitle>
              <CardDescription className="text-[11px] truncate">
                {table.description || ' '}
              </CardDescription>
              <div className="text-[10px] text-muted-foreground mt-0.5">
                {colCount > 0 ? `${colCount} ${tDb('dbs.biSearch.columns')} · ` : ''}
                {updatedAt}
              </div>
            </div>
            <div onClick={e => e.stopPropagation()}>
              <Button
                type="button"
                variant="ghost"
                isIcon
                className="shrink-0 w-6 h-6 hover:bg-highlight/20"
                onClick={e => {
                  e.stopPropagation();
                  setExpanded(v => !v);
                }}
                aria-label={expanded ? tDb('dbs.biSearch.collapse') : tDb('dbs.biSearch.expand')}
              >
                <ChevronDown
                  size={16}
                  className={`transition-transform text-secondary-foreground ${expanded ? '' : '-rotate-90'}`}
                />
              </Button>
            </div>
          </div>
        </CardHeader>
        {expanded && (
          <div className="px-2.5 pb-2.5 pt-1 border-t bg-muted">
            <Separator className="mb-1.5" />
            <div className="text-[11px] text-muted-foreground mb-1.5">
              {tDb('dbs.biSearch.tableFields')}{' '}
              {colCount > 0 ? tDb('dbs.biSearch.totalFields', { count: colCount }) : ''}
            </div>
            {colsLoading ? (
              <div className="space-y-1">
                {Array.from({ length: 9 }).map((_, i) => (
                  <Skeleton key={i} className="h-4 w-1/2" />
                ))}
              </div>
            ) : (
              <div className="flex flex-wrap gap-x-2 gap-y-1">
                {columns.map(col => (
                  <Tooltip key={col.id}>
                    <TooltipTrigger asChild>
                      <Badge variant="outline">{col.name}</Badge>
                    </TooltipTrigger>
                    <TooltipContent side="top" align="start">
                      <div className="text-xs max-w-xs">
                        <div className="font-medium">{col.name}</div>
                        <div className="text-muted-foreground">
                          {formatType(col.type)}
                          {col.description ? ` · ${col.description}` : ''}
                        </div>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                ))}
              </div>
            )}
          </div>
        )}
      </Card>
    );
  };

  // Handle chat send with webapp SSE
  const handleSend = useCallback(
    (
      items: Array<{ id: string; conversationId: string | null }>,
      userInput: { query: string; files?: ChatAttachment[]; inputs: Record<string, unknown> }
    ) => {
      const [{ conversationId }] = items;

      // Build final inputs with data_source and model_config
      const finalInputs: Record<string, unknown> = {
        ...(userInput.inputs ?? {}),
        data_source: {
          source: dataSourceSource,
          tables: selectedTablesPayload,
        },
        model_config: toModelConfig(modelSelectorValue),
      };

      const runPayload = {
        query: userInput.query,
        conversation_id: conversationId ?? undefined,
        files: userInput.files,
        inputs: finalInputs,
      };

      void (async () => {
        try {
          const precheck = await precheckMutation.mutateAsync(runPayload);
          const warnings = getWorkflowPrecheckWarnings(precheck.data);
          if (precheck.data.status === 'warning' && warnings.length > 0) {
            setPrecheckWarnings(warnings);
          } else {
            setPrecheckWarnings([]);
          }

          runnerRef.current = onAgentRun(convId, {
            onWorkflowStarted: () => {},
            onTextChunk: () => {},
            onNodeStarted: () => {},
            onNodeFinished: () => {},
            onError: () => {},
            onWorkflowFinished: () => {},
          });
          runnerRef.current.onWorkflowStarted({ query: userInput.query });

          await startWebAppStream(runPayload, {
            onWorkflowStarted: () => {
              // Already handled above
            },
            onNodeStarted: payload => {
              const d = unwrap(payload);
              runnerRef.current?.onNodeStarted?.({
                status: 'running',
                nodeId: typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined,
                nodeType:
                  typeof d['node_type'] === 'string' ? (d['node_type'] as string) : undefined,
                title: typeof d['title'] === 'string' ? (d['title'] as string) : undefined,
                data: { input: d['inputs'], output: undefined },
              });
            },
            onNodeFinished: payload => {
              const d = unwrap(payload);
              const statusRaw =
                typeof d['status'] === 'string' ? (d['status'] as string) : 'running';
              const status = (
                statusRaw === 'failed'
                  ? 'failed'
                  : statusRaw === 'stopped'
                    ? 'stopped'
                    : statusRaw === 'success' || statusRaw === 'succeeded'
                      ? 'success'
                      : 'running'
              ) as 'failed' | 'success' | 'running' | 'stopped';
              runnerRef.current?.onNodeFinished?.({
                status,
                nodeId: typeof d['node_id'] === 'string' ? (d['node_id'] as string) : undefined,
                nodeType:
                  typeof d['node_type'] === 'string' ? (d['node_type'] as string) : undefined,
                title: typeof d['title'] === 'string' ? (d['title'] as string) : undefined,
                elapsedTime:
                  typeof d['elapsed_time'] === 'number' ? (d['elapsed_time'] as number) : 0,
                error: getWorkflowRunErrorText(d['error']),
                data: { input: d['inputs'], output: d['outputs'] },
              });
            },
            onTextChunk: payload => {
              const d = unwrap(payload);
              const text = typeof d['text'] === 'string' ? (d['text'] as string) : '';
              if (text) runnerRef.current?.onTextChunk(text);
            },
            onMessage: payload => {
              const d = unwrap(payload);
              const answer = typeof d['answer'] === 'string' ? (d['answer'] as string) : '';
              const serverConvId =
                typeof d['conversation_id'] === 'string' ? (d['conversation_id'] as string) : '';
              const messageId =
                typeof d['message_id'] === 'string' ? (d['message_id'] as string) : undefined;

              if (serverConvId && chatConv?.conversationId !== serverConvId) {
                updateConversation(convId, { conversationId: serverConvId });
              }
              lastMessageIdRef.current = messageId;
              if (answer) runnerRef.current?.onTextChunk(answer);
            },
            onMessageEnd: () => {
              // Optional handling
            },
            onWorkflowFinished: payload => {
              const d = unwrap(payload);
              const status =
                d['status'] === 'failed'
                  ? 'error'
                  : d['status'] === 'stopped'
                    ? 'stopped'
                    : 'completed';
              const elapsed = d['elapsed_time'];
              runnerRef.current?.onWorkflowFinished({
                status,
                messageId: lastMessageIdRef.current,
                elapsedTime: typeof elapsed === 'number' ? elapsed : undefined,
                error: getWorkflowRunErrorText(d['error']),
              });
              if (status === 'error') {
                notifyBillingError(d['error']);
              }
            },
            onError: err => {
              runnerRef.current?.onWorkflowFinished({
                status: 'error',
                error: getWorkflowRunErrorText(err) ?? String(err ?? 'Error'),
              });
              notifyBillingError(err);
            },
          });
        } catch (error) {
          runnerRef.current?.onWorkflowFinished({
            status: 'error',
            error: getWorkflowRunErrorText(error) ?? String(error ?? 'Error'),
          });
          notifyBillingError(error);
        }
      })();
    },
    [
      convId,
      getWorkflowRunErrorText,
      notifyBillingError,
      onAgentRun,
      precheckMutation,
      startWebAppStream,
      dataSourceSource,
      selectedTablesPayload,
      chatConv?.conversationId,
      updateConversation,
      toModelConfig,
      modelSelectorValue,
    ]
  );

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cancelStream();
      runnerRef.current?.dispose?.();
    };
  }, [cancelStream]);

  // Check if chat is ready
  const isChatReady = Boolean(webAppId) && !isWorkflowLoading;

  // Check if at least one table is selected
  const hasSelectedTables = selectedIds.size > 0;

  // Input disabled overlay when conditions not met
  const inputDisabledOverlay = useMemo(() => {
    if (isWorkflowLoading) {
      return (
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <Loader2 className="h-6 w-6 animate-spin" />
          <span className="text-sm font-medium">{tDb('dbs.biSearch.loadingWorkflow')}</span>
        </div>
      );
    }

    // Priority: model first, then tables
    if (!isModelSelected) {
      return (
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <MousePointerClick className="h-6 w-6" />
          <span className="text-sm font-medium">{tDb('dbs.biSearch.modelRequired')}</span>
        </div>
      );
    }
    if (!hasSelectedTables) {
      return (
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <MousePointerClick className="h-6 w-6" />
          <span className="text-sm font-medium">{tDb('dbs.biSearch.tableRequired')}</span>
        </div>
      );
    }
    return undefined;
  }, [isModelSelected, hasSelectedTables, tDb, isWorkflowLoading]);

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canAiQuery) {
    return <PermissionDeniedState />;
  }

  if (isWorkflowLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center p-4">
        <div className="flex flex-col items-center gap-4">
          <Skeleton className="h-12 w-12 rounded-full" />
          <div className="text-sm text-muted-foreground">{tDb('dbs.biSearch.loadingWorkflow')}</div>
        </div>
      </div>
    );
  }

  if (isConfigMissing) {
    return (
      <div className="flex h-full w-full items-center justify-center p-4 bg-muted/20">
        <div className="flex flex-col items-center gap-3 text-muted-foreground p-8 rounded-xl border bg-background shadow-sm">
          <div className="h-12 w-12 rounded-full bg-muted flex items-center justify-center">
            <AlertCircle className="h-6 w-6 text-muted-foreground/70" />
          </div>
          <p className="text-sm font-medium">{t('chat.biChatNotConfigured')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full">
      <div className="flex h-full">
        <div className="overflow-hidden flex flex-col w-80">
          <div className="p-2 border-b text-sm font-medium">{tDb('dbs.biSearch.title')}</div>
          <div className="py-1 px-2.5 border-b flex items-center gap-2">
            <Input
              className="h-8 flex-1 text-xs"
              value={search}
              onChange={e => setSearch(e.target.value)}
              placeholder={tDb('dbs.biSearch.searchPlaceholder')}
            />
            <div className="flex items-center gap-2">
              <Checkbox checked={selectAllState} onCheckedChange={handleSelectAll} />
              <span className="text-[11px] text-muted-foreground">
                {tDb('dbs.biSearch.selectAll')}
              </span>
            </div>
          </div>
          <div className="flex-1 min-h-0">
            {isLoading ? (
              <div className="p-3 space-y-2">
                {Array.from({ length: 10 }).map((_, i) => (
                  <Skeleton key={i} className="h-6 w-full" />
                ))}
              </div>
            ) : (
              <ScrollArea className="h-full">
                <div className="p-2.5 grid grid-cols-1 gap-1.5">
                  {filteredTables.map(tbl => {
                    const selected = selectedIds.has(tbl.id);
                    return (
                      <TableCard
                        key={tbl.id}
                        dbId={dbId}
                        table={tbl}
                        selected={selected}
                        onToggle={() => toggleSelect(tbl.id)}
                      />
                    );
                  })}
                </div>
              </ScrollArea>
            )}
          </div>
          <div className="p-1.5 text-sm bg-accent text-center border-t">
            {tDb('dbs.biSearch.selectedCount', { count: selectedIds.size })}
          </div>
        </div>
        <div className="border-l h-full flex-1 flex flex-col">
          {/* Model selector header */}
          <div className="flex items-center gap-4 px-4 py-2 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
            <div className="flex items-center justify-center gap-2 flex-1">
              <span className="text-sm font-medium text-muted-foreground shrink-0">
                {tDb('dbs.biSearch.modelConfig')}
              </span>
              <div className="w-full max-w-md">
                <ModelSelectorParameter
                  modelType="text-chat"
                  value={modelSelectorValue}
                  onChange={handleModelChange}
                />
              </div>
            </div>
          </div>
          <div className="max-w-6xl w-full h-full mx-auto flex-1 min-h-0">
            <Chat
              mode="singleTest"
              className="h-full w-full"
              conversation={{ id: convId, conversationId: chatConv?.conversationId ?? '' }}
              onSend={handleSend}
              onStop={handleStop}
              isRunning={isWebAppRunning}
              isStopping={isStopping}
              enableUpload={false}
              inputDisabled={
                !isChatReady ||
                !dataSourceSource ||
                isDbLoading ||
                !hasSelectedTables ||
                !isModelSelected
              }
              inputDisabledOverlay={inputDisabledOverlay}
              showWorkflowRunHeader
              showWorkflowDetail
              showWorkflowNodeDetail
              inputTopNotice={
                precheckWarnings.length > 0 ? (
                  <WorkflowPrecheckWarningBanner
                    warnings={precheckWarnings}
                    scope="webapp"
                    storageKey={`bi-search:${dbId ?? webAppId ?? 'bi'}`}
                  />
                ) : null
              }
            />
          </div>
        </div>
      </div>
    </div>
  );
}

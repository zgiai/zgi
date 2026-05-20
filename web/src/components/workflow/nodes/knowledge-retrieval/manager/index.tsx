'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { Database, SearchX } from 'lucide-react';
import NodeValueSelector from '../../../common/node-value-selector';
import type { KnowledgeRetrievalNodeData, WorkflowVariable } from '../../../store/type';
import { useDatasets } from '@/hooks/dataset/use-datasets';
import { cn } from '@/lib/utils';
import { useInitializeDefaultModelByUseCase } from '@/hooks/model/use-default-model-by-use-case';
import RecallSettingsDialog from './recall-settings-dialog';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n';
import { useWorkflowEditor } from '@/components/workflow/hooks/use-workflow-editor';
import OutputVariablesView from '../../../common/output-variables-view';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';

interface KnowledgeRetrievalManagerProps {
  id: string;
  className?: string;
  // When true, render in read-only mode and disable interactive controls
  readOnly?: boolean;
}

// Helper to merge dataset ids immutably and keep uniqueness
const toggleId = (ids: string[], id: string, checked: boolean): string[] => {
  if (checked) {
    return ids.includes(id) ? ids : [...ids, id];
  }
  return ids.filter(x => x !== id);
};

const KnowledgeRetrievalManager: React.FC<KnowledgeRetrievalManagerProps> = ({
  id: nodeId,
  className,
  readOnly = false,
}) => {
  const [keyword, setKeyword] = useState<string>('');
  // removed recallOpen local state as dialog manages its own open state
  const debouncedKeyword = useDebouncedValue(keyword, 400);
  const t = useT();
  const { workspaceId } = useWorkflowEditor();

  const nodeData = useNodeData<KnowledgeRetrievalNodeData>(nodeId);
  const updateNodeData = useNodeDataUpdate<KnowledgeRetrievalNodeData>(nodeId);

  // Load datasets with infinite query
  const { pages, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useDatasets(
    {
      limit: 20,
      keyword: debouncedKeyword || undefined,
      include_all: true,
      workspace_id: workspaceId,
    },
    { enabled: true, refetchOnWindowFocus: false }
  );

  // Use fine-grained selector to only get current node instead of entire nodes array
  // This prevents re-render when other nodes change (e.g., during hover)
  const outputs = useNodeOutputVariables(nodeId);

  const flatDatasets = useMemo(() => (pages ?? []).flat(), [pages]);
  const hasSearchKeyword = keyword.trim().length > 0;
  const hasDatasets = flatDatasets.length > 0;
  const loadMoreRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const el = loadMoreRef.current;
    if (!el) return;
    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && hasNextPage && !isFetchingNextPage) {
          void fetchNextPage();
        }
      },
      { rootMargin: '200px' }
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  const updateDatasetIds = useCallback(
    (nextIds: string[]) => {
      updateNodeData({ dataset_ids: nextIds, retrieval_mode: 'multiple' });
    },
    [updateNodeData]
  );

  const updateMultipleConfig = useCallback(
    (patch: Partial<KnowledgeRetrievalNodeData['multiple_retrieval_config']>) => {
      if (!nodeData) return;
      updateNodeData({
        multiple_retrieval_config: {
          ...(nodeData.multiple_retrieval_config || {}),
          ...patch,
        },
      });
    },
    [nodeData, updateNodeData]
  );

  // Auto-fill default rerank model when enabling reranking and switching to model mode
  useInitializeDefaultModelByUseCase({
    useCase: 'rerank',
    currentModel: nodeData?.multiple_retrieval_config.reranking_model ?? {},
    enabled:
      nodeData?.multiple_retrieval_config.reranking_enable &&
      nodeData?.multiple_retrieval_config.reranking_mode === 'reranking_model',
    onInitialize: v => {
      updateMultipleConfig({ reranking_model: { provider: v.provider, model: v.model } });
    },
  });

  return (
    <div className={cn('space-y-4', className)}>
      {/* Query Variable Section (moved ahead) */}
      <div>
        <h3 className="text-lg font-semibold">{t('nodes.knowledgeRetrieval.queryVariable')}</h3>
        <div className="py-2">
          <div>
            <NodeValueSelector
              nodeId={nodeId}
              value={
                Array.isArray(nodeData?.query_variable_selector) &&
                nodeData.query_variable_selector.length > 0
                  ? nodeData.query_variable_selector
                  : undefined
              }
              typeFilter={(type: WorkflowVariable['type']) =>
                type === 'string' || type === 'number'
              }
              onChange={(payload: {
                sourceId: string;
                key: string;
                valuePath: string[];
                type: WorkflowVariable['type'];
              }) =>
                updateNodeData({
                  query_variable_selector: payload.valuePath,
                })
              }
              disabled={readOnly}
            />
          </div>
        </div>
      </div>

      {/* Dataset Section */}
      <div>
        {/* Datasets header moved into row below */}
        <div className="flex items-center justify-between">
          <h3 className="text-lg font-semibold">{t('nodes.knowledgeRetrieval.datasets')}</h3>
          <RecallSettingsDialog id={nodeId} />
        </div>
        <div className="mt-4 space-y-3">
          <div className="flex items-center gap-2">
            <Input
              placeholder={t('nodes.knowledgeRetrieval.searchDatasets')}
              value={keyword}
              onChange={e => setKeyword(e.target.value)}
              disabled={readOnly}
            />
          </div>
          {isLoading ? (
            <>
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="flex items-center gap-3 p-2">
                  <Skeleton className="h-4 w-4 rounded" />
                  <Skeleton className="h-4 w-48" />
                </div>
              ))}
            </>
          ) : (
            <>
              <div className="space-y-2 max-h-80 overflow-auto pr-1">
                {hasDatasets ? (
                  flatDatasets.map(ds => (
                    <label
                      key={ds.id}
                      className="flex items-center gap-3 rounded border p-2 hover:bg-muted/50"
                    >
                      <Checkbox
                        checked={nodeData?.dataset_ids.includes(ds.id) || false}
                        onCheckedChange={checked =>
                          updateDatasetIds(
                            toggleId(nodeData?.dataset_ids || [], ds.id, Boolean(checked))
                          )
                        }
                        disabled={readOnly}
                      />
                      <div className="flex items-center gap-2">
                        <Database className="h-4 w-4 shrink-0 text-muted-foreground" />
                        <div>
                          <div className="text-sm font-medium">{ds.name}</div>
                          {ds.description ? (
                            <div className="text-xs text-muted-foreground">{ds.description}</div>
                          ) : null}
                        </div>
                      </div>
                    </label>
                  ))
                ) : (
                  <div className="flex min-h-40 flex-col items-center justify-center rounded-xl border border-dashed bg-muted/20 px-4 py-8 text-center">
                    <div className="mb-3 flex size-11 items-center justify-center rounded-full bg-background shadow-sm">
                      {hasSearchKeyword ? (
                        <SearchX className="size-5 text-muted-foreground" />
                      ) : (
                        <Database className="size-5 text-muted-foreground" />
                      )}
                    </div>
                    <div className="text-sm font-medium text-foreground">
                      {hasSearchKeyword
                        ? t('nodes.knowledgeRetrieval.empty.noSearchResultsTitle')
                        : t('nodes.knowledgeRetrieval.empty.noDatasetsTitle')}
                    </div>
                    <div className="mt-1 max-w-[320px] text-xs leading-5 text-muted-foreground">
                      {hasSearchKeyword
                        ? t('nodes.knowledgeRetrieval.empty.noSearchResultsDescription')
                        : t('nodes.knowledgeRetrieval.empty.noDatasetsDescription')}
                    </div>
                    {hasSearchKeyword ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="mt-4"
                        onClick={() => setKeyword('')}
                        disabled={readOnly}
                      >
                        {t('nodes.knowledgeRetrieval.empty.clearSearch')}
                      </Button>
                    ) : null}
                  </div>
                )}
                {hasDatasets ? <div ref={loadMoreRef} /> : null}
              </div>
              {isFetchingNextPage && (
                <div className="flex items-center gap-3 p-2">
                  <Skeleton className="h-4 w-4 rounded" />
                  <Skeleton className="h-4 w-48" />
                </div>
              )}
            </>
          )}
        </div>
      </div>
      <OutputVariablesView variables={outputs} />
    </div>
  );
};

export default KnowledgeRetrievalManager;

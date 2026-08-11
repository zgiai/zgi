'use client';

import React, { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { Target, Sparkles } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { useDataset } from '@/hooks/dataset/use-datasets';
// Removed mobile detection
import { useHitTestingHistory } from '@/hooks/dataset/use-hit-testing-history';
import {
  useVectorRetrieval,
  useGraphRetrieval,
  useExternalHitTesting,
} from '@/hooks/dataset/use-hit-testing';
import {
  QueryTextarea,
  RecordsTable,
  ResultItemExternal,
  RetrievalConfigModal,
  ResultsPanel,
} from './components';
import type {
  HitTestingRecord,
  RetrievalConfig,
  ExternalDatasetHitTestingResponse,
  HitTestingResponse,
} from './types';
import { toast } from 'sonner';
import { useUpdateDataset } from '@/hooks/dataset/use-datasets';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';
import { DATASET_KEYS } from '@/hooks/query-keys';
import { useDatasetGraphStatus } from '@/hooks/dataset/use-dataset-graph';

const getExternalResultKey = (
  result: ExternalDatasetHitTestingResponse['records'][number],
  index: number
) =>
  [
    result.metadata?.['x-amz-bedrock-kb-source-uri'] || result.title,
    result.metadata?.['x-amz-bedrock-kb-data-source-id'] || 'unknown',
    result.score,
    index,
  ].join(':');

const isGraphVisibilityNotReadyError = (error: unknown) => {
  const candidate = error as
    | {
        message?: string;
        businessError?: { code?: string };
        response?: { data?: { code?: string; message?: string } };
      }
    | undefined;
  const code = candidate?.response?.data?.code || candidate?.businessError?.code;
  const message = candidate?.response?.data?.message || candidate?.message;

  return (
    code === 'graph_visibility_not_ready' ||
    message?.toLowerCase().includes('knowledge graph visibility is not ready') === true
  );
};

/**
 * HitTestingPage Component
 * Main page component with left-right layout
 * Left: History records and query input
 * Right: Search results (collapsible on mobile)
 */
export default function HitTestingPage() {
  const { datasetId } = useParams<{ datasetId: string }>();
  const t = useT('datasets');
  const queryClient = useQueryClient();
  const { data: datasetData, isLoading: isDatasetLoading } = useDataset(datasetId);

  // State management
  const [query, setQuery] = useState('');
  const [isSearching, setIsSearching] = useState(false);
  const [vectorResults, setVectorResults] = useState<HitTestingResponse | null>(null);
  const [externalResults, setExternalResults] = useState<ExternalDatasetHitTestingResponse | null>(
    null
  );
  const [graphResults, setGraphResults] = useState<HitTestingResponse | null>(null);
  const [isVectorSearching, setIsVectorSearching] = useState(false);
  const [isGraphSearching, setIsGraphSearching] = useState(false);
  const [graphVisibilityBlockedByRequest, setGraphVisibilityBlockedByRequest] = useState(false);
  const {
    records,
    isLoading,
    hasMore,
    fetchNextPage,
    total,
    isFetchingNextPage,
    hasPreviousPage,
    currentPage,
    totalPages,
    fetchPreviousPage,
    goToPage,
  } = useHitTestingHistory(datasetId);
  // Results panel now always visible on the right
  const [configModalOpen, setConfigModalOpen] = useState(false);
  const updateDataset = useUpdateDataset(datasetId);

  // Retrieval hooks
  const vectorRetrieval = useVectorRetrieval(datasetId);
  const graphRetrieval = useGraphRetrieval(datasetId);
  const externalRetrieval = useExternalHitTesting(datasetId);

  const dataset = datasetData?.data;
  const isExternalDataSource = !!dataset?.external_knowledge_info?.external_knowledge_id;
  const graphConfigured = !!dataset?.enable_graph_flow && !isExternalDataSource;
  const { data: graphStatusResponse } = useDatasetGraphStatus(datasetId, graphConfigured);
  const graphStatus = graphStatusResponse?.data;
  const supportsGraphFlow = graphConfigured && graphStatus?.can_search === true;
  const graphVisibilityRevisionMismatch =
    graphConfigured &&
    graphStatus !== undefined &&
    graphStatus.graph_visibility_revision !== graphStatus.graph_projected_visibility_revision;
  const isGraphVisibilitySyncing =
    graphVisibilityRevisionMismatch || graphVisibilityBlockedByRequest;
  const graphUnavailableReason = !graphConfigured
    ? t('hitTesting.graphNotEnabled')
    : graphStatus?.error_message || t('hitTesting.graphNotReady');
  const graphPanelNotice = isGraphVisibilitySyncing
    ? {
        title: t('hitTesting.graphVisibilitySyncTitle'),
        description: t('hitTesting.graphVisibilitySyncDescription'),
      }
    : graphConfigured && graphStatus && !supportsGraphFlow
      ? {
          title: t('hitTesting.graphUnavailableTitle'),
          description: graphUnavailableReason,
        }
      : undefined;
  // Initialize retrieval config (defaults, then hydrate from dataset.retrieval_model_dict once)
  const [retrievalConfig, setRetrievalConfig] = useState<RetrievalConfig>({
    search_method: 'graph_search',
    reranking_enable: true,
    top_k: 10,
    score_threshold_enabled: false,
    score_threshold: 0.35,
    hop_depth: 3,
  });
  const vectorPanelTitle = t('hitTesting.hybridResults');
  const isCombinedGraphSearch = retrievalConfig.search_method === 'graph_search' && graphConfigured;

  useEffect(() => {
    if (!graphStatus) return;
    if (graphStatus.graph_visibility_revision !== graphStatus.graph_projected_visibility_revision) {
      setGraphResults(null);
    } else {
      setGraphVisibilityBlockedByRequest(false);
    }
  }, [
    graphStatus,
    graphStatus?.graph_projected_visibility_revision,
    graphStatus?.graph_visibility_revision,
  ]);

  useEffect(() => {
    if (!dataset?.retrieval_config) return;
    const server = dataset.retrieval_config;
    const normalizedSearchMethod = normalizeDatasetSearchMethod(
      server.search_method as RetrievalConfig['search_method'],
      graphConfigured
    );
    const hydrated: RetrievalConfig = {
      search_method: normalizedSearchMethod,
      reranking_enable: true,
      reranking_model: server.reranking_model
        ? {
            reranking_provider_name: server.reranking_model.reranking_provider_name,
            reranking_model_name: server.reranking_model.reranking_model_name,
          }
        : undefined,
      top_k: server.top_k ?? 4,
      score_threshold_enabled: !!server.score_threshold_enabled,
      score_threshold: typeof server.score_threshold === 'number' ? server.score_threshold : 0.5,
      hop_depth: 3 as const,
    };
    setRetrievalConfig(hydrated);
  }, [dataset?.retrieval_config, dataset?.id, graphConfigured]);
  const handleLoadMoreHistory = () => {
    if (hasMore) {
      fetchNextPage();
    }
  };

  const refreshHistory = async () => {
    await queryClient.invalidateQueries({
      queryKey: DATASET_KEYS.hitTesting(datasetId),
    });
    goToPage(1);
  };

  // Real hit testing function
  const handleHitTesting = async (options?: { queryText?: string; recordHistory?: boolean }) => {
    const queryText = (options?.queryText ?? query).trim();
    if (!queryText) return;

    const recordHistory = options?.recordHistory ?? true;

    const retrievalModel = {
      search_method: retrievalConfig.search_method,
      reranking_enable: true,
      reranking_model: retrievalConfig.reranking_model,
      top_k: retrievalConfig.top_k,
      score_threshold_enabled: retrievalConfig.score_threshold_enabled,
      score_threshold: retrievalConfig.score_threshold,
      fallback_policy:
        retrievalConfig.search_method === 'graph_search'
          ? (retrievalConfig.fallback_policy ?? 'none')
          : retrievalConfig.fallback_policy,
      hop_depth: 3 as const,
    };

    try {
      setIsSearching(true);

      // External data source: use external retrieval hook
      if (isExternalDataSource) {
        const result = await externalRetrieval.mutateAsync({
          query: queryText,
          external_retrieval_model: {
            search_method: retrievalConfig.search_method,
            top_k: retrievalConfig.top_k,
            score_threshold_enabled: retrievalConfig.score_threshold_enabled,
            score_threshold: retrievalConfig.score_threshold,
            reranking_enable: true,
          },
          record_history: recordHistory,
        });
        // Store in externalResults for external data source
        setExternalResults(result.data);
        if (recordHistory) {
          await refreshHistory();
        }
        return;
      }

      // Combined graph mode always shows hybrid text retrieval and graph retrieval together.
      if (isCombinedGraphSearch) {
        setIsVectorSearching(true);
        setIsGraphSearching(supportsGraphFlow);

        const vectorRequestData = {
          query: queryText,
          retrieval_model: {
            ...retrievalModel,
            search_method: 'hybrid_search' as const,
          },
          record_history: recordHistory,
        };
        const graphRequestData = {
          query: queryText,
          retrieval_model: {
            ...retrievalModel,
            search_method: 'graph_search' as const,
          },
          // A combined test is one user action. The hybrid request owns its
          // history entry; the parallel graph request only supplies results.
          record_history: false,
        };

        const retrievals: Array<Promise<boolean>> = [
          vectorRetrieval
            .mutateAsync(vectorRequestData)
            .then(response => {
              setVectorResults(response.data);
              return true;
            })
            .catch(error => {
              console.error('Vector retrieval failed:', error);
              toast.error(t('hitTesting.vectorRetrievalFailed'));
              return false;
            })
            .finally(() => setIsVectorSearching(false)),
        ];

        if (supportsGraphFlow) {
          retrievals.push(
            graphRetrieval
              .mutateAsync(graphRequestData)
              .then(response => {
                setGraphResults(response.data);
                return true;
              })
              .catch(error => {
                if (isGraphVisibilityNotReadyError(error)) {
                  setGraphResults(null);
                  setGraphVisibilityBlockedByRequest(true);
                  void queryClient.invalidateQueries({
                    queryKey: DATASET_KEYS.graphStatus(datasetId),
                  });
                  return false;
                }
                console.error('Graph retrieval failed:', error);
                toast.error(t('hitTesting.graphRetrievalFailed'));
                return false;
              })
              .finally(() => setIsGraphSearching(false))
          );
        }

        const retrievalResults = await Promise.all(retrievals);
        if (retrievalResults.some(Boolean) && recordHistory) {
          await refreshHistory();
        }
      } else {
        // Hybrid text retrieval only.
        setIsVectorSearching(true);
        const result = await vectorRetrieval.mutateAsync({
          query: queryText,
          retrieval_model: retrievalModel,
          record_history: recordHistory,
        });
        setVectorResults(result.data);
        if (recordHistory) {
          await refreshHistory();
        }
      }
    } catch (error) {
      if (isGraphVisibilityNotReadyError(error)) {
        setGraphResults(null);
        setGraphVisibilityBlockedByRequest(true);
        void queryClient.invalidateQueries({
          queryKey: DATASET_KEYS.graphStatus(datasetId),
        });
        return;
      }
      console.error('Hit testing failed:', error);
      toast.error(t('hitTesting.hitTestingFailed'));
    } finally {
      setIsSearching(false);
      setIsVectorSearching(false);
      setIsGraphSearching(false);
    }
  };

  // Load query from history
  const handleLoadFromHistory = (record: HitTestingRecord) => {
    if (isSearching) return;

    setQuery(record.content);
    void handleHitTesting({ queryText: record.content, recordHistory: false });
  };

  // Handle config save
  const handleConfigSave = (config: RetrievalConfig) => {
    setRetrievalConfig(config);
    // Submit retrieval_config to persist retrieval settings to dataset
    const payload = {
      retrieval_config: {
        search_method: normalizeDatasetSearchMethod(config.search_method, graphConfigured),
        top_k: config.top_k,
        score_threshold_enabled: config.score_threshold_enabled,
        score_threshold: config.score_threshold,
        fallback_policy:
          config.search_method === 'graph_search'
            ? (config.fallback_policy ?? 'none')
            : config.fallback_policy,
        hop_depth: 3 as const,
        reranking_enable: true,
        reranking_model: config.reranking_model,
      },
    };
    updateDataset.mutate(payload);
  };

  // Check if dataset has completed documents (disabled for now)
  // const hasCompletedDocuments = (dataset?.available_document_count ?? 0) > 0;

  if (isDatasetLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  // Empty state when no completed documents (disabled for now)
  // if (!hasCompletedDocuments) {
  //   return (
  //     <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
  //       <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
  //         <Target className="w-8 h-8 text-muted-foreground" />
  //       </div>
  //       <div className="space-y-2">
  //         <h2 className="text-lg font-semibold text-foreground">
  //           {t('hitTesting.noCompletedDocuments')}
  //         </h2>
  //         <p className="text-sm text-muted-foreground max-w-md">
  //           {t('hitTesting.noCompletedDocumentsDesc')}
  //         </p>
  //       </div>
  //     </div>
  //   );
  // }

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="border-b px-6 py-5">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-2xl font-semibold tracking-tight text-foreground">
              {t('hitTestingTitle')}
            </h1>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('hitTestingDescription')}
            </p>
          </div>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(380px,440px)_minmax(0,1fr)]">
        <div className="flex min-h-0 flex-col gap-5 border-r bg-muted/10 px-6 py-5">
          {/* Query Input */}
          <div className="min-h-[300px] flex-[1.05]">
            <QueryTextarea
              query={query}
              onQueryChange={setQuery}
              onSubmit={handleHitTesting}
              isLoading={isSearching}
              isExternalDataSource={isExternalDataSource}
              retrievalConfig={retrievalConfig}
              onConfigChange={() => setConfigModalOpen(true)}
            />
          </div>

          {/* History Records (scroll capped) */}
          <div className="min-h-[260px] flex-1 overflow-hidden">
            <RecordsTable
              records={records}
              isLoading={isLoading}
              onLoadQuery={handleLoadFromHistory}
              onLoadMore={handleLoadMoreHistory}
              hasMore={hasMore}
              hasPreviousPage={hasPreviousPage}
              isFetchingNextPage={isFetchingNextPage}
              total={total}
              currentPage={currentPage}
              totalPages={totalPages}
              onLoadPrevious={fetchPreviousPage}
            />
          </div>
        </div>

        <div className="flex min-h-0 min-w-0 flex-col bg-muted/20">
          {/* Results Content */}
          <div className="min-w-0 flex-1 overflow-hidden">
            {isCombinedGraphSearch ? (
              // Combined mode: hybrid and graph results side by side.
              <div className="flex h-full min-w-0">
                {/* Vector Results Panel */}
                <div className="flex-1 min-w-0 border-r overflow-hidden">
                  <ResultsPanel
                    title={vectorPanelTitle}
                    results={
                      vectorResults
                        ? vectorResults.records.filter(
                            r => r.match_type === 'original' || !r.match_type
                          )
                        : undefined
                    }
                    isSearching={isVectorSearching}
                    type="vector"
                    elapsedTime={vectorResults?.elapsed_time}
                  />
                </div>

                {/* Graph Results Panel */}
                <div className="flex-1 min-w-0 overflow-hidden">
                  <ResultsPanel
                    title={t('hitTesting.graphResults')}
                    results={graphResults?.records ?? undefined}
                    isSearching={isGraphSearching}
                    type="graph"
                    graphExecution={graphResults?.graph_execution}
                    elapsedTime={graphResults?.elapsed_time}
                    notice={graphPanelNotice}
                  />
                </div>
              </div>
            ) : isExternalDataSource ? (
              // External data source: legacy display
              <div className="px-6">
                {isSearching ? (
                  <div className="space-y-4 h-full flex flex-col py-8">
                    <div className="flex items-center gap-2">
                      <Sparkles className="h-4 w-4 animate-spin" />
                      <span className="text-sm">{t('hitTesting.searching')}</span>
                    </div>
                    <div className="space-y-3 h-0 grow overflow-y-auto">
                      {Array.from({ length: 3 }).map((_, i) => (
                        <Card key={i}>
                          <CardContent className="p-4">
                            <div className="space-y-2">
                              <Skeleton className="h-4 w-1/4" />
                              <Skeleton className="h-3 w-full" />
                              <Skeleton className="h-3 w-3/4" />
                            </div>
                          </CardContent>
                        </Card>
                      ))}
                    </div>
                  </div>
                ) : externalResults ? (
                  <div className="space-y-4 h-full flex flex-col py-8">
                    <div className="flex items-center justify-between">
                      <h3 className="font-semibold">{t('hitTesting.results')}</h3>
                    </div>
                    <div className="space-y-4 h-0 grow overflow-y-auto">
                      {externalResults.records.map((result, index: number) => (
                        <ResultItemExternal
                          key={getExternalResultKey(result, index)}
                          result={result}
                          index={index}
                        />
                      ))}
                    </div>
                  </div>
                ) : (
                  <div className="text-center py-8 pt-[50%]">
                    <Target className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
                    <p className="text-muted-foreground">{t('hitTesting.noResults')}</p>
                    <p className="text-sm text-muted-foreground mt-2">
                      {t('hitTesting.startTesting')}
                    </p>
                  </div>
                )}
              </div>
            ) : (
              // Hybrid retrieval only.
              <ResultsPanel
                title={vectorPanelTitle}
                results={vectorResults?.records ?? undefined}
                isSearching={isVectorSearching}
                type="vector"
                elapsedTime={vectorResults?.elapsed_time}
              />
            )}
          </div>
        </div>
      </div>

      {/* Configuration Modal */}
      <RetrievalConfigModal
        open={configModalOpen}
        onOpenChange={setConfigModalOpen}
        config={retrievalConfig}
        onConfigChange={setRetrievalConfig}
        onSave={handleConfigSave}
        onSaveAsTest={setRetrievalConfig}
        isGraphEnabled={graphConfigured}
        graphUnavailableReason={graphUnavailableReason}
      />
    </div>
  );
}

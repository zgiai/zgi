'use client';

import React, { useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { useT } from '@/i18n';
import { GitCompareArrows, Target, Sparkles } from 'lucide-react';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
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
  const supportsGraphFlow = !!dataset?.enable_graph_flow && !isExternalDataSource;
  // Initialize retrieval config (defaults, then hydrate from dataset.retrieval_model_dict once)
  const [retrievalConfig, setRetrievalConfig] = useState<RetrievalConfig>({
    search_method: 'semantic_search',
    reranking_enable: false,
    top_k: 4,
    score_threshold_enabled: false,
    score_threshold: 0.5,
  });
  // Comparison mode: show both vector and graph results side by side
  // Persist to localStorage
  const COMPARISON_MODE_KEY = 'hit-testing-comparison-mode';
  const [comparisonMode, setComparisonMode] = useState(() => {
    if (typeof window === 'undefined') return true;
    const stored = localStorage.getItem(COMPARISON_MODE_KEY);
    return stored !== null ? stored === 'true' : true;
  });

  // Sync comparison mode to localStorage
  useEffect(() => {
    localStorage.setItem(COMPARISON_MODE_KEY, String(comparisonMode));
  }, [comparisonMode]);

  useEffect(() => {
    if (!dataset?.retrieval_config) return;
    const server = dataset.retrieval_config;
    const normalizedSearchMethod = normalizeDatasetSearchMethod(
      server.search_method as RetrievalConfig['search_method'],
      supportsGraphFlow
    );
    const hydrated: RetrievalConfig = {
      search_method: normalizedSearchMethod,
      reranking_enable: !!server.reranking_enable,
      reranking_model: server.reranking_model
        ? {
            reranking_provider_name: server.reranking_model.reranking_provider_name,
            reranking_model_name: server.reranking_model.reranking_model_name,
          }
        : undefined,
      top_k: server.top_k ?? 4,
      score_threshold_enabled: !!server.score_threshold_enabled,
      score_threshold: typeof server.score_threshold === 'number' ? server.score_threshold : 0.5,
    };
    setRetrievalConfig(hydrated);
  }, [dataset?.retrieval_config, dataset?.id, supportsGraphFlow]);
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
  const handleHitTesting = async () => {
    if (!query.trim()) return;

    const retrievalModel = {
      search_method: retrievalConfig.search_method,
      reranking_enable: retrievalConfig.reranking_enable,
      reranking_model: retrievalConfig.reranking_model,
      top_k: retrievalConfig.top_k,
      score_threshold_enabled: retrievalConfig.score_threshold_enabled,
      score_threshold: retrievalConfig.score_threshold,
    };

    try {
      setIsSearching(true);

      // External data source: use external retrieval hook
      if (isExternalDataSource) {
        const result = await externalRetrieval.mutateAsync({
          query: query.trim(),
          external_retrieval_model: {
            search_method: retrievalConfig.search_method,
            top_k: retrievalConfig.top_k,
            score_threshold_enabled: retrievalConfig.score_threshold_enabled,
            score_threshold: retrievalConfig.score_threshold,
            reranking_enable: retrievalConfig.reranking_enable,
          },
        });
        // Store in externalResults for external data source
        setExternalResults(result.data);
        await refreshHistory();
        return;
      }

      // Internal dataset: check comparison mode and search method
      if (comparisonMode && supportsGraphFlow) {
        // Comparison mode: parallel call both APIs
        setIsVectorSearching(true);
        setIsGraphSearching(true);

        const requestData = {
          query: query.trim(),
          retrieval_model: retrievalModel,
        };

        const [vectorResponse, graphResponse] = await Promise.allSettled([
          vectorRetrieval.mutateAsync(requestData),
          graphRetrieval.mutateAsync(requestData),
        ]);
        let hasSuccessfulRetrieval = false;

        // Handle vector retrieval result
        if (vectorResponse.status === 'fulfilled') {
          setVectorResults(vectorResponse.value.data);
          hasSuccessfulRetrieval = true;
        } else {
          console.error('Vector retrieval failed:', vectorResponse.reason);
          toast.error(t('hitTesting.vectorRetrievalFailed'));
        }

        // Handle graph retrieval result
        if (graphResponse.status === 'fulfilled') {
          setGraphResults(graphResponse.value.data);
          hasSuccessfulRetrieval = true;
        } else {
          console.error('Graph retrieval failed:', graphResponse.reason);
          toast.error(t('hitTesting.graphRetrievalFailed'));
        }

        if (hasSuccessfulRetrieval) {
          await refreshHistory();
        }
      } else if (retrievalConfig.search_method === 'graph_search' && supportsGraphFlow) {
        // Graph search mode: only call graph retrieval
        setIsGraphSearching(true);
        const result = await graphRetrieval.mutateAsync({
          query: query.trim(),
          retrieval_model: retrievalModel,
        });
        setGraphResults(result.data);
        await refreshHistory();
      } else {
        // Semantic search mode (default): only call vector retrieval
        setIsVectorSearching(true);
        const result = await vectorRetrieval.mutateAsync({
          query: query.trim(),
          retrieval_model: retrievalModel,
        });
        setVectorResults(result.data);
        await refreshHistory();
      }
    } catch (error) {
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
    setQuery(record.content);
  };

  // Handle config save
  const handleConfigSave = (config: RetrievalConfig) => {
    setRetrievalConfig(config);
    // Submit retrieval_config to persist retrieval settings to dataset
    const payload = {
      retrieval_config: {
        search_method: normalizeDatasetSearchMethod(config.search_method, supportsGraphFlow),
        top_k: config.top_k,
        score_threshold_enabled: config.score_threshold_enabled,
        score_threshold: config.score_threshold,
        reranking_enable: config.reranking_enable,
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

          {supportsGraphFlow && (
            <div className="flex shrink-0 items-center gap-3 rounded-lg border bg-card px-3 py-2 shadow-sm">
              <GitCompareArrows className="h-4 w-4 text-muted-foreground" />
              <Label htmlFor="comparison-mode" className="text-sm font-medium text-foreground">
                {t('hitTesting.comparisonMode')}
              </Label>
              <Switch
                id="comparison-mode"
                checked={comparisonMode}
                onCheckedChange={setComparisonMode}
              />
            </div>
          )}
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-cols-[minmax(320px,380px)_minmax(0,1fr)]">
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
            {comparisonMode && supportsGraphFlow ? (
              // Comparison mode: dual panel layout
              <div className="flex h-full min-w-0">
                {/* Vector Results Panel */}
                <div className="flex-1 min-w-0 border-r overflow-hidden">
                  <ResultsPanel
                    title={t('hitTesting.vectorResults')}
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
            ) : retrievalConfig.search_method === 'graph_search' && supportsGraphFlow ? (
              // Graph search mode: show graph results only
              <ResultsPanel
                title={t('hitTesting.graphResults')}
                results={graphResults?.records ?? undefined}
                isSearching={isGraphSearching}
                type="graph"
                graphExecution={graphResults?.graph_execution}
                elapsedTime={graphResults?.elapsed_time}
              />
            ) : (
              // Semantic search mode (default): show vector results only
              <ResultsPanel
                title={t('hitTesting.vectorResults')}
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
        isGraphEnabled={supportsGraphFlow}
      />
    </div>
  );
}

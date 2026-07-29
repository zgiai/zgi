'use client';

import * as React from 'react';
import { useParams } from 'next/navigation';
import {
  useDatasetGraph,
  useDatasetGraphStatus,
  useRebuildDatasetGraph,
} from '@/hooks/dataset/use-dataset-graph';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { useT } from '@/i18n';
import { AlertCircle, Loader2, Minus, Network, Plus, RotateCcw, Search, X } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { KnowledgeGraph, DetailPanel } from '@/components/datasets/knowledge-graph';
import { getCategoryColorMap } from '@/components/datasets/knowledge-graph/utils/color';
import type { DatasetGraph, GraphNode } from '@/services/types/dataset';
import type { KnowledgeGraphHandle } from '@/components/datasets/knowledge-graph';
import { cn } from '@/lib/utils';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

const GRAPH_QUERY_LIMIT_ERROR = 'graph_query_limit_exceeded';
const DEFAULT_OVERVIEW_NODE_LIMIT = 200;
const MIN_OVERVIEW_NODE_LIMIT = 100;
const OVERVIEW_NODE_STEP = 100;
const EXPLORATION_NODE_LIMIT = 150;
const EXPANSION_NODE_LIMIT = 50;
const VISIBLE_NODE_LIMIT = 200;
const VISIBLE_EDGE_LIMIT = 600;

const graphEdgeKey = (edge: DatasetGraph['edges'][number]) =>
  `${edge.source}\u0000${edge.target}\u0000${edge.label}`;

const mergeGraphData = (
  current: DatasetGraph,
  incoming: DatasetGraph,
  nodeLimit: number
): DatasetGraph => {
  const nodes = [...current.nodes];
  const nodeIds = new Set(nodes.map(node => node.id));
  for (const node of incoming.nodes) {
    if (nodeIds.has(node.id) || nodes.length >= nodeLimit) continue;
    nodeIds.add(node.id);
    nodes.push(node);
  }

  const edges = [...current.edges];
  const edgeKeys = new Set(edges.map(graphEdgeKey));
  for (const edge of incoming.edges) {
    const key = graphEdgeKey(edge);
    if (
      edgeKeys.has(key) ||
      !nodeIds.has(edge.source) ||
      !nodeIds.has(edge.target) ||
      edges.length >= VISIBLE_EDGE_LIMIT
    ) {
      continue;
    }
    edgeKeys.add(key);
    edges.push(edge);
  }

  const categoriesById = new Map(
    [...current.categories, ...incoming.categories].map(category => [category.id, category])
  );

  return {
    nodes,
    edges,
    categories: Array.from(categoriesById.values()),
    node_count: nodes.length,
    edge_count: edges.length,
    total_node_count: current.total_node_count,
    total_edge_count: current.total_edge_count,
  };
};

export default function DatasetGraphPage() {
  const { datasetId } = useParams<{ datasetId: string }>();
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canViewGraph = hasAnyPermission([
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphView,
    ...KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage,
  ]);
  const canManageGraph = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.graphManage);
  const rebuildGraph = useRebuildDatasetGraph(datasetId);
  const { data: datasetData, isLoading: _isDatasetLoading } = useDataset(datasetId, {
    enabled: canViewGraph,
  });
  const t = useT('datasets');
  const [selectedNode, setSelectedNode] = React.useState<GraphNode | null>(null);
  const [overviewNodeLimit, setOverviewNodeLimit] = React.useState(DEFAULT_OVERVIEW_NODE_LIMIT);
  const [searchQuery, setSearchQuery] = React.useState('');
  const [isSearchOpen, setIsSearchOpen] = React.useState(false);
  const [explorationRootId, setExplorationRootId] = React.useState<string | undefined>();
  const [expansionNodeId, setExpansionNodeId] = React.useState<string | undefined>();
  const [visibleGraph, setVisibleGraph] = React.useState<DatasetGraph | null>(null);
  const [expandedNodeIds, setExpandedNodeIds] = React.useState<Set<string>>(() => new Set());
  const initializedRootRef = React.useRef<string | null>(null);
  const { data: graphStatusData, isLoading: isStatusLoading } = useDatasetGraphStatus(
    datasetId,
    canViewGraph
  );
  const graphStatus = graphStatusData?.data;
  const canQueryGraph = canViewGraph && graphStatus?.can_search === true;
  const trimmedSearchQuery = searchQuery.trim();

  const overviewGraphQuery = useDatasetGraph(
    datasetId,
    {
      overview: true,
      node_limit: overviewNodeLimit,
      edge_limit: overviewNodeLimit * 3,
    },
    { enabled: canQueryGraph }
  );
  const explorationGraphQuery = useDatasetGraph(
    datasetId,
    {
      seed_node_id: explorationRootId,
      hop_depth: 1,
      node_limit: EXPLORATION_NODE_LIMIT,
      edge_limit: EXPLORATION_NODE_LIMIT * 3,
    },
    { enabled: canQueryGraph && Boolean(explorationRootId) }
  );
  const expansionGraphQuery = useDatasetGraph(
    datasetId,
    {
      seed_node_id: expansionNodeId,
      hop_depth: 1,
      node_limit: EXPANSION_NODE_LIMIT,
      edge_limit: EXPANSION_NODE_LIMIT * 3,
    },
    { enabled: canQueryGraph && Boolean(expansionNodeId) }
  );
  const searchGraphQuery = useDatasetGraph(
    datasetId,
    {
      keyword: trimmedSearchQuery || undefined,
      node_limit: 20,
      edge_limit: 60,
    },
    { enabled: canQueryGraph && Boolean(trimmedSearchQuery) }
  );

  const graphRef = React.useRef<KnowledgeGraphHandle>(null);
  const searchInputRef = React.useRef<HTMLInputElement>(null);
  const searchContainerRef = React.useRef<HTMLDivElement>(null);

  const _dataset = datasetData?.data;
  const overviewGraph = overviewGraphQuery.data?.data;
  const explorationGraph = explorationGraphQuery.data?.data;
  const expansionGraph = expansionGraphQuery.data?.data;
  const searchGraph = searchGraphQuery.data?.data;
  const graph =
    visibleGraph || (explorationRootId ? explorationGraph || overviewGraph : overviewGraph);
  const activeGraphQuery = explorationRootId ? explorationGraphQuery : overviewGraphQuery;
  const isGraphLoading = !graph && activeGraphQuery.isLoading;
  const isGraphError = activeGraphQuery.isError;
  const graphError = activeGraphQuery.error;
  const graphErrorMessage = graphError instanceof Error ? graphError.message : '';
  const graphLimitExceeded = graphErrorMessage.includes(GRAPH_QUERY_LIMIT_ERROR);
  const totalNodeCount = overviewGraph?.total_node_count ?? graph?.total_node_count ?? 0;
  const visibleNodeCount = graph?.nodes.length ?? 0;
  const isVisibleLimitReached = visibleNodeCount >= VISIBLE_NODE_LIMIT;
  const canDecreaseOverview =
    !explorationRootId &&
    totalNodeCount > MIN_OVERVIEW_NODE_LIMIT &&
    overviewNodeLimit > MIN_OVERVIEW_NODE_LIMIT;
  const canIncreaseOverview =
    !explorationRootId && totalNodeCount > 0 && visibleNodeCount < totalNodeCount;

  // Check if dataset has completed documents (disabled for now)
  // const hasCompletedDocuments = (dataset?.available_document_count ?? 0) > 0;

  const categoryColorMap = React.useMemo(() => {
    const categoriesById = new Map(
      [
        ...(overviewGraph?.categories || []),
        ...(graph?.categories || []),
        ...(searchGraph?.categories || []),
      ].map(category => [category.id, category])
    );
    return getCategoryColorMap(Array.from(categoriesById.values()));
  }, [graph?.categories, overviewGraph?.categories, searchGraph?.categories]);

  const filteredEntities = React.useMemo(() => {
    if (!searchGraph?.nodes || !trimmedSearchQuery) return [];
    return searchGraph.nodes.slice(0, 20);
  }, [searchGraph?.nodes, trimmedSearchQuery]);

  React.useEffect(() => {
    if (explorationRootId || !overviewGraph) return;
    setVisibleGraph(overviewGraph);
    setSelectedNode(current =>
      current && !overviewGraph.nodes.some(node => node.id === current.id) ? null : current
    );
  }, [explorationRootId, overviewGraph]);

  React.useEffect(() => {
    if (
      !explorationRootId ||
      !explorationGraph ||
      initializedRootRef.current === explorationRootId
    ) {
      return;
    }
    initializedRootRef.current = explorationRootId;
    setVisibleGraph(explorationGraph);
    setExpandedNodeIds(new Set([explorationRootId]));
    setSelectedNode(
      current => explorationGraph.nodes.find(node => node.id === explorationRootId) || current
    );
  }, [explorationGraph, explorationRootId]);

  React.useEffect(() => {
    if (!expansionNodeId || !expansionGraph) return;
    setVisibleGraph(current =>
      current ? mergeGraphData(current, expansionGraph, VISIBLE_NODE_LIMIT) : expansionGraph
    );
    setExpandedNodeIds(current => new Set(current).add(expansionNodeId));
    setExpansionNodeId(undefined);
  }, [expansionGraph, expansionNodeId]);

  React.useEffect(() => {
    if (expansionNodeId && expansionGraphQuery.isError) {
      setExpansionNodeId(undefined);
    }
  }, [expansionGraphQuery.isError, expansionNodeId]);

  const handleEntitySelect = (node: GraphNode) => {
    initializedRootRef.current = null;
    setExplorationRootId(node.id);
    setExpansionNodeId(undefined);
    setExpandedNodeIds(new Set());
    setSelectedNode(node);
    setSearchQuery('');
    setIsSearchOpen(false);
  };

  const handleNodeSelect = (nodeId: string) => {
    if (graph) {
      const node = graph.nodes.find(n => n.id === nodeId);
      if (node) {
        setSelectedNode(node);
        graphRef.current?.focusNode(nodeId);
      }
    }
  };

  const handleExpandNeighbors = (nodeId: string) => {
    if (!explorationRootId) {
      initializedRootRef.current = null;
      setExplorationRootId(nodeId);
      setExpansionNodeId(undefined);
      setExpandedNodeIds(new Set());
      return;
    }
    if (isVisibleLimitReached || expandedNodeIds.has(nodeId)) return;
    setExpansionNodeId(nodeId);
  };

  const handleBackToOverview = () => {
    initializedRootRef.current = null;
    setExplorationRootId(undefined);
    setExpansionNodeId(undefined);
    setExpandedNodeIds(new Set());
    setSelectedNode(null);
    if (overviewGraph) {
      setVisibleGraph(overviewGraph);
    }
  };

  const handleDecreaseOverview = () => {
    setOverviewNodeLimit(current =>
      Math.max(MIN_OVERVIEW_NODE_LIMIT, current - OVERVIEW_NODE_STEP)
    );
  };

  const handleIncreaseOverview = () => {
    setOverviewNodeLimit(current => Math.min(totalNodeCount, current + OVERVIEW_NODE_STEP));
  };

  // Close dropdown when clicking outside
  React.useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        searchContainerRef.current &&
        !searchContainerRef.current.contains(event.target as Node)
      ) {
        setIsSearchOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Open dropdown when typing
  React.useEffect(() => {
    if (searchQuery.trim()) {
      setIsSearchOpen(true);
    }
  }, [searchQuery]);

  // Loading state (disabled dataset loading check)
  // if (isDatasetLoading) {
  //   return (
  //     <div className="flex-1 flex items-center justify-center">
  //       <Loader2 className="w-8 h-8 text-primary animate-spin" />
  //     </div>
  //   );
  // }

  // Empty state when no completed documents (disabled for now)
  // if (!hasCompletedDocuments) {
  //   return (
  //     <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
  //       <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
  //         <Network className="w-8 h-8 text-muted-foreground" />
  //       </div>
  //       <div className="space-y-2">
  //         <h2 className="text-lg font-semibold text-foreground">
  //           {t('knowledgeGraph.noCompletedDocuments')}
  //         </h2>
  //         <p className="text-sm text-muted-foreground max-w-md">
  //           {t('knowledgeGraph.noCompletedDocumentsDesc')}
  //         </p>
  //       </div>
  //     </div>
  //   );
  // }

  if (isPermissionsLoading) {
    return <PermissionLoadingState />;
  }

  if (!canViewGraph) {
    return <PermissionDeniedState />;
  }

  return (
    <div className="flex flex-col h-full p-6 gap-6">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <Network className="w-6 h-6 text-primary" />
            {t('knowledgeGraphTitle')}
          </h1>
          <p className="text-sm text-muted-foreground mt-1">{t('knowledgeGraphDescription')}</p>
        </div>
        <div className="flex items-center gap-3">
          {canManageGraph && (
            <ConfirmDialog
              trigger={
                <Button type="button" variant="outline" disabled={rebuildGraph.isPending}>
                  {rebuildGraph.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  {t('graph.rebuild')}
                </Button>
              }
              title={t('graph.rebuildConfirmationTitle')}
              description={t('graph.rebuildConfirmationDescription')}
              confirmText={t('graph.confirmRebuild')}
              cancelText={t('actions.cancel')}
              onConfirm={() => rebuildGraph.mutate()}
              loading={rebuildGraph.isPending}
            />
          )}
          {/* Entity Search with Dropdown */}
          <div ref={searchContainerRef} className="relative w-72">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
            <Input
              ref={searchInputRef}
              placeholder={t('hitTesting.entitySearch.placeholder')}
              className="pl-9 pr-8 h-9"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              onFocus={() => searchQuery.trim() && setIsSearchOpen(true)}
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => {
                  setSearchQuery('');
                  setIsSearchOpen(false);
                  searchInputRef.current?.focus();
                }}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1 rounded-sm hover:bg-muted"
              >
                <X className="h-3.5 w-3.5 text-muted-foreground" />
              </button>
            )}

            {/* Search Results Dropdown */}
            {isSearchOpen && searchQuery.trim() && (
              <div className="absolute top-full left-0 right-0 mt-1 bg-popover border border-border rounded-lg shadow-lg z-50 max-h-80 overflow-auto">
                {searchGraphQuery.isFetching && filteredEntities.length === 0 ? (
                  <div className="flex items-center justify-center px-3 py-4">
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                  </div>
                ) : filteredEntities.length > 0 ? (
                  <>
                    <div className="px-3 py-2 text-xs text-muted-foreground border-b border-border">
                      {t('knowledgeGraph.searchResultsSummary', {
                        visible: filteredEntities.length,
                        total: searchGraph?.total_node_count ?? filteredEntities.length,
                      })}
                    </div>
                    <div className="py-1">
                      {filteredEntities.map(node => {
                        const colors = categoryColorMap[node.category];
                        return (
                          <button
                            key={node.id}
                            type="button"
                            onClick={() => handleEntitySelect(node)}
                            className={cn(
                              'w-full px-3 py-2 text-left hover:bg-accent transition-colors',
                              'flex items-center justify-between gap-2'
                            )}
                          >
                            <span className="font-medium truncate">{node.label}</span>
                            <Badge
                              variant="secondary"
                              className="shrink-0 text-xs"
                              style={
                                colors
                                  ? {
                                      backgroundColor: colors.fill,
                                      color: colors.text,
                                      borderColor: colors.stroke,
                                    }
                                  : undefined
                              }
                            >
                              {node.category}
                            </Badge>
                          </button>
                        );
                      })}
                    </div>
                  </>
                ) : (
                  <div className="px-3 py-4 text-sm text-muted-foreground text-center">
                    {t('hitTesting.entitySearch.noResults')}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 flex gap-6 min-h-0">
        {/* Left Side: Graph Visualization */}
        <div className="flex-1 flex flex-col min-w-0 bg-card rounded-xl border border-border shadow-sm overflow-hidden relative">
          {isStatusLoading || isGraphLoading ? (
            <div className="flex-1 flex items-center justify-center">
              <Loader2 className="w-8 h-8 text-primary animate-spin" />
            </div>
          ) : graphStatus?.status === 'failed' || graphStatus?.status === 'unavailable' ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
              <AlertCircle className="h-10 w-10 text-destructive" />
              <p className="font-medium">
                {graphStatus.error_message || t('graph.runtimeUnavailable')}
              </p>
            </div>
          ) : graphStatus?.status === 'waiting_content' ? (
            <div className="flex flex-1 items-center justify-center p-8 text-center text-muted-foreground">
              <p>{t('graph.emptyStatusDescription')}</p>
            </div>
          ) : graphStatus && !graphStatus.can_search ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center text-muted-foreground">
              <Loader2 className="h-8 w-8 animate-spin" />
              <p>
                {t('graph.statusDescription', {
                  status: t(`graph.statuses.${graphStatus.status}`),
                  progress: graphStatus.progress,
                })}
              </p>
            </div>
          ) : isGraphError ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 p-8 text-center">
              <AlertCircle className="h-10 w-10 text-destructive" />
              <p>{graphLimitExceeded ? GRAPH_QUERY_LIMIT_ERROR : graphErrorMessage}</p>
            </div>
          ) : graph && graph.nodes.length > 0 ? (
            <div className="flex-1 relative">
              <KnowledgeGraph
                ref={graphRef}
                data={graph}
                onNodeClick={setSelectedNode}
                categoryColorMap={categoryColorMap}
                legendHint={t(
                  explorationRootId
                    ? 'knowledgeGraph.nodeWeightHint'
                    : 'knowledgeGraph.overviewHint'
                )}
                className="w-full h-full"
              />
              <div className="absolute right-4 top-4 z-20 flex items-center gap-2">
                <Badge variant="secondary" className="bg-background/90 shadow-sm">
                  {t('knowledgeGraph.visibleEntities', {
                    visible: visibleNodeCount,
                    total: totalNodeCount,
                  })}
                </Badge>
                {!explorationRootId && (
                  <div className="flex items-center overflow-hidden rounded-md border border-border bg-background/90 shadow-sm">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-8 w-8 rounded-none"
                      onClick={handleDecreaseOverview}
                      disabled={!canDecreaseOverview || overviewGraphQuery.isFetching}
                      title={t('knowledgeGraph.decreaseOverviewEntities')}
                      aria-label={t('knowledgeGraph.decreaseOverviewEntities')}
                    >
                      <Minus className="h-3.5 w-3.5" />
                    </Button>
                    <div className="h-4 w-px bg-border" />
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="h-8 w-8 rounded-none"
                      onClick={handleIncreaseOverview}
                      disabled={!canIncreaseOverview || overviewGraphQuery.isFetching}
                      title={t('knowledgeGraph.increaseOverviewEntities')}
                      aria-label={t('knowledgeGraph.increaseOverviewEntities')}
                    >
                      {overviewGraphQuery.isFetching ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : (
                        <Plus className="h-3.5 w-3.5" />
                      )}
                    </Button>
                  </div>
                )}
                {explorationRootId && (
                  <Button
                    type="button"
                    variant="secondary"
                    size="sm"
                    onClick={handleBackToOverview}
                  >
                    <RotateCcw className="mr-1 h-3.5 w-3.5" />
                    {t('knowledgeGraph.backToOverview')}
                  </Button>
                )}
              </div>
              {explorationRootId && isVisibleLimitReached && (
                <div className="pointer-events-none absolute bottom-4 right-4 max-w-sm rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 shadow-sm">
                  {t('knowledgeGraph.visibleLimitReached')}
                </div>
              )}
              {explorationRootId && explorationGraphQuery.isFetching && (
                <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-background/20">
                  <Loader2 className="h-6 w-6 animate-spin text-primary" />
                </div>
              )}
              {expansionGraphQuery.isFetching && (
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  disabled
                  className="absolute bottom-4 left-1/2 -translate-x-1/2"
                >
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t('knowledgeGraph.expandingNeighbors')}
                </Button>
              )}
            </div>
          ) : (
            <div className="flex-1 flex items-center justify-center text-muted-foreground">
              {t('hitTesting.noGraphData')}
            </div>
          )}
        </div>

        {/* Right Side: Detail Panel */}
        <div className="w-80 shrink-0 flex flex-col gap-6">
          <DetailPanel
            selectedNode={selectedNode}
            graphData={graph || null}
            categoryColorMap={categoryColorMap}
            onNodeSelect={handleNodeSelect}
            onExpandNeighbors={handleExpandNeighbors}
            isExpanding={
              Boolean(selectedNode) &&
              ((explorationGraphQuery.isFetching &&
                explorationRootId === selectedNode?.id &&
                initializedRootRef.current !== explorationRootId) ||
                (expansionGraphQuery.isFetching && expansionNodeId === selectedNode?.id))
            }
            isExpanded={Boolean(selectedNode && expandedNodeIds.has(selectedNode.id))}
            expandDisabled={Boolean(explorationRootId) && isVisibleLimitReached}
            className="flex-1"
          />
        </div>
      </div>
    </div>
  );
}

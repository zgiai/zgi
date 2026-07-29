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
import { AlertCircle, Loader2, Network, Search, X } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { KnowledgeGraph, DetailPanel } from '@/components/datasets/knowledge-graph';
import { getCategoryColorMap } from '@/components/datasets/knowledge-graph/utils/color';
import type { GraphNode } from '@/services/types/dataset';
import type { KnowledgeGraphHandle } from '@/components/datasets/knowledge-graph';
import { cn } from '@/lib/utils';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  PermissionDeniedState,
  PermissionLoadingState,
} from '@/components/common/permission-gate-state';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { Button } from '@/components/ui/button';

const GRAPH_QUERY_LIMIT_ERROR = 'graph_query_limit_exceeded';

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
  const [searchQuery, setSearchQuery] = React.useState('');
  const [isSearchOpen, setIsSearchOpen] = React.useState(false);
  const [cursor, setCursor] = React.useState<string | undefined>();
  const [seedNodeId, setSeedNodeId] = React.useState<string | undefined>();
  const { data: graphStatusData, isLoading: isStatusLoading } = useDatasetGraphStatus(
    datasetId,
    canViewGraph
  );
  const graphStatus = graphStatusData?.data;
  const graphQuery = React.useMemo(
    () => ({
      keyword: searchQuery.trim() || undefined,
      seed_node_id: seedNodeId,
      cursor,
      hop_depth: seedNodeId ? 1 : undefined,
      node_limit: 300,
      edge_limit: 900,
    }),
    [cursor, searchQuery, seedNodeId]
  );
  const {
    data: graphData,
    isLoading: isGraphLoading,
    isError: isGraphError,
    error: graphError,
  } = useDatasetGraph(datasetId, graphQuery, {
    enabled: canViewGraph && graphStatus?.can_search === true,
  });

  const graphRef = React.useRef<KnowledgeGraphHandle>(null);
  const searchInputRef = React.useRef<HTMLInputElement>(null);
  const searchContainerRef = React.useRef<HTMLDivElement>(null);

  const _dataset = datasetData?.data;
  const graph = graphData?.data;
  const graphErrorMessage = graphError instanceof Error ? graphError.message : '';
  const graphLimitExceeded = graphErrorMessage.includes(GRAPH_QUERY_LIMIT_ERROR);

  // Check if dataset has completed documents (disabled for now)
  // const hasCompletedDocuments = (dataset?.available_document_count ?? 0) > 0;

  const categoryColorMap = React.useMemo(() => {
    if (!graph?.categories) return {};
    return getCategoryColorMap(graph.categories);
  }, [graph?.categories]);

  // Filter entities based on search query
  const filteredEntities = React.useMemo(() => {
    if (!graph?.nodes || !searchQuery.trim()) return [];
    const query = searchQuery.toLowerCase().trim();
    return graph.nodes.filter(node => node.label.toLowerCase().includes(query)).slice(0, 20); // Limit to 20 results for performance
  }, [graph?.nodes, searchQuery]);

  // Handle entity selection from search results
  const handleEntitySelect = (node: GraphNode) => {
    setSelectedNode(node);
    graphRef.current?.focusNode(node.id);
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
            <Button
              type="button"
              variant="outline"
              disabled={rebuildGraph.isPending}
              onClick={() => rebuildGraph.mutate()}
            >
              {rebuildGraph.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t('graph.rebuild')}
            </Button>
          )}
          {/* Entity Search with Dropdown */}
          <div ref={searchContainerRef} className="relative w-72">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground pointer-events-none" />
            <Input
              ref={searchInputRef}
              placeholder={t('hitTesting.entitySearch.placeholder')}
              className="pl-9 pr-8 h-9"
              value={searchQuery}
              onChange={e => {
                setSearchQuery(e.target.value);
                setCursor(undefined);
                setSeedNodeId(undefined);
              }}
              onFocus={() => searchQuery.trim() && setIsSearchOpen(true)}
            />
            {searchQuery && (
              <button
                type="button"
                onClick={() => {
                  setSearchQuery('');
                  setCursor(undefined);
                  setSeedNodeId(undefined);
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
                {filteredEntities.length > 0 ? (
                  <>
                    <div className="px-3 py-2 text-xs text-muted-foreground border-b border-border">
                      {t('hitTesting.entitySearch.resultsCount', {
                        count: filteredEntities.length,
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
                className="w-full h-full"
              />
              {graph.next_cursor && (
                <Button
                  type="button"
                  variant="secondary"
                  className="absolute bottom-4 right-4"
                  onClick={() => setCursor(graph.next_cursor)}
                >
                  {t('loadMore')}
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
            onExpandNeighbors={nodeId => {
              setSeedNodeId(nodeId);
              setCursor(undefined);
            }}
            className="flex-1"
          />
        </div>
      </div>
    </div>
  );
}

'use client';

import React from 'react';
import { useT } from '@/i18n';
import { Search, Sparkles, Target } from 'lucide-react';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { ScrollArea } from '@/components/ui/scroll-area';
import { GraphResultCard } from './graph-result-card';
import { ResultItem } from './result-item';
import { GraphExecutionDetails } from './graph-execution-details';
import type { HitTestingResponse } from '../types';
import type { HitTestingResult } from '@/services';

interface ResultsPanelProps {
  title: string;
  results?: HitTestingResult[];
  isSearching: boolean;
  type: 'vector' | 'graph';
  graphExecution?: HitTestingResponse['graph_execution'];
  elapsedTime?: number;
}

const getResultKey = (result: HitTestingResult, index: number) => {
  const childChunkKey = result.child_chunks?.map(chunk => chunk.id).join(':') || 'none';

  return [
    result.segment.id,
    result.match_type || 'default',
    result.retrieval_source?.method || 'unknown',
    childChunkKey,
    index,
  ].join(':');
};

export function ResultsPanel({
  title,
  results,
  isSearching,
  type,
  graphExecution,
  elapsedTime,
}: ResultsPanelProps) {
  const t = useT('datasets');

  const renderHeader = (count?: number) => (
    <div className="mb-4 flex items-center justify-between">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold text-foreground">{title}</h3>
        {count !== undefined && (
          <Badge variant="secondary" className="rounded-full bg-muted px-2 text-xs">
            {count}
          </Badge>
        )}
      </div>
      {elapsedTime !== undefined && (
        <div className="text-xs text-muted-foreground">{(elapsedTime / 1000).toFixed(2)}s</div>
      )}
    </div>
  );

  if (isSearching) {
    return (
      <div className="flex h-full min-w-0 flex-col space-y-4 p-6">
        {renderHeader()}
        <div className="flex items-center gap-2 text-primary animate-pulse">
          <Sparkles className="h-4 w-4 animate-spin" />
          <span className="text-sm font-medium">{t('hitTesting.searching')}</span>
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
    );
  }

  // Case 1: Initial state (results is undefined)
  if (results === undefined) {
    return (
      <div className="flex h-full min-w-0 flex-col p-6">
        {renderHeader()}
        <div className="flex flex-1 flex-col items-center justify-center">
          <div className="flex w-full max-w-sm flex-col items-center rounded-xl border border-dashed bg-background/70 p-8 text-center">
            <div className="mb-4 rounded-full bg-muted p-3">
              <Search className="h-6 w-6 text-muted-foreground" />
            </div>
            <h3 className="mb-2 text-sm font-semibold text-foreground">
              {t('hitTesting.results')}
            </h3>
            <p className="text-center text-sm leading-6 text-muted-foreground">
              {t('hitTesting.startTesting')}
            </p>
          </div>
        </div>
      </div>
    );
  }

  // Case 2: No results found after search (results is empty array)
  if (results.length === 0) {
    return (
      <div className="flex h-full min-w-0 flex-col space-y-4 p-6">
        {renderHeader(0)}

        {type === 'graph' && graphExecution && <GraphExecutionDetails execution={graphExecution} />}

        {/* Empty state message */}
        <div className="flex flex-1 flex-col items-center justify-center text-center">
          <div className="mb-4 rounded-full bg-muted p-3">
            <Target className="h-6 w-6 text-muted-foreground" />
          </div>
          <h3 className="text-sm font-semibold text-foreground">{t('hitTesting.noResults')}</h3>
          <p className="mt-2 max-w-[240px] text-sm leading-6 text-muted-foreground">
            {t('hitTesting.noResultsHint')}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full min-w-0 flex-col space-y-4 p-6">
      {/* Header */}
      {renderHeader(results.length)}

      {/* Graph Execution Details (only for graph type) */}
      {type === 'graph' && graphExecution && <GraphExecutionDetails execution={graphExecution} />}

      {/* Results List */}
      <ScrollArea className="h-0 min-w-0 grow">
        <div className="space-y-4 pr-4">
          {results.map((result, index) =>
            type === 'graph' ? (
              <GraphResultCard key={getResultKey(result, index)} result={result} index={index} />
            ) : (
              <ResultItem key={getResultKey(result, index)} result={result} index={index} />
            )
          )}
        </div>
      </ScrollArea>
    </div>
  );
}

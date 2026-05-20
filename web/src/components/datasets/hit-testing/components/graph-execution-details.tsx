'use client';

import React, { useState } from 'react';
import { useT } from '@/i18n';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { Network, ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { HitTestingResponse } from '../types';

interface GraphExecutionDetailsProps {
  execution: NonNullable<HitTestingResponse['graph_execution']>;
}

export function GraphExecutionDetails({ execution }: GraphExecutionDetailsProps) {
  const t = useT('datasets');
  const [expanded, setExpanded] = useState(false);

  // Safe access to debug_info with default values
  const debugInfo = execution.debug_info ?? {
    chunks_count: 0,
    entities_count: 0,
    hop_depth: 0,
    seeds: [],
    triples_count: 0,
  };

  return (
    <Collapsible open={expanded} onOpenChange={setExpanded}>
      <CollapsibleTrigger className="w-full">
        <Card className="hover:bg-accent/50 transition-colors cursor-pointer">
          <CardContent className="flex min-w-0 items-center justify-between gap-3 p-3">
            <div className="flex min-w-0 items-center gap-2">
              <Network className="h-4 w-4 text-purple-500" />
              <span className="min-w-0 text-sm font-medium [overflow-wrap:anywhere]">
                {t('hitTesting.graphExecution')}
              </span>
              {debugInfo.chunks_count > 0 && (
                <Badge variant="secondary" className="shrink-0 text-xs">
                  {debugInfo.chunks_count} {t('hitTesting.chunks')}
                </Badge>
              )}
            </div>
            <ChevronDown
              className={cn(
                'h-4 w-4 transition-transform text-muted-foreground',
                expanded && 'rotate-180'
              )}
            />
          </CardContent>
        </Card>
      </CollapsibleTrigger>

      <CollapsibleContent className="mt-2">
        <Card>
          <CardContent className="p-4 space-y-4">
            {/* Summary */}
            <div className="rounded-md bg-muted/50 p-3 text-sm text-muted-foreground [overflow-wrap:anywhere]">
              {execution.summary}
            </div>

            {/* Statistics - only show if debug_info exists */}
            {execution.debug_info && (
              <div className="grid grid-cols-3 gap-3">
                <div className="text-center p-3 bg-blue-50 dark:bg-blue-950/20 rounded-md">
                  <div className="text-2xl font-bold text-blue-600 dark:text-blue-400">
                    {debugInfo.entities_count}
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">
                    {t('hitTesting.entitiesCount')}
                  </div>
                </div>
                <div className="text-center p-3 bg-green-50 dark:bg-green-950/20 rounded-md">
                  <div className="text-2xl font-bold text-green-600 dark:text-green-400">
                    {debugInfo.chunks_count}
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">
                    {t('hitTesting.chunksCount')}
                  </div>
                </div>
                <div className="text-center p-3 bg-purple-50 dark:bg-purple-950/20 rounded-md">
                  <div className="text-2xl font-bold text-purple-600 dark:text-purple-400">
                    {debugInfo.hop_depth}
                  </div>
                  <div className="text-xs text-muted-foreground mt-1">
                    {t('hitTesting.hopDepth')}
                  </div>
                </div>
              </div>
            )}

            {/* Execution Steps */}
            {execution.steps && execution.steps.length > 0 && (
              <div className="space-y-2">
                <div className="text-sm font-medium">{t('hitTesting.executionSteps')}:</div>
                <div className="space-y-2">
                  {execution.steps.map((step, index) => (
                    <div
                      key={`${step.step}-${index}`}
                      className="flex min-w-0 items-start gap-3 rounded-md bg-muted/30 p-2 text-xs"
                    >
                      <Badge variant="outline" className="text-xs shrink-0">
                        {step.step}
                      </Badge>
                      <div className="flex-1 space-y-1">
                        <div className="font-medium [overflow-wrap:anywhere]">{step.description}</div>
                        <div className="text-muted-foreground [overflow-wrap:anywhere]">
                          {step.result}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Seed Entities */}
            {debugInfo.seeds && debugInfo.seeds.length > 0 && (
              <div className="space-y-2">
                <div className="text-sm font-medium">{t('hitTesting.seedEntities')}:</div>
                <div className="flex flex-wrap gap-1">
                  {debugInfo.seeds.map((seed, idx) => (
                    <Badge
                      key={idx}
                      variant="secondary"
                      className="h-auto max-w-full whitespace-normal text-xs [overflow-wrap:anywhere]"
                    >
                      {seed}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </CollapsibleContent>
    </Collapsible>
  );
}

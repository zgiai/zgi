'use client';

import React from 'react';
import { useT } from '@/i18n';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Network } from 'lucide-react';
import type { HitTestingResult } from '@/services';

interface GraphResultCardProps {
  result: HitTestingResult;
  index: number;
}

export function GraphResultCard({ result, index }: GraphResultCardProps) {
  const t = useT('datasets');
  const { segment, score, retrieval_source } = result;

  return (
    <Card className="min-w-0 overflow-hidden transition-shadow hover:shadow-md">
      <CardContent className="p-4">
        {/* Header with badges */}
        <div className="flex items-center gap-2 mb-3 flex-wrap">
          <Badge variant="default" className="bg-purple-500">
            <Network className="h-3 w-3 mr-1" />#{index + 1}
          </Badge>
          <Badge variant="outline">
            {t('hitTesting.score')}: {score.toFixed(4)}
          </Badge>
        </div>

        {/* Content */}
        <p className="mb-4 text-sm leading-relaxed text-foreground [overflow-wrap:anywhere]">
          {segment.content}
        </p>

        {/* Matched Entities Section */}
        <div className="pt-3 border-t">
          <div className="flex items-center gap-1.5 mb-2">
            <span className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
              {t('hitTesting.matchedEntities')}
            </span>
          </div>
          {retrieval_source?.matched_entities && retrieval_source.matched_entities.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {retrieval_source.matched_entities.map((entity, idx) => (
                <Badge
                  key={idx}
                  variant="secondary"
                  className="h-auto max-w-full whitespace-normal bg-purple-50 px-2 py-1 text-[11px] text-purple-700 border-purple-100 transition-colors hover:bg-purple-100 [overflow-wrap:anywhere]"
                >
                  {entity}
                </Badge>
              ))}
            </div>
          ) : (
            <span className="text-xs text-muted-foreground italic">
              {t('hitTesting.noMatchedEntities')}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

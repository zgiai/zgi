'use client';

import React from 'react';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import { Target, Sparkles } from 'lucide-react';
import { useT } from '@/i18n';
import { Skeleton } from '@/components/ui/skeleton';

import type { ResultElement } from '../type';
interface ResultPanelProps {
  query: string | null;
  resultData: ResultElement | undefined;
  isSearching: boolean;
}

const getBatchResultKey = (
  result: NonNullable<ResultElement['result']>['records'][number],
  index: number
) => {
  const segment = 'segment' in result ? result.segment : undefined;
  const childChunkKey =
    'child_chunks' in result && result.child_chunks
      ? result.child_chunks.map(chunk => chunk.id).join(':')
      : 'none';

  return [segment?.id || segment?.position || 'segment', result.score, childChunkKey, index].join(
    ':'
  );
};

export function BatchResultPanel(props: ResultPanelProps) {
  const t = useT('datasets');

  const { query, resultData, isSearching } = props;
  if (isSearching) {
    return (
      <div className="space-y-4 h-full flex flex-col w-full px-12 py-6">
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
    );
  }
  if (!resultData?.result?.records) {
    return (
      <div className="text-center py-8 pt-[20%] w-full">
        <Target className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
        <p className="text-muted-foreground">{t('hitTesting.noResults')}</p>
        <p className="text-sm text-muted-foreground mt-2">{t('hitTesting.startTesting')}</p>
      </div>
    );
  }

  return (
    <div className="w-full space-y-4  px-12 h-full  py-6">
      <div className="text-[16px] font-medium mb-2 flex justify-between">
        {query}
        <div className="text-sm">
          <span>耗时：{resultData?.finished_at - resultData.started_at}s</span>
          {/* <span>测试时间：</span> */}
        </div>
      </div>
      <div className="space-y-4 ">
        {resultData.result?.records.map((result, index) => (
          <Card key={getBatchResultKey(result, index)} className="mb-3">
            <CardHeader className="pb-3">
              <div className="flex items-center gap-2">
                <Badge variant="default">一级切片</Badge>
                <span className="text-[var(--tag-primary-text)]">#{result.segment.position}</span>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              {'segment' in result ? (
                <div className="text-sm leading-relaxed  line-clamp-3 m-2 text-primary">
                  {result.segment.content}
                </div>
              ) : null}
              {'child_chunks' in result && result.child_chunks && result.child_chunks.length > 0 ? (
                <>
                  <Separator />
                  <div className="space-y-2">
                    <div className=" space-y-2">
                      {result.child_chunks?.map((chunk, index) => (
                        <div
                          key={`${chunk.id}_${index}`}
                          className="bg-muted/50 rounded text-xs mb-[16px]"
                        >
                          <div className="flex items-center  mb-1">
                            <Badge variant="subtle" className="cursor-pointer">
                              二级切片
                            </Badge>
                            <span className="text-[var(--tag-normal-text)] text-xs ml-3">{`#S-${chunk.position}`}</span>
                          </div>
                          <div className="text-primary whitespace-pre-wrap line-clamp-2 m-2">
                            {chunk.content}
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                </>
              ) : null}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

export default BatchResultPanel;

'use client';

import React from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
// import { ChevronDown, ChevronUp } from 'lucide-react';
// import { useState } from 'react';
import type { ResultElement } from '../type';

interface RetrievalDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  resultData: ResultElement | null;
}

const getRetrievalRecordKey = (
  record: ResultElement['result']['records'][number],
  index: number
) => {
  const childChunkKey = record.child_chunks?.map(chunk => chunk.id).join(':') || 'none';

  return [
    record.segment?.id || record.segment?.position || 'segment',
    record.match_type || 'default',
    record.score,
    childChunkKey,
    index,
  ].join(':');
};

export function RetrievalDetailDialog({
  open,
  onOpenChange,
  resultData,
}: RetrievalDetailDialogProps) {
  const t = useT('datasets');

  // const [expandedItems, setExpandedItems] = useState<Set<number>>(new Set());

  // const toggleExpanded = (index: number) => {
  //   const newExpanded = new Set(expandedItems);
  //   if (newExpanded.has(index)) {
  //     newExpanded.delete(index);
  //   } else {
  //     newExpanded.add(index);
  //   }
  //   setExpandedItems(newExpanded);
  // };

  if (!resultData) return null;

  const records = resultData.result?.records || [];
  const bestRecord = records[0];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-lg font-semibold">
            {t('hitTesting.retrievalDetail')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 flex-1">
          <div className="space-y-6">
            {/* Test Question */}
            <div>
              <h3 className="text-sm font-medium text-muted-foreground mb-2">
                {t('hitTesting.testQuestion')}
              </h3>
              <p className="text-base py-3 px-4 bg-neutral-50/80 rounded-xl border border-neutral-100 font-medium leading-relaxed">
                {resultData.query}
              </p>
            </div>

            {/* Retrieval Summary */}
            <div>
              <h3 className="text-sm font-medium text-muted-foreground mb-3">
                {t('hitTesting.retrievalSummary')}
              </h3>
              <div className="grid grid-cols-3 gap-4">
                <div className="bg-blue-50/50 dark:bg-blue-950/20 p-4 rounded-xl border border-blue-100/50">
                  <div className="text-[10px] uppercase tracking-wider mb-1.5 text-blue-500 font-bold">
                    {t('hitTesting.bestRecallMethod')}
                  </div>
                  <div className="text-sm font-bold">
                    {bestRecord?.match_type
                      ? t(
                          `hitTesting.matchTypes.${bestRecord.match_type as 'original' | 'question'}`
                        )
                      : t('common.notAvailable')}
                  </div>
                </div>
                <div className="bg-emerald-50/50 dark:bg-green-950/20 p-4 rounded-xl border border-emerald-100/50">
                  <div className="text-[10px] uppercase tracking-wider mb-1.5 text-emerald-500 font-bold">
                    {t('hitTesting.highestSimilarity')}
                  </div>
                  <div className="text-sm font-bold">
                    {bestRecord?.score?.toFixed(3) || t('common.notAvailable')}
                  </div>
                </div>
                <div className="bg-violet-50/50 dark:bg-purple-950/20 p-4 rounded-xl border border-violet-100/50">
                  <div className="text-[10px] uppercase tracking-wider mb-1.5 text-violet-500 font-bold">
                    {t('hitTesting.bestMatchSource')}
                  </div>
                  <div className="text-sm font-bold truncate">
                    {bestRecord?.segment?.document?.name || t('common.notAvailable')}
                  </div>
                </div>
              </div>
            </div>

            {/* Retrieval Results */}
            <div className="pb-4">
              <h3 className="text-sm font-medium text-muted-foreground mb-3">
                {t('hitTesting.retrievalResults')}
              </h3>
              <div className="space-y-3">
                {records.map((record, index) => {
                  const matchType = record.match_type as 'original' | 'question' | undefined;
                  const matchTypeText = matchType
                    ? t(`hitTesting.matchTypes.${matchType}`)
                    : t('common.notAvailable');

                  return (
                    <Card
                      key={getRetrievalRecordKey(record, index)}
                      className={cn(
                        'border-neutral-200 shadow-sm overflow-hidden',
                        index === 0 && 'ring-1 ring-blue-500/20 border-blue-200'
                      )}
                    >
                      <CardContent className="p-0">
                        <div className="p-4 bg-white">
                          <div className="flex items-center justify-between mb-4">
                            <div className="flex items-center gap-3">
                              <div className="size-7 rounded-full flex items-center justify-center text-xs font-bold bg-neutral-100 text-neutral-600 border border-neutral-200">
                                {index + 1}
                              </div>
                              <div className="flex-1 min-w-0">
                                <div className="text-sm font-bold truncate">
                                  {record.segment?.document?.name || t('common.notAvailable')}
                                </div>
                                <div className="text-[11px] text-muted-foreground mt-0.5 font-medium uppercase tracking-tight">
                                  {matchTypeText} • {t('hitTesting.segmentLabel')}{' '}
                                  {record.segment?.position || t('common.notAvailable')}
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-4 text-right">
                              <div className="space-y-0.5">
                                <div className="text-[10px] text-muted-foreground uppercase tracking-wider font-bold">
                                  {t('hitTesting.similarity')}
                                </div>
                                <div className="text-xs font-bold text-blue-600 bg-blue-50 px-2 py-0.5 rounded-md">
                                  {record.score?.toFixed(3) || t('common.notAvailable')}
                                </div>
                              </div>
                              <div className="space-y-0.5">
                                <div className="text-[10px] text-muted-foreground uppercase tracking-wider font-bold">
                                  {t('hitTesting.responseTime')}
                                </div>
                                <div className="text-xs font-bold text-neutral-600 bg-neutral-50 px-2 py-0.5 rounded-md">
                                  {(resultData.result?.elapsed_time / 1000).toFixed(2)}s
                                </div>
                              </div>
                            </div>
                          </div>

                          <div className="pt-4 border-t border-neutral-100">
                            <p className="text-sm text-neutral-600 leading-relaxed font-medium">
                              {record.segment?.content || t('common.notAvailable')}
                            </p>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  );
                })}
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="px-8 font-semibold"
          >
            {t('close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

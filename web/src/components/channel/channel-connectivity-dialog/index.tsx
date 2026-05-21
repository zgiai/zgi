'use client';

import React, { useEffect, useMemo, useState, useCallback } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { ModelIcon } from 'modelicons';
import { Info } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useBatchTestChannelModels, useChannel } from '@/hooks';
import type { BatchTestModelResult, ChannelDetail } from '@/services/types/channel';
import {
  getChannelLatencies,
  getModelLatency,
  saveModelLatency,
  classifyFailure,
  type ModelLatencyRecord,
} from '@/utils/channel-latency';

interface ChannelConnectivityDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: ChannelDetail | null;
}

type DisplayStatus =
  | { kind: 'success'; ms: number }
  | { kind: 'connectionFailed' }
  | { kind: 'connectionTimeout' }
  | { kind: 'notTested' };

function toDisplay(record: ModelLatencyRecord | null): DisplayStatus {
  if (!record) return { kind: 'notTested' };
  if (record.status === 'success') return { kind: 'success', ms: record.lastMs };
  if (record.status === 'connectionTimeout') return { kind: 'connectionTimeout' };
  return { kind: 'connectionFailed' };
}

export default function ChannelConnectivityDialog(
  props: ChannelConnectivityDialogProps
): JSX.Element | null {
  const { open, onOpenChange, channel } = props;
  const t = useT('channels');

  const channelId = channel?.id;
  const { channel: detail, isLoading } = useChannel(channelId);

  const models = useMemo(() => {
    const source = detail?.models ?? channel?.models ?? [];
    return Array.isArray(source) ? source : [];
  }, [detail?.models, channel?.models]);

  const [selected, setSelected] = useState<Record<string, boolean>>({});

  useEffect(() => {
    // Initialize selection to all when list changes
    const init: Record<string, boolean> = {};
    models.forEach(m => (init[m] = false));
    setSelected(init);
  }, [models]);

  const [latencyMap, setLatencyMap] = useState<Record<string, ModelLatencyRecord>>({});

  useEffect(() => {
    if (!channelId) return;
    const map = getChannelLatencies(channelId);
    setLatencyMap(map);
  }, [channelId, open]);

  const handleTestResult = useCallback(
    (result: BatchTestModelResult) => {
      if (!channelId) return;
      const status = result.success ? 'success' : classifyFailure(result.message);
      const payload: ModelLatencyRecord = {
        lastMs: result.response_time_ms,
        at: Date.now(),
        status,
        message: result.message,
      };
      saveModelLatency(channelId, result.model, payload);
      setLatencyMap(prev => ({ ...prev, [result.model]: payload }));
    },
    [channelId]
  );

  const batchTestOptions = useMemo(
    () => ({
      onResult: handleTestResult,
    }),
    [handleTestResult]
  );

  const { batchTest, abort, isRunning, results, completedResult } =
    useBatchTestChannelModels(batchTestOptions);

  const allChecked = useMemo(() => {
    if (!models.length) return false;
    return models.every(m => selected[m]);
  }, [models, selected]);

  const anyChecked = useMemo(() => models.some(m => selected[m]), [models, selected]);

  const toggleAll = useCallback(
    (checked: boolean) => {
      const next: Record<string, boolean> = {};
      models.forEach(m => (next[m] = checked));
      setSelected(next);
    },
    [models]
  );

  const toggleOne = useCallback((model: string, checked: boolean) => {
    setSelected(prev => ({ ...prev, [model]: checked }));
  }, []);

  const runTest = useCallback(
    (scope: 'all' | 'selected') => {
      if (!channelId) return;
      const targets = scope === 'all' ? models : models.filter(m => selected[m]);
      if (!targets.length) return;
      batchTest(channelId, { models: targets });
    },
    [batchTest, channelId, models, selected]
  );

  if (!open) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('connectivityTest.title')}
          </DialogTitle>
          <DialogDescription className="text-neutral-500">
            {t('connectivityTest.description')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="py-6 space-y-6">
          <div className="flex items-center justify-between">
            <div className="text-sm font-medium text-neutral-500">
              {isRunning
                ? t('connectivityTest.testing')
                : completedResult
                  ? t('connectivityTest.completed')
                  : ''}
            </div>
            <div className="flex items-center gap-3">
              <Button
                variant="outline"
                onClick={() => runTest('selected')}
                disabled={!anyChecked || isRunning}
                className="h-10 rounded-xl px-4 border-neutral-200 hover:bg-neutral-50 transition-all"
              >
                {t('connectivityTest.buttons.testSelected')}
              </Button>
              <Button
                onClick={() => runTest('all')}
                disabled={!models.length || isRunning}
                className="h-10 rounded-xl px-6 shadow-premium transition-all active:scale-95"
              >
                {t('connectivityTest.buttons.testAll')}
              </Button>
              {isRunning && (
                <Button
                  variant="ghost"
                  onClick={abort}
                  className="h-10 rounded-xl px-4 text-red-600 hover:text-red-700 hover:bg-red-50"
                >
                  {t('connectivityTest.buttons.abort')}
                </Button>
              )}
            </div>
          </div>

          <div className="border rounded-2xl overflow-hidden bg-white shadow-sm border-neutral-100 transition-all hover:shadow-md">
            <div className="flex items-center gap-2 px-4 py-3 border-b bg-neutral-50/50">
              <Checkbox
                checked={allChecked}
                onCheckedChange={v => toggleAll(Boolean(v))}
                className="rounded-md border-neutral-300 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
              />
              <span className="text-xs font-bold text-neutral-400 tracking-wider uppercase">
                {t('connectivityTest.columns.model')}
              </span>
            </div>
            <div className="max-h-[380px] overflow-auto">
              <div className="divide-y divide-neutral-100">
                {isLoading
                  ? Array.from({ length: 6 }).map((_, i) => (
                      <div key={i} className="flex items-center gap-4 px-4 py-3">
                        <Skeleton className="h-5 w-5 rounded-md" />
                        <Skeleton className="h-5 w-48 rounded-lg" />
                        <div className="ml-auto">
                          <Skeleton className="h-5 w-20 rounded-lg" />
                        </div>
                      </div>
                    ))
                  : models.map(model => {
                      const record =
                        getModelLatency(channelId || '', model) ?? latencyMap[model] ?? null;
                      const display = toDisplay(record);
                      const color =
                        display.kind === 'success'
                          ? 'text-emerald-600'
                          : display.kind === 'connectionTimeout'
                            ? 'text-amber-600'
                            : display.kind === 'connectionFailed'
                              ? 'text-red-600'
                              : 'text-neutral-400';
                      const text =
                        display.kind === 'success'
                          ? `${display.ms} ms`
                          : display.kind === 'connectionTimeout'
                            ? t('connectivityTest.status.connectionTimeout')
                            : display.kind === 'connectionFailed'
                              ? t('connectivityTest.status.connectionFailed')
                              : t('connectivityTest.status.notTested');
                      const errText = record?.error || record?.message || '';
                      return (
                        <div
                          key={model}
                          className="flex items-center gap-4 px-4 py-3 hover:bg-neutral-50/80 transition-colors group"
                        >
                          <Checkbox
                            checked={Boolean(selected[model])}
                            onCheckedChange={v => toggleOne(model, Boolean(v))}
                            className="rounded-md border-neutral-300 data-[state=checked]:bg-blue-600 data-[state=checked]:border-blue-600"
                          />
                          <div className="flex items-center gap-3">
                            <div className="p-1.5 rounded-lg bg-neutral-100 group-hover:bg-white border border-transparent group-hover:border-neutral-200 transition-all shadow-sm">
                              <ModelIcon model={model} size={18} />
                            </div>
                            <div className="text-sm font-medium text-neutral-700 tracking-tight">
                              {model}
                            </div>
                          </div>
                          <div
                            className={`text-sm ml-auto ${color} font-bold flex items-center gap-2 tabular-nums`}
                          >
                            <span>{text}</span>
                            {display.kind !== 'success' && errText ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span className="inline-flex items-center cursor-help">
                                    <Info className="h-4 w-4 opacity-40 hover:opacity-100 transition-opacity" />
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent
                                  side="top"
                                  className="max-w-[320px] bg-neutral-900 text-white border-neutral-800 p-3 rounded-xl shadow-premium"
                                >
                                  <p className="text-xs leading-relaxed">{errText}</p>
                                </TooltipContent>
                              </Tooltip>
                            ) : null}
                          </div>
                        </div>
                      );
                    })}
              </div>
            </div>
          </div>

          {completedResult && (
            <div className="bg-blue-50/50 rounded-xl p-4 border border-blue-100/50 flex items-center gap-3 animate-in fade-in slide-in-from-bottom-2 duration-300">
              <div className="p-2 bg-blue-100 rounded-lg">
                <Info className="h-4 w-4 text-blue-600" />
              </div>
              <div className="text-xs font-medium text-blue-800 leading-relaxed">
                {t('connectivityTest.summary', {
                  total: results.length || (completedResult.total_tests ?? 0),
                  success:
                    results.length > 0
                      ? results.filter(r => r.success).length
                      : (completedResult.success_count ?? 0),
                  failure:
                    results.length > 0
                      ? results.filter(r => !r.success).length
                      : (completedResult.failure_count ?? 0),
                })}
              </div>
            </div>
          )}
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button
            variant="ghost"
            onClick={() => onOpenChange(false)}
            className="font-bold rounded-xl h-11 px-8 hover:bg-neutral-100"
          >
            {t('actions.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

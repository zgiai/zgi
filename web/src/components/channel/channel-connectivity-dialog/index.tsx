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
import { Switch } from '@/components/ui/switch';
import { ModelIcon } from 'modelicons';
import { ExternalLink, Info, Trash2 } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useBatchTestChannelModels, useChannel, useUpdateChannel } from '@/hooks';
import type {
  BatchTestModelResult,
  ChannelDetail,
  ChannelModelTestStatus,
  ChannelModelTestParams,
} from '@/services/types/channel';
import {
  getChannelLatencies,
  getModelLatency,
  removeModelLatencies,
  saveModelLatency,
  classifyFailure,
  type ModelLatencyRecord,
} from '@/utils/channel-latency';
import { toast } from 'sonner';
import { useRouter } from 'next/navigation';

interface ChannelConnectivityDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: ChannelDetail | null;
}

type DisplayStatus =
  | { kind: 'success'; ms: number }
  | { kind: 'connectionFailed' }
  | { kind: 'connectionTimeout' }
  | { kind: 'skipped' }
  | { kind: 'notTested' };

function toDisplay(record: ModelLatencyRecord | null): DisplayStatus {
  if (!record) return { kind: 'notTested' };
  if (record.status === 'success') return { kind: 'success', ms: record.lastMs };
  if (record.status === 'connectionTimeout') return { kind: 'connectionTimeout' };
  return { kind: 'connectionFailed' };
}

function getResultStatus(result: BatchTestModelResult | undefined): ChannelModelTestStatus | null {
  if (!result) return null;
  if (result.status === 'success' || result.status === 'failed' || result.status === 'skipped') {
    return result.status;
  }
  return result.success ? 'success' : 'failed';
}

const modelPricingNotConfiguredCode = 'model_pricing_not_configured';

function isPricingNotConfiguredResult(result: BatchTestModelResult | undefined): boolean {
  return result?.code === modelPricingNotConfiguredCode;
}

function stringParam(params: ChannelModelTestParams | undefined, key: string): string {
  const value = params?.[key];
  return typeof value === 'string' ? value.trim() : '';
}

export default function ChannelConnectivityDialog(
  props: ChannelConnectivityDialogProps
): JSX.Element | null {
  const { open, onOpenChange, channel } = props;
  const t = useT('channels');
  const router = useRouter();

  const channelId = channel?.id;
  const { channel: detail, isLoading } = useChannel(channelId);
  const { updateChannel, isUpdating } = useUpdateChannel();
  const isOfficial = Boolean(detail?.is_official ?? channel?.is_official);

  const buildPricingURL = useCallback(
    (result: BatchTestModelResult | undefined, fallbackModel: string) => {
      const providerSlug =
        stringParam(result?.params, 'provider') ||
        detail?.provider ||
        detail?.channel_provider ||
        channel?.provider ||
        channel?.channel_provider;
      const model = stringParam(result?.params, 'model') || fallbackModel;
      if (!providerSlug || !model) {
        return '/dashboard/settings/pricing';
      }
      return `/dashboard/provider/${encodeURIComponent(providerSlug)}?pricing=1&model=${encodeURIComponent(model)}`;
    },
    [channel?.channel_provider, channel?.provider, detail?.channel_provider, detail?.provider]
  );

  const models = useMemo(() => {
    const source = detail?.models ?? channel?.models ?? [];
    return Array.isArray(source) ? source : [];
  }, [detail?.models, channel?.models]);

  const [selected, setSelected] = useState<Record<string, boolean>>({});
  const [stream, setStream] = useState(false);

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
      const resultStatus = getResultStatus(result);
      if (resultStatus === 'skipped') {
        removeModelLatencies(channelId, [result.model]);
        setLatencyMap(prev => {
          const next = { ...prev };
          delete next[result.model];
          return next;
        });
        return;
      }
      const status = resultStatus === 'success' ? 'success' : classifyFailure(result.message);
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

  const { batchTest, abort, reset, isRunning, results, completedResult } =
    useBatchTestChannelModels(batchTestOptions);

  useEffect(() => {
    if (open) reset();
  }, [channelId, open, reset]);

  const currentResultsByModel = useMemo(() => {
    const map: Record<string, BatchTestModelResult> = {};
    results.forEach(result => {
      map[result.model] = result;
    });
    return map;
  }, [results]);

  const failedModels = useMemo(
    () => models.filter(model => getResultStatus(currentResultsByModel[model]) === 'failed'),
    [currentResultsByModel, models]
  );

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
      batchTest(channelId, { models: targets, stream });
    },
    [batchTest, channelId, models, selected, stream]
  );

  const removeModels = useCallback(
    async (targets: string[]) => {
      if (!channelId || isOfficial || targets.length === 0) return;
      const targetSet = new Set(targets);
      const nextModels = models.filter(model => !targetSet.has(model));
      if (nextModels.length === 0) {
        toast.error(t('connectivityTest.toast.removeAllBlocked'));
        return;
      }
      await updateChannel(channelId, { models: nextModels });
      removeModelLatencies(channelId, targets);
      setLatencyMap(prev => {
        const next = { ...prev };
        targets.forEach(model => {
          delete next[model];
        });
        return next;
      });
      setSelected(prev => {
        const next = { ...prev };
        targets.forEach(model => {
          delete next[model];
        });
        return next;
      });
    },
    [channelId, isOfficial, models, t, updateChannel]
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
              <label className="flex items-center gap-2 text-sm font-medium text-neutral-600">
                <Switch checked={stream} disabled={isRunning} onCheckedChange={setStream} />
                <span>{t('connectivityTest.stream')}</span>
              </label>
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
                      const currentResult = currentResultsByModel[model];
                      const currentStatus = getResultStatus(currentResult);
                      const record =
                        getModelLatency(channelId || '', model) ?? latencyMap[model] ?? null;
                      const failedInCurrentRun = currentStatus === 'failed';
                      const skippedInCurrentRun = currentStatus === 'skipped';
                      const pricingSkippedInCurrentRun =
                        skippedInCurrentRun && isPricingNotConfiguredResult(currentResult);
                      const display = skippedInCurrentRun
                        ? { kind: 'skipped' as const }
                        : toDisplay(record);
                      const color =
                        display.kind === 'success'
                          ? 'text-emerald-600'
                          : display.kind === 'skipped'
                            ? 'text-neutral-500'
                            : display.kind === 'connectionTimeout'
                              ? 'text-amber-600'
                              : display.kind === 'connectionFailed'
                                ? 'text-red-600'
                                : 'text-neutral-400';
                      const text =
                        display.kind === 'success'
                          ? `${display.ms} ms`
                          : display.kind === 'skipped'
                            ? pricingSkippedInCurrentRun
                              ? t('connectivityTest.status.pricingNotConfigured')
                              : t('connectivityTest.status.skipped')
                            : display.kind === 'connectionTimeout'
                              ? t('connectivityTest.status.connectionTimeout')
                              : display.kind === 'connectionFailed'
                                ? t('connectivityTest.status.connectionFailed')
                                : t('connectivityTest.status.notTested');
                      const errText = skippedInCurrentRun
                        ? pricingSkippedInCurrentRun
                          ? t('connectivityTest.pricingNotConfiguredHint')
                          : t('connectivityTest.imageSkippedHint')
                        : currentResult?.message || record?.error || record?.message || '';
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
                            {!isOfficial && failedInCurrentRun ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                disabled={isRunning || isUpdating}
                                onClick={() => void removeModels([model])}
                                className="h-7 px-2 text-xs text-red-600 hover:bg-red-50 hover:text-red-700"
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                                {t('connectivityTest.buttons.remove')}
                              </Button>
                            ) : null}
                            {pricingSkippedInCurrentRun ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                disabled={isRunning}
                                onClick={() => {
                                  onOpenChange(false);
                                  router.push(buildPricingURL(currentResult, model));
                                }}
                                className="h-7 px-2 text-xs text-blue-600 hover:bg-blue-50 hover:text-blue-700"
                              >
                                <ExternalLink className="h-3.5 w-3.5" />
                                {t('connectivityTest.buttons.setPrice')}
                              </Button>
                            ) : skippedInCurrentRun ? (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                disabled={isRunning}
                                onClick={() => {
                                  onOpenChange(false);
                                  router.push('/console/work/image');
                                }}
                                className="h-7 px-2 text-xs text-blue-600 hover:bg-blue-50 hover:text-blue-700"
                              >
                                <ExternalLink className="h-3.5 w-3.5" />
                                {t('connectivityTest.buttons.testImage')}
                              </Button>
                            ) : null}
                          </div>
                        </div>
                      );
                    })}
              </div>
            </div>
          </div>

          {completedResult && (
            <div className="bg-blue-50/50 rounded-xl p-4 border border-blue-100/50 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between animate-in fade-in slide-in-from-bottom-2 duration-300">
              <div className="flex items-center gap-3">
                <div className="p-2 bg-blue-100 rounded-lg">
                  <Info className="h-4 w-4 text-blue-600" />
                </div>
                <div className="text-xs font-medium text-blue-800 leading-relaxed">
                  {t('connectivityTest.summary', {
                    total: results.length || (completedResult.total_tests ?? 0),
                    success:
                      results.length > 0
                        ? results.filter(r => getResultStatus(r) === 'success').length
                        : (completedResult.success_count ?? 0),
                    failure:
                      results.length > 0
                        ? results.filter(r => getResultStatus(r) === 'failed').length
                        : (completedResult.failure_count ?? 0),
                    skipped:
                      results.length > 0
                        ? results.filter(r => getResultStatus(r) === 'skipped').length
                        : (completedResult.skipped_count ?? 0),
                  })}
                </div>
              </div>
              {!isOfficial && failedModels.length > 0 ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={isRunning || isUpdating}
                  onClick={() => void removeModels(failedModels)}
                  className="h-8 shrink-0 border-red-200 text-xs text-red-600 hover:bg-red-50 hover:text-red-700"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  {t('connectivityTest.buttons.removeFailed', { count: failedModels.length })}
                </Button>
              ) : null}
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

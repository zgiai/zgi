'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { AlertCircle, FileText, Search } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  useCreateDatasetFileRefs,
  useDatasetFileCandidates,
  useDatasetFileCandidateEmbeddingTasks,
  useGenerateDatasetFileCandidateEmbeddings,
} from '@/hooks/dataset/use-dataset-file-refs';
import type { DatasetFileCandidate } from '@/services/types/dataset';
import type { FileProcessingRequestView } from '@/services/types/file';
import { cn } from '@/lib/utils';
import { formatDate, formatFileSize } from '@/utils/format';

interface DatasetFileAssetDialogProps {
  datasetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmitted?: () => void;
}

type CandidateFilter = 'addable' | 'added' | 'blocked';

const FILTERS: CandidateFilter[] = ['addable', 'added', 'blocked'];
const TERMINAL_EMBEDDING_TASK_STATUSES = new Set(['completed', 'failed', 'cancelled']);

function candidateReasonKey(reason?: string) {
  switch (reason) {
    case 'already_added':
      return 'documents.fileAssets.reasons.alreadyAdded';
    case 'embedding_model_mismatch':
      return 'documents.fileAssets.reasons.embeddingModelMismatch';
    case 'missing_chunks':
      return 'documents.fileAssets.reasons.missingChunks';
    case 'missing_embedding':
      return 'documents.fileAssets.reasons.missingEmbedding';
    case 'missing_dataset_embedding':
      return 'documents.fileAssets.reasons.missingDatasetEmbedding';
    case 'dataset_embedding_model_missing':
      return 'documents.fileAssets.reasons.datasetEmbeddingModelMissing';
    case 'not_ready':
      return 'documents.fileAssets.reasons.notReady';
    default:
      return 'documents.fileAssets.reasons.unavailable';
  }
}

function fileExtension(candidate: DatasetFileCandidate) {
  if (candidate.file_extension) return candidate.file_extension.toLowerCase();
  const ext = candidate.name.split('.').pop();
  return ext && ext !== candidate.name ? ext.toLowerCase() : 'file';
}

function matchesFilter(candidate: DatasetFileCandidate, filter: CandidateFilter) {
  if (filter === 'addable') return candidate.addable || candidate.requires_embedding_generation;
  if (filter === 'added') return candidate.already_added;
  return !candidate.addable && !candidate.requires_embedding_generation && !candidate.already_added;
}

function processingStatusLabel(t: ReturnType<typeof useT<'datasets'>>, status: string) {
  switch (status) {
    case 'ready':
      return t('documents.fileAssets.processingStatus.ready');
    case 'stored_only':
      return t('documents.fileAssets.processingStatus.storedOnly');
    case 'parsing':
      return t('documents.fileAssets.processingStatus.parsing');
    case 'confirming':
      return t('documents.fileAssets.processingStatus.confirming');
    case 'generating':
      return t('documents.fileAssets.processingStatus.generating');
    case 'parse_failed':
      return t('documents.fileAssets.processingStatus.parseFailed');
    default:
      return status || '-';
  }
}

function numberFromMetadata(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function embeddingTaskProgress(request?: FileProcessingRequestView) {
  const metadata = request?.execution_metadata ?? {};
  const completed = numberFromMetadata(metadata.progress_completed);
  const total = numberFromMetadata(metadata.progress_total);
  if (completed === undefined || total === undefined || total <= 0) {
    return undefined;
  }
  return {
    completed: Math.min(completed, total),
    total,
  };
}

function embeddingTaskLabel(
  t: ReturnType<typeof useT<'datasets'>>,
  request?: FileProcessingRequestView
) {
  if (!request) return t('documents.fileAssets.generateDatasetEmbeddingQueued');
  if (request.status === 'failed' || request.status === 'cancelled') {
    return t('documents.fileAssets.generateDatasetEmbeddingFailed');
  }
  const progress = embeddingTaskProgress(request);
  if (progress) {
    return t('documents.fileAssets.generateDatasetEmbeddingProgress', progress);
  }
  return t('documents.fileAssets.generateDatasetEmbeddingQueued');
}

export function DatasetFileAssetDialog({
  datasetId,
  open,
  onOpenChange,
  onSubmitted,
}: DatasetFileAssetDialogProps) {
  const t = useT('datasets');
  const [selectedAssetIds, setSelectedAssetIds] = useState<string[]>([]);
  const [activeFilter, setActiveFilter] = useState<CandidateFilter>('addable');
  const [batchGeneratingAssetIds, setBatchGeneratingAssetIds] = useState<Set<string>>(
    () => new Set()
  );
  const [queuedEmbeddingTasks, setQueuedEmbeddingTasks] = useState<Map<string, string>>(
    () => new Map()
  );
  const hasQueuedEmbeddingGeneration =
    queuedEmbeddingTasks.size > 0 || batchGeneratingAssetIds.size > 0;
  const createRefsMutation = useCreateDatasetFileRefs(datasetId);
  const generateEmbeddingsMutation = useGenerateDatasetFileCandidateEmbeddings(datasetId);
  const { candidates, total, keyword, setKeyword, isLoading, isFetching, refetch } =
    useDatasetFileCandidates(
      datasetId,
      { filter: 'all', limit: 100 },
      {
        enabled: open,
        debounceDelay: 300,
        refetchInterval: hasQueuedEmbeddingGeneration ? 3000 : false,
      }
    );
  const embeddingTaskRefs = useMemo(
    () =>
      Array.from(queuedEmbeddingTasks.entries()).map(([assetId, requestId]) => ({
        assetId,
        requestId,
      })),
    [queuedEmbeddingTasks]
  );
  const embeddingTasksByAssetId = useDatasetFileCandidateEmbeddingTasks(
    datasetId,
    embeddingTaskRefs,
    {
      enabled: open && embeddingTaskRefs.length > 0,
      refetchInterval: 2000,
    }
  );

  useEffect(() => {
    if (!open) {
      setSelectedAssetIds([]);
      setKeyword('');
      setActiveFilter('addable');
      setBatchGeneratingAssetIds(new Set());
      setQueuedEmbeddingTasks(new Map());
    }
  }, [open, setKeyword]);

  useEffect(() => {
    if (queuedEmbeddingTasks.size === 0) return;

    const candidatesByAssetId = new Map(candidates.map(candidate => [candidate.asset_id, candidate]));
    const nextQueued = new Map(queuedEmbeddingTasks);
    let changed = false;
    queuedEmbeddingTasks.forEach((requestId, assetId) => {
      const candidate = candidatesByAssetId.get(assetId);
      const task = embeddingTasksByAssetId.get(assetId);
      const taskDone = task && TERMINAL_EMBEDDING_TASK_STATUSES.has(String(task.status));
      if (taskDone || (candidate && candidate.requires_embedding_generation !== true)) {
        nextQueued.delete(assetId);
        changed = true;
      }
      if (!requestId) {
        nextQueued.delete(assetId);
        changed = true;
      }
    });

    if (changed) {
      setQueuedEmbeddingTasks(nextQueued);
    }
  }, [candidates, embeddingTasksByAssetId, queuedEmbeddingTasks]);

  useEffect(() => {
    const hasTerminalTask = Array.from(embeddingTasksByAssetId.values()).some(task =>
      TERMINAL_EMBEDDING_TASK_STATUSES.has(String(task.status))
    );
    if (hasTerminalTask) {
      void refetch();
    }
  }, [embeddingTasksByAssetId, refetch]);

  const summary = useMemo(
    () =>
      candidates.reduce(
        (acc, candidate) => {
          if (candidate.addable || candidate.requires_embedding_generation) acc.addable += 1;
          else if (candidate.already_added) acc.added += 1;
          else acc.blocked += 1;
          return acc;
        },
        { addable: 0, added: 0, blocked: 0 }
      ),
    [candidates]
  );
  const visibleCandidates = useMemo(
    () => candidates.filter(candidate => matchesFilter(candidate, activeFilter)),
    [activeFilter, candidates]
  );
  const selectedSet = useMemo(() => new Set(selectedAssetIds), [selectedAssetIds]);
  const visibleAddableIds = useMemo(
    () => visibleCandidates.filter(item => item.addable).map(item => item.asset_id),
    [visibleCandidates]
  );
  const embeddingGenerationCandidateIds = useMemo(
    () =>
      candidates
        .filter(candidate => candidate.requires_embedding_generation === true)
        .map(candidate => candidate.asset_id),
    [candidates]
  );
  const allVisibleAddableSelected =
    visibleAddableIds.length > 0 && visibleAddableIds.every(id => selectedSet.has(id));
  const isBatchGenerating = batchGeneratingAssetIds.size > 0;
  const activeEmbeddingAssetId =
    typeof generateEmbeddingsMutation.variables === 'string'
      ? generateEmbeddingsMutation.variables
      : generateEmbeddingsMutation.variables?.assetId;

  const toggleCandidate = useCallback((candidate: DatasetFileCandidate, checked: boolean) => {
    if (!candidate.addable) return;
    setSelectedAssetIds(prev =>
      checked
        ? Array.from(new Set([...prev, candidate.asset_id]))
        : prev.filter(id => id !== candidate.asset_id)
    );
  }, []);

  const toggleCandidateSelection = useCallback((candidate: DatasetFileCandidate) => {
    if (!candidate.addable) return;
    setSelectedAssetIds(prev =>
      prev.includes(candidate.asset_id)
        ? prev.filter(id => id !== candidate.asset_id)
        : [...prev, candidate.asset_id]
    );
  }, []);

  const toggleAllVisible = useCallback(
    (checked: boolean) => {
      setSelectedAssetIds(prev =>
        checked
          ? Array.from(new Set([...prev, ...visibleAddableIds]))
          : prev.filter(id => !visibleAddableIds.includes(id))
      );
    },
    [visibleAddableIds]
  );

  const handleConfirm = useCallback(async () => {
    if (selectedAssetIds.length === 0) return;
    try {
      await createRefsMutation.mutateAsync(selectedAssetIds);
      onSubmitted?.();
      onOpenChange(false);
    } catch {
      // The mutation hook already shows the API error toast. Keep the dialog open for retry.
    }
  }, [createRefsMutation, onOpenChange, onSubmitted, selectedAssetIds]);

  const handleGenerateEmbeddings = useCallback(
    async (candidate: DatasetFileCandidate) => {
      try {
        const response = await generateEmbeddingsMutation.mutateAsync(candidate.asset_id);
        const requestID = response.data?.processing_request?.id;
        if (response.data?.accepted && requestID) {
          setQueuedEmbeddingTasks(prev => {
            const next = new Map(prev);
            next.set(candidate.asset_id, requestID);
            return next;
          });
        }
      } catch {
        // The mutation hook already shows the API error toast.
      }
    },
    [generateEmbeddingsMutation]
  );

  const handleBatchGenerateEmbeddings = useCallback(async () => {
    if (embeddingGenerationCandidateIds.length === 0 || hasQueuedEmbeddingGeneration) return;

    const assetIds = [...embeddingGenerationCandidateIds];
    setBatchGeneratingAssetIds(new Set(assetIds));

    let successCount = 0;
    let failedCount = 0;

    try {
      for (const assetId of assetIds) {
        try {
          const response = await generateEmbeddingsMutation.mutateAsync({
            assetId,
            silent: true,
          });
          if (response.data?.accepted) {
            const requestID = response.data.processing_request?.id;
            if (requestID) {
              setQueuedEmbeddingTasks(prev => {
                const next = new Map(prev);
                next.set(assetId, requestID);
                return next;
              });
            }
            successCount += 1;
          } else if (response.data?.addable) {
            successCount += 1;
          } else {
            failedCount += 1;
          }
        } catch {
          failedCount += 1;
        }
      }

      if (successCount > 0) {
        toast.success(
          t('messages.fileCandidateEmbeddingBatchGenerateSuccess', { count: successCount })
        );
      }
      if (failedCount > 0) {
        toast.error(
          t('messages.fileCandidateEmbeddingBatchGeneratePartialFailed', { count: failedCount })
        );
      }
    } finally {
      setBatchGeneratingAssetIds(new Set());
    }
  }, [
    embeddingGenerationCandidateIds,
    generateEmbeddingsMutation,
    hasQueuedEmbeddingGeneration,
    t,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="full"
        className="max-h-[calc(100vh-2rem)] gap-0 overflow-hidden p-0 sm:max-w-[calc(100vw-2.5rem)]"
      >
        <DialogHeader className="border-b px-5 py-4 pr-12">
          <DialogTitle>{t('documents.fileAssets.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('documents.fileAssets.dialogDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="flex min-h-0 flex-col overflow-hidden px-0 py-0">
          <div className="border-b px-5 py-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <div className="text-lg font-semibold">
                  {t('documents.fileAssets.total', { count: total })}
                </div>
                <div className="mt-1 text-sm text-muted-foreground">
                  {t('documents.fileAssets.sourceSummary', {
                    addable: summary.addable,
                    blocked: summary.blocked,
                    added: summary.added,
                  })}
                </div>
              </div>
              <div className="flex max-w-full flex-wrap items-center gap-2">
                <div className="relative w-[360px] max-w-full">
                  <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    type="search"
                    value={keyword}
                    onChange={event => setKeyword(event.target.value)}
                    placeholder={t('documents.fileAssets.searchPlaceholder')}
                    className="h-10 rounded-lg pl-10"
                  />
                </div>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-2">
              {FILTERS.map(filter => (
                <Button
                  key={filter}
                  type="button"
                  variant={activeFilter === filter ? 'default' : 'outline'}
                  className={cn(
                    'h-9 rounded-lg px-4',
                    activeFilter === filter ? '' : 'bg-background'
                  )}
                  onClick={() => setActiveFilter(filter)}
                >
                  {t(`documents.fileAssets.filters.${filter}`)}
                </Button>
              ))}
            </div>

            {embeddingGenerationCandidateIds.length > 0 ? (
              <div className="mt-3 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-sm">
                <div className="flex min-w-0 items-center gap-2 text-destructive">
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  <span className="min-w-0">
                    {t('documents.fileAssets.embeddingGenerationNotice', {
                      count: embeddingGenerationCandidateIds.length,
                    })}
                  </span>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 whitespace-nowrap border-destructive/30 bg-background text-destructive hover:bg-destructive/10 hover:text-destructive"
                  loading={isBatchGenerating}
                  disabled={hasQueuedEmbeddingGeneration || generateEmbeddingsMutation.isPending}
                  onClick={handleBatchGenerateEmbeddings}
                >
                  {t('documents.fileAssets.batchGenerateDatasetEmbedding', {
                    count: embeddingGenerationCandidateIds.length,
                  })}
                </Button>
              </div>
            ) : null}
          </div>

          <div className="min-h-0 flex-1 overflow-auto p-5">
            <div className="overflow-hidden rounded-xl border">
              <Table className="min-w-[1280px] table-fixed">
                <colgroup>
                  <col className="w-[44px]" />
                  <col />
                  <col className="w-[132px]" />
                  <col className="w-[260px]" />
                  <col className="w-[112px]" />
                  <col className="w-[150px]" />
                  <col className="w-[160px]" />
                  <col className="w-[170px]" />
                </colgroup>
                <TableHeader className="bg-muted/40">
                  <TableRow className="hover:bg-muted/40">
                    <TableHead className="px-3">
                      <Checkbox
                        checked={allVisibleAddableSelected}
                        disabled={visibleAddableIds.length === 0}
                        onCheckedChange={checked => toggleAllVisible(checked === true)}
                        aria-label={t('documents.fileAssets.selectAll')}
                      />
                    </TableHead>
                    <TableHead>{t('documents.fileAssets.fileName')}</TableHead>
                    <TableHead>{t('documents.fileAssets.status')}</TableHead>
                    <TableHead>{t('documents.fileAssets.availability')}</TableHead>
                    <TableHead>{t('documents.fileAssets.chunks')}</TableHead>
                    <TableHead>{t('documents.fileAssets.references')}</TableHead>
                    <TableHead>{t('documents.fileAssets.updatedAt')}</TableHead>
                    <TableHead className="text-right">
                      {t('documents.fileAssets.actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {isLoading
                    ? Array.from({ length: 7 }).map((_, index) => (
                        <TableRow key={index}>
                          <TableCell colSpan={8}>
                            <Skeleton className="h-9 w-full" />
                          </TableCell>
                        </TableRow>
                      ))
                    : null}

                  {!isLoading && visibleCandidates.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="h-40 text-center text-muted-foreground">
                        {t('documents.fileAssets.empty')}
                      </TableCell>
                    </TableRow>
                  ) : null}

                  {!isLoading &&
                    visibleCandidates.map(candidate => {
                      const selected = selectedSet.has(candidate.asset_id);
                      const addable = candidate.addable;
                      const requiresEmbeddingGeneration =
                        candidate.requires_embedding_generation === true;
                      const needsFileManagement =
                        !requiresEmbeddingGeneration &&
                        !candidate.already_added &&
                        candidate.processing_status !== 'ready';
                      const ext = fileExtension(candidate);
                      const isGenerating =
                        queuedEmbeddingTasks.has(candidate.asset_id) ||
                        batchGeneratingAssetIds.has(candidate.asset_id) ||
                        (generateEmbeddingsMutation.isPending &&
                          activeEmbeddingAssetId === candidate.asset_id);
                      const embeddingTask = embeddingTasksByAssetId.get(candidate.asset_id);
                      const embeddingProgressLabel = embeddingTaskLabel(t, embeddingTask);
                      return (
                        <TableRow
                          key={candidate.asset_id}
                          data-state={selected ? 'selected' : undefined}
                          aria-selected={selected}
                          onClick={() => toggleCandidateSelection(candidate)}
                          className={cn(
                            'h-16 hover:bg-muted/30 data-[state=selected]:bg-primary/5',
                            addable &&
                              'cursor-pointer hover:bg-primary/5 data-[state=selected]:bg-primary/10',
                            requiresEmbeddingGeneration && 'bg-muted/20'
                          )}
                        >
                          <TableCell className="px-3" onClick={event => event.stopPropagation()}>
                            <Checkbox
                              checked={selected}
                              disabled={!addable}
                              onCheckedChange={checked =>
                                toggleCandidate(candidate, checked === true)
                              }
                              aria-label={candidate.name}
                            />
                          </TableCell>
                          <TableCell className="max-w-0">
                            <div className="flex min-w-0 items-center gap-3">
                              <FileText
                                className={cn(
                                  'h-5 w-5 shrink-0',
                                  ext === 'docx' ? 'text-primary' : 'text-destructive'
                                )}
                              />
                              <div className="min-w-0">
                                <div
                                  className={cn(
                                    'truncate text-sm font-medium',
                                    addable ? 'text-foreground' : 'text-muted-foreground'
                                  )}
                                  title={candidate.name}
                                >
                                  {candidate.name}
                                </div>
                                <div className="mt-0.5 truncate text-xs text-muted-foreground">
                                  {ext} · {formatFileSize(candidate.file_size)}
                                </div>
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge
                              variant={
                                candidate.processing_status === 'ready' ? 'success' : 'warning'
                              }
                            >
                              {processingStatusLabel(t, candidate.processing_status)}
                            </Badge>
                          </TableCell>
                          <TableCell className="max-w-0">
                            {candidate.addable ? (
                              <Badge variant="success">{t('documents.fileAssets.ready')}</Badge>
                            ) : (
                              <div className="min-w-0 space-y-1">
                                <Badge
                                  variant={
                                    requiresEmbeddingGeneration
                                      ? 'outline'
                                      : candidate.already_added
                                        ? 'subtle'
                                        : 'warning'
                                  }
                                  className={
                                    requiresEmbeddingGeneration
                                      ? 'border-destructive/30 bg-destructive/10 text-destructive'
                                      : undefined
                                  }
                                >
                                  {requiresEmbeddingGeneration ? (
                                    <AlertCircle className="h-3 w-3" />
                                  ) : null}
                                  <span>
                                    {candidate.already_added
                                      ? t('documents.fileAssets.reasons.alreadyAdded')
                                      : t(candidateReasonKey(candidate.reason))}
                                  </span>
                                </Badge>
                                {requiresEmbeddingGeneration ? (
                                  <div
                                    className="whitespace-normal break-words text-xs leading-4 text-muted-foreground"
                                    title={t(
                                      'documents.fileAssets.reasons.missingDatasetEmbeddingDetail',
                                      {
                                        model: candidate.target_embedding_model || '-',
                                      }
                                    )}
                                  >
                                    {isGenerating ? (
                                      <span className="font-semibold">{embeddingProgressLabel}</span>
                                    ) : (
                                      <>
                                        <span>
                                          {t(
                                            'documents.fileAssets.reasons.missingDatasetEmbeddingDetailPrefix',
                                            {
                                              model: candidate.target_embedding_model || '-',
                                            }
                                          )}
                                        </span>
                                        <span className="font-semibold">
                                          {t(
                                            'documents.fileAssets.reasons.missingDatasetEmbeddingAction'
                                          )}
                                        </span>
                                      </>
                                    )}
                                  </div>
                                ) : null}
                              </div>
                            )}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {candidate.addable ||
                            candidate.already_added ||
                            requiresEmbeddingGeneration
                              ? candidate.chunk_count
                              : '-'}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {candidate.reference_count && candidate.reference_count > 0
                              ? t('documents.fileAssets.referenceCount', {
                                  count: candidate.reference_count,
                                })
                              : t('documents.fileAssets.noReference')}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {candidate.updated_at ? formatDate(candidate.updated_at) : '-'}
                          </TableCell>
                          <TableCell
                            className="text-right"
                            onClick={event => event.stopPropagation()}
                          >
                            {requiresEmbeddingGeneration ? (
                              <Button
                                variant="outline"
                                size="sm"
                                className="h-8 px-2 text-xs"
                                loading={isGenerating}
                                disabled={
                                  isGenerating ||
                                  isBatchGenerating ||
                                  generateEmbeddingsMutation.isPending
                                }
                                onClick={() => handleGenerateEmbeddings(candidate)}
                              >
                                {isGenerating
                                  ? embeddingProgressLabel
                                  : t('documents.fileAssets.generateDatasetEmbedding')}
                              </Button>
                            ) : needsFileManagement ? (
                              <Button
                                asChild
                                variant="ghost"
                                size="sm"
                                className="h-8 px-2 text-xs"
                              >
                                <Link href={`/console/files/${candidate.file_id}`}>
                                  {t('documents.fileAssets.openFile')}
                                </Link>
                              </Button>
                            ) : (
                              <span className="text-xs text-muted-foreground">-</span>
                            )}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                </TableBody>
              </Table>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="border-t px-5 py-3">
          <div className="flex min-w-0 flex-1 items-center text-sm text-muted-foreground">
            {t('documents.fileAssets.selectedSummary', { count: selectedAssetIds.length })}
            {isFetching ? ` ${t('loading')}` : ''}
          </div>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            loading={createRefsMutation.isPending}
            disabled={selectedAssetIds.length === 0}
          >
            {t('documents.fileAssets.addSelected', { count: selectedAssetIds.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

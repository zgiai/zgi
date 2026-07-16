'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Link from 'next/link';
import { AlertCircle, Search } from 'lucide-react';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
import { FileTypeIcon } from '@/components/files/file-type-icon';

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
  const [partialAddConfirmOpen, setPartialAddConfirmOpen] = useState(false);
  const [activeFilter, setActiveFilter] = useState<CandidateFilter>('addable');
  const [batchGeneratingAssetIds, setBatchGeneratingAssetIds] = useState<Set<string>>(
    () => new Set()
  );
  const [autoSelectPendingAssetIds, setAutoSelectPendingAssetIds] = useState<Set<string>>(
    () => new Set()
  );
  const [autoAddPendingAssetIds, setAutoAddPendingAssetIds] = useState<Set<string>>(
    () => new Set()
  );
  const [isSelectionProcessing, setIsSelectionProcessing] = useState(false);
  const [queuedEmbeddingTasks, setQueuedEmbeddingTasks] = useState<Map<string, string>>(
    () => new Map()
  );
  const autoAddInFlightRef = useRef(false);
  const hasQueuedEmbeddingGeneration =
    queuedEmbeddingTasks.size > 0 ||
    batchGeneratingAssetIds.size > 0 ||
    autoSelectPendingAssetIds.size > 0 ||
    autoAddPendingAssetIds.size > 0;
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
      setPartialAddConfirmOpen(false);
      setAutoAddPendingAssetIds(new Set());
      setIsSelectionProcessing(false);
      autoAddInFlightRef.current = false;
      setKeyword('');
      setActiveFilter('addable');
      setBatchGeneratingAssetIds(new Set());
      setAutoSelectPendingAssetIds(new Set());
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

  useEffect(() => {
    if (!open || autoSelectPendingAssetIds.size === 0) return;

    const pendingIds = new Set(autoSelectPendingAssetIds);
    const addableAssetIds = candidates
      .filter(candidate => pendingIds.has(candidate.asset_id) && candidate.addable)
      .map(candidate => candidate.asset_id);
    const failedAssetIds = Array.from(embeddingTasksByAssetId.entries())
      .filter(([assetId, task]) => {
        if (!pendingIds.has(assetId)) return false;
        return task.status === 'failed' || task.status === 'cancelled';
      })
      .map(([assetId]) => assetId);

    if (addableAssetIds.length === 0 && failedAssetIds.length === 0) return;

    if (addableAssetIds.length > 0) {
      setSelectedAssetIds(prev => Array.from(new Set([...prev, ...addableAssetIds])));
    }

    const completedIds = new Set([...addableAssetIds, ...failedAssetIds]);
    setAutoSelectPendingAssetIds(
      new Set(Array.from(pendingIds).filter(assetId => !completedIds.has(assetId)))
    );
  }, [autoSelectPendingAssetIds, candidates, embeddingTasksByAssetId, open]);

  useEffect(() => {
    if (!open || autoAddPendingAssetIds.size === 0 || autoAddInFlightRef.current) return;

    const pendingIds = new Set(autoAddPendingAssetIds);
    const autoAddReadyCandidates = candidates.filter(
      candidate => pendingIds.has(candidate.asset_id) && candidate.addable
    );
    const failedAssetIds = Array.from(embeddingTasksByAssetId.entries())
      .filter(([assetId, task]) => {
        if (!pendingIds.has(assetId)) return false;
        return task.status === 'failed' || task.status === 'cancelled';
      })
      .map(([assetId]) => assetId);

    if (autoAddReadyCandidates.length === 0 && failedAssetIds.length === 0) return;

    autoAddInFlightRef.current = true;
    void (async () => {
      let shouldClose = false;
      try {
        if (autoAddReadyCandidates.length > 0) {
          await createRefsMutation.mutateAsync(
            autoAddReadyCandidates.map(candidate => candidate.asset_id)
          );
        }

        const completedIds = new Set([
          ...autoAddReadyCandidates.map(candidate => candidate.asset_id),
          ...failedAssetIds,
        ]);
        const remainingIds = Array.from(pendingIds).filter(assetId => !completedIds.has(assetId));

        if (failedAssetIds.length > 0) {
          toast.error(
            t('messages.fileCandidateEmbeddingBatchGeneratePartialFailed', {
              count: failedAssetIds.length,
            })
          );
        }

        setAutoAddPendingAssetIds(new Set(remainingIds));
        shouldClose = remainingIds.length === 0;
      } catch {
        const completedIds = new Set([
          ...autoAddReadyCandidates.map(candidate => candidate.asset_id),
          ...failedAssetIds,
        ]);
        const remainingIds = Array.from(pendingIds).filter(assetId => !completedIds.has(assetId));
        setAutoAddPendingAssetIds(new Set(remainingIds));
        if (remainingIds.length === 0) {
          setIsSelectionProcessing(false);
        }
      } finally {
        autoAddInFlightRef.current = false;
        if (shouldClose) {
          setIsSelectionProcessing(false);
          onSubmitted?.();
          onOpenChange(false);
        }
      }
    })();
  }, [
    autoAddPendingAssetIds,
    candidates,
    createRefsMutation,
    embeddingTasksByAssetId,
    onOpenChange,
    onSubmitted,
    open,
    t,
  ]);

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
  const visibleSelectableIds = useMemo(
    () =>
      visibleCandidates
        .filter(item => item.addable || item.requires_embedding_generation === true)
        .map(item => item.asset_id),
    [visibleCandidates]
  );
  const selectedReadyAssetIds = useMemo(
    () =>
      candidates
        .filter(candidate => selectedSet.has(candidate.asset_id) && candidate.addable)
        .map(candidate => candidate.asset_id),
    [candidates, selectedSet]
  );
  const selectedEmbeddingGenerationAssetIds = useMemo(
    () =>
      candidates
        .filter(
          candidate =>
            selectedSet.has(candidate.asset_id) &&
            candidate.requires_embedding_generation === true
        )
        .map(candidate => candidate.asset_id),
    [candidates, selectedSet]
  );
  const allVisibleSelectableSelected =
    visibleSelectableIds.length > 0 && visibleSelectableIds.every(id => selectedSet.has(id));
  const isBatchGenerating = batchGeneratingAssetIds.size > 0;
  const activeEmbeddingAssetId =
    typeof generateEmbeddingsMutation.variables === 'string'
      ? generateEmbeddingsMutation.variables
      : generateEmbeddingsMutation.variables?.assetId;

  const toggleCandidate = useCallback((candidate: DatasetFileCandidate, checked: boolean) => {
    if (!candidate.addable && candidate.requires_embedding_generation !== true) return;
    setSelectedAssetIds(prev =>
      checked
        ? Array.from(new Set([...prev, candidate.asset_id]))
        : prev.filter(id => id !== candidate.asset_id)
    );
  }, []);

  const toggleCandidateSelection = useCallback((candidate: DatasetFileCandidate) => {
    if (!candidate.addable && candidate.requires_embedding_generation !== true) return;
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
          ? Array.from(new Set([...prev, ...visibleSelectableIds]))
          : prev.filter(id => !visibleSelectableIds.includes(id))
      );
    },
    [visibleSelectableIds]
  );

  const handleConfirmAddReady = useCallback(async () => {
    if (selectedReadyAssetIds.length === 0) return;
    try {
      await createRefsMutation.mutateAsync(selectedReadyAssetIds);
      onSubmitted?.();
      onOpenChange(false);
    } catch {
      // The mutation hook already shows the API error toast. Keep the dialog open for retry.
    }
  }, [createRefsMutation, onOpenChange, onSubmitted, selectedReadyAssetIds]);

  const handleAddSelected = useCallback(() => {
    if (selectedAssetIds.length === 0) return;
    if (selectedEmbeddingGenerationAssetIds.length > 0) {
      setPartialAddConfirmOpen(true);
      return;
    }
    void handleConfirmAddReady();
  }, [
    handleConfirmAddReady,
    selectedAssetIds.length,
    selectedEmbeddingGenerationAssetIds.length,
  ]);

  const handleConfirmAddSelected = useCallback(async () => {
    const readyAssetIds = [...selectedReadyAssetIds];
    const pendingAssetIds = [...selectedEmbeddingGenerationAssetIds];
    if (readyAssetIds.length === 0 && pendingAssetIds.length === 0) return;

    setPartialAddConfirmOpen(false);
    setIsSelectionProcessing(true);
    let keepProcessing = false;

    try {
      if (readyAssetIds.length > 0) {
        await createRefsMutation.mutateAsync(readyAssetIds);
      }

      if (pendingAssetIds.length === 0) {
        onSubmitted?.();
        onOpenChange(false);
        return;
      }

      setAutoAddPendingAssetIds(new Set(pendingAssetIds));
      setBatchGeneratingAssetIds(new Set(pendingAssetIds));

      const immediateAddableAssetIds: string[] = [];
      const queuedAssetIds: string[] = [];
      let failedCount = 0;

      for (const assetId of pendingAssetIds) {
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
              queuedAssetIds.push(assetId);
            } else {
              failedCount += 1;
            }
          } else if (response.data?.addable) {
            immediateAddableAssetIds.push(assetId);
          } else {
            failedCount += 1;
          }
        } catch {
          failedCount += 1;
        }
      }

      if (immediateAddableAssetIds.length > 0) {
        await createRefsMutation.mutateAsync(immediateAddableAssetIds);
      }

      if (failedCount > 0) {
        toast.error(
          t('messages.fileCandidateEmbeddingBatchGeneratePartialFailed', { count: failedCount })
        );
      }

      const remainingPendingAssetIds = queuedAssetIds;
      keepProcessing = remainingPendingAssetIds.length > 0;
      setAutoAddPendingAssetIds(new Set(remainingPendingAssetIds));

      if (remainingPendingAssetIds.length === 0) {
        onSubmitted?.();
        onOpenChange(false);
      }
    } catch {
      // Mutation hooks already show API errors. Keep the dialog open so the user can retry.
    } finally {
      setBatchGeneratingAssetIds(new Set());
      if (!keepProcessing) {
        setIsSelectionProcessing(false);
      }
    }
  }, [
    createRefsMutation,
    generateEmbeddingsMutation,
    onOpenChange,
    onSubmitted,
    selectedEmbeddingGenerationAssetIds,
    selectedReadyAssetIds,
    t,
  ]);

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
    if (selectedEmbeddingGenerationAssetIds.length === 0 || hasQueuedEmbeddingGeneration) return;

    const assetIds = [...selectedEmbeddingGenerationAssetIds];
    setBatchGeneratingAssetIds(new Set(assetIds));
    setAutoSelectPendingAssetIds(prev => new Set([...prev, ...assetIds]));

    const immediateAddableAssetIds: string[] = [];
    const failedAssetIds: string[] = [];
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
            } else {
              failedCount += 1;
              failedAssetIds.push(assetId);
            }
          } else if (!response.data?.addable) {
            failedCount += 1;
            failedAssetIds.push(assetId);
          } else {
            immediateAddableAssetIds.push(assetId);
          }
        } catch {
          failedCount += 1;
          failedAssetIds.push(assetId);
        }
      }

      if (immediateAddableAssetIds.length > 0) {
        setSelectedAssetIds(prev => Array.from(new Set([...prev, ...immediateAddableAssetIds])));
      }

      if (failedCount > 0) {
        toast.error(
          t('messages.fileCandidateEmbeddingBatchGeneratePartialFailed', { count: failedCount })
        );
      }
    } finally {
      const completedIds = new Set([...immediateAddableAssetIds, ...failedAssetIds]);
      if (completedIds.size > 0) {
        setAutoSelectPendingAssetIds(
          prev => new Set(Array.from(prev).filter(assetId => !completedIds.has(assetId)))
        );
      }
      setBatchGeneratingAssetIds(new Set());
    }
  }, [
    generateEmbeddingsMutation,
    hasQueuedEmbeddingGeneration,
    selectedEmbeddingGenerationAssetIds,
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
          <div className="px-5 py-4">
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

            {selectedEmbeddingGenerationAssetIds.length > 0 ? (
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="h-8 whitespace-nowrap border-primary/30 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
                  loading={isBatchGenerating}
                  disabled={
                    isSelectionProcessing ||
                    hasQueuedEmbeddingGeneration ||
                    generateEmbeddingsMutation.isPending
                  }
                  onClick={handleBatchGenerateEmbeddings}
                >
                  {t('documents.fileAssets.batchGenerateDatasetEmbedding', {
                    count: selectedEmbeddingGenerationAssetIds.length,
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
                        checked={allVisibleSelectableSelected}
                        disabled={visibleSelectableIds.length === 0}
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
                      const selectable =
                        candidate.addable || candidate.requires_embedding_generation === true;
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
                            selectable &&
                              'cursor-pointer hover:bg-primary/5 data-[state=selected]:bg-primary/10',
                            requiresEmbeddingGeneration && 'bg-muted/20'
                          )}
                        >
                          <TableCell className="px-3" onClick={event => event.stopPropagation()}>
                            <Checkbox
                              checked={selected}
                              disabled={!selectable}
                              onCheckedChange={checked =>
                                toggleCandidate(candidate, checked === true)
                              }
                              aria-label={candidate.name}
                            />
                          </TableCell>
                          <TableCell className="max-w-0">
                            <div className="flex min-w-0 items-center gap-3">
                              <FileTypeIcon
                                extension={ext}
                                filename={candidate.name}
                                className="h-5 w-5 shrink-0"
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
            onClick={handleAddSelected}
            loading={isSelectionProcessing || createRefsMutation.isPending}
            disabled={selectedAssetIds.length === 0 || isSelectionProcessing}
          >
            {t('documents.fileAssets.addSelected', { count: selectedAssetIds.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
      <ConfirmDialog
        variant="default"
        open={partialAddConfirmOpen}
        onOpenChange={setPartialAddConfirmOpen}
        title={t('documents.fileAssets.partialAddConfirmTitle')}
        description={t('documents.fileAssets.partialAddConfirmDescription', {
          selected: selectedAssetIds.length,
          ready: selectedReadyAssetIds.length,
          pending: selectedEmbeddingGenerationAssetIds.length,
        })}
        confirmText={t('documents.fileAssets.partialAddConfirmAction')}
        cancelText={t('actions.cancel')}
        loading={isSelectionProcessing || createRefsMutation.isPending}
        onConfirm={handleConfirmAddSelected}
      />
    </Dialog>
  );
}

'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { FileText, Search } from 'lucide-react';
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
} from '@/hooks/dataset/use-dataset-file-refs';
import type { DatasetFileCandidate } from '@/services/types/dataset';
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
  if (filter === 'addable') return candidate.addable;
  if (filter === 'added') return candidate.already_added;
  return !candidate.addable && !candidate.already_added;
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

export function DatasetFileAssetDialog({
  datasetId,
  open,
  onOpenChange,
  onSubmitted,
}: DatasetFileAssetDialogProps) {
  const t = useT('datasets');
  const [selectedAssetIds, setSelectedAssetIds] = useState<string[]>([]);
  const [activeFilter, setActiveFilter] = useState<CandidateFilter>('addable');
  const createRefsMutation = useCreateDatasetFileRefs(datasetId);
  const { candidates, total, keyword, setKeyword, isLoading, isFetching } =
    useDatasetFileCandidates(
      datasetId,
      { filter: 'all', limit: 100 },
      { enabled: open, debounceDelay: 300 }
    );

  useEffect(() => {
    if (!open) {
      setSelectedAssetIds([]);
      setKeyword('');
      setActiveFilter('addable');
    }
  }, [open, setKeyword]);

  const summary = useMemo(
    () =>
      candidates.reduce(
        (acc, candidate) => {
          if (candidate.addable) acc.addable += 1;
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
  const allVisibleAddableSelected =
    visibleAddableIds.length > 0 && visibleAddableIds.every(id => selectedSet.has(id));

  const toggleCandidate = useCallback((candidate: DatasetFileCandidate, checked: boolean) => {
    if (!candidate.addable) return;
    setSelectedAssetIds(prev =>
      checked
        ? Array.from(new Set([...prev, candidate.asset_id]))
        : prev.filter(id => id !== candidate.asset_id)
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

            <div className="mt-4 flex flex-wrap items-center gap-2">
              {FILTERS.map(filter => (
                <Button
                  key={filter}
                  type="button"
                  variant={activeFilter === filter ? 'default' : 'outline'}
                  className={cn('h-9 rounded-lg px-4', activeFilter === filter ? '' : 'bg-background')}
                  onClick={() => setActiveFilter(filter)}
                >
                  {t(`documents.fileAssets.filters.${filter}`)}
                </Button>
              ))}
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-auto p-5">
            <div className="overflow-hidden rounded-xl border">
              <Table className="min-w-[1040px] table-fixed">
                <colgroup>
                  <col className="w-[44px]" />
                  <col />
                  <col className="w-[132px]" />
                  <col className="w-[152px]" />
                  <col className="w-[112px]" />
                  <col className="w-[140px]" />
                  <col className="w-[170px]" />
                  <col className="w-[140px]" />
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
                    <TableHead className="text-right">{t('documents.fileAssets.actions')}</TableHead>
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
                      const ext = fileExtension(candidate);
                      return (
                        <TableRow
                          key={candidate.asset_id}
                          data-state={selected ? 'selected' : undefined}
                          className="h-16 hover:bg-muted/30 data-[state=selected]:bg-primary/5"
                        >
                          <TableCell className="px-3">
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
                            <Badge variant={candidate.processing_status === 'ready' ? 'success' : 'warning'}>
                              {processingStatusLabel(t, candidate.processing_status)}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            {candidate.addable ? (
                              <Badge variant="success">{t('documents.fileAssets.ready')}</Badge>
                            ) : (
                              <Badge variant={candidate.already_added ? 'subtle' : 'warning'}>
                                {candidate.already_added
                                  ? t('documents.fileAssets.reasons.alreadyAdded')
                                  : t(candidateReasonKey(candidate.reason))}
                              </Badge>
                            )}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {candidate.addable || candidate.already_added
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
                          <TableCell className="text-right">
                            <Button asChild variant="ghost" size="sm" className="h-8 px-2 text-xs">
                              <Link href={`/console/files/${candidate.file_id}`}>
                                {t('documents.fileAssets.openFile')}
                              </Link>
                            </Button>
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

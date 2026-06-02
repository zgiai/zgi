'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
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

interface DatasetFileAssetDialogProps {
  datasetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmitted?: () => void;
}

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

export function DatasetFileAssetDialog({
  datasetId,
  open,
  onOpenChange,
  onSubmitted,
}: DatasetFileAssetDialogProps) {
  const t = useT('datasets');
  const [selectedAssetIds, setSelectedAssetIds] = useState<string[]>([]);
  const createRefsMutation = useCreateDatasetFileRefs(datasetId);
  const { candidates, total, keyword, setKeyword, isLoading, isFetching } =
    useDatasetFileCandidates(
      datasetId,
      { filter: 'all', limit: 50 },
      { enabled: open, debounceDelay: 300 }
    );

  useEffect(() => {
    if (!open) {
      setSelectedAssetIds([]);
      setKeyword('');
    }
  }, [open, setKeyword]);

  const selectedSet = useMemo(() => new Set(selectedAssetIds), [selectedAssetIds]);
  const addableCandidates = useMemo(() => candidates.filter(item => item.addable), [candidates]);
  const allVisibleAddableSelected =
    addableCandidates.length > 0 && addableCandidates.every(item => selectedSet.has(item.asset_id));

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
      const visibleIds = addableCandidates.map(item => item.asset_id);
      setSelectedAssetIds(prev =>
        checked
          ? Array.from(new Set([...prev, ...visibleIds]))
          : prev.filter(id => !visibleIds.includes(id))
      );
    },
    [addableCandidates]
  );

  const handleConfirm = useCallback(async () => {
    if (selectedAssetIds.length === 0) return;
    await createRefsMutation.mutateAsync(selectedAssetIds);
    onSubmitted?.();
    onOpenChange(false);
  }, [createRefsMutation, onOpenChange, onSubmitted, selectedAssetIds]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl">
        <DialogHeader>
          <DialogTitle>{t('documents.fileAssets.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('documents.fileAssets.dialogDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-4">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={keyword}
              onChange={event => setKeyword(event.target.value)}
              placeholder={t('documents.fileAssets.searchPlaceholder')}
              className="h-9 pl-10"
            />
          </div>

          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={allVisibleAddableSelected}
                      disabled={addableCandidates.length === 0}
                      onCheckedChange={checked => toggleAllVisible(checked === true)}
                      aria-label={t('documents.fileAssets.selectAll')}
                    />
                  </TableHead>
                  <TableHead>{t('documents.fileAssets.fileName')}</TableHead>
                  <TableHead>{t('documents.fileAssets.status')}</TableHead>
                  <TableHead>{t('documents.fileAssets.chunks')}</TableHead>
                  <TableHead>{t('documents.fileAssets.embeddingModel')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {isLoading &&
                  Array.from({ length: 5 }).map((_, index) => (
                    <TableRow key={index}>
                      <TableCell colSpan={5}>
                        <Skeleton className="h-8 w-full" />
                      </TableCell>
                    </TableRow>
                  ))}

                {!isLoading &&
                  candidates.map(candidate => (
                    <TableRow key={candidate.asset_id}>
                      <TableCell>
                        <Checkbox
                          checked={selectedSet.has(candidate.asset_id)}
                          disabled={!candidate.addable}
                          onCheckedChange={checked => toggleCandidate(candidate, checked === true)}
                          aria-label={candidate.name}
                        />
                      </TableCell>
                      <TableCell className="max-w-[280px]">
                        <div className="flex min-w-0 items-center gap-2">
                          <FileText className="h-4 w-4 shrink-0 text-muted-foreground" />
                          <span className="truncate font-medium">{candidate.name}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        {candidate.addable ? (
                          <Badge variant="success">{t('documents.fileAssets.ready')}</Badge>
                        ) : (
                          <Badge variant="warning">{t(candidateReasonKey(candidate.reason))}</Badge>
                        )}
                      </TableCell>
                      <TableCell>{candidate.chunk_count}</TableCell>
                      <TableCell className="max-w-[220px] truncate">
                        {candidate.embedding_model || '-'}
                      </TableCell>
                    </TableRow>
                  ))}

                {!isLoading && candidates.length === 0 && (
                  <TableRow>
                    <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">
                      {t('documents.fileAssets.empty')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>

          <div className="text-xs text-muted-foreground">
            {t('documents.fileAssets.resultSummary', { count: candidates.length, total })}
            {isFetching ? ` ${t('loading')}` : ''}
          </div>
        </DialogBody>

        <DialogFooter>
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

'use client';

import React from 'react';
import Link from 'next/link';
import { ExternalLink, RefreshCcw, Trash2 } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { DatasetFileRef, Document } from '@/services/types/dataset';
import { formatDate } from '@/utils/format';

interface DatasetFileRefPanelProps {
  refs: DatasetFileRef[];
  documents: Document[];
  canEdit?: boolean;
  retryingRefId?: string;
  removingRefId?: string;
  onRetry?: (ref: DatasetFileRef) => void;
  onRemove?: (ref: DatasetFileRef) => void;
}

function syncStatusBadgeVariant(status: string) {
  switch (status) {
    case 'synced':
      return 'success';
    case 'failed':
      return 'destructive';
    case 'syncing':
      return 'info';
    default:
      return 'warning';
  }
}

function syncStatusLabel(t: ReturnType<typeof useT<'datasets'>>, status: string) {
  switch (status) {
    case 'synced':
      return t('documents.fileRefs.status.synced');
    case 'syncing':
      return t('documents.fileRefs.status.syncing');
    case 'failed':
      return t('documents.fileRefs.status.failed');
    default:
      return t('documents.fileRefs.status.pending');
  }
}

export function DatasetFileRefPanel({
  refs,
  documents,
  canEdit = true,
  retryingRefId,
  removingRefId,
  onRetry,
  onRemove,
}: DatasetFileRefPanelProps) {
  const t = useT('datasets');
  const documentsById = React.useMemo(() => {
    return new Map(documents.map(document => [document.id, document]));
  }, [documents]);

  if (refs.length === 0) {
    return null;
  }

  return (
    <div className="rounded-md border overflow-hidden">
      <div className="flex items-center justify-between border-b px-3 py-2">
        <div>
          <div className="text-sm font-medium">{t('documents.fileRefs.title')}</div>
          <div className="text-xs text-muted-foreground">{t('documents.fileRefs.description')}</div>
        </div>
      </div>

      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('documents.fileRefs.fileName')}</TableHead>
            <TableHead>{t('documents.fileRefs.syncStatus')}</TableHead>
            <TableHead>{t('documents.fileRefs.documentState')}</TableHead>
            <TableHead>{t('documents.fileRefs.generation')}</TableHead>
            <TableHead>{t('documents.fileRefs.lastSyncedAt')}</TableHead>
            <TableHead className="w-[180px]" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {refs.map(ref => {
            const document = ref.dataset_document_id
              ? documentsById.get(ref.dataset_document_id)
              : undefined;
            const documentEnabled = ref.dataset_document_enabled ?? document?.enabled;
            const hasDatasetDocument =
              Boolean(ref.dataset_document_id) && typeof documentEnabled === 'boolean';
            const isSynced = ref.sync_status === 'synced';
            const isFailed = ref.sync_status === 'failed';
            return (
              <TableRow key={ref.id}>
                <TableCell className="max-w-[280px]">
                  <div className="truncate font-medium" title={ref.file_name}>
                    {ref.file_name}
                  </div>
                  {isFailed && ref.sync_error_message ? (
                    <div className="mt-1 max-w-[280px] truncate text-xs text-destructive">
                      {ref.sync_error_message}
                    </div>
                  ) : null}
                </TableCell>
                <TableCell>
                  <Badge variant={syncStatusBadgeVariant(ref.sync_status)}>
                    {syncStatusLabel(t, ref.sync_status)}
                  </Badge>
                </TableCell>
                <TableCell>
                  {hasDatasetDocument ? (
                    <Badge variant={documentEnabled && isSynced ? 'success' : 'subtle'}>
                      {documentEnabled && isSynced
                        ? t('documents.fileRefs.documentEnabled')
                        : t('documents.fileRefs.documentDisabled')}
                    </Badge>
                  ) : (
                    <Badge variant="subtle">{t('documents.fileRefs.noDocument')}</Badge>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {ref.synced_generation_no ?? '-'} / {ref.generation_no}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {ref.last_synced_at ? formatDate(ref.last_synced_at) : '-'}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button asChild variant="ghost" size="sm">
                      <Link href={`/console/files/${ref.file_id}`}>
                        <ExternalLink className="h-4 w-4" />
                        {t('documents.fileRefs.openFile')}
                      </Link>
                    </Button>
                    {canEdit && isFailed && (
                      <Button
                        variant="ghost"
                        size="sm"
                        loading={retryingRefId === ref.id}
                        onClick={() => onRetry?.(ref)}
                      >
                        <RefreshCcw className="h-4 w-4" />
                        {t('documents.fileRefs.retry')}
                      </Button>
                    )}
                    {canEdit && (
                      <Button
                        variant="ghost"
                        size="sm"
                        loading={removingRefId === ref.id}
                        onClick={() => onRemove?.(ref)}
                        className="text-destructive hover:text-destructive"
                      >
                        <Trash2 className="h-4 w-4" />
                        {t('actions.delete')}
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

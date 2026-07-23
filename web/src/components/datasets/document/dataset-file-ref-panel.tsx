'use client';

import Link from 'next/link';
import { usePathname, useSearchParams } from 'next/navigation';
import { ExternalLink, HelpCircle, RefreshCcw, Trash2 } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { FileTypeIcon } from '@/components/files/file-type-icon';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { DatasetFileRef } from '@/services/types/dataset';
import { formatDate } from '@/utils/format';
import { cn } from '@/lib/utils';

interface DatasetFileRefPanelProps {
  refs: DatasetFileRef[];
  canEdit?: boolean;
  canOpenSourceFile?: boolean;
  canToggleEnabled?: boolean;
  canRetry?: boolean;
  canRemove?: boolean;
  retryingRefId?: string;
  removingRefId?: string;
  togglingRefId?: string;
  selectedRefIds?: string[];
  onRetry?: (ref: DatasetFileRef) => void;
  onRemove?: (ref: DatasetFileRef) => void;
  onToggleEnabled?: (ref: DatasetFileRef, enabled: boolean) => void;
  onSelectionChange?: (refIds: string[]) => void;
  onBatchRemove?: () => void;
}

function fileExtension(name: string) {
  const ext = name.split('.').pop();
  return ext && ext !== name ? ext.toLowerCase() : 'file';
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

function processingStatusBadgeClassName(status: string) {
  switch (status) {
    case 'ready':
      return 'border-success/25 bg-success/10 text-success';
    case 'parse_failed':
      return 'border-destructive/25 bg-destructive/10 text-destructive';
    case 'parsing':
    case 'generating':
      return 'border-info/25 bg-info/10 text-info';
    default:
      return 'border-warning/25 bg-warning/10 text-warning';
  }
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

function TableHeadWithHelp({ label, tooltip }: { label: string; tooltip: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <span>{label}</span>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="inline-flex h-5 w-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
            aria-label={tooltip}
          >
            <HelpCircle className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" align="start" className="max-w-72 text-sm leading-6">
          {tooltip}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}

export function DatasetFileRefPanel({
  refs,
  canEdit = true,
  canOpenSourceFile = true,
  canToggleEnabled,
  canRetry,
  canRemove,
  retryingRefId,
  removingRefId,
  togglingRefId,
  selectedRefIds = [],
  onRetry,
  onRemove,
  onToggleEnabled,
  onSelectionChange,
  onBatchRemove,
}: DatasetFileRefPanelProps) {
  const t = useT('datasets');
  const pathname = usePathname();
  const canToggleEnabledAction = canToggleEnabled ?? canEdit;
  const canRetryAction = canRetry ?? canEdit;
  const canRemoveAction = canRemove ?? canEdit;
  const canSelectRefs = canRemoveAction && Boolean(onSelectionChange);
  const searchParams = useSearchParams();
  const currentSearch = searchParams.toString();
  const returnTo = `${pathname}${currentSearch ? `?${currentSearch}` : ''}`;
  const visibleRefIds = refs.map(ref => ref.id);
  const selectedVisibleRefIds = visibleRefIds.filter(refId => selectedRefIds.includes(refId));
  const allVisibleSelected = refs.length > 0 && selectedVisibleRefIds.length === refs.length;
  const someVisibleSelected = selectedVisibleRefIds.length > 0 && !allVisibleSelected;

  const handleSelectAll = (checked: boolean) => {
    if (!onSelectionChange) return;

    if (checked) {
      onSelectionChange(Array.from(new Set([...selectedRefIds, ...visibleRefIds])));
      return;
    }

    onSelectionChange(selectedRefIds.filter(refId => !visibleRefIds.includes(refId)));
  };

  const handleSelectRef = (refId: string, checked: boolean) => {
    if (!onSelectionChange) return;

    if (checked) {
      onSelectionChange(Array.from(new Set([...selectedRefIds, refId])));
      return;
    }

    onSelectionChange(selectedRefIds.filter(selectedRefId => selectedRefId !== refId));
  };

  return (
    <div className="overflow-hidden rounded-xl border bg-background">
      {canSelectRefs && selectedRefIds.length > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/30 px-4 py-3">
          <span className="text-sm text-muted-foreground">
            {t('documents.fileRefs.selectedCount', { count: selectedRefIds.length })}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => onSelectionChange?.([])}>
              {t('documents.fileRefs.clearSelection')}
            </Button>
            <Button variant="destructive" size="sm" onClick={onBatchRemove}>
              <Trash2 className="h-4 w-4" />
              {t('documents.fileRefs.batchRemove')}
            </Button>
          </div>
        </div>
      ) : null}
      <Table className="min-w-[960px] table-fixed">
        <colgroup>
          {canSelectRefs ? <col className="w-[48px]" /> : null}
          <col />
          <col className="w-[132px]" />
          <col className="w-[116px]" />
          <col className="w-[96px]" />
          <col className="w-[172px]" />
          <col className="w-[180px]" />
        </colgroup>
        <TableHeader className="bg-muted/40">
          <TableRow className="hover:bg-muted/40">
            {canSelectRefs ? (
              <TableHead className="px-4">
                <Checkbox
                  checked={allVisibleSelected ? true : someVisibleSelected ? 'indeterminate' : false}
                  onCheckedChange={checked => handleSelectAll(checked === true)}
                  disabled={refs.length === 0}
                  aria-label={t('documents.fileRefs.selectAll')}
                />
              </TableHead>
            ) : null}
            <TableHead className="text-sm">{t('documents.fileRefs.fileName')}</TableHead>
            <TableHead className="text-sm">{t('documents.fileRefs.fileStatus')}</TableHead>
            <TableHead className="text-sm">
              <TableHeadWithHelp
                label={t('documents.fileRefs.enabled')}
                tooltip={t('documents.fileRefs.enabledTooltip')}
              />
            </TableHead>
            <TableHead className="text-sm">
              <TableHeadWithHelp
                label={t('documents.fileRefs.chunks')}
                tooltip={t('documents.fileRefs.chunksTooltip')}
              />
            </TableHead>
            <TableHead className="text-sm">{t('documents.fileRefs.lastSyncedAt')}</TableHead>
            <TableHead className="text-right text-sm">{t('documents.fileRefs.actions')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {refs.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={canSelectRefs ? 7 : 6}
                className="h-40 text-center text-sm text-muted-foreground"
              >
                {t('documents.fileRefs.empty')}
              </TableCell>
            </TableRow>
          ) : null}
          {refs.map(ref => {
            const isSynced = ref.sync_status === 'synced';
            const isFailed = ref.sync_status === 'failed';
            const enabled = Boolean(ref.dataset_document_enabled && isSynced);
            const canToggle =
              canToggleEnabledAction && isSynced && Boolean(ref.dataset_document_id);
            const ext = fileExtension(ref.file_name);

            return (
              <TableRow key={ref.id} className="h-16 hover:bg-muted/30">
                {canSelectRefs ? (
                  <TableCell className="px-4">
                    <Checkbox
                      checked={selectedRefIds.includes(ref.id)}
                      onCheckedChange={checked => handleSelectRef(ref.id, checked === true)}
                      aria-label={t('documents.fileRefs.selectFile', { name: ref.file_name })}
                    />
                  </TableCell>
                ) : null}
                <TableCell className="max-w-0">
                  <div className="flex min-w-0 items-center gap-3">
                    <FileTypeIcon extension={ext} className="h-5 w-5 shrink-0" />
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium" title={ref.file_name}>
                        {ref.file_name}
                      </div>
                      <div className="mt-0.5 truncate text-xs text-muted-foreground">{ext}</div>
                      {isFailed && ref.sync_error_message ? (
                        <div className="mt-1 truncate text-xs text-destructive">
                          {ref.sync_error_message}
                        </div>
                      ) : null}
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <span
                    className={cn(
                      'inline-flex h-5 shrink-0 items-center justify-center rounded-full border px-2 text-[12px] font-medium leading-none',
                      processingStatusBadgeClassName(ref.processing_status)
                    )}
                  >
                    {processingStatusLabel(t, ref.processing_status)}
                  </span>
                </TableCell>
                <TableCell>
                  <Switch
                    checked={enabled}
                    disabled={!canToggle || togglingRefId === ref.id}
                    aria-label={t('documents.fileRefs.toggleEnabled', { name: ref.file_name })}
                    onCheckedChange={checked => onToggleEnabled?.(ref, Boolean(checked))}
                  />
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {ref.dataset_document_segment_count ?? '-'}
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  <div className="flex flex-col gap-1">
                    <Badge variant={syncStatusBadgeVariant(ref.sync_status)}>
                      {syncStatusLabel(t, ref.sync_status)}
                    </Badge>
                    <span>{ref.last_synced_at ? formatDate(ref.last_synced_at) : '-'}</span>
                  </div>
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    {canOpenSourceFile && ref.source_file_available ? (
                      <Button asChild variant="ghost" size="sm" className="h-8 px-2 text-xs">
                        <Link
                          href={`/console/files/${ref.file_id}?returnTo=${encodeURIComponent(returnTo)}`}
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          {t('documents.fileRefs.openFile')}
                        </Link>
                      </Button>
                    ) : null}
                    {canRetryAction && isFailed ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 px-2 text-xs"
                        loading={retryingRefId === ref.id}
                        onClick={() => onRetry?.(ref)}
                      >
                        <RefreshCcw className="h-3.5 w-3.5" />
                        {t('documents.fileRefs.retry')}
                      </Button>
                    ) : null}
                    {canRemoveAction ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        isIcon
                        className="h-8 w-8 text-muted-foreground hover:text-destructive"
                        loading={removingRefId === ref.id}
                        aria-label={t('documents.fileRefs.removeFile', { name: ref.file_name })}
                        onClick={() => onRemove?.(ref)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    ) : null}
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

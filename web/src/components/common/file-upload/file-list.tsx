'use client';

import React from 'react';
import {
  Table,
  TableHeader,
  TableRow,
  TableHead,
  TableBody,
  TableCell,
} from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Progress } from '@/components/ui/progress';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { formatFileSize } from '@/utils/format';
import { AlertCircle, Trash2 } from 'lucide-react';

export interface FileListItem {
  id: string;
  name: string;
  size: number;
  status: 'uploading' | 'success' | 'error' | 'pending';
  progress?: number;
  errorMsg?: string;
}

interface FileListProps {
  items: FileListItem[];
  onRetry?: (id: string) => void;
  onRemove?: (id: string) => void;
  className?: string;
  queueSummaryNamespace?: 'files' | 'ui';
  showRetryAction?: boolean;
  tableWrapperClassName?: string;
}

export const FileList: React.FC<FileListProps> = ({
  items,
  onRetry,
  onRemove,
  queueSummaryNamespace = 'ui',
  showRetryAction = false,
  tableWrapperClassName,
}) => {
  const tUi = useT('ui');
  const tFiles = useT('files');

  if (!items.length) return null;

  const failedCount = items.filter(item => item.status === 'error').length;
  const successCount = items.filter(item => item.status === 'success').length;
  const shouldShowQueueSummary = failedCount > 0 || successCount > 0;
  const tQueue = (key: string, values?: Record<string, number>) =>
    queueSummaryNamespace === 'files'
      ? tFiles(`upload.${key}` as never, values)
      : tUi(`fileUpload.${key}` as never, values);

  return (
    <div
      className={cn('w-full rounded-lg border border-border overflow-auto', tableWrapperClassName)}
    >
      <div className="space-y-2 border-b border-border px-4 py-3">
        <div>
          <h3 className="text-sm font-semibold text-foreground">
            {tQueue('selectedFilesTitle', { count: items.length })}
          </h3>
          {shouldShowQueueSummary ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {failedCount > 0
                ? tQueue('selectedFilesValidationSummary', {
                    count: items.length,
                    failedCount,
                  })
                : tQueue('selectedFilesResultSummary', {
                    successCount,
                    failedCount,
                  })}
            </p>
          ) : null}
        </div>
        {failedCount > 0 ? (
          <p className="flex items-center gap-1.5 text-xs text-destructive" role="status">
            <AlertCircle className="size-3.5" />
            {tQueue('removeInvalidBeforeUpload')}
          </p>
        ) : null}
      </div>
      <Table containerClassName="overflow-x-auto">
        <TableHeader className="sticky top-0 z-10">
          <TableRow>
            <TableHead className="px-3 py-3">{tUi('fileUpload.fileName')}</TableHead>
            <TableHead className="px-3 py-3">{tUi('fileUpload.size')}</TableHead>
            <TableHead className="px-3 py-3">{tUi('fileUpload.status')}</TableHead>
            <TableHead className="px-3 py-3">{tUi('fileUpload.action')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className="divide-y divide-border">
          {items.map(item => (
            <TableRow key={item.id} className={cn(item.status === 'error' && 'bg-destructive/5')}>
              <TableCell className="px-3 py-2 max-w-[200px] truncate" title={item.name}>
                {item.name}
              </TableCell>
              <TableCell className="px-3 py-2">{formatFileSize(item.size)}</TableCell>
              <TableCell className="px-3 py-2 min-w-[100px]">
                {item.status === 'uploading' && (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span>{tUi('fileUpload.uploading')}</span>
                      <span className="text-muted-foreground">{item.progress ?? 0}%</span>
                    </div>
                    <Progress value={item.progress ?? 0} className="h-2" />
                  </div>
                )}
                {item.status === 'success' && (
                  <Badge variant="secondary">{tUi('fileUpload.success')}</Badge>
                )}
                {item.status === 'error' && (
                  <div className="space-y-1.5">
                    <Badge
                      variant="destructive"
                      className="gap-1.5 rounded-full px-2.5 py-1 text-xs"
                    >
                      <AlertCircle className="size-3.5" />
                      {tQueue('cannotUpload')}
                    </Badge>
                    {item.errorMsg ? (
                      <p className="text-xs leading-5 text-destructive">{item.errorMsg}</p>
                    ) : null}
                  </div>
                )}
                {item.status === 'pending' && (
                  <Badge variant="outline">{tUi('fileUpload.pending')}</Badge>
                )}
              </TableCell>
              <TableCell className="px-3 py-2 space-x-2">
                {showRetryAction && item.status === 'error' && onRetry && (
                  <Button
                    size="sm"
                    type="button"
                    variant="secondary"
                    onClick={() => onRetry(item.id)}
                  >
                    {tUi('fileUpload.retry')}
                  </Button>
                )}
                {onRemove && (
                  <Button isIcon type="button" variant="outline" onClick={() => onRemove(item.id)}>
                    <Trash2 className="w-4 h-4 text-destructive" />
                  </Button>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
};

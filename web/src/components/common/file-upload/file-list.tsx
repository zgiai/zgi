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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { formatFileSize } from '@/utils/format';
import { AlertCircle, RefreshCw, Trash2 } from 'lucide-react';

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
  tableWrapperClassName?: string;
}

export const FileList: React.FC<FileListProps> = ({
  items,
  onRetry,
  onRemove,
  tableWrapperClassName,
}) => {
  const t = useT('ui');

  if (!items.length) return null;

  return (
    <div
      className={cn('w-full rounded-lg border border-border overflow-auto', tableWrapperClassName)}
    >
      <Table containerClassName="overflow-x-auto">
        <TableHeader className="sticky top-0 z-10">
          <TableRow>
            <TableHead className="px-3 py-3">{t('fileUpload.fileName')}</TableHead>
            <TableHead className="px-3 py-3">{t('fileUpload.size')}</TableHead>
            <TableHead className="px-3 py-3">{t('fileUpload.status')}</TableHead>
            <TableHead className="px-3 py-3">{t('fileUpload.action')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className="divide-y divide-border">
          {items.map(item => (
            <TableRow key={item.id}>
              <TableCell className="px-3 py-2 max-w-[200px] truncate" title={item.name}>
                {item.name}
              </TableCell>
              <TableCell className="px-3 py-2">{formatFileSize(item.size)}</TableCell>
              <TableCell className="px-3 py-2 min-w-[100px]">
                {item.status === 'uploading' && (
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span>{t('fileUpload.uploading')}</span>
                      <span className="text-muted-foreground">{item.progress ?? 0}%</span>
                    </div>
                    <Progress value={item.progress ?? 0} className="h-2" />
                  </div>
                )}
                {item.status === 'success' && (
                  <Badge variant="secondary">{t('fileUpload.success')}</Badge>
                )}
                {item.status === 'error' && (
                  <div className="flex items-center gap-2">
                    <Badge variant="destructive">{t('fileUpload.error')}</Badge>
                    {item.errorMsg && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <button
                            type="button"
                            className="inline-flex items-center justify-center rounded-sm text-destructive/80 hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            aria-label={t('fileUpload.errorDetails')}
                          >
                            <AlertCircle className="size-4" />
                          </button>
                        </TooltipTrigger>
                        <TooltipContent side="top" className="max-w-xs break-all">
                          {item.errorMsg}
                        </TooltipContent>
                      </Tooltip>
                    )}
                  </div>
                )}
                {item.status === 'pending' && (
                  <Badge variant="outline">{t('fileUpload.pending')}</Badge>
                )}
              </TableCell>
              <TableCell className="px-3 py-2 space-x-2">
                {item.status === 'error' && onRetry && (
                  <Button
                    size="sm"
                    type="button"
                    variant="secondary"
                    onClick={() => onRetry(item.id)}
                  >
                    <RefreshCw className="w-4 h-4" />
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

'use client';

import { History, ChevronLeft, ChevronRight } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';

import type { RecordsTableProps } from '../types';
import { useT } from '@/i18n';

const getRecordKey = (record: RecordsTableProps['records'][number]) =>
  record.id || `${record.created_at}:${record.source}:${record.content}`;

/**
 * RecordsTable Component
 * Displays query history records in a table format with pagination
 */
export function RecordsTable({
  records,
  isLoading,
  onLoadQuery,
  currentPage,
  totalPages,
  onLoadPrevious,
  hasPreviousPage,
  total: _total = 0,
  onLoadMore,
}: RecordsTableProps) {
  const t = useT();

  // Format ISO date string to readable date
  const formatDate = (isoString: string) => {
    return new Date(isoString).toLocaleString();
  };

  if (isLoading && records.length === 0) {
    return (
      <Card className="flex h-full flex-col shadow-sm">
        <CardHeader className="border-b px-4 py-3">
          <CardTitle className="flex items-center gap-2 text-sm">
            <History className="h-4 w-4 text-muted-foreground" />
            {t('datasets.hitTesting.testHistory')}
          </CardTitle>
        </CardHeader>
        <CardContent className="h-0 grow overflow-y-auto p-4">
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center justify-between p-3 border rounded-lg">
                <div className="space-y-2 flex-1">
                  <Skeleton className="h-4 w-3/4" />
                  <Skeleton className="h-3 w-1/2" />
                </div>
                <Skeleton className="h-6 w-16" />
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="flex h-full flex-col shadow-sm">
      <CardHeader className="border-b px-4 py-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-sm">
              <History className="h-4 w-4 text-muted-foreground" />
              {t('datasets.hitTesting.testHistory')}
            </CardTitle>
            <p className="mt-1 text-xs text-muted-foreground">
              {t('datasets.hitTesting.historyDescription')}
            </p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="h-0 grow overflow-y-auto p-0">
        {records.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center px-6 py-8 text-center">
            <div className="mb-4 rounded-full bg-muted p-3">
              <History className="h-6 w-6 text-muted-foreground" />
            </div>
            <p className="text-sm font-medium text-foreground">
              {t('datasets.hitTesting.noHistory')}
            </p>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">
              {t('datasets.messages.noHistoryDesc')}
            </p>
          </div>
        ) : (
          <div>
            {/* Desktop Table View */}
            <div className="relative overflow-hidden">
              <Table className="table-fixed">
                <colgroup>
                  <col className="w-[52%]" />
                  <col className="w-[96px]" />
                  <col className="w-[132px]" />
                </colgroup>
                <TableHeader className="bg-muted/40">
                  <TableRow>
                    <TableHead className="h-9 w-[52%] text-xs text-muted-foreground">
                      {t('datasets.hitTesting.tableTestText')}
                    </TableHead>
                    <TableHead className="h-9 w-[96px] text-xs text-muted-foreground">
                      {t('datasets.hitTesting.tableElapsedTime')}
                    </TableHead>
                    <TableHead className="h-9 w-[132px] text-xs text-muted-foreground">
                      {t('datasets.hitTesting.tableTime')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {records.map(record => (
                    <TableRow
                      key={getRecordKey(record)}
                      className="cursor-pointer hover:bg-muted/50"
                      onClick={() => onLoadQuery(record)}
                    >
                      <TableCell className="max-w-0 py-2 text-sm font-medium">
                        <div className="truncate" title={record.content}>
                          {record.content}
                        </div>
                      </TableCell>
                      <TableCell className="py-2 whitespace-nowrap">
                        <span className="text-sm text-muted-foreground">
                          {(record.elapsed_time / 1000).toFixed(2)}s
                        </span>
                      </TableCell>
                      <TableCell className="py-2 whitespace-nowrap">
                        <span className="text-sm text-muted-foreground">
                          {formatDate(record.created_at)}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        )}
      </CardContent>
      {/* Pagination */}
      {records.length > 0 && (
        <div className="flex items-center justify-end gap-4 border-t px-3 py-2 text-muted-foreground">
          <Button
            variant="ghost"
            isIcon
            className="h-6 w-6"
            onClick={onLoadPrevious}
            disabled={!hasPreviousPage}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>
          <span className="text-sm">
            {currentPage} / {totalPages}
          </span>
          <Button
            variant="ghost"
            isIcon
            className="h-6 w-6"
            onClick={onLoadMore}
            disabled={currentPage >= totalPages}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>
      )}
    </Card>
  );
}

'use client';

import { useMemo, useState, useEffect } from 'react';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { Copy, Eye, History, RefreshCcw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { useDbSqlOperationsPaged } from '@/hooks/db/use-db-sql-operations';
import type { OperationType, SqlOperationStatus } from '@/services/types/db';
import { toast } from 'sonner';
import { formatDate } from '@/utils/format';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';

export default function DbRecordPage() {
  const params = useParams();
  const dbId = (params?.dbId as string) ?? '';
  const t = useT();

  const [operationType, setOperationType] = useState<OperationType | undefined>(undefined);
  const [status, setStatus] = useState<SqlOperationStatus | undefined>(undefined);
  const [startTime, setStartTime] = useState<string | undefined>(undefined);
  const [endTime, setEndTime] = useState<string | undefined>(undefined);

  const filters = useMemo(
    () => ({
      operation_type: operationType,
      status,
      // Convert local datetime-local format to RFC3339 (ISO 8601) for API
      start_time: startTime ? new Date(startTime).toISOString() : undefined,
      end_time: endTime ? new Date(endTime).toISOString() : undefined,
    }),
    [operationType, status, startTime, endTime]
  );

  const [page, setPage] = useState<number>(1);
  const [pageSize, setPageSize] = useState<number>(20);
  const [reloading, setReloading] = useState<boolean>(false);
  const [viewingSql, setViewingSql] = useState<string | null>(null);

  // Reset to first page when filters or pageSize change
  useEffect(() => {
    setPage(1);
  }, [operationType, status, startTime, endTime, pageSize]);

  const { items, total, isLoading, refetch } = useDbSqlOperationsPaged(
    dbId,
    filters,
    page,
    pageSize,
    {
      refetchOnWindowFocus: false,
      staleTime: 30_000,
      gcTime: 600_000,
    }
  );

  const totalPages = useMemo(() => Math.max(1, Math.ceil(total / pageSize)), [total, pageSize]);

  return (
    <div className="h-full overflow-auto p-6 flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <h1 className="text-xl font-semibold">{t('dbs.sqlOps.title')}</h1>
          <Button
            variant="ghost"
            isIcon
            className="w-7 h-7"
            disabled={reloading}
            onClick={() => {
              setReloading(true);
              Promise.resolve(refetch())
                .then(() => {
                  toast.success(t('common.refreshSuccess'));
                })
                .finally(() => setReloading(false));
            }}
          >
            <RefreshCcw className={`h-4 w-4 ${reloading ? 'animate-spin' : ''}`} />
          </Button>
        </div>
      </div>

      <div className="flex gap-3">
        <div className="flex flex-col gap-1 w-[120px]">
          <Label>{t('dbs.tableData.rowsPerPage')}</Label>
          <Select value={String(pageSize)} onValueChange={v => setPageSize(Number(v))}>
            <SelectTrigger>
              <SelectValue placeholder={String(pageSize)} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="10">10</SelectItem>
              <SelectItem value="20">20</SelectItem>
              <SelectItem value="50">50</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1 w-[120px]">
          <Label>{t('dbs.operationType')}</Label>
          <Select
            value={operationType ?? '__all__'}
            onValueChange={v =>
              setOperationType(v === '__all__' ? undefined : (v as OperationType))
            }
          >
            <SelectTrigger>
              <SelectValue placeholder={t('dbs.all')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">{t('dbs.all')}</SelectItem>
              <SelectItem value="create">{t('dbs.sqlOps.types.create')}</SelectItem>
              <SelectItem value="update">{t('dbs.sqlOps.types.update')}</SelectItem>
              <SelectItem value="delete">{t('dbs.sqlOps.types.delete')}</SelectItem>
              <SelectItem value="query">{t('dbs.sqlOps.types.query')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1 w-[120px]">
          <Label>{t('common.status')}</Label>
          <Select
            value={status ?? '__all__'}
            onValueChange={v => setStatus(v === '__all__' ? undefined : (v as SqlOperationStatus))}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('dbs.all')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__all__">{t('dbs.all')}</SelectItem>
              <SelectItem value="success">{t('dbs.sqlOps.status.success')}</SelectItem>
              <SelectItem value="failed">{t('dbs.sqlOps.status.failed')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <Label>{t('dbs.startTime')}</Label>
          <input
            type="datetime-local"
            className="h-9 px-2 py-1 rounded-md border bg-background text-sm"
            value={startTime ?? ''}
            onChange={e => {
              setStartTime(e.target.value || undefined);
            }}
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label>{t('dbs.endTime')}</Label>
          <input
            type="datetime-local"
            className="h-9 px-2 py-1 rounded-md border bg-background text-sm"
            value={endTime ?? ''}
            onChange={e => {
              setEndTime(e.target.value || undefined);
            }}
          />
        </div>
      </div>

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('dbs.time')}</TableHead>
              <TableHead>{t('dbs.operation')}</TableHead>
              <TableHead>{t('common.status')}</TableHead>
              <TableHead>{t('dbs.table')}</TableHead>
              <TableHead>{t('dbs.user')}</TableHead>
              <TableHead>{t('dbs.sql')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={`s-${i}`}>
                  <TableCell>
                    <Skeleton className="h-4 w-36" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-16" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-16" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-28" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-[480px]" />
                  </TableCell>
                </TableRow>
              ))
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6}>
                  <div className="py-8 flex flex-col items-center gap-2 text-muted-foreground">
                    <History className="h-6 w-6" />
                    <div className="text-base">{t('dbs.noData')}</div>
                  </div>
                </TableCell>
              </TableRow>
            ) : (
              items.map(op => (
                <TableRow key={op.id}>
                  <TableCell className="text-xs text-muted-foreground">
                    <div>{formatDate(op.start_time)}</div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary" className="capitalize">
                      {t(`dbs.sqlOps.types.${op.operation_type}`)}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={op.status === 'success' ? 'success' : 'destructive'}
                      className="capitalize"
                    >
                      {t(`dbs.sqlOps.status.${op.status}`)}
                    </Badge>
                  </TableCell>
                  <TableCell className="truncate max-w-[160px]" title={op.table_name}>
                    {op.table_name}
                  </TableCell>
                  <TableCell className="truncate max-w-[160px]" title={op.created_by_name}>
                    {op.created_by_name}
                  </TableCell>
                  <TableCell className="font-mono text-xs max-w-[760px]">
                    <div className="flex items-center gap-2">
                      <span className="truncate flex-1">{op.sql_statement}</span>
                      <div className="flex items-center gap-1 shrink-0">
                        <Button
                          variant="ghost"
                          isIcon
                          className="h-7 w-7"
                          onClick={() => setViewingSql(op.sql_statement)}
                          aria-label={t('common.view')}
                        >
                          <Eye size={16} />
                        </Button>
                        <Button
                          variant="ghost"
                          isIcon
                          className="h-7 w-7"
                          onClick={() => {
                            void navigator.clipboard.writeText(op.sql_statement).then(() => {
                              toast.success(t('common.toasts.copySuccess'));
                            });
                          }}
                          aria-label={t('common.copy')}
                        >
                          <Copy size={16} />
                        </Button>
                      </div>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Pagination
        className="mt-2"
        currentPage={page}
        totalPages={totalPages}
        total={total}
        pageSize={pageSize}
        onPageChange={setPage}
        prevLabel={t('common.pagination.prev')}
        nextLabel={t('common.pagination.next')}
        renderInfo={(start, end, totalItems) =>
          t('common.pagination.info', {
            start,
            end,
            total: totalItems,
          })
        }
        jumpPlaceholder={t('common.pagination.jumpPlaceholder')}
        jumpButtonLabel={t('common.pagination.jump')}
      />

      {/* SQL View Dialog */}
      <Dialog open={!!viewingSql} onOpenChange={open => !open && setViewingSql(null)}>
        <DialogContent className="max-w-2xl pb-4" aria-describedby={undefined}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {t('dbs.sql')}
              <Button
                variant="ghost"
                isIcon
                className="h-7 w-7"
                onClick={() => {
                  if (viewingSql) {
                    void navigator.clipboard.writeText(viewingSql).then(() => {
                      toast.success(t('common.toasts.copySuccess'));
                    });
                  }
                }}
              >
                <Copy size={16} />
              </Button>
            </DialogTitle>
          </DialogHeader>
          <DialogBody>
            <pre className="p-4 rounded-lg bg-muted/50 text-sm font-mono whitespace-pre-wrap break-all max-h-[60vh] overflow-auto">
              {viewingSql}
            </pre>
          </DialogBody>
        </DialogContent>
      </Dialog>
    </div>
  );
}

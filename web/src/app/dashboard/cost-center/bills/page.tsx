'use client';

import { useState, useMemo, useCallback } from 'react';
import { useT } from '@/i18n';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Download } from 'lucide-react';
import { useBills, useExportBills } from '@/hooks/pay/use-bills';
import type { TransactionType } from '@/services/types/pay';

import { Pagination } from '@/components/ui/pagination';
import { formatDate } from '@/utils/format';
import { useCloudOnlyPage } from '@/hooks/use-cloud-only-page';
import { toast } from 'sonner';
import { normalizeToastDescription } from '@/utils/error-notifications';

function formatDateToRFC(dateStr: string, isEndDate = false): string {
  if (!dateStr) return '';
  const time = isEndDate ? 'T23:59:59+08:00' : 'T00:00:00+08:00';
  return `${dateStr}${time}`;
}

/**
 * Get transaction type badge variant
 */
function getTransactionTypeVariant(
  type?: TransactionType
): 'subtle' | 'default' | 'destructive' | 'warning' | 'outline' {
  switch (type) {
    case 'recharge_purchase':
      return 'subtle';
    case 'other':
      return 'warning';
    default:
      return 'outline';
  }
}

const BILL_TRANSACTION_TYPES: TransactionType[] = ['recharge_purchase', 'other'];

// Filter state interface
interface FilterState {
  startDate: string;
  endDate: string;
  transactionType: TransactionType | 'all';
  keyword: string;
}

const initialFilterState: FilterState = {
  startDate: '',
  endDate: '',
  transactionType: 'all',
  keyword: '',
};

/**
 * @component BillsPage
 * @category Page
 * @status Stable
 * @description Cost center bills page powered by the purchase-view transactions API.
 * @usage Use in the dashboard cost center bills route.
 */
export default function BillsPage() {
  const isCloud = useCloudOnlyPage();
  const t = useT('dashboard');

  // Filter form (user input)
  const [filters, setFilters] = useState<FilterState>(initialFilterState);

  // Search params (applied filters for API query)
  const [searchParams, setSearchParams] = useState<FilterState>(initialFilterState);

  // Pagination
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const currencyFormatter = useMemo(
    () =>
      new Intl.NumberFormat(undefined, {
        style: 'currency',
        currency: 'CNY',
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
      }),
    []
  );

  const formatCurrencyAmount = useCallback(
    (amount: number | undefined, showSign = true): string => {
      if (amount === undefined || amount === null) return '-';

      if (!showSign) {
        return currencyFormatter.format(amount);
      }

      if (amount === 0) {
        return currencyFormatter.format(0);
      }

      const sign = amount > 0 ? '+ ' : '- ';
      return `${sign}${currencyFormatter.format(Math.abs(amount))}`;
    },
    [currencyFormatter]
  );

  // Build API request filters from searchParams
  const apiFilters = useMemo(
    () => ({
      start_time: searchParams.startDate
        ? formatDateToRFC(searchParams.startDate, false)
        : undefined,
      end_time: searchParams.endDate ? formatDateToRFC(searchParams.endDate, true) : undefined,
      transaction_type:
        searchParams.transactionType === 'all' ? undefined : searchParams.transactionType,
      keyword: searchParams.keyword || undefined,
      page,
      limit: pageSize,
    }),
    [searchParams, page, pageSize]
  );

  // Fetch bills data
  const { data, isLoading } = useBills(apiFilters, { enabled: isCloud });

  // Export hook
  const { mutateAsync: exportBills, isPending: isExporting } = useExportBills();

  // Handle search - apply filters
  const handleSearch = useCallback(() => {
    setSearchParams(filters);
    setPage(1);
  }, [filters]);

  // Handle reset - clear all filters
  const handleReset = useCallback(() => {
    const resetState = initialFilterState;
    setFilters(resetState);
    setSearchParams(resetState);
    setPage(1);
  }, []);

  // Handle export - use current search params
  const handleExport = useCallback(async () => {
    try {
      const blob = await exportBills({
        start_time: searchParams.startDate
          ? formatDateToRFC(searchParams.startDate, false)
          : undefined,
        end_time: searchParams.endDate ? formatDateToRFC(searchParams.endDate, true) : undefined,
        transaction_type:
          searchParams.transactionType === 'all' ? undefined : searchParams.transactionType,
        keyword: searchParams.keyword || undefined,
      });

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${t('costCenter.bills.exportFileName')}${formatDate(new Date(), 'YYYY-MM-DD')}.xlsx`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);

      toast.success(t('costCenter.bills.exportSuccess'), {
        description: t('costCenter.bills.exportSuccessDesc'),
      });
    } catch (error) {
      const title = t('costCenter.bills.exportFailed');
      const description = (error as Error).message || t('costCenter.bills.exportFailedDesc');
      toast.error(title, {
        description: normalizeToastDescription(title, description),
      });
    }
  }, [searchParams, exportBills, t]);

  if (!isCloud) {
    return null;
  }

  const transactions = data?.data ?? [];
  const total = data?.total ?? 0;
  const currentPage = data?.page ?? 1;
  const limit = data?.limit ?? pageSize;
  const totalPages = Math.ceil(total / limit);

  return (
    <div className="p-4 space-y-5 h-full overflow-y-auto">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold mb-1">{t('costCenter.bills.title')}</h1>
      </div>

      {/* Filters */}
      <div className="p-4 border rounded-lg bg-card">
        <div className="flex items-center justify-between gap-3 flex-wrap">
          <div className="flex items-center gap-3 flex-wrap">
            {/* Date Range */}
            <div className="flex items-center gap-2">
              <Input
                type="date"
                value={filters.startDate}
                onChange={e => setFilters(prev => ({ ...prev, startDate: e.target.value }))}
                max={filters.endDate || undefined}
                className="w-[150px]"
                placeholder={t('costCenter.bills.filters.startDate')}
              />
              <span className="text-muted-foreground">-</span>
              <Input
                type="date"
                value={filters.endDate}
                onChange={e => setFilters(prev => ({ ...prev, endDate: e.target.value }))}
                min={filters.startDate || undefined}
                className="w-[150px]"
                placeholder={t('costCenter.bills.filters.endDate')}
              />
            </div>

            {/* Transaction Type */}
            <div className="w-[140px]">
              <Select
                value={filters.transactionType}
                onValueChange={v =>
                  setFilters(prev => ({
                    ...prev,
                    transactionType: v as TransactionType | 'all',
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue placeholder={t('costCenter.bills.filters.allTypes')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('costCenter.bills.filters.allTypes')}</SelectItem>
                  {BILL_TRANSACTION_TYPES.map(type => (
                    <SelectItem key={type} value={type}>
                      {t(`costCenter.bills.transactionTypes.${type}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* Keyword */}
            <div className="w-[240px]">
              <Input
                placeholder={t('costCenter.bills.filters.keywordPlaceholder')}
                value={filters.keyword}
                onChange={e => setFilters(prev => ({ ...prev, keyword: e.target.value }))}
                onKeyDown={e => {
                  if (e.key === 'Enter') {
                    handleSearch();
                  }
                }}
              />
            </div>
          </div>

          {/* Action Buttons */}
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={handleReset}>
              {t('costCenter.bills.actions.reset')}
            </Button>
            <Button onClick={handleSearch}>{t('costCenter.bills.actions.search')}</Button>
          </div>
        </div>
      </div>

      {/* Bill Details Table */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">{t('costCenter.bills.table.title')}</h2>
          <Button onClick={handleExport} disabled={isExporting || isLoading}>
            <Download className="h-4 w-4 mr-2" />
            {isExporting
              ? t('costCenter.bills.actions.exporting')
              : t('costCenter.bills.actions.exportExcel')}
          </Button>
        </div>

        <div className="border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('costCenter.bills.table.transactionId')}</TableHead>
                <TableHead>{t('costCenter.bills.table.time')}</TableHead>
                <TableHead>{t('costCenter.bills.table.transactionType')}</TableHead>
                <TableHead>{t('costCenter.bills.table.details')}</TableHead>
                <TableHead className="text-right">
                  {t('costCenter.bills.table.rechargeAmount')}
                </TableHead>
                <TableHead className="text-right">
                  {t('costCenter.bills.table.walletChange')}
                </TableHead>
                <TableHead className="text-right">
                  {t('costCenter.bills.table.balanceAfter')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                Array.from({ length: 5 }).map((_, i) => (
                  <TableRow key={i}>
                    <TableCell>
                      <Skeleton className="h-4 w-24" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-4 w-32" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-5 w-20 rounded-full" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-4 w-48" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-4 w-32" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-4 w-32" />
                    </TableCell>
                    <TableCell>
                      <Skeleton className="h-4 w-32" />
                    </TableCell>
                  </TableRow>
                ))
              ) : transactions.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-center text-muted-foreground py-8">
                    {t('costCenter.bills.table.noData')}
                  </TableCell>
                </TableRow>
              ) : (
                transactions.map(transaction => {
                  return (
                    <TableRow key={transaction.id ?? transaction.batch_id}>
                      <TableCell className="font-mono text-sm">{transaction.id ?? '-'}</TableCell>
                      <TableCell>
                        {formatDate(transaction.created_at, 'YYYY-MM-DD HH:mm:ss')}
                      </TableCell>
                      <TableCell>
                        <Badge variant={getTransactionTypeVariant(transaction.transaction_type)}>
                          {t(`costCenter.bills.transactionTypes.${transaction.transaction_type}`)}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-md truncate">{transaction.detail_text || '-'}</TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrencyAmount(transaction.recharge_amount, false)}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrencyAmount(transaction.wallet_change_amount, true)}
                      </TableCell>
                      <TableCell className="text-right font-medium tabular-nums">
                        {formatCurrencyAmount(transaction.balance_after, false)}
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>

          {/* Pagination */}
          {!isLoading && transactions.length > 0 && totalPages > 1 && (
            <div className="p-4 border-t">
              <Pagination
                currentPage={currentPage}
                totalPages={totalPages}
                total={total}
                pageSize={limit}
                onPageChange={setPage}
                showInfo
                showJump
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

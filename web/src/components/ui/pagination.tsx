'use client';

import React from 'react';
import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';

interface PaginationProps {
  currentPage: number;
  totalPages: number;
  total: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  showInfo?: boolean;
  className?: string;
  /** Optional label for the previous button (i18n-friendly). */
  prevLabel?: string;
  /** Optional label for the next button (i18n-friendly). */
  nextLabel?: string;
  /** Optional render function for the info text (i18n-friendly). */
  renderInfo?: (start: number, end: number, total: number) => React.ReactNode;
  /** Show the jump-to-page control */
  showJump?: boolean;
  /** Placeholder for the jump input (i18n-friendly). */
  jumpPlaceholder?: string;
  /** Label for the jump action button (i18n-friendly). */
  jumpButtonLabel?: string;
}

export function Pagination({
  currentPage,
  totalPages,
  total,
  pageSize,
  onPageChange,
  showInfo = true,
  className,
  prevLabel,
  nextLabel,
  renderInfo,
  showJump = true,
  jumpPlaceholder,
  jumpButtonLabel,
}: PaginationProps) {
  // Generate page numbers to display
  const tCommon = useT('common');

  const safePrevLabel = prevLabel ?? tCommon('pagination.prev', { defaultMessage: 'Previous' });
  const safeNextLabel = nextLabel ?? tCommon('pagination.next', { defaultMessage: 'Next' });
  const safeJumpPlaceholder =
    jumpPlaceholder ?? tCommon('pagination.jumpPlaceholder', { defaultMessage: 'Page' });
  const safeJumpButtonLabel =
    jumpButtonLabel ?? tCommon('pagination.jump', { defaultMessage: 'Go' });

  const getPageNumbers = () => {
    const delta = 2; // Number of pages to show on each side of current page
    const pages: Array<number | 'ellipsis'> = [];

    // Always show first page
    pages.push(1);

    // Calculate range around current page
    const rangeStart = Math.max(2, currentPage - delta);
    const rangeEnd = Math.min(totalPages - 1, currentPage + delta);

    // Add ellipsis before range if needed
    if (rangeStart > 2) {
      pages.push('ellipsis');
    }

    // Add range pages
    for (let i = rangeStart; i <= rangeEnd; i++) {
      if (i !== 1 && i !== totalPages) {
        pages.push(i);
      }
    }

    // Add ellipsis after range if needed
    if (rangeEnd < totalPages - 1) {
      pages.push('ellipsis');
    }

    // Always show last page if more than 1 page
    if (totalPages > 1) {
      pages.push(totalPages);
    }

    return pages;
  };

  const pages = getPageNumbers();
  const startItem = (currentPage - 1) * pageSize + 1;
  const endItem = Math.min(currentPage * pageSize, total);

  // Local state for jump-to-page input
  const [inputPage, setInputPage] = React.useState<string>(String(currentPage));
  React.useEffect(() => {
    setInputPage(String(currentPage));
  }, [currentPage]);

  // Helper to clamp and jump
  const doJump = React.useCallback(() => {
    const n = parseInt(inputPage, 10);
    if (!Number.isFinite(n)) return;
    const clamped = Math.min(Math.max(n, 1), totalPages);
    if (clamped !== currentPage) onPageChange(clamped);
  }, [inputPage, totalPages, currentPage, onPageChange]);

  if (totalPages <= 1) {
    return null;
  }

  return (
    <div className={cn('flex items-center justify-between', className)}>
      {showInfo && (
        <div className="text-sm text-muted-foreground">
          {renderInfo ? (
            renderInfo(startItem, endItem, total)
          ) : (
            <span>
              显示 {startItem} - {endItem} 项，共 {total} 项
            </span>
          )}
        </div>
      )}

      <div className="flex items-center space-x-2">
        {/* Previous button */}
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(currentPage - 1)}
          disabled={currentPage === 1}
          className="flex items-center gap-1"
        >
          <ChevronLeft className="h-4 w-4" />
          {safePrevLabel}
        </Button>

        {/* Page numbers */}
        <div className="flex items-center space-x-1">
          {pages.map((page, index) => {
            if (page === 'ellipsis') {
              return (
                <div key={`ellipsis-${index}`} className="px-2">
                  <MoreHorizontal className="h-4 w-4 text-muted-foreground" />
                </div>
              );
            }

            return (
              <Button
                key={page}
                variant={page === currentPage ? 'default' : 'outline'}
                size="sm"
                onClick={() => onPageChange(page)}
                className="min-w-8"
              >
                {page}
              </Button>
            );
          })}
        </div>

        {/* Next button */}
        <Button
          variant="outline"
          size="sm"
          onClick={() => onPageChange(currentPage + 1)}
          disabled={currentPage === totalPages}
          className="flex items-center gap-1"
        >
          {safeNextLabel}
          <ChevronRight className="h-4 w-4" />
        </Button>

        {/* Jump to page */}
        {showJump && (
          <div className="flex items-center gap-2 pl-2">
            <Input
              type="number"
              min={1}
              max={totalPages}
              value={inputPage}
              onChange={e => setInputPage(e.target.value)}
              onKeyDown={e => {
                if (e.key === 'Enter') doJump();
              }}
              placeholder={safeJumpPlaceholder}
              className="w-20 h-8"
            />
            <Button size="sm" variant="outline" onClick={doJump}>
              {safeJumpButtonLabel}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

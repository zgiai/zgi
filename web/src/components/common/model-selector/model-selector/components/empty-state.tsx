'use client';

import { memo } from 'react';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

export interface EmptyStateProps {
  searchQuery: string;
  noModelsTitle: string;
  noResultsText: string;
  noModelsText: string;
  isAdminOrOwner?: boolean;
  contactAdminText?: string;
  configureText?: string;
  configureDescription?: string;
  clearSearchText?: string;
  onClearSearch?: () => void;
}

export const EmptyState = memo(function EmptyState({
  searchQuery,
  noModelsTitle,
  noResultsText,
  noModelsText,
  isAdminOrOwner = false,
  contactAdminText,
  configureText,
  configureDescription,
  clearSearchText,
  onClearSearch,
}: EmptyStateProps) {
  if (searchQuery) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 p-6 text-center">
        <p className="text-sm text-muted-foreground">
          {noResultsText} "{searchQuery}"
        </p>
        {onClearSearch && clearSearchText && (
          <Button type="button" variant="outline" size="sm" onClick={onClearSearch}>
            {clearSearchText}
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center gap-3 p-6 text-center">
      <div className="space-y-1">
        <p className="text-sm font-medium text-foreground">{noModelsTitle}</p>
        <p className="text-sm text-muted-foreground">{noModelsText}</p>
        {configureDescription && (
          <p className="text-xs text-muted-foreground">{configureDescription}</p>
        )}
      </div>
      {isAdminOrOwner ? (
        <Button variant="default" size="sm" asChild>
          <Link href="/dashboard/provider">{configureText || 'Configure'}</Link>
        </Button>
      ) : (
        contactAdminText && <p className="text-xs text-muted-foreground">{contactAdminText}</p>
      )}
    </div>
  );
});

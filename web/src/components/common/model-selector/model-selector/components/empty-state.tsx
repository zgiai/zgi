'use client';

import { memo } from 'react';
import { Button } from '@/components/ui/button';
import Link from 'next/link';

export interface EmptyStateProps {
  searchQuery: string;
  noResultsText: string;
  noModelsText: string;
  isAdminOrOwner?: boolean;
  contactAdminText?: string;
  configureText?: string;
  modelTypeLabel?: string;
}

// Empty state component for no results or no models
export const EmptyState = memo(function EmptyState({
  searchQuery,
  noResultsText,
  noModelsText,
  isAdminOrOwner = false,
  contactAdminText,
  configureText,
}: EmptyStateProps) {
  if (searchQuery) {
    return (
      <div className="p-4 text-center text-sm text-muted-foreground">
        {noResultsText} "{searchQuery}"
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center p-6 space-y-4 text-center">
      <p className="text-sm text-muted-foreground">{noModelsText}</p>
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

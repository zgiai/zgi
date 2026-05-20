'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { useProviders } from '@/hooks/provider/use-provider';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';

export interface ProviderSelectorProps {
  // Current selected provider name
  value?: string;
  // Change callback
  onChange?: (provider: string) => void;
  // Placeholder text
  placeholder?: string;
  // Disable control
  disabled?: boolean;
  // Page size to fetch providers
  limit?: number;
  className?: string;
}

/**
 * ProviderSelector - Single-select provider dropdown powered by provider API.
 * - Loads providers (first page, large limit) with skeleton on first load.
 * - Emits provider name string on change.
 */
export default function ProviderSelector({
  value,
  onChange,
  placeholder = 'Select a provider',
  disabled = false,
  limit = 20,
  className,
}: ProviderSelectorProps): JSX.Element {
  const getProviderName = useProviderI18n();
  const { items, page, hasMore, isLoading, isFetching, goToPage } = useProviders({
    limit,
    initialPage: 1,
    refetchOnWindowFocus: false,
  });

  interface Option {
    value: string;
    label: string;
  }
  const [accOptions, setAccOptions] = useState<Option[]>([]);
  const lastProcessedPageRef = useRef<number>(0);

  // Accumulate options across pages with dedup
  useEffect(() => {
    const pageOptions: Option[] = items.map(p => ({
      value: p.provider,
      label: getProviderName(p.provider, p.provider_name),
    }));

    setAccOptions(prev => {
      // Page 1 should replace, but avoid unnecessary state updates via equality check
      if (page === 1) {
        const sameLength = prev.length === pageOptions.length;
        const sameContent =
          sameLength &&
          prev.every(
            (o, i) => o.value === pageOptions[i]?.value && o.label === pageOptions[i]?.label
          );
        if (sameContent) return prev;
        lastProcessedPageRef.current = 1;
        return pageOptions;
      }

      // For subsequent pages, only process when page advances
      if (lastProcessedPageRef.current === page) return prev;
      lastProcessedPageRef.current = page;

      const seen = new Set(prev.map(o => o.value));
      const merged = [...prev];
      for (const opt of pageOptions) {
        if (!seen.has(opt.value)) {
          merged.push(opt);
          seen.add(opt.value);
        }
      }
      return merged;
    });
  }, [getProviderName, items, page]);

  // Ensure current value is present as an option for label rendering
  const renderedOptions = useMemo(() => {
    if (!value) return accOptions;
    const exists = accOptions.some(o => o.value === value);
    return exists ? accOptions : [{ value, label: getProviderName(value) }, ...accOptions];
  }, [accOptions, getProviderName, value]);

  const handleLoadMore = useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (hasMore && !isFetching) {
        goToPage(page + 1);
      }
    },
    [goToPage, hasMore, isFetching, page]
  );

  if (isLoading && accOptions.length === 0) {
    return <Skeleton className="h-10 w-full" />;
  }

  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger className={className} isLoading={isLoading || isFetching}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {renderedOptions.map(opt => (
          <SelectItem key={opt.value} value={opt.value}>
            {opt.label}
          </SelectItem>
        ))}

        {hasMore && (
          <div className="px-1 pt-1">
            <button
              type="button"
              className="w-full text-xs px-2 py-1 rounded border hover:bg-accent/50"
              onMouseDown={handleLoadMore}
              disabled={isFetching}
            >
              {isFetching ? 'Loading…' : 'Load more'}
            </button>
          </div>
        )}
      </SelectContent>
    </Select>
  );
}

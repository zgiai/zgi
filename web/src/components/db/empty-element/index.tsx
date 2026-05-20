'use client';

import React from 'react';
import { useT } from '@/i18n';
import { Database, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface DbEmptyAction {
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
  variant?: 'default' | 'outline' | 'ghost';
  highlight?: boolean;
}

interface DbEmptyElementProps {
  type?: 'search' | 'generic';
  title?: string;
  description?: string;
  actions?: DbEmptyAction[];
  className?: string;
}

const IllustrationSearch = (
  <div className="w-20 h-20 bg-gradient-to-br from-gray-100 to-gray-200 rounded-2xl flex items-center justify-center">
    <Search className="h-10 w-10 text-gray-600" />
  </div>
);

const IllustrationDatabase = (
  <div className="relative">
    <div className="w-20 h-20 bg-accent rounded-2xl flex items-center justify-center">
      <Database className="h-10 w-10 text-foreground" />
    </div>
  </div>
);

export function DbEmptyElement({
  type = 'generic',
  title,
  description,
  actions = [],
  className,
}: DbEmptyElementProps) {
  const t = useT('dbs');

  const isSearch = type === 'search';
  const resolvedTitle = title || (isSearch ? t('search.noResults') : t('empty'));
  const resolvedDesc = description || (isSearch ? t('search.noResultsDesc') : t('emptyDesc'));
  const illustration = isSearch ? IllustrationSearch : IllustrationDatabase;

  return (
    <div
      className={cn('flex flex-col items-center justify-center py-12 px-6 text-center', className)}
    >
      <div className="mb-6">{illustration}</div>
      <div className="max-w-md space-y-2 mb-8">
        <h3 className="text-lg font-semibold text-foreground">{resolvedTitle}</h3>
        <p className="text-sm text-muted-foreground leading-relaxed">{resolvedDesc}</p>
      </div>
      {actions.length > 0 && (
        <div className="flex flex-wrap gap-3 mb-8">
          {actions.map((action, index) => (
            <Button
              key={index}
              variant={action.variant || 'default'}
              onClick={action.onClick}
              className={cn(
                'flex items-center gap-2',
                action.highlight && 'bg-highlight text-primary-foreground hover:bg-highlight/80'
              )}
            >
              {action.icon}
              {action.label}
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}

export function DbEmptySearchResults({
  query,
  onClearSearch,
  className,
}: {
  query?: string;
  onClearSearch?: () => void;
  className?: string;
}) {
  const t = useT('dbs');
  const actions = onClearSearch
    ? [
        {
          label: t('search.clearFilters'),
          onClick: onClearSearch,
          variant: 'outline' as const,
        },
      ]
    : [];

  return (
    <DbEmptyElement
      type="search"
      title={query ? t('search.noResultsFor', { query }) : t('search.noResults')}
      description={t('search.noResultsDesc')}
      actions={actions}
      className={className}
    />
  );
}

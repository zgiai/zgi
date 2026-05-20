'use client';

import React from 'react';
import { useT } from '@/i18n';
import { Atom, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';

interface AgentEmptyAction {
  label: string;
  icon?: React.ReactNode;
  onClick: () => void;
  variant?: 'default' | 'outline' | 'ghost';
  highlight?: boolean;
}

interface AgentEmptyElementProps {
  type?: 'search' | 'generic';
  title?: string;
  description?: string;
  actions?: AgentEmptyAction[];
  className?: string;
}

const IllustrationSearch = (
  <div className="w-20 h-20 bg-gradient-to-br from-gray-100 to-gray-200 rounded-2xl flex items-center justify-center">
    <Search className="h-10 w-10 text-gray-600" />
  </div>
);

const IllustrationAgent = (
  <div className="relative">
    <div className="w-20 h-20 bg-accent rounded-2xl flex items-center justify-center">
      <Atom className="h-10 w-10 text-foreground" />
    </div>
  </div>
);

export function AgentEmptyElement({
  type = 'generic',
  title,
  description,
  actions = [],
  className,
}: AgentEmptyElementProps) {
  const t = useT('agents');

  const isSearch = type === 'search';
  const resolvedTitle = title || (isSearch ? t('noResults') : t('noAgentsYet'));
  const resolvedDesc =
    description ||
    (isSearch ? t('noResultsDescription', { keyword: '' }) : t('noAgentsDescription'));
  const illustration = isSearch ? IllustrationSearch : IllustrationAgent;

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

export function AgentEmptySearchResults({
  query,
  onClearSearch,
  className,
}: {
  query?: string;
  onClearSearch?: () => void;
  className?: string;
}) {
  const t = useT('agents');
  const actions = onClearSearch
    ? [
        {
          label: t('clearSearch'),
          onClick: onClearSearch,
          variant: 'outline' as const,
        },
      ]
    : [];

  return (
    <AgentEmptyElement
      type="search"
      title={query ? t('noResultsDescription', { keyword: query }) : t('noResults')} // Adjusting title to look better
      description={t('noResults')}
      actions={actions}
      className={className}
    />
  );
}

'use client';

import { Filter, Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { getKindFilterLabel, type TemplateTranslator } from './template-labels';
import type { AgentTemplateKindFilter } from './types';

interface TemplateSearchBarProps {
  kindFilter: AgentTemplateKindFilter;
  kindOptions: AgentTemplateKindFilter[];
  query: string;
  disabled: boolean;
  onKindFilterChange: (value: AgentTemplateKindFilter) => void;
  onQueryChange: (value: string) => void;
}

export function TemplateSearchBar({
  kindFilter,
  kindOptions,
  query,
  disabled,
  onKindFilterChange,
  onQueryChange,
}: TemplateSearchBarProps) {
  const t = useT();
  const templateT = t as TemplateTranslator;

  return (
    <div className="flex h-10 min-w-0 flex-1 items-center rounded-lg border bg-background shadow-sm">
      <div className="flex shrink-0 items-center gap-2 pl-3">
        <Filter className="size-3.5 text-muted-foreground" />
      </div>
      <Select
        value={kindFilter}
        disabled={disabled}
        onValueChange={value => onKindFilterChange(value as AgentTemplateKindFilter)}
      >
        <SelectTrigger className="h-9 w-28 border-0 bg-transparent px-2 text-sm shadow-none hover:border-0 focus-visible:border-0 sm:w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {kindOptions.map(kind => (
            <SelectItem key={kind} value={kind}>
              {getKindFilterLabel(templateT, kind)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <div className="h-5 w-px shrink-0 bg-border" />
      <div className="relative min-w-0 flex-1">
        <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={query}
          disabled={disabled}
          onChange={event => onQueryChange(event.target.value)}
          placeholder={t('agents.templates.searchPlaceholder')}
          className="h-9 border-0 bg-transparent pl-9 pr-3 text-sm shadow-none focus-visible:ring-0"
        />
      </div>
    </div>
  );
}

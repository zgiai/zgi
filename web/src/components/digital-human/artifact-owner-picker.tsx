'use client';

import { useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { Check, ChevronsUpDown, Search, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Skeleton } from '@/components/ui/skeleton';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import type { ScopedTranslations } from '@/i18n/translations';
import type { DigitalHumanArtifact, DigitalHumanArtifactOwner, DigitalHumanArtifactOwnerKind } from '@/services/types/digital-human';

/**
 * Picks one owner of the artifacts the reader sees. The list is searched on
 * the server (name, email, end user identifier, or ID prefix) and paged, so
 * it works however many people keep artifacts.
 */
export function ArtifactOwnerPicker({
  digitalHumanId,
  kind,
  archived,
  disabled,
  value,
  valueLabel,
  onChange,
}: {
  digitalHumanId: string;
  kind?: DigitalHumanArtifactOwnerKind;
  archived?: 'archived' | 'all';
  disabled?: boolean;
  value: string;
  // The chosen owner's name, when the page already knows it.
  valueLabel?: string;
  onChange: (owner: DigitalHumanArtifactOwner | null) => void;
}) {
  const t = useT('digitalHumans');
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const query = useDebouncedValue(search.trim(), 300);
  const list = useInfiniteQuery({
    queryKey: ['digital-human-artifact-owners', digitalHumanId, kind ?? '', archived ?? '', query],
    queryFn: ({ pageParam }) =>
      digitalHumanService.listArtifactOwners(digitalHumanId, { query: query || undefined, kind, archived, page: pageParam, page_size: 20 }),
    initialPageParam: 1,
    getNextPageParam: last => (last.data.has_more ? last.data.page + 1 : undefined),
    enabled: open,
  });
  const items = list.data?.pages.flatMap(page => page.data.items) ?? [];
  const label = value ? valueLabel || value.slice(0, 8) : t('artifacts.filters.anyPerson');

  return (
    <div className="flex min-w-0 items-center xl:w-[180px]">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label={t('artifacts.filters.person')}
            disabled={disabled}
            className={cn('h-9 w-full min-w-0 justify-between px-3 font-normal', value && 'rounded-r-none border-r-0')}
          >
            <span className={cn('truncate', !value && 'text-muted-foreground')}>{label}</span>
            <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-[320px] p-0">
          <div className="border-b p-2">
            <Input
              autoFocus
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('artifacts.filters.personSearch')}
              leftIcon={<Search />}
              className="h-8"
            />
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {list.isLoading ? (
              <div className="space-y-1 p-1">
                <Skeleton className="h-8 w-full" />
                <Skeleton className="h-8 w-full" />
              </div>
            ) : items.length === 0 ? (
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t('artifacts.filters.noPeople')}</p>
            ) : (
              items.map(item => (
                <button
                  key={`${item.kind}-${item.id}`}
                  type="button"
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
                  onClick={() => {
                    onChange(item);
                    setOpen(false);
                  }}
                >
                  <Check className={cn('size-4 shrink-0', item.id === value ? 'opacity-100' : 'opacity-0')} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{ownerLabel(t, item)}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {item.kind === 'account' && item.email && item.email !== item.name
                        ? item.email
                        : t(`artifacts.filters.ownerKinds.${item.kind}`)}
                    </span>
                  </span>
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{item.count}</span>
                </button>
              ))
            )}
            {list.hasNextPage ? (
              <Button
                variant="ghost"
                size="sm"
                className="mt-1 w-full"
                disabled={list.isFetchingNextPage}
                onClick={() => void list.fetchNextPage()}
              >
                {t('artifacts.filters.morePeople')}
              </Button>
            ) : null}
          </div>
        </PopoverContent>
      </Popover>
      {value ? (
        <Button
          variant="outline"
          className="h-9 shrink-0 rounded-l-none px-2"
          aria-label={t('artifacts.filters.clearPerson')}
          title={t('artifacts.filters.clearPerson')}
          onClick={() => onChange(null)}
        >
          <X className="size-4" />
        </Button>
      ) : null}
    </div>
  );
}

/** Whom an artifact belongs to, in words: a member, an end user, or the team. */
export function artifactOwnerLabel(
  t: ScopedTranslations<'digitalHumans'>,
  artifact: Pick<DigitalHumanArtifact, 'owner_kind' | 'owner_id' | 'owner_name'>
) {
  switch (artifact.owner_kind) {
    case 'workspace':
      return t('artifacts.owner.team');
    case 'application':
      return t('artifacts.owner.application');
    case 'api_end_user':
      return artifact.owner_name
        ? t('artifacts.owner.endUserNamed', { name: artifact.owner_name })
        : t('artifacts.owner.endUser', { id: artifact.owner_id.slice(0, 8) });
    default:
      return artifact.owner_name || t('artifacts.owner.member');
  }
}

function ownerLabel(t: ScopedTranslations<'digitalHumans'>, owner: DigitalHumanArtifactOwner) {
  return artifactOwnerLabel(t, { owner_kind: owner.kind, owner_id: owner.id, owner_name: owner.name });
}

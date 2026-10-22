'use client';

import { useState } from 'react';
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Check, ChevronsUpDown, PencilLine, Search, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Skeleton } from '@/components/ui/skeleton';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import { getErrorMessage } from '@/utils/error-notifications';
import type { DigitalHumanTaskRequester, DigitalHumanTaskRequesterKind } from '@/services/types/digital-human';

type Translate = ReturnType<typeof useT<'digitalHumans'>>;

export function requesterName(requester: DigitalHumanTaskRequester, t: Translate) {
  if (requester.kind === 'api_user') {
    return requester.name || t('tasks.requester.apiUser', { id: (requester.id ?? '').slice(0, 8) });
  }
  if (requester.kind === 'channel') return t('tasks.requester.channel');
  return requester.name || requester.email || t('tasks.requester.unknownMember');
}

/**
 * Picks one person who asked for tasks. The list is searched on the server
 * (name, email, API user label, ID prefix, or a task's client reference) and
 * paged, so it works however many people use the DigitalHuman.
 */
export function TaskRequesterPicker({
  digitalHumanId,
  kind,
  value,
  valueLabel,
  onChange,
}: {
  digitalHumanId: string;
  kind?: DigitalHumanTaskRequesterKind;
  value: string;
  // The chosen person's name, when the page already knows it.
  valueLabel?: string;
  onChange: (requester: DigitalHumanTaskRequester | null) => void;
}) {
  const t = useT('digitalHumans');
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const query = useDebouncedValue(search.trim(), 300);
  const pickable = kind === 'channel' ? undefined : kind;
  const list = useInfiniteQuery({
    queryKey: ['digital-human-task-requesters', digitalHumanId, pickable ?? '', query],
    queryFn: ({ pageParam }) =>
      digitalHumanService.listTaskRequesters(digitalHumanId, { query: query || undefined, kind: pickable, page: pageParam, page_size: 20 }),
    initialPageParam: 1,
    getNextPageParam: last => (last.data.has_more ? last.data.page + 1 : undefined),
    enabled: open,
  });
  const items = list.data?.pages.flatMap(page => page.data.items) ?? [];
  const label = value ? valueLabel || value.slice(0, 8) : t('tasks.filters.anyPerson');

  return (
    <div className="flex min-w-0 items-center xl:w-[180px]">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={open}
            aria-label={t('tasks.filters.person')}
            disabled={kind === 'channel'}
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
              placeholder={t('tasks.filters.personSearch')}
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
              <p className="px-3 py-6 text-center text-sm text-muted-foreground">{t('tasks.filters.noPeople')}</p>
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
                    <span className="block truncate">{requesterName(item, t)}</span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {item.kind === 'member' ? item.email || t('tasks.requester.member') : t('tasks.requester.api_user')}
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
                {t('tasks.filters.morePeople')}
              </Button>
            ) : null}
          </div>
        </PopoverContent>
      </Popover>
      {value ? (
        <Button
          variant="outline"
          className="h-9 shrink-0 rounded-l-none px-2"
          aria-label={t('tasks.filters.clearPerson')}
          title={t('tasks.filters.clearPerson')}
          onClick={() => onChange(null)}
        >
          <X className="size-4" />
        </Button>
      ) : null}
    </div>
  );
}

/** Names an API end user for the task center; ZGI keeps no name for them. */
export function TaskRequesterLabelButton({
  digitalHumanId,
  requester,
}: {
  digitalHumanId: string;
  requester: DigitalHumanTaskRequester;
}) {
  const t = useT('digitalHumans');
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [label, setLabel] = useState(requester.name ?? '');
  const save = useMutation({
    mutationFn: () => digitalHumanService.labelTaskRequester(digitalHumanId, requester.id ?? '', label.trim()),
    onSuccess: () => {
      toast.success(t('tasks.requester.labelSaved'));
      setOpen(false);
      void client.invalidateQueries({ queryKey: ['digital-human-task-requesters', digitalHumanId] });
      void client.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.tasks(digitalHumanId) });
    },
    onError: error => toast.error(getErrorMessage(error) || t('tasks.requester.labelFailed')),
  });
  if (requester.kind !== 'api_user' || !requester.id) return null;
  return (
    <>
      <button
        type="button"
        className="inline-flex shrink-0 items-center text-muted-foreground hover:text-foreground"
        aria-label={t('tasks.requester.label')}
        title={t('tasks.requester.label')}
        onClick={event => {
          event.stopPropagation();
          setLabel(requester.name ?? '');
          setOpen(true);
        }}
      >
        <PencilLine className="size-3.5" />
      </button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent onClick={event => event.stopPropagation()}>
          <DialogHeader>
            <DialogTitle>{t('tasks.requester.label')}</DialogTitle>
            <DialogDescription>{t('tasks.requester.labelDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <Input
              autoFocus
              value={label}
              maxLength={128}
              onChange={event => setLabel(event.target.value)}
              placeholder={t('tasks.requester.labelPlaceholder')}
            />
            <p className="mt-2 font-mono text-xs text-muted-foreground">{requester.id}</p>
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <Button disabled={save.isPending} onClick={() => save.mutate()}>
              {t('common.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

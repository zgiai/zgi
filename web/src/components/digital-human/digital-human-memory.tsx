'use client';

import { useMemo, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { useDigitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import {
  AlertCircle,
  BrainCircuit,
  ChevronDown,
  ChevronRight,
  Clock3,
  ExternalLink,
  FileCheck2,
  FilterX,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  RotateCcw,
  Eye,
  EyeOff,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  useDigitalHumanMemoryPages,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanMemory as DigitalHumanMemoryRecord,
  DigitalHumanMemoryFacets,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryOrigin,
  DigitalHumanMemoryStatus,
  DigitalHumanMemoryVisibility,
  DigitalHumanMemoryType,
} from '@/services/types/digital-human';

const all = 'all';
const memoryTypes: DigitalHumanMemoryType[] = [
  'identity',
  'experience',
  'decision',
  'fact',
  'open_loop',
  'reflection',
];
const visibilities: DigitalHumanMemoryVisibility[] = ['everyone', 'internal'];
const origins: DigitalHumanMemoryOrigin[] = ['operator', 'member', 'channel', 'end_user'];
// Past this many memories the filters are shown without asking.
const filtersShownFrom = 20;

export function DigitalHumanMemory({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const canManage = permissions.hasPermission('digital_human.memory.manage');
  const [search, setSearch] = useState('');
  // Invalidated memories no longer shape behavior; they are hidden unless
  // the reader asks for them.
  const [status, setStatus] = useState<typeof all | DigitalHumanMemoryStatus>('active');
  const [visibility, setVisibility] = useState<typeof all | DigitalHumanMemoryVisibility>(all);
  const [origin, setOrigin] = useState<typeof all | DigitalHumanMemoryOrigin>(all);
  const [channel, setChannel] = useState<string>(all);
  const [source, setSource] = useState('');
  // Null until the reader opens or closes the filters themselves.
  const [filtersOpen, setFiltersOpen] = useState<boolean | null>(null);
  const [repairOpen, setRepairOpen] = useState(false);
  const [selected, setSelected] = useState<DigitalHumanMemoryRecord | null>(null);
  // Memories picked for a bulk action, by ID.
  const [picked, setPicked] = useState<Set<string>>(() => new Set());
  const [bulkInvalidateOpen, setBulkInvalidateOpen] = useState(false);
  const params = useMemo<DigitalHumanMemoryListParams>(
    () => ({
      page_size: 100,
      search: search.trim() || undefined,
      status: status === all ? undefined : status,
      source: source.trim() || undefined,
      visibility: visibility === all ? undefined : visibility,
      origin: origin === all ? undefined : origin,
      origin_channel_id: channel === all ? undefined : channel,
    }),
    [channel, origin, search, source, status, visibility]
  );
  const query = useDigitalHumanMemoryPages(id, params);
  const mutations = useDigitalHumanMutations(id);
  const data = query.data?.pages[0]?.data;
  const memories = useMemo(
    () => query.data?.pages.flatMap(page => page.data?.items ?? []) ?? [],
    [query.data]
  );
  const facets = data?.facets;
  const hasFilters = Boolean(
    search.trim() ||
      status !== 'active' ||
      source.trim() ||
      visibility !== all ||
      origin !== all ||
      channel !== all
  );
  const pickedMemories = memories.filter(memory => picked.has(memory.id));
  const showFilters = filtersOpen ?? (hasFilters || (data?.total ?? 0) > filtersShownFrom);

  const clearFilters = () => {
    setSearch('');
    setStatus('active');
    setSource('');
    setVisibility(all);
    setOrigin(all);
    setChannel(all);
  };

  const togglePicked = (ids: string[], on: boolean) => {
    setPicked(current => {
      const next = new Set(current);
      ids.forEach(memoryId => (on ? next.add(memoryId) : next.delete(memoryId)));
      return next;
    });
  };

  const applyBulk = async (
    action: 'set_visibility' | 'invalidate',
    next?: DigitalHumanMemoryVisibility
  ) => {
    if (pickedMemories.length === 0) return;
    try {
      const result = await mutations.applyMemoryBulk.mutateAsync({
        digitalHumanId: id,
        request: {
          operation_id: crypto.randomUUID(),
          action,
          visibility: next,
          memory_ids: pickedMemories.map(memory => memory.id),
        },
      });
      toast.success(
        t('memory.bulk.done', {
          updated: result.data.updated,
          skipped: result.data.skipped.length,
        })
      );
      setPicked(new Set());
      setBulkInvalidateOpen(false);
    } catch {
      // The mutation already reports the error.
    }
  };

  const invalidate = async (memory: DigitalHumanMemoryRecord) => {
    const state = data?.state;
    if (!state) return;
    await mutations.applyMemoryProposal.mutateAsync({
      digitalHumanId: id,
      request: {
        operation_id: crypto.randomUUID(),
        key: memory.memory_key,
        action: 'clear',
        memory_type: memory.memory_type,
        expected_revision: memory.revision,
        config_revision: state.config_revision,
        memory_epoch: state.memory_epoch,
        evidence: [{ type: 'manual', id: `operator:${crypto.randomUUID()}` }],
        source_context: {
          private_conversation: false,
          promoted_by_authorized_actor: true,
        },
        admin_approved: true,
        occurred_at: new Date().toISOString(),
      },
    });
    toast.success(t('memory.invalidated'));
    setSelected(null);
  };

  const changeVisibility = async (
    memory: DigitalHumanMemoryRecord,
    next: DigitalHumanMemoryVisibility
  ) => {
    const state = data?.state;
    if (!state) return;
    try {
      await mutations.setMemoryVisibility.mutateAsync({
        digitalHumanId: id,
        memoryId: memory.id,
        request: {
          operation_id: crypto.randomUUID(),
          expected_revision: memory.revision,
          config_revision: state.config_revision,
          memory_epoch: state.memory_epoch,
          visibility: next,
        },
      });
    } catch {
      // The mutation already reports the error.
      return;
    }
    toast.success(t('memory.visibility.done'));
    setSelected(null);
  };

  const undo = async (memory: DigitalHumanMemoryRecord) => {
    await mutations.undoMemory.mutateAsync({
      digitalHumanId: id,
      operationId: memory.last_operation_id,
      expectedRevision: memory.revision,
    });
    toast.success(t('memory.undone'));
    setSelected(null);
  };

  const repairMemory = async () => {
    const result = await mutations.repairMemory.mutateAsync({ digitalHumanId: id });
    toast.success(t('memory.repaired', { count: result.data.memory_count }));
  };

  return (
    <div className="mx-auto flex max-w-[1600px] flex-col gap-5 px-4 py-6 md:px-8 xl:py-8">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <BrainCircuit className="size-5" />
            </div>
            <div>
              <h2 className="text-xl font-semibold">{t('memory.title')}</h2>
              <p className="mt-0.5 max-w-2xl text-sm text-muted-foreground">
                {t('memory.description')}
              </p>
            </div>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {data?.state ? (
            <>
              <span className="text-sm text-muted-foreground">
                {t('memory.count', { count: data.total })}
              </span>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge
                    variant={data.state.auto_extraction_enabled ? 'success' : 'subtle'}
                    className="cursor-default"
                  >
                    {data.state.auto_extraction_enabled ? t('memory.workOn') : t('memory.workOff')}
                  </Badge>
                </TooltipTrigger>
                <TooltipContent className="max-w-72 leading-5">
                  {t('memory.workChipHint')}
                </TooltipContent>
              </Tooltip>
            </>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} />
            {t('common.refresh')}
          </Button>
          {canManage ? (
            <>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="sm" isIcon aria-label={t('memory.more')}>
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="max-w-72">
                  <DropdownMenuItem
                    disabled={mutations.repairMemory.isPending}
                    onSelect={() => setRepairOpen(true)}
                  >
                    {mutations.repairMemory.isPending ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Wrench className="size-4" />
                    )}
                    <div className="min-w-0">
                      <div>
                        {mutations.repairMemory.isPending
                          ? t('memory.repairing')
                          : t('memory.repair')}
                      </div>
                      <div className="text-xs text-muted-foreground">{t('memory.repairHint')}</div>
                    </div>
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
              <ConfirmDialog
                open={repairOpen}
                onOpenChange={setRepairOpen}
                variant="default"
                title={t('memory.repairTitle')}
                description={t('memory.repairDescription')}
                confirmText={t('memory.repair')}
                cancelText={t('common.cancel')}
                loading={mutations.repairMemory.isPending}
                onConfirm={() => void repairMemory()}
              />
            </>
          ) : null}
        </div>
      </header>

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="space-y-2 border-b p-4">
          <div className="flex gap-2">
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('memory.searchPlaceholder')}
              leftIcon={<Search />}
              className="flex-1"
            />
            <Button
              variant={showFilters ? 'secondary' : 'outline'}
              size="sm"
              className="h-9 shrink-0"
              aria-expanded={showFilters}
              onClick={() => setFiltersOpen(!showFilters)}
            >
              <SlidersHorizontal className="size-4" />
              {t('memory.filters')}
            </Button>
          </div>
          {showFilters ? (
            <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-[repeat(3,minmax(150px,1fr))_minmax(180px,1fr)_auto]">
              <MemorySelect
                value={status}
                onValueChange={value => setStatus(value as typeof status)}
                placeholder={t('memory.allStatuses')}
              >
                <SelectItem value={all}>{t('memory.allStatuses')}</SelectItem>
                <SelectItem value="active">{t('memory.status.active')}</SelectItem>
                <SelectItem value="invalidated">{t('memory.status.invalidated')}</SelectItem>
                <SelectItem value="archived">{t('memory.status.archived')}</SelectItem>
              </MemorySelect>
              <MemorySelect
                value={visibility}
                onValueChange={value => setVisibility(value as typeof visibility)}
                placeholder={t('memory.visibility.all')}
              >
                <SelectItem value={all}>{t('memory.visibility.all')}</SelectItem>
                {visibilities.map(value => (
                  <SelectItem key={value} value={value}>
                    {withCount(t(`memory.visibility.${value}`), facets?.visibility[value])}
                  </SelectItem>
                ))}
              </MemorySelect>
              <MemorySelect
                value={origin}
                onValueChange={value => {
                  setOrigin(value as typeof origin);
                  if (value !== 'channel') setChannel(all);
                }}
                placeholder={t('memory.origin.all')}
              >
                <SelectItem value={all}>{t('memory.origin.all')}</SelectItem>
                {origins.map(value => (
                  <SelectItem key={value} value={value}>
                    {withCount(t(`memory.origin.${value}`), facets?.origins[value])}
                  </SelectItem>
                ))}
              </MemorySelect>
              <Input
                value={source}
                onChange={event => setSource(event.target.value)}
                placeholder={t('memory.sourcePlaceholder')}
              />
              <Button
                variant="ghost"
                size="sm"
                className="h-9"
                onClick={clearFilters}
                disabled={!hasFilters}
              >
                <FilterX className="size-4" />
                {t('memory.clearFilters')}
              </Button>
            </div>
          ) : null}
          {showFilters && origin === 'channel' && facets?.channels.length ? (
            <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-[minmax(240px,1fr)_minmax(150px,0.4fr)]">
              <div className="self-center text-xs text-muted-foreground">
                {t('memory.channelHint')}
              </div>
              <MemorySelect
                value={channel}
                onValueChange={setChannel}
                placeholder={t('memory.allChannels')}
              >
                <SelectItem value={all}>{t('memory.allChannels')}</SelectItem>
                {facets.channels.map(item => (
                  <SelectItem key={item.id} value={item.id}>
                    {withCount(t('memory.channelLabel', { id: shortID(item.id) }), item.count)}
                  </SelectItem>
                ))}
              </MemorySelect>
            </div>
          ) : null}
        </div>

        {canManage && picked.size > 0 ? (
          <div className="flex flex-wrap items-center gap-2 border-b bg-primary/5 px-4 py-2 text-sm">
            <span className="font-medium">{t('memory.bulk.selected', { count: picked.size })}</span>
            <Button
              variant="outline"
              size="sm"
              disabled={mutations.applyMemoryBulk.isPending}
              onClick={() => void applyBulk('set_visibility', 'everyone')}
            >
              <Eye className="size-4" />
              {t('memory.visibility.makeEveryone')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={mutations.applyMemoryBulk.isPending}
              onClick={() => void applyBulk('set_visibility', 'internal')}
            >
              <EyeOff className="size-4" />
              {t('memory.visibility.makeInternal')}
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={mutations.applyMemoryBulk.isPending}
              onClick={() => setBulkInvalidateOpen(true)}
            >
              <Trash2 className="size-4" />
              {t('memory.invalidate')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto"
              onClick={() => setPicked(new Set())}
            >
              {t('memory.bulk.clearSelection')}
            </Button>
            <ConfirmDialog
              open={bulkInvalidateOpen}
              onOpenChange={setBulkInvalidateOpen}
              variant="danger"
              title={t('memory.bulk.invalidateTitle', { count: picked.size })}
              description={t('memory.bulk.invalidateDescription')}
              confirmText={t('memory.invalidate')}
              cancelText={t('common.cancel')}
              loading={mutations.applyMemoryBulk.isPending}
              onConfirm={() => void applyBulk('invalidate')}
            />
          </div>
        ) : null}

        <MemoryGroups
          loading={query.isLoading}
          error={query.isError}
          memories={memories}
          facets={facets}
          total={data?.total ?? 0}
          hasFilters={hasFilters}
          canPick={canManage}
          picked={picked}
          onPick={togglePicked}
          onRetry={() => void query.refetch()}
          onClearFilters={clearFilters}
          onSelect={setSelected}
        />
        {query.hasNextPage ? (
          <div className="flex items-center justify-center gap-3 border-t px-4 py-3 text-xs text-muted-foreground">
            <span>{t('memory.showing', { shown: memories.length, total: data?.total ?? 0 })}</span>
            <Button
              variant="outline"
              size="sm"
              disabled={query.isFetchingNextPage}
              onClick={() => void query.fetchNextPage()}
            >
              {query.isFetchingNextPage ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('memory.loadMore')}
            </Button>
          </div>
        ) : null}
      </section>

      <MemoryDetail
        digitalHumanId={id}
        memory={selected}
        canManage={canManage}
        onOpenChange={open => !open && setSelected(null)}
        onInvalidate={invalidate}
        onSetVisibility={changeVisibility}
        onUndo={undo}
        invalidating={
          mutations.applyMemoryProposal.isPending || mutations.setMemoryVisibility.isPending
        }
        undoing={mutations.undoMemory.isPending}
      />
    </div>
  );
}

function MemorySelect({
  children,
  value,
  onValueChange,
  placeholder,
}: {
  children: ReactNode;
  value: string;
  onValueChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <Select value={value} onValueChange={onValueChange}>
      <SelectTrigger className="w-full">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>{children}</SelectContent>
    </Select>
  );
}

function MemoryGroups({
  loading,
  error,
  memories,
  facets,
  total,
  hasFilters,
  canPick,
  picked,
  onPick,
  onRetry,
  onClearFilters,
  onSelect,
}: {
  loading: boolean;
  error: boolean;
  memories: DigitalHumanMemoryRecord[];
  facets?: DigitalHumanMemoryFacets;
  total: number;
  hasFilters: boolean;
  canPick: boolean;
  picked: Set<string>;
  onPick: (ids: string[], on: boolean) => void;
  onRetry: () => void;
  onClearFilters: () => void;
  onSelect: (memory: DigitalHumanMemoryRecord) => void;
}) {
  const t = useT('digitalHumans');
  const [collapsed, setCollapsed] = useState<Set<DigitalHumanMemoryType>>(() => new Set());
  if (loading) {
    return (
      <div className="flex min-h-72 items-center justify-center">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (error) {
    return (
      <div className="flex min-h-72 flex-col items-center justify-center gap-3 px-6 text-center">
        <AlertCircle className="size-7 text-destructive" />
        <p className="text-sm font-medium">{t('memory.loadError')}</p>
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t('memory.retry')}
        </Button>
      </div>
    );
  }
  if (memories.length === 0) {
    return (
      <div className="flex min-h-72 flex-col items-center justify-center px-6 text-center">
        <div className="rounded-full bg-muted p-4">
          <BrainCircuit className="size-7 text-muted-foreground" />
        </div>
        <p className="mt-4 text-sm font-medium">{t('memory.empty')}</p>
        <p className="mt-1 max-w-md text-sm text-muted-foreground">
          {t('memory.emptyDescription')}
        </p>
        {hasFilters ? (
          <Button variant="outline" size="sm" className="mt-3" onClick={onClearFilters}>
            <FilterX className="size-4" />
            {t('memory.clearFilters')}
          </Button>
        ) : null}
      </div>
    );
  }
  const groups = memoryTypes
    .map(type => ({ type, items: memories.filter(memory => memory.memory_type === type) }))
    .filter(group => group.items.length > 0);
  // Without filters the facet count is the group's full size; with filters
  // only the loaded entries are known.
  const groupTotal = (type: DigitalHumanMemoryType, loaded: number) =>
    !hasFilters && memories.length < total ? (facets?.types[type] ?? loaded) : loaded;
  return (
    <div className="divide-y">
      {groups.map(group => {
        const open = !collapsed.has(group.type);
        const ids = group.items.map(memory => memory.id);
        const allPicked = ids.every(memoryId => picked.has(memoryId));
        const somePicked = !allPicked && ids.some(memoryId => picked.has(memoryId));
        return (
          <section key={group.type}>
            <div className="flex items-center gap-3 bg-muted/30 px-4 py-2">
              {canPick ? (
                <Checkbox
                  checked={allPicked ? true : somePicked ? 'indeterminate' : false}
                  aria-label={t('memory.bulk.pickGroup')}
                  onCheckedChange={checked => onPick(ids, checked === true)}
                />
              ) : null}
              <button
                type="button"
                className="flex flex-1 items-center gap-2 text-left text-sm font-medium"
                aria-expanded={open}
                onClick={() =>
                  setCollapsed(current => {
                    const next = new Set(current);
                    if (open) next.add(group.type);
                    else next.delete(group.type);
                    return next;
                  })
                }
              >
                {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                {t(`memory.type.${group.type}`)}
                <Badge variant="subtle">{groupTotal(group.type, group.items.length)}</Badge>
              </button>
            </div>
            {open ? (
              <div className="divide-y">
                {group.items.map(memory => (
                  <MemoryRow
                    key={memory.id}
                    memory={memory}
                    canPick={canPick}
                    picked={picked.has(memory.id)}
                    onPick={on => onPick([memory.id], on)}
                    onSelect={() => onSelect(memory)}
                  />
                ))}
              </div>
            ) : null}
          </section>
        );
      })}
    </div>
  );
}

// One compact line per memory: its first line and the attributes an operator
// scans for; the path shows on hover, the full entry in the side sheet.
function MemoryRow({
  memory,
  canPick,
  picked,
  onPick,
  onSelect,
}: {
  memory: DigitalHumanMemoryRecord;
  canPick: boolean;
  picked: boolean;
  onPick: (on: boolean) => void;
  onSelect: () => void;
}) {
  const t = useT('digitalHumans');
  return (
    <div
      className={cn(
        'group flex items-center gap-3 px-4 py-2.5 transition-colors hover:bg-muted/30',
        picked && 'bg-primary/5'
      )}
    >
      {canPick ? (
        <Checkbox
          checked={picked}
          aria-label={t('memory.bulk.pick')}
          onCheckedChange={checked => onPick(checked === true)}
        />
      ) : null}
      <button
        type="button"
        onClick={onSelect}
        title={memory.path || undefined}
        className="grid min-w-0 flex-1 gap-1 text-left md:grid-cols-[minmax(0,1fr)_auto] md:items-center md:gap-4"
      >
        <div className="min-w-0">
          <p
            className={cn(
              'truncate text-sm',
              memory.status !== 'active' && 'text-muted-foreground line-through'
            )}
          >
            {memory.content.split('\n')[0]}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground md:justify-end">
          {memory.status !== 'active' ? <MemoryStatusBadge status={memory.status} /> : null}
          <MemoryVisibilityBadge memory={memory} compact />
          <span className="whitespace-nowrap">{formatShortDate(memory.updated_at)}</span>
          <ExternalLink className="size-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
        </div>
      </button>
    </div>
  );
}

function MemoryDetail({
  digitalHumanId,
  memory,
  canManage,
  onOpenChange,
  onInvalidate,
  onSetVisibility,
  onUndo,
  invalidating,
  undoing,
}: {
  digitalHumanId: string;
  memory: DigitalHumanMemoryRecord | null;
  canManage: boolean;
  onOpenChange: (open: boolean) => void;
  onInvalidate: (memory: DigitalHumanMemoryRecord) => Promise<void>;
  onSetVisibility: (
    memory: DigitalHumanMemoryRecord,
    visibility: DigitalHumanMemoryVisibility
  ) => Promise<void>;
  onUndo: (memory: DigitalHumanMemoryRecord) => Promise<void>;
  invalidating: boolean;
  undoing: boolean;
}) {
  const t = useT('digitalHumans');
  const objectHref = useDigitalHumanObjectHref();
  if (!memory) return null;
  const undoAvailable = Boolean(
    memory.undoable_until &&
      !memory.undo_consumed &&
      new Date(memory.undoable_until).getTime() > Date.now()
  );
  return (
    <Sheet open onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <div className="flex flex-wrap items-center gap-2 pr-8">
            <SheetTitle>{memory.memory_key}</SheetTitle>
            <MemoryStatusBadge status={memory.status} />
            <MemoryVisibilityBadge memory={memory} />
          </div>
          <SheetDescription>
            {t(memoryOriginKey(memory))} · {t(`memory.type.${memory.memory_type}`)}
            {memory.path ? <code className="ml-2 text-xs">{memory.path}</code> : null}
          </SheetDescription>
        </SheetHeader>
        <div className="mt-6 space-y-6">
          <section className="rounded-xl border bg-muted/20 p-4">
            <p className="whitespace-pre-wrap text-sm leading-6">{memory.content}</p>
          </section>
          <DetailSection icon={FileCheck2} title={t('memory.evidence')}>
            {memory.evidence?.length ? (
              <div className="space-y-2">
                {memory.evidence.map((evidence, index) => (
                  <div
                    key={`${evidence.type}:${evidence.id}:${index}`}
                    className="rounded-lg border px-3 py-2 text-xs"
                  >
                    <div className="flex items-center justify-between gap-3">
                      <Badge variant="outline">{evidence.type}</Badge>
                      {evidence.type === 'run' ? (
                        <Button variant="ghost" size="sm" asChild>
                          <Link
                            href={objectHref('run', evidence.id, digitalHumanId)}
                            scroll={false}
                          >
                            {t('memory.details')}
                            <ExternalLink className="size-3" />
                          </Link>
                        </Button>
                      ) : null}
                    </div>
                    <div className="mt-2 break-all font-mono text-muted-foreground">
                      {evidence.id}
                    </div>
                    {evidence.digest ? (
                      <div className="mt-1 truncate font-mono text-muted-foreground">
                        {evidence.digest}
                      </div>
                    ) : null}
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t('memory.noEvidence')}</p>
            )}
          </DetailSection>
          <DetailSection icon={ShieldCheck} title={t('memory.provenance')}>
            <dl className="grid gap-3 text-xs sm:grid-cols-2">
              <DetailValue label={t('memory.source')} value={memory.source_principal_id || '—'} />
              <DetailValue label={t('memory.operation')} value={memory.last_operation_id} />
              <DetailValue label={t('memory.configRevision')} value={memory.config_revision} />
              <DetailValue label={t('memory.memoryEpoch')} value={String(memory.memory_epoch)} />
            </dl>
          </DetailSection>
          <section className="rounded-xl border p-4">
            <div className="flex items-start gap-3">
              <Clock3 className="mt-0.5 size-4 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">
                  {undoAvailable && memory.undoable_until
                    ? t('memory.undoAvailable', { date: formatDate(memory.undoable_until) })
                    : t('memory.undoUnavailable')}
                </p>
                {canManage ? (
                  <div className="mt-4 flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={!undoAvailable || undoing || invalidating}
                      onClick={() => void onUndo(memory)}
                    >
                      {undoing ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <RotateCcw className="size-4" />
                      )}
                      {undoing ? t('memory.undoing') : t('memory.undo')}
                    </Button>
                    {memory.status === 'active' && memory.memory_type !== 'identity' ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={invalidating || undoing}
                        title={t('memory.visibility.hint')}
                        onClick={() =>
                          void onSetVisibility(
                            memory,
                            memory.visibility === 'everyone' ? 'internal' : 'everyone'
                          )
                        }
                      >
                        {memory.visibility === 'everyone' ? (
                          <EyeOff className="size-4" />
                        ) : (
                          <Eye className="size-4" />
                        )}
                        {memory.visibility === 'everyone'
                          ? t('memory.visibility.makeInternal')
                          : t('memory.visibility.makeEveryone')}
                      </Button>
                    ) : null}
                    {memory.status === 'active' ? (
                      <ConfirmDialog
                        variant="danger"
                        title={t('memory.invalidateTitle')}
                        description={t('memory.invalidateDescription')}
                        confirmText={t('memory.invalidate')}
                        cancelText={t('common.cancel')}
                        loading={invalidating}
                        confirmDisabled={invalidating || undoing}
                        onConfirm={() => void onInvalidate(memory)}
                        trigger={
                          <Button
                            variant="destructive"
                            size="sm"
                            disabled={invalidating || undoing}
                          >
                            {invalidating ? (
                              <Loader2 className="size-4 animate-spin" />
                            ) : (
                              <Trash2 className="size-4" />
                            )}
                            {invalidating ? t('memory.invalidating') : t('memory.invalidate')}
                          </Button>
                        }
                      />
                    ) : null}
                  </div>
                ) : null}
              </div>
            </div>
          </section>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function DetailSection({
  icon: Icon,
  title,
  children,
}: {
  icon: typeof ShieldCheck;
  title: string;
  children: ReactNode;
}) {
  return (
    <section>
      <div className="mb-3 flex items-center gap-2 text-sm font-semibold">
        <Icon className="size-4 text-primary" />
        {title}
      </div>
      {children}
    </section>
  );
}

function DetailValue({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all font-mono text-foreground">{value}</dd>
    </div>
  );
}

function MemoryStatusBadge({ status }: { status: DigitalHumanMemoryStatus }) {
  const t = useT('digitalHumans');
  return (
    <Badge
      variant={status === 'active' ? 'success' : status === 'invalidated' ? 'warning' : 'subtle'}
    >
      {t(`memory.status.${status}`)}
    </Badge>
  );
}

// Shows who may see one of the DigitalHuman's memories and where it came
// from (ADR 0015).
function MemoryVisibilityBadge({
  memory,
  compact = false,
}: {
  memory: DigitalHumanMemoryRecord;
  compact?: boolean;
}) {
  const t = useT('digitalHumans');
  const internal = memory.visibility === 'internal';
  return (
    <>
      {internal || !compact ? (
        <Badge variant={internal ? 'warning' : 'subtle'}>
          {t(`memory.visibility.${internal ? 'internal' : 'everyone'}`)}
        </Badge>
      ) : null}
      {memory.origin && memory.origin !== 'operator' ? (
        <Badge variant="outline">{t(`memory.origin.${memory.origin}`)}</Badge>
      ) : null}
    </>
  );
}

function memoryOriginKey(memory: DigitalHumanMemoryRecord) {
  return `memory.origin.${memory.origin ?? 'operator'}` as const;
}

function formatShortDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString();
}

function withCount(label: string, count?: number) {
  return count === undefined ? label : `${label} (${count})`;
}

function shortID(value: string) {
  return value.length <= 8 ? value : value.slice(0, 8);
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

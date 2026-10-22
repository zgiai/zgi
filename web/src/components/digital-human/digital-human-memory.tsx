'use client';

import { useMemo, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { useDigitalHumanObjectHref } from '@/components/digital-human/digital-human-object-link';
import {
  AlertCircle,
  ArrowRightLeft,
  BrainCircuit,
  Clock3,
  ExternalLink,
  FileCheck2,
  FilterX,
  Loader2,
  RefreshCw,
  RotateCcw,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
  useDigitalHumanMemories,
  useDigitalHumanMutations,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanMemory as DigitalHumanMemoryRecord,
  DigitalHumanMemoryClass,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryScopeType,
  DigitalHumanMemoryStatus,
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
const scopeTypes: DigitalHumanMemoryScopeType[] = [
  'workspace',
  'project',
  'customer',
  'campaign',
  'channel',
];

export function DigitalHumanMemory({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const canManage = permissions.hasPermission('digital_human.memory.manage');
  const [search, setSearch] = useState('');
  const [memoryClass, setMemoryClass] = useState<typeof all | DigitalHumanMemoryClass>(all);
  const [memoryType, setMemoryType] = useState<typeof all | DigitalHumanMemoryType>(all);
  // Invalidated memories no longer shape behavior; they are hidden unless
  // the reader asks for them.
  const [status, setStatus] = useState<typeof all | DigitalHumanMemoryStatus>('active');
  const [scopeType, setScopeType] = useState<typeof all | DigitalHumanMemoryScopeType>(all);
  const [scopeId, setScopeId] = useState('');
  const [source, setSource] = useState('');
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [repairOpen, setRepairOpen] = useState(false);
  const [selected, setSelected] = useState<DigitalHumanMemoryRecord | null>(null);
  const params = useMemo<DigitalHumanMemoryListParams>(
    () => ({
      page: 1,
      page_size: 100,
      search: search.trim() || undefined,
      memory_class: memoryClass === all ? undefined : memoryClass,
      memory_type: memoryType === all ? undefined : memoryType,
      status: status === all ? undefined : status,
      scope_type: scopeType === all ? undefined : scopeType,
      scope_id: scopeId.trim() || undefined,
      source: source.trim() || undefined,
    }),
    [memoryClass, memoryType, scopeId, scopeType, search, source, status]
  );
  const query = useDigitalHumanMemories(id, params);
  const mutations = useDigitalHumanMutations(id);
  const data = query.data?.data;
  const hasFilters = Boolean(
    search.trim() ||
      memoryClass !== all ||
      memoryType !== all ||
      status !== 'active' ||
      scopeType !== all ||
      scopeId.trim() ||
      source.trim()
  );

  const clearFilters = () => {
    setSearch('');
    setMemoryClass(all);
    setMemoryType(all);
    setStatus('active');
    setScopeType(all);
    setScopeId('');
    setSource('');
  };

  const toggleFuse = async () => {
    const state = data?.state;
    if (!state) return;
    await mutations.updateMemoryState.mutateAsync({
      digitalHumanId: id,
      expectedMemoryEpoch: state.memory_epoch,
      autoExtractionEnabled: !state.auto_extraction_enabled,
    });
    toast.success(t('memory.fuseUpdated'));
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
        memory_class: memory.memory_class,
        memory_type: memory.memory_type,
        scope:
          memory.memory_class === 'business' && memory.scope_type && memory.scope_id
            ? { type: memory.scope_type, id: memory.scope_id }
            : undefined,
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

  const rescope = async (memory: DigitalHumanMemoryRecord, target: 'self' | 'workspace') => {
    const state = data?.state;
    if (!state) return;
    try {
      await mutations.rescopeMemory.mutateAsync({
        digitalHumanId: id,
        memoryId: memory.id,
        request: {
          operation_id: crypto.randomUUID(),
          expected_revision: memory.revision,
          config_revision: state.config_revision,
          memory_epoch: state.memory_epoch,
          // The server always uses the current workspace as the scope.
          ...(target === 'self'
            ? { memory_class: 'self' as const }
            : { memory_class: 'business' as const, scope: { type: 'workspace' as const, id: '' } }),
        },
      });
    } catch {
      // The mutation already reports the error.
      return;
    }
    toast.success(t('memory.rescope.done'));
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
        <Button
          variant="outline"
          size="sm"
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} />
          {t('common.refresh')}
        </Button>
      </header>

      {data?.state ? (
        <section className="flex flex-col gap-3 rounded-xl border bg-muted/20 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3">
            <ShieldCheck className="mt-0.5 size-5 shrink-0 text-primary" />
            <div>
              <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                <span>{t('memory.count', { count: data.total })}</span>
                <Badge variant={data.state.auto_extraction_enabled ? 'success' : 'subtle'}>
                  {data.state.auto_extraction_enabled ? t('memory.fuseOn') : t('memory.fuseOff')}
                </Badge>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{t('memory.fuseHint')}</p>
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-3">
            {canManage ? (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={mutations.repairMemory.isPending}
                      onClick={() => setRepairOpen(true)}
                    >
                      {mutations.repairMemory.isPending ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : (
                        <Wrench className="size-4" />
                      )}
                      {mutations.repairMemory.isPending
                        ? t('memory.repairing')
                        : t('memory.repair')}
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-72 leading-5">
                    {t('memory.repairHint')}
                  </TooltipContent>
                </Tooltip>
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
            {canManage ? (
              <Button
                variant="outline"
                size="sm"
                disabled={mutations.updateMemoryState.isPending}
                onClick={() => void toggleFuse()}
              >
                {mutations.updateMemoryState.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <ShieldCheck className="size-4" />
                )}
                {mutations.updateMemoryState.isPending
                  ? t('memory.updatingFuse')
                  : data.state.auto_extraction_enabled
                    ? t('memory.disableFuse')
                    : t('memory.enableFuse')}
              </Button>
            ) : null}
          </div>
        </section>
      ) : null}

      <section className="overflow-hidden rounded-xl border bg-background">
        <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen} className="border-b p-4">
          <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-[minmax(260px,1fr)_repeat(3,minmax(160px,0.45fr))_auto]">
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('memory.searchPlaceholder')}
              leftIcon={<Search />}
            />
            <MemorySelect
              value={memoryClass}
              onValueChange={value => setMemoryClass(value as typeof memoryClass)}
              placeholder={t('memory.allClasses')}
            >
              <SelectItem value={all}>{t('memory.allClasses')}</SelectItem>
              <SelectItem value="self">{t('memory.class.self')}</SelectItem>
              <SelectItem value="business">{t('memory.class.business')}</SelectItem>
            </MemorySelect>
            <MemorySelect
              value={memoryType}
              onValueChange={value => setMemoryType(value as typeof memoryType)}
              placeholder={t('memory.allTypes')}
            >
              <SelectItem value={all}>{t('memory.allTypes')}</SelectItem>
              {memoryTypes.map(type => (
                <SelectItem key={type} value={type}>
                  {t(`memory.type.${type}`)}
                </SelectItem>
              ))}
            </MemorySelect>
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
            <CollapsibleTrigger asChild>
              <Button variant="outline" size="sm" className="justify-start xl:justify-center">
                <SlidersHorizontal className="size-4" />
                {advancedOpen ? t('memory.hideAdvancedFilters') : t('memory.showAdvancedFilters')}
              </Button>
            </CollapsibleTrigger>
          </div>
          <CollapsibleContent>
            <div className="mt-3 grid gap-2 rounded-lg bg-muted/30 p-3 md:grid-cols-2 xl:grid-cols-[minmax(180px,1fr)_minmax(180px,1fr)_minmax(180px,1fr)_auto]">
              <Input
                value={source}
                onChange={event => setSource(event.target.value)}
                placeholder={t('memory.sourcePlaceholder')}
              />
              <Input
                value={scopeId}
                onChange={event => setScopeId(event.target.value)}
                placeholder={t('memory.scopeIdPlaceholder')}
              />
              <MemorySelect
                value={scopeType}
                onValueChange={value => setScopeType(value as typeof scopeType)}
                placeholder={t('memory.allScopes')}
              >
                <SelectItem value={all}>{t('memory.allScopes')}</SelectItem>
                {scopeTypes.map(type => (
                  <SelectItem key={type} value={type}>
                    {t(`memory.scope.${type}`)}
                  </SelectItem>
                ))}
              </MemorySelect>
              <Button variant="ghost" size="sm" onClick={clearFilters} disabled={!hasFilters}>
                <FilterX className="size-4" />
                {t('memory.clearFilters')}
              </Button>
            </div>
          </CollapsibleContent>
        </Collapsible>

        <MemoryBody
          loading={query.isLoading}
          error={query.isError}
          memories={data?.items ?? []}
          hasFilters={hasFilters}
          onRetry={() => void query.refetch()}
          onSelect={setSelected}
        />
      </section>

      <MemoryDetail
        digitalHumanId={id}
        memory={selected}
        canManage={canManage}
        onOpenChange={open => !open && setSelected(null)}
        onInvalidate={invalidate}
        onRescope={rescope}
        onUndo={undo}
        invalidating={mutations.applyMemoryProposal.isPending || mutations.rescopeMemory.isPending}
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

function MemoryBody({
  loading,
  error,
  memories,
  hasFilters,
  onRetry,
  onSelect,
}: {
  loading: boolean;
  error: boolean;
  memories: DigitalHumanMemoryRecord[];
  hasFilters: boolean;
  onRetry: () => void;
  onSelect: (memory: DigitalHumanMemoryRecord) => void;
}) {
  const t = useT('digitalHumans');
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
          <Badge variant="outline" className="mt-3">
            {t('memory.clearFilters')}
          </Badge>
        ) : null}
      </div>
    );
  }
  return (
    <div className="divide-y">
      {memories.map(memory => (
        <button
          key={memory.id}
          type="button"
          onClick={() => onSelect(memory)}
          className="group grid w-full gap-3 px-4 py-4 text-left transition-colors hover:bg-muted/30 md:grid-cols-[minmax(0,1fr)_minmax(160px,auto)_minmax(150px,auto)] md:items-center"
        >
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={memory.memory_class === 'self' ? 'info' : 'secondary'}>
                {t(`memory.class.${memory.memory_class}`)}
              </Badge>
              <Badge variant="outline">{t(`memory.type.${memory.memory_type}`)}</Badge>
              <MemoryStatusBadge status={memory.status} />
            </div>
            <p
              className={cn(
                'line-clamp-2 text-sm font-medium leading-6',
                memory.status !== 'active' && 'text-muted-foreground line-through'
              )}
            >
              {memory.content}
            </p>
          </div>
          <div className="text-xs text-muted-foreground">
            <div className="font-medium text-foreground">{t(memoryScopeKey(memory))}</div>
            <div className="mt-1">
              {t(`memory.confidenceLevel.${memory.confidence >= 0.8 ? 'high' : memory.confidence >= 0.5 ? 'medium' : 'low'}`)}
            </div>
            <div className="mt-1">
              {t('memory.sourceSummary', {
                source: maskMemorySource(memory.source_principal_id),
              })}
            </div>
          </div>
          <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground md:justify-end">
            <div className="text-right">
              <div>{t('memory.updated', { date: formatDate(memory.updated_at) })}</div>
            </div>
            <ExternalLink className="size-4 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100" />
          </div>
        </button>
      ))}
    </div>
  );
}

function MemoryDetail({
  digitalHumanId,
  memory,
  canManage,
  onOpenChange,
  onInvalidate,
  onRescope,
  onUndo,
  invalidating,
  undoing,
}: {
  digitalHumanId: string;
  memory: DigitalHumanMemoryRecord | null;
  canManage: boolean;
  onOpenChange: (open: boolean) => void;
  onInvalidate: (memory: DigitalHumanMemoryRecord) => Promise<void>;
  onRescope: (memory: DigitalHumanMemoryRecord, target: 'self' | 'workspace') => Promise<void>;
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
          </div>
          <SheetDescription>
            {t(memoryScopeKey(memory))} · {t(`memory.type.${memory.memory_type}`)}
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
                    {memory.status === 'active'
                      ? memoryRescopeTargets(memory).map(target => (
                          <Button
                            key={target}
                            variant="outline"
                            size="sm"
                            disabled={invalidating || undoing}
                            title={t('memory.rescope.hint')}
                            onClick={() => void onRescope(memory, target)}
                          >
                            <ArrowRightLeft className="size-4" />
                            {target === 'self' ? t('memory.rescope.toSelf') : t('memory.rescope.toWorkspace')}
                          </Button>
                        ))
                      : null}
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

// Memories move between self memory and workspace business memory. Identity
// always stays self memory; other scopes are resolved by the runtime.
function memoryRescopeTargets(memory: DigitalHumanMemoryRecord): Array<'self' | 'workspace'> {
  if (memory.memory_class === 'self') {
    return memory.memory_type === 'identity' ? [] : ['workspace'];
  }
  return memory.scope_type === 'workspace' ? ['self'] : ['self', 'workspace'];
}

function memoryScopeKey(memory: DigitalHumanMemoryRecord) {
  return memory.memory_class === 'self'
    ? ('memory.scope.self' as const)
    : (`memory.scope.${memory.scope_type || 'workspace'}` as const);
}

function formatDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function maskMemorySource(value?: string | null) {
  const normalized = value?.trim();
  if (!normalized) return '—';
  if (normalized.length <= 8) return normalized;
  return `${normalized.slice(0, 3)}…${normalized.slice(-4)}`;
}

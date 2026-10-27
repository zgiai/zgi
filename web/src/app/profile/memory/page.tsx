'use client';

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Brain,
  ChevronLeft,
  ChevronRight,
  Download,
  MoreHorizontal,
  Plus,
  Trash2,
} from 'lucide-react';
import { toast } from 'sonner';
import { AccountPageHeader } from '@/components/account/account-shell';
import { MemoryArchivePanel } from '@/components/memory/memory-archive-panel';
import { MemoryBoard, type MemoryTopic } from '@/components/memory/memory-board';
import { MemoryHistoryPanel } from '@/components/memory/memory-history-panel';
import { InfoHint } from '@/components/memory/info-hint';
import { MemoryNotesPanel } from '@/components/memory/memory-notes-panel';
import { joinFacts, memoryTopicTitle, splitFacts } from '@/components/memory/memory-topics';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import { relationshipMemoryService } from '@/services/relationship-memory.service';
import type {
  UserMemoryConsumerType,
  UserMemoryRelationship,
  UserMemoryRelationshipDetail,
  UserMemoryRelationshipStatus,
  UserRelationshipMemoryItem,
} from '@/services/types/relationship-memory';
import { useAuthStore } from '@/store/auth-store';
import { formatDate } from '@/utils/format';
import { getErrorMessage } from '@/utils/error-notifications';

const memoryKeys = ['user-relationship-memories'] as const;
const allFilterValue = 'all';
// The assistant switcher lists one page; the API serves at most 100.
const relationshipPageSize = 100;
// Filters only help once there are more assistants than fit at a glance.
const filterThreshold = 6;
type DetailTab = 'memory' | 'notes' | 'archive' | 'history';

function initialsOf(name: string) {
  return Array.from(name.trim()).slice(0, 2).join('').toUpperCase() || '?';
}

function downloadJSON(filename: string, value: unknown) {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' })
  );
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function stillUndoable(until?: string) {
  return Boolean(until && new Date(until).getTime() > Date.now());
}

export default function ProfileMemoryPage() {
  const t = useT('profile');
  const tItems = useT('common.memoryItems');
  const tHistory = useT('common.memoryHistory');
  const tArchive = useT('common.memoryArchive');
  const tNotes = useT('common.memoryNotes');
  const queryClient = useQueryClient();
  const sessionStatus = useAuthStore.use.sessionStatus();
  const memoryQueriesEnabled = sessionStatus === 'authenticated';
  const [consumerType, setConsumerType] = useState<UserMemoryConsumerType | ''>('');
  const [status, setStatus] = useState<UserMemoryRelationshipStatus | ''>('');
  const [page, setPage] = useState(1);
  const [selectedId, setSelectedId] = useState('');
  const [tab, setTab] = useState<DetailTab>('memory');
  const [confirm, setConfirm] = useState<'clear' | 'delete' | null>(null);
  const [addOpen, setAddOpen] = useState(false);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const requestedType = params.get('consumer_type');
    if (requestedType === 'agent' || requestedType === 'digital_human') {
      setConsumerType(requestedType);
    }
  }, []);

  const listQuery = useQuery({
    queryKey: [...memoryKeys, 'list', consumerType, status, page],
    queryFn: () =>
      relationshipMemoryService.list({
        consumer_type: consumerType || undefined,
        status: status || undefined,
        page,
        page_size: relationshipPageSize,
      }),
    enabled: memoryQueriesEnabled,
  });
  const detailQuery = useQuery({
    queryKey: [...memoryKeys, 'detail', selectedId],
    queryFn: () => relationshipMemoryService.get(selectedId),
    enabled: memoryQueriesEnabled && Boolean(selectedId),
  });
  const relationships = useMemo(() => listQuery.data?.data.items ?? [], [listQuery.data]);
  const total = listQuery.data?.data.total ?? 0;
  const detail = detailQuery.data?.data;
  // The DigitalHuman's notes on the person are shown in their own tab.
  const ownItems = useMemo(
    () => detail?.items.filter(item => !item.key.startsWith('notes/')) ?? [],
    [detail]
  );
  const showFilters = total > filterThreshold || Boolean(consumerType || status);
  const autoCaptureAvailable = detail?.auto_capture_available !== false;

  useEffect(() => {
    if (selectedId || relationships.length === 0) return;
    const requestedConsumerID = new URLSearchParams(window.location.search).get('consumer_id');
    if (!requestedConsumerID) {
      // Open the first assistant instead of an empty panel.
      setSelectedId(relationships[0].id);
      return;
    }
    const requested = relationships.find(item => item.consumer_id === requestedConsumerID);
    if (requested) {
      setSelectedId(requested.id);
    } else if (page * relationshipPageSize < total) {
      // A shortcut from an Agent or DigitalHuman must locate the relationship
      // even when it is not on the first organization-wide page.
      setPage(value => value + 1);
    }
  }, [total, page, relationships, selectedId]);
  const grouped = useMemo(() => {
    const groups = new Map<string, { id: string; name: string; items: UserMemoryRelationship[] }>();
    relationships.forEach(item => {
      const group = groups.get(item.workspace_id);
      if (group) {
        group.items.push(item);
      } else {
        groups.set(item.workspace_id, {
          id: item.workspace_id,
          name: item.workspace_name || item.workspace_id,
          items: [item],
        });
      }
    });
    return [...groups.values()];
  }, [relationships]);

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: memoryKeys }),
      selectedId
        ? queryClient.invalidateQueries({ queryKey: [...memoryKeys, 'detail', selectedId] })
        : Promise.resolve(),
    ]);
  };
  const settings = useMutation({
    mutationFn: async (task: () => Promise<unknown>) => task(),
    onSuccess: async () => {
      await refresh();
      toast.success(t('memory.saved'));
    },
    onError: error => {
      toast.error(getErrorMessage(error) || t('memory.failed'));
      void refresh();
    },
  });
  // Board actions report their own success; failures are reported here and
  // rethrown so the board keeps the edit open.
  const act = async <T,>(task: () => Promise<T>): Promise<T> => {
    try {
      return await task();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memory.failed'));
      throw error;
    } finally {
      await refresh();
    }
  };

  const selectRelationship = (relationship: UserMemoryRelationship) => {
    setSelectedId(relationship.id);
    setTab('memory');
  };

  const statusLine = (item: UserMemoryRelationship) =>
    item.status === 'archived'
      ? t('memory.archivedConsumer')
      : !item.memory_enabled
        ? t('memory.memoryOff')
        : item.item_count === 1
          ? t('memory.itemCountOne')
          : t('memory.itemCount', { count: item.item_count });

  const switcher =
    relationships.length > 0 ? (
      <div className="flex items-center gap-1">
        {total > relationshipPageSize ? (
          <Button
            variant="ghost"
            size="sm"
            isIcon
            aria-label={t('memory.previous')}
            disabled={page <= 1}
            onClick={() => {
              setPage(value => value - 1);
              setSelectedId('');
            }}
          >
            <ChevronLeft className="size-4" />
          </Button>
        ) : null}
        <Select
          value={selectedId}
          onValueChange={id => {
            const relationship = relationships.find(item => item.id === id);
            if (relationship) selectRelationship(relationship);
          }}
        >
          <SelectTrigger
            className="h-10 w-[min(300px,80vw)]"
            aria-label={t('memory.switchAssistant')}
          >
            <SelectValue placeholder={t('memory.switchAssistant')} />
          </SelectTrigger>
          <SelectContent align="end" className="max-h-96">
            {grouped.map(group => (
              <SelectGroup key={group.id}>
                {grouped.length > 1 ? <SelectLabel>{group.name}</SelectLabel> : null}
                {group.items.map(item => (
                  <SelectItem key={item.id} value={item.id}>
                    <span className="flex min-w-0 items-center gap-2">
                      <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-[10px] font-medium text-primary">
                        {initialsOf(item.consumer_name_snapshot || item.consumer_id)}
                      </span>
                      <span className="truncate">
                        {item.consumer_name_snapshot || item.consumer_id}
                      </span>
                      <span className="shrink-0 text-xs text-muted-foreground">
                        {statusLine(item)}
                      </span>
                    </span>
                  </SelectItem>
                ))}
              </SelectGroup>
            ))}
          </SelectContent>
        </Select>
        {total > relationshipPageSize ? (
          <Button
            variant="ghost"
            size="sm"
            isIcon
            aria-label={t('memory.next')}
            disabled={page * relationshipPageSize >= total}
            onClick={() => {
              setPage(value => value + 1);
              setSelectedId('');
            }}
          >
            <ChevronRight className="size-4" />
          </Button>
        ) : null}
      </div>
    ) : null;

  return (
    <div>
      <AccountPageHeader
        title={t('memory.title')}
        description={t('memory.description')}
        actions={switcher}
      />

      {showFilters ? (
        <div className="mb-4 flex flex-wrap gap-3">
          <Select
            value={consumerType || allFilterValue}
            onValueChange={value => {
              setConsumerType(value === allFilterValue ? '' : (value as UserMemoryConsumerType));
              setPage(1);
            }}
          >
            <SelectTrigger className="w-full sm:w-[200px]" aria-label={t('memory.consumerFilter')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={allFilterValue}>{t('memory.allConsumers')}</SelectItem>
              <SelectItem value="agent">{t('memory.agent')}</SelectItem>
              <SelectItem value="digital_human">{t('memory.digitalHuman')}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={status || allFilterValue}
            onValueChange={value => {
              setStatus(value === allFilterValue ? '' : (value as UserMemoryRelationshipStatus));
              setPage(1);
            }}
          >
            <SelectTrigger className="w-full sm:w-[200px]" aria-label={t('memory.statusFilter')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={allFilterValue}>{t('memory.allStatuses')}</SelectItem>
              <SelectItem value="active">{t('memory.active')}</SelectItem>
              <SelectItem value="archived">{t('memory.archived')}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      ) : null}

      {listQuery.isLoading ? <Skeleton className="h-[420px] rounded-xl" /> : null}
      {listQuery.isError ? (
        <section className="rounded-xl border bg-background">
          <EmptyState
            title={t('memory.loadFailed')}
            action={
              <Button size="sm" onClick={() => listQuery.refetch()}>
                {t('memory.retry')}
              </Button>
            }
          />
        </section>
      ) : null}
      {listQuery.isSuccess && relationships.length === 0 ? (
        <section className="rounded-xl border bg-background">
          <EmptyState title={t('memory.empty')} description={t('memory.emptyDescription')} />
        </section>
      ) : null}

      {selectedId ? (
        <section className="min-h-[420px] rounded-xl border bg-background">
          {detailQuery.isLoading ? (
            <div className="space-y-4 p-6">
              <Skeleton className="h-8 w-56" />
              <Skeleton className="h-24" />
              <Skeleton className="h-32" />
            </div>
          ) : null}
          {detailQuery.isError ? (
            <EmptyState
              title={t('memory.loadFailed')}
              action={
                <Button size="sm" onClick={() => detailQuery.refetch()}>
                  {t('memory.retry')}
                </Button>
              }
            />
          ) : null}
          {detail ? (
            <div>
              <div className="border-b px-5 py-3.5">
                <div className="flex flex-wrap items-center justify-between gap-3">
                  <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
                    <SettingToggle
                      id="memory-enabled"
                      label={t('memory.memoryEnabled')}
                      hint={t('memory.memoryEnabledHint')}
                    >
                      <Switch
                        aria-labelledby="memory-enabled"
                        checked={detail.memory_enabled}
                        disabled={!detail.can_manage || settings.isPending}
                        onCheckedChange={checked =>
                          settings.mutate(() =>
                            relationshipMemoryService.updateSettings(detail.id, {
                              memory_enabled: checked,
                              expected_revision: detail.settings_revision,
                            })
                          )
                        }
                      />
                    </SettingToggle>
                    {/* Not offered when the assistant or the deployment has no automatic memory. */}
                    {autoCaptureAvailable ? (
                      <SettingToggle
                        id="memory-auto-capture"
                        label={t('memory.autoCapture')}
                        hint={t('memory.autoCaptureHint')}
                      >
                        <Switch
                          aria-labelledby="memory-auto-capture"
                          checked={detail.auto_capture_enabled}
                          disabled={
                            !detail.can_manage || !detail.memory_enabled || settings.isPending
                          }
                          onCheckedChange={checked =>
                            settings.mutate(() =>
                              relationshipMemoryService.updateSettings(detail.id, {
                                auto_capture_enabled: checked,
                                expected_revision: detail.settings_revision,
                              })
                            )
                          }
                        />
                      </SettingToggle>
                    ) : null}
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="outline" size="sm" aria-label={t('memory.moreActions')}>
                        <MoreHorizontal className="mr-1 size-4" />
                        {t('memory.more')}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onSelect={async () => {
                          const value = await relationshipMemoryService.export(detail.id);
                          downloadJSON(`user-memory-${detail.id}.json`, value.data);
                        }}
                      >
                        <Download />
                        {t('memory.export')}
                      </DropdownMenuItem>
                      {detail.can_manage && ownItems.length > 0 ? (
                        <DropdownMenuItem onSelect={() => setConfirm('clear')}>
                          <Trash2 />
                          {t('memory.clearAll')}
                        </DropdownMenuItem>
                      ) : null}
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onSelect={() => setConfirm('delete')}>
                        <Trash2 />
                        {t('memory.permanentDelete')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                {!detail.can_manage ? (
                  <p className="mt-3 text-xs text-amber-700 dark:text-amber-400">
                    {t('memory.readOnly')}
                  </p>
                ) : null}
              </div>

              <Tabs value={tab} onValueChange={value => setTab(value as DetailTab)} className="p-5">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <TabsList>
                    <TabsTrigger value="memory">{t('memory.tabs.memory')}</TabsTrigger>
                    {detail.consumer_type === 'digital_human' ? (
                      <TabsTrigger value="notes">{tNotes('title')}</TabsTrigger>
                    ) : null}
                    <TabsTrigger value="history">{tHistory('title')}</TabsTrigger>
                    <TabsTrigger value="archive">{tArchive('title')}</TabsTrigger>
                  </TabsList>
                  {tab === 'memory' && detail.can_manage && ownItems.length > 0 ? (
                    <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
                      <Plus className="mr-1 size-4" />
                      {tItems('add')}
                    </Button>
                  ) : null}
                </div>
                <TabsContent value="memory" className="mt-4">
                  <RelationshipMemoryBoard
                    key={detail.id}
                    detail={detail}
                    items={ownItems}
                    act={act}
                    tItems={tItems}
                    empty={t('memory.noItems')}
                    addOpen={addOpen}
                    onAddOpenChange={setAddOpen}
                  />
                </TabsContent>
                {detail.consumer_type === 'digital_human' ? (
                  <TabsContent value="notes" className="mt-4">
                    {tab === 'notes' ? (
                      <MemoryNotesPanel
                        key={detail.id}
                        load={async () =>
                          (await relationshipMemoryService.notes(detail.id)).data.notes
                        }
                        remove={
                          detail.can_manage
                            ? note =>
                                relationshipMemoryService.deleteNote(
                                  detail.id,
                                  note.key,
                                  note.revision
                                )
                            : undefined
                        }
                        refreshKey={detailQuery.dataUpdatedAt}
                      />
                    ) : null}
                  </TabsContent>
                ) : null}
                <TabsContent value="archive" className="mt-4">
                  {tab === 'archive' ? (
                    <MemoryArchivePanel
                      key={detail.id}
                      load={async () =>
                        (await relationshipMemoryService.archive(detail.id)).data.entries
                      }
                      remove={entryId =>
                        relationshipMemoryService.deleteArchiveEntry(detail.id, entryId)
                      }
                      update={(entryId, content) =>
                        relationshipMemoryService.updateArchiveEntry(detail.id, entryId, content)
                      }
                      canManage={detail.can_manage}
                      refreshKey={detailQuery.dataUpdatedAt}
                    />
                  ) : null}
                </TabsContent>
                <TabsContent value="history" className="mt-4">
                  {tab === 'history' ? (
                    <MemoryHistoryPanel
                      key={detail.id}
                      load={async before =>
                        (await relationshipMemoryService.history(detail.id, { before })).data
                      }
                      revert={versionId => relationshipMemoryService.revert(detail.id, versionId)}
                      labelFor={key => {
                        const item = detail.items.find(candidate => candidate.key === key);
                        return memoryTopicTitle(key, item?.display_name, topic =>
                          tItems(`topics.${topic}`)
                        );
                      }}
                      canRevert={detail.can_manage}
                      refreshKey={detailQuery.dataUpdatedAt}
                      onReverted={refresh}
                    />
                  ) : null}
                </TabsContent>
              </Tabs>

              <ConfirmDialog
                open={confirm === 'clear'}
                onOpenChange={next => !next && setConfirm(null)}
                title={t('memory.clearConfirmTitle')}
                description={t('memory.clearConfirm')}
                confirmText={t('memory.clearAll')}
                cancelText={t('memory.cancel')}
                variant="danger"
                loading={settings.isPending}
                onConfirm={() =>
                  settings.mutate(async () => {
                    await relationshipMemoryService.clear(detail.id);
                    setConfirm(null);
                  })
                }
              />
              <ConfirmDialog
                open={confirm === 'delete'}
                onOpenChange={next => !next && setConfirm(null)}
                title={t('memory.permanentDeleteTitle')}
                description={t('memory.deleteRelationshipConfirm')}
                confirmText={t('memory.permanentDelete')}
                cancelText={t('memory.cancel')}
                variant="danger"
                loading={settings.isPending}
                onConfirm={() =>
                  settings.mutate(async () => {
                    await relationshipMemoryService.permanentlyDelete(detail.id);
                    setConfirm(null);
                    setSelectedId('');
                  })
                }
              />
            </div>
          ) : null}
        </section>
      ) : null}
    </div>
  );
}

// The memory tab: the person's memory files with an Agent or a DigitalHuman
// (ADR 0019), shown as topics of facts.
function RelationshipMemoryBoard({
  detail,
  items,
  act,
  tItems,
  empty,
  addOpen,
  onAddOpenChange,
}: {
  detail: UserMemoryRelationshipDetail;
  items: UserRelationshipMemoryItem[];
  act: <T>(task: () => Promise<T>) => Promise<T>;
  tItems: ReturnType<typeof useT<'common.memoryItems'>>;
  empty: ReactNode;
  addOpen: boolean;
  onAddOpenChange: (open: boolean) => void;
}) {
  const byKey = new Map(items.map(item => [item.key, item]));
  const titleOf = (item: UserRelationshipMemoryItem) =>
    memoryTopicTitle(item.key, item.display_name, topic => tItems(`topics.${topic}`));

  const topics: MemoryTopic[] = items.map(item => ({
    key: item.key,
    title: titleOf(item),
    content: item.content,
    meta: tItems('meta', {
      time: formatDate(item.updated_at),
      source: tItems(`sources.${item.source_kind}`),
    }),
    undoAutomatic:
      item.source_kind === 'automatic' &&
      item.last_operation_id &&
      stillUndoable(item.undoable_until)
        ? () =>
            act(() => relationshipMemoryService.undo(detail.id, item.last_operation_id as string))
        : undefined,
  }));

  const save = (key: string, content: string) => {
    const item = byKey.get(key);
    return act(() =>
      relationshipMemoryService.putItem(detail.id, key, {
        display_name: item?.display_name || key,
        category: item?.category || undefined,
        content,
        expected_revision: item?.revision ?? 0,
      })
    );
  };

  const addTargets = [
    ...items.map(item => ({ value: item.key, label: titleOf(item) })),
    ...(['profile', 'preferences', 'work', 'notes'] as const)
      .filter(topic => !byKey.has(topic))
      .map(topic => ({ value: topic, label: tItems(`topics.${topic}`) })),
  ];

  return (
    <MemoryBoard
      topics={topics}
      canEdit={detail.can_manage}
      empty={empty}
      layout="auto"
      addOpen={addOpen}
      onAddOpenChange={onAddOpenChange}
      onSave={save}
      onDeleteTopic={async key => {
        const item = byKey.get(key);
        if (!item) return undefined;
        const response = await act(() =>
          relationshipMemoryService.deleteItem(detail.id, key, item.revision)
        );
        const result = response.data;
        return result.deleted && stillUndoable(result.undoable_until)
          ? {
              undo: () => act(() => relationshipMemoryService.undo(detail.id, result.operation_id)),
            }
          : undefined;
      }}
      addTargets={addTargets}
      onAdd={(target, fact) =>
        save(target, joinFacts([...splitFacts(byKey.get(target)?.content ?? ''), fact]))
      }
    />
  );
}

function EmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-40 flex-col items-center justify-center p-8 text-center">
      <Brain className="size-8 text-muted-foreground/50" />
      <p className="mt-3 text-sm font-medium">{title}</p>
      {description ? (
        <p className="mt-1 max-w-md text-xs leading-5 text-muted-foreground">{description}</p>
      ) : null}
      {action ? <div className="mt-3">{action}</div> : null}
    </div>
  );
}

function SettingToggle({
  id,
  label,
  hint,
  note,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  /** A short reason shown beside a switch that cannot be used. */
  note?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center gap-2">
      <span id={id} className="text-sm">
        {label}
      </span>
      <InfoHint text={hint} />
      {children}
      {note ? <span className="text-xs text-muted-foreground">{note}</span> : null}
    </div>
  );
}

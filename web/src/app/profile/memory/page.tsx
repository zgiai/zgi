'use client';

import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ArrowLeft,
  Bot,
  Brain,
  Download,
  Plus,
  RotateCcw,
  Save,
  Trash2,
  UserRound,
} from 'lucide-react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { relationshipMemoryService } from '@/services/relationship-memory.service';
import type {
  UserMemoryCategory,
  UserMemoryConsumerType,
  UserMemoryRelationship,
  UserMemoryRelationshipStatus,
} from '@/services/types/relationship-memory';
import { useAuthStore } from '@/store/auth-store';
import { getErrorMessage } from '@/utils/error-notifications';

const memoryKeys = ['user-relationship-memories'] as const;
const categories: UserMemoryCategory[] = ['profile', 'preference', 'constraint', 'relationship'];
const allFilterValue = 'all';

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

export default function ProfileMemoryPage() {
  const t = useT('profile');
  const router = useRouter();
  const queryClient = useQueryClient();
  const sessionStatus = useAuthStore.use.sessionStatus();
  const memoryQueriesEnabled = sessionStatus === 'authenticated';
  const [consumerType, setConsumerType] = useState<UserMemoryConsumerType | ''>('');
  const [status, setStatus] = useState<UserMemoryRelationshipStatus | ''>('');
  const [page, setPage] = useState(1);
  const [selectedId, setSelectedId] = useState('');
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [newItem, setNewItem] = useState<{
    key: string;
    category: UserMemoryCategory;
    content: string;
  }>({ key: '', category: 'preference', content: '' });
  const [deletedUndo, setDeletedUndo] = useState<{
    relationshipId: string;
    operationId: string;
    key: string;
    undoableUntil: string;
  } | null>(null);

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
        page_size: 20,
      }),
    enabled: memoryQueriesEnabled,
  });
  const detailQuery = useQuery({
    queryKey: [...memoryKeys, 'detail', selectedId],
    queryFn: () => relationshipMemoryService.get(selectedId),
    enabled: memoryQueriesEnabled && Boolean(selectedId),
  });
  const relationships = useMemo(() => listQuery.data?.data.items ?? [], [listQuery.data]);
  const detail = detailQuery.data?.data;

  useEffect(() => {
    if (selectedId || relationships.length === 0) return;
    const requestedConsumerID = new URLSearchParams(window.location.search).get('consumer_id');
    if (!requestedConsumerID) return;
    const requested = relationships.find(item => item.consumer_id === requestedConsumerID);
    if (requested) {
      setSelectedId(requested.id);
    } else if (page * 20 < (listQuery.data?.data.total ?? 0)) {
      // A shortcut from an Agent or DigitalHuman must locate the relationship
      // even when it is not on the first organization-wide page.
      setPage(value => value + 1);
    }
  }, [listQuery.data?.data.total, page, relationships, selectedId]);
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
  const operation = useMutation({
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

  const selectRelationship = (relationship: UserMemoryRelationship) => {
    setSelectedId(relationship.id);
    setDrafts({});
  };

  return (
    <ProtectedRoute>
      <div className="min-h-screen bg-muted/30">
        <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
          <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
            <div>
              <Button
                variant="ghost"
                size="sm"
                className="mb-3 -ml-2"
                onClick={() => router.back()}
              >
                <ArrowLeft className="mr-1 size-4" />
                {t('backButton')}
              </Button>
              <div className="flex items-center gap-3">
                <div className="flex size-11 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <Brain className="size-5" />
                </div>
                <div>
                  <h1 className="text-2xl font-semibold">{t('memory.title')}</h1>
                  <p className="mt-1 text-sm text-muted-foreground">{t('memory.description')}</p>
                </div>
              </div>
            </div>
            <div className="rounded-lg border bg-background px-4 py-3 text-sm text-muted-foreground">
              {t('memory.autoCaptureDefault')}
            </div>
          </div>

          <div className="mb-5 flex flex-wrap gap-3 rounded-xl border bg-background p-4">
            <Select
              value={consumerType || allFilterValue}
              onValueChange={value => {
                setConsumerType(value === allFilterValue ? '' : (value as UserMemoryConsumerType));
                setPage(1);
              }}
            >
              <SelectTrigger
                className="w-full sm:w-[200px]"
                aria-label={t('memory.consumerFilter')}
              >
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

          <div className="grid gap-5 lg:grid-cols-[minmax(280px,0.85fr)_minmax(0,1.7fr)]">
            <section className="space-y-4">
              {listQuery.isLoading
                ? [0, 1, 2].map(item => <Skeleton key={item} className="h-28 rounded-xl" />)
                : null}
              {listQuery.isError ? (
                <EmptyState
                  title={t('memory.loadFailed')}
                  action={
                    <Button size="sm" onClick={() => listQuery.refetch()}>
                      {t('memory.retry')}
                    </Button>
                  }
                />
              ) : null}
              {!listQuery.isLoading && !listQuery.isError && grouped.length === 0 ? (
                <EmptyState title={t('memory.empty')} description={t('memory.emptyDescription')} />
              ) : null}
              {grouped.map(group => (
                <div key={group.id}>
                  <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                    {group.name}
                  </div>
                  <div className="space-y-2">
                    {group.items.map(item => (
                      <button
                        key={item.id}
                        type="button"
                        onClick={() => selectRelationship(item)}
                        className={`w-full rounded-xl border bg-background p-4 text-left transition-colors hover:border-primary/40 ${selectedId === item.id ? 'border-primary ring-1 ring-primary/20' : ''}`}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className="flex min-w-0 items-center gap-3">
                            <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted">
                              {item.consumer_type === 'agent' ? (
                                <Bot className="size-4" />
                              ) : (
                                <UserRound className="size-4" />
                              )}
                            </div>
                            <div className="min-w-0">
                              <div className="truncate font-medium">
                                {item.consumer_name_snapshot || item.consumer_id}
                              </div>
                              <div className="mt-1 text-xs text-muted-foreground">
                                {item.item_count} {t('memory.items')}
                              </div>
                            </div>
                          </div>
                          <Badge variant={item.status === 'active' ? 'secondary' : 'outline'}>
                            {t(`memory.${item.status}`)}
                          </Badge>
                        </div>
                      </button>
                    ))}
                  </div>
                </div>
              ))}
              {(listQuery.data?.data.total ?? 0) > 20 ? (
                <div className="flex items-center justify-between">
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page <= 1}
                    onClick={() => setPage(value => value - 1)}
                  >
                    {t('memory.previous')}
                  </Button>
                  <span className="text-xs text-muted-foreground">{page}</span>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={page * 20 >= (listQuery.data?.data.total ?? 0)}
                    onClick={() => setPage(value => value + 1)}
                  >
                    {t('memory.next')}
                  </Button>
                </div>
              ) : null}
            </section>

            <section className="min-h-[420px] rounded-xl border bg-background">
              {!selectedId ? (
                <EmptyState
                  title={t('memory.selectRelationship')}
                  description={t('memory.selectRelationshipDescription')}
                />
              ) : null}
              {selectedId && detailQuery.isLoading ? (
                <div className="space-y-4 p-6">
                  <Skeleton className="h-8 w-56" />
                  <Skeleton className="h-24" />
                  <Skeleton className="h-32" />
                </div>
              ) : null}
              {selectedId && detailQuery.isError ? (
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
                  <div className="border-b p-5">
                    <div className="flex flex-wrap items-start justify-between gap-4">
                      <div>
                        <h2 className="text-lg font-semibold">
                          {detail.consumer_name_snapshot || detail.consumer_id}
                        </h2>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {detail.workspace_name} ·{' '}
                          {detail.consumer_type === 'agent'
                            ? t('memory.agent')
                            : t('memory.digitalHuman')}
                        </p>
                      </div>
                      <div className="flex gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={async () => {
                            const value = await relationshipMemoryService.export(detail.id);
                            downloadJSON(`user-memory-${detail.id}.json`, value.data);
                          }}
                        >
                          <Download className="mr-1 size-4" />
                          {t('memory.export')}
                        </Button>
                        <Button
                          variant="outline"
                          size="sm"
                          className="text-destructive"
                          onClick={() => {
                            if (!window.confirm(t('memory.deleteRelationshipConfirm'))) return;
                            operation.mutate(() =>
                              relationshipMemoryService.permanentlyDelete(detail.id)
                            );
                            setSelectedId('');
                          }}
                        >
                          <Trash2 className="mr-1 size-4" />
                          {t('memory.permanentDelete')}
                        </Button>
                      </div>
                    </div>
                    <div className="mt-5 grid gap-3 sm:grid-cols-2">
                      <SettingRow
                        label={t('memory.memoryEnabled')}
                        description={t('memory.memoryEnabledHint')}
                      >
                        <Switch
                          checked={detail.memory_enabled}
                          disabled={!detail.can_manage || operation.isPending}
                          onCheckedChange={checked =>
                            operation.mutate(() =>
                              relationshipMemoryService.updateSettings(detail.id, {
                                memory_enabled: checked,
                                expected_revision: detail.settings_revision,
                              })
                            )
                          }
                        />
                      </SettingRow>
                      <SettingRow
                        label={t('memory.autoCapture')}
                        description={t('memory.autoCaptureHint')}
                      >
                        <Switch
                          checked={detail.auto_capture_enabled}
                          disabled={
                            !detail.can_manage || !detail.memory_enabled || operation.isPending
                          }
                          onCheckedChange={checked =>
                            operation.mutate(() =>
                              relationshipMemoryService.updateSettings(detail.id, {
                                auto_capture_enabled: checked,
                                expected_revision: detail.settings_revision,
                              })
                            )
                          }
                        />
                      </SettingRow>
                    </div>
                    {!detail.can_manage ? (
                      <p className="mt-3 text-xs text-amber-700 dark:text-amber-400">
                        {t('memory.readOnly')}
                      </p>
                    ) : null}
                  </div>

                  <div className="space-y-4 p-5">
                    {deletedUndo?.relationshipId === detail.id &&
                    new Date(deletedUndo.undoableUntil).getTime() > Date.now() ? (
                      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-950 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-100">
                        <span>{t('memory.deletedUndoAvailable', { key: deletedUndo.key })}</span>
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() =>
                            operation.mutate(async () => {
                              const response = await relationshipMemoryService.undo(
                                detail.id,
                                deletedUndo.operationId
                              );
                              setDeletedUndo(null);
                              return response;
                            })
                          }
                        >
                          <RotateCcw className="mr-1 size-4" />
                          {t('memory.undo')}
                        </Button>
                      </div>
                    ) : null}
                    {detail.items.length === 0 ? (
                      <EmptyState
                        title={t('memory.noItems')}
                        description={t('memory.noItemsDescription')}
                      />
                    ) : null}
                    {detail.items.map(item => {
                      const draft = drafts[item.id] ?? item.content;
                      const changed = draft.trim() !== item.content;
                      const lastOperationId = item.last_operation_id;
                      const undoable = Boolean(
                        lastOperationId &&
                          item.undoable_until &&
                          new Date(item.undoable_until).getTime() > Date.now()
                      );
                      return (
                        <div key={item.id} className="rounded-xl border p-4">
                          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                            <div className="flex items-center gap-2">
                              <span className="font-medium">{item.display_name || item.key}</span>
                              <Badge variant="outline">{item.key}</Badge>
                              {item.category ? (
                                <Badge variant="secondary">
                                  {t(`memory.category.${item.category}`)}
                                </Badge>
                              ) : null}
                            </div>
                            <span className="text-xs text-muted-foreground">
                              r{item.revision} · {t(`memory.source.${item.source_kind}`)} ·{' '}
                              {new Date(item.updated_at).toLocaleString()}
                            </span>
                          </div>
                          <Textarea
                            value={draft}
                            disabled={!detail.can_manage || operation.isPending}
                            maxLength={2000}
                            onChange={event =>
                              setDrafts(value => ({ ...value, [item.id]: event.target.value }))
                            }
                            className="min-h-24"
                          />
                          {detail.can_manage ? (
                            <div className="mt-3 flex justify-end gap-2">
                              {undoable && lastOperationId ? (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() =>
                                    operation.mutate(() =>
                                      relationshipMemoryService.undo(detail.id, lastOperationId)
                                    )
                                  }
                                >
                                  <RotateCcw className="mr-1 size-4" />
                                  {t('memory.undo')}
                                </Button>
                              ) : null}
                              <Button
                                variant="outline"
                                size="sm"
                                className="text-destructive"
                                onClick={() => {
                                  if (window.confirm(t('memory.deleteItemConfirm'))) {
                                    operation.mutate(async () => {
                                      const response = await relationshipMemoryService.deleteItem(
                                        detail.id,
                                        item.key,
                                        item.revision
                                      );
                                      if (response.data.deleted && response.data.undoable_until) {
                                        setDeletedUndo({
                                          relationshipId: detail.id,
                                          operationId: response.data.operation_id,
                                          key: item.display_name || item.key,
                                          undoableUntil: response.data.undoable_until,
                                        });
                                      }
                                      return response;
                                    });
                                  }
                                }}
                              >
                                <Trash2 className="mr-1 size-4" />
                                {t('memory.delete')}
                              </Button>
                              <Button
                                size="sm"
                                disabled={!changed || !draft.trim()}
                                onClick={() =>
                                  operation.mutate(() =>
                                    relationshipMemoryService.putItem(detail.id, item.key, {
                                      display_name: item.display_name,
                                      category: item.category,
                                      content: draft.trim(),
                                      expected_revision: item.revision,
                                    })
                                  )
                                }
                              >
                                <Save className="mr-1 size-4" />
                                {t('memory.save')}
                              </Button>
                            </div>
                          ) : null}
                        </div>
                      );
                    })}

                    {detail.can_manage && detail.consumer_type === 'digital_human' ? (
                      <div className="rounded-xl border border-dashed p-4">
                        <h3 className="mb-3 text-sm font-medium">{t('memory.addItem')}</h3>
                        <div className="grid gap-3 sm:grid-cols-2">
                          <Input
                            value={newItem.key}
                            maxLength={64}
                            placeholder={t('memory.keyPlaceholder')}
                            onChange={event =>
                              setNewItem(value => ({ ...value, key: event.target.value }))
                            }
                          />
                          <Select
                            value={newItem.category}
                            onValueChange={category =>
                              setNewItem(value => ({
                                ...value,
                                category: category as UserMemoryCategory,
                              }))
                            }
                          >
                            <SelectTrigger
                              className="w-full"
                              aria-label={t('memory.categoryLabel')}
                            >
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {categories.map(category => (
                                <SelectItem key={category} value={category}>
                                  {t(`memory.category.${category}`)}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <Textarea
                          className="mt-3"
                          value={newItem.content}
                          maxLength={2000}
                          placeholder={t('memory.contentPlaceholder')}
                          onChange={event =>
                            setNewItem(value => ({ ...value, content: event.target.value }))
                          }
                        />
                        <Button
                          className="mt-3"
                          size="sm"
                          disabled={!newItem.key.trim() || !newItem.content.trim()}
                          onClick={() =>
                            operation.mutate(async () => {
                              await relationshipMemoryService.putItem(
                                detail.id,
                                newItem.key.trim(),
                                {
                                  display_name: newItem.key.trim(),
                                  category: newItem.category,
                                  content: newItem.content.trim(),
                                  expected_revision: 0,
                                }
                              );
                              setNewItem({ key: '', category: 'preference', content: '' });
                            })
                          }
                        >
                          <Plus className="mr-1 size-4" />
                          {t('memory.add')}
                        </Button>
                      </div>
                    ) : null}
                    {detail.can_manage && detail.items.length > 0 ? (
                      <div className="flex justify-end border-t pt-4">
                        <Button
                          variant="destructive"
                          size="sm"
                          onClick={() => {
                            if (window.confirm(t('memory.clearConfirm'))) {
                              operation.mutate(() => relationshipMemoryService.clear(detail.id));
                            }
                          }}
                        >
                          {t('memory.clearAll')}
                        </Button>
                      </div>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </section>
          </div>
        </div>
      </div>
    </ProtectedRoute>
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

function SettingRow({
  label,
  description,
  children,
}: {
  label: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border bg-muted/20 p-3">
      <div>
        <div className="text-sm font-medium">{label}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{description}</div>
      </div>
      {children}
    </div>
  );
}

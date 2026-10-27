'use client';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  Brain,
  Check,
  Loader2,
  MoreHorizontal,
  Pencil,
  Plus,
  Search,
  Trash2,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { MemoryTopicIcon } from './memory-topic-icon';
import { joinFacts, splitFacts } from './memory-topics';

// Long enough to notice a mistake and reach the Undo button.
const undoToastDuration = 10_000;
// How long a fact that was just written stays highlighted.
const highlightDuration = 2_500;

export interface MemoryTopic {
  key: string;
  title: string;
  content: string;
  /** Picks the topic's icon when the key itself is not a known topic. */
  iconKey?: string;
  /** When and how the topic last changed, already localized, e.g. "10/26 06:28 · 你修改的". */
  meta?: string;
  /** Present while the last automatic change can still be undone. */
  undoAutomatic?: () => Promise<unknown>;
}

export interface MemoryAddTarget {
  value: string;
  label: string;
}

export interface MemoryDeleteResult {
  /** Present when the deletion can be undone for a while. */
  undo?: () => Promise<unknown>;
}

interface MemoryBoardProps {
  topics: MemoryTopic[];
  canEdit: boolean;
  /** Writes a topic's whole content; the caller resolves revisions. */
  onSave: (key: string, content: string) => Promise<unknown>;
  onDeleteTopic: (key: string) => Promise<MemoryDeleteResult | void>;
  /** Where "Add memory" may put a new fact, and how to add it. */
  addTargets?: MemoryAddTarget[];
  onAdd?: (target: string, fact: string) => Promise<unknown>;
  /**
   * Opens "Add memory" from a button the caller places itself, such as in a
   * tab bar. Without it the board shows its own button.
   */
  addOpen?: boolean;
  onAddOpenChange?: (open: boolean) => void;
  /** Actions for the whole memory, such as export and clear all. */
  menu?: ReactNode;
  empty?: ReactNode;
  /** "auto" lays topics out in two columns on wide screens. */
  layout?: 'auto' | 'single';
}

// Shows memory the way people think about it: topics, each a short list of
// things the assistant knows. A fact is edited, removed, or added on its own;
// the topic is written back as a whole. Deleting offers undo instead of
// asking twice; only removing a whole topic asks for confirmation.
export function MemoryBoard({
  topics,
  canEdit,
  onSave,
  onDeleteTopic,
  addTargets = [],
  onAdd,
  addOpen: controlledAddOpen,
  onAddOpenChange,
  menu,
  empty,
  layout = 'single',
}: MemoryBoardProps) {
  const t = useT('common.memoryItems');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<{ key: string; index: number; text: string } | null>(null);
  const [adding, setAdding] = useState<{ key: string; text: string } | null>(null);
  const [pendingTopic, setPendingTopic] = useState<MemoryTopic | null>(null);
  const [ownAddOpen, setOwnAddOpen] = useState(false);
  const [addTarget, setAddTarget] = useState('');
  const [addText, setAddText] = useState('');
  const [highlight, setHighlight] = useState<{ key: string; fact: string } | null>(null);
  const addControlled = onAddOpenChange !== undefined;
  const addOpen = addControlled ? Boolean(controlledAddOpen) : ownAddOpen;
  const setAddOpen = addControlled ? onAddOpenChange : setOwnAddOpen;
  const canAdd = canEdit && Boolean(onAdd) && addTargets.length > 0;
  // Undo runs after the save has refreshed the topics, so it must use the
  // latest onSave, which carries the new revision.
  const onSaveRef = useRef(onSave);
  useEffect(() => {
    onSaveRef.current = onSave;
  }, [onSave]);

  // A dialog opened from outside starts from the first topic and an empty
  // fact, the same as one opened from the board's own button.
  const firstTarget = addTargets[0]?.value ?? '';
  useEffect(() => {
    if (!addOpen) return;
    setAddTarget(current =>
      addTargets.some(target => target.value === current) ? current : firstTarget
    );
    // Only the opening matters; targets change as memory refreshes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addOpen]);

  useEffect(() => {
    if (!highlight) return;
    const timer = window.setTimeout(() => setHighlight(null), highlightDuration);
    return () => window.clearTimeout(timer);
  }, [highlight]);

  const shown = useMemo(() => {
    const terms = search.trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (terms.length === 0) return topics;
    return topics.filter(topic => {
      const text = `${topic.title} ${topic.content}`.toLowerCase();
      return terms.every(term => text.includes(term));
    });
  }, [topics, search]);

  const run = async (task: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await task();
      return true;
    } catch {
      // The caller reports the error.
      return false;
    } finally {
      setBusy(false);
    }
  };

  const writeFacts = async (topic: MemoryTopic, facts: string[], message: string) => {
    const previous = topic.content;
    const ok = await run(() => onSave(topic.key, joinFacts(facts)));
    if (!ok) return false;
    toast.success(message, {
      duration: undoToastDuration,
      action: {
        label: t('undo'),
        onClick: () => void run(() => onSaveRef.current(topic.key, previous)),
      },
    });
    return true;
  };

  const removeFact = async (topic: MemoryTopic, index: number) => {
    const facts = splitFacts(topic.content);
    if (facts.length <= 1) {
      setPendingTopic(topic);
      return;
    }
    await writeFacts(
      topic,
      facts.filter((_, current) => current !== index),
      t('factDeleted')
    );
  };

  const saveEdit = async (topic: MemoryTopic) => {
    if (!editing) return;
    const facts = splitFacts(topic.content);
    const text = editing.text.trim();
    if (!text || text === facts[editing.index]) {
      setEditing(null);
      return;
    }
    facts[editing.index] = text;
    if (await writeFacts(topic, facts, t('factSaved'))) {
      setEditing(null);
      setHighlight({ key: topic.key, fact: text });
    }
  };

  const saveAdded = async (topic: MemoryTopic) => {
    const text = adding?.text.trim() ?? '';
    if (!text) {
      setAdding(null);
      return;
    }
    if (await writeFacts(topic, [...splitFacts(topic.content), text], t('factSaved'))) {
      setAdding(null);
      setHighlight({ key: topic.key, fact: text });
    }
  };

  const deleteTopic = async (topic: MemoryTopic) => {
    setBusy(true);
    try {
      const result = await onDeleteTopic(topic.key);
      setPendingTopic(null);
      const undo = result && result.undo;
      toast.success(
        t('topicDeleted', { topic: topic.title }),
        undo
          ? {
              duration: undoToastDuration,
              action: { label: t('undo'), onClick: () => void run(undo) },
            }
          : undefined
      );
    } catch {
      // The caller reports the error.
    } finally {
      setBusy(false);
    }
  };

  const openAdd = () => {
    setAddTarget(firstTarget);
    setAddText('');
    setAddOpen(true);
  };

  const submitAdd = async () => {
    const text = addText.trim();
    if (!onAdd || !addTarget || !text) return;
    if (await run(() => onAdd(addTarget, text))) {
      toast.success(t('factSaved'));
      setAddOpen(false);
      setAddText('');
      setHighlight({ key: addTarget, fact: text });
    }
  };

  const showToolbar = topics.length > 3 || (canAdd && !addControlled && topics.length > 0) || menu;

  return (
    <div className="space-y-3">
      {showToolbar ? (
        <div className="flex flex-wrap items-center gap-2">
          {topics.length > 3 ? (
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('search')}
              aria-label={t('search')}
              leftIcon={<Search />}
              className="min-w-48 flex-1"
            />
          ) : (
            <div className="flex-1" />
          )}
          {canAdd && !addControlled && topics.length > 0 ? (
            <Button size="sm" variant="outline" onClick={openAdd} disabled={busy}>
              <Plus className="mr-1 size-4" />
              {t('add')}
            </Button>
          ) : null}
          {menu}
        </div>
      ) : null}

      {topics.length === 0 ? (
        <div className="flex flex-col items-center rounded-xl border border-dashed px-6 py-10 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Brain className="size-5" />
          </span>
          <p className="mt-3 text-sm font-medium">{empty ?? t('empty')}</p>
          <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">{t('emptyHint')}</p>
          {canAdd ? (
            <Button size="sm" className="mt-4" onClick={openAdd} disabled={busy}>
              <Plus className="mr-1 size-4" />
              {t('add')}
            </Button>
          ) : null}
        </div>
      ) : shown.length === 0 ? (
        <div className="rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('noMatch')}
        </div>
      ) : (
        <div className={cn('gap-3', layout === 'auto' && 'lg:columns-2')}>
          {shown.map(topic => {
            const facts = splitFacts(topic.content);
            return (
              <section
                key={topic.key}
                className="group/topic mb-3 break-inside-avoid rounded-xl border bg-background px-2 pb-1.5 pt-2.5 transition-shadow hover:shadow-sm"
              >
                <header className="flex items-start gap-2.5 px-1.5 pb-1.5">
                  <MemoryTopicIcon topicKey={topic.iconKey ?? topic.key} />
                  <div className="min-w-0 flex-1">
                    <h3 className="flex items-baseline gap-2 text-sm font-medium">
                      <span className="truncate">{topic.title}</span>
                      <span className="shrink-0 text-xs font-normal text-muted-foreground">
                        {facts.length === 1
                          ? t('factCountOne')
                          : t('factCount', { count: facts.length })}
                      </span>
                    </h3>
                    {topic.meta ? (
                      <p className="truncate text-[11px] leading-4 text-muted-foreground/80">
                        {topic.meta}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex shrink-0 items-center gap-0.5">
                    {canEdit && topic.undoAutomatic ? (
                      <Button
                        variant="ghost"
                        size="xs"
                        className="text-primary"
                        disabled={busy}
                        onClick={() => {
                          const undo = topic.undoAutomatic;
                          if (undo) void run(undo);
                        }}
                      >
                        {t('undoAutomatic')}
                      </Button>
                    ) : null}
                    {canEdit ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="xs"
                            isIcon
                            className="sm:opacity-0 sm:transition-opacity sm:focus-visible:opacity-100 sm:group-focus-within/topic:opacity-100 sm:group-hover/topic:opacity-100 data-[state=open]:opacity-100"
                            aria-label={t('topicActions', { topic: topic.title })}
                          >
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            variant="destructive"
                            onSelect={() => setPendingTopic(topic)}
                          >
                            <Trash2 />
                            {t('deleteTopic')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </div>
                </header>
                <ul>
                  {facts.map((fact, index) => {
                    const isEditing = editing?.key === topic.key && editing.index === index;
                    const isFresh = highlight?.key === topic.key && highlight.fact === fact;
                    return (
                      <li
                        key={`${index}-${fact}`}
                        className={cn(
                          'group flex items-start gap-1 rounded-lg px-1.5 py-1 transition-colors duration-700',
                          !isEditing && 'hover:bg-muted/60',
                          isFresh && 'bg-amber-100/70 dark:bg-amber-500/15'
                        )}
                      >
                        {isEditing && editing ? (
                          <>
                            <Textarea
                              autoFocus
                              value={editing.text}
                              disabled={busy}
                              className="min-h-16 flex-1"
                              aria-label={t('editFact')}
                              onChange={event =>
                                setEditing({ ...editing, text: event.target.value })
                              }
                              onKeyDown={event => {
                                if (event.key === 'Enter' && !event.shiftKey) {
                                  event.preventDefault();
                                  void saveEdit(topic);
                                }
                                if (event.key === 'Escape') setEditing(null);
                              }}
                            />
                            <Button
                              size="xs"
                              isIcon
                              aria-label={t('save')}
                              disabled={busy}
                              onClick={() => void saveEdit(topic)}
                            >
                              {busy ? <Loader2 className="animate-spin" /> : <Check />}
                            </Button>
                            <Button
                              variant="ghost"
                              size="xs"
                              isIcon
                              aria-label={t('cancel')}
                              disabled={busy}
                              onClick={() => setEditing(null)}
                            >
                              <X />
                            </Button>
                          </>
                        ) : (
                          <>
                            <span
                              aria-hidden="true"
                              className="mt-[9px] size-1 shrink-0 rounded-full bg-muted-foreground/40"
                            />
                            {canEdit ? (
                              <button
                                type="button"
                                title={t('editFact')}
                                disabled={busy}
                                className="min-w-0 flex-1 cursor-text whitespace-pre-wrap break-words rounded px-1 text-left text-sm leading-6 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                                onClick={() => setEditing({ key: topic.key, index, text: fact })}
                              >
                                {fact}
                              </button>
                            ) : (
                              <p className="min-w-0 flex-1 whitespace-pre-wrap break-words px-1 text-sm leading-6">
                                {fact}
                              </p>
                            )}
                            {canEdit ? (
                              <div className="flex shrink-0 gap-0.5 opacity-100 sm:opacity-0 sm:transition-opacity sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
                                <Button
                                  variant="ghost"
                                  size="xs"
                                  isIcon
                                  aria-label={t('editFact')}
                                  disabled={busy}
                                  onClick={() => setEditing({ key: topic.key, index, text: fact })}
                                >
                                  <Pencil />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="xs"
                                  isIcon
                                  aria-label={t('deleteFact')}
                                  disabled={busy}
                                  className="hover:text-destructive"
                                  onClick={() => void removeFact(topic, index)}
                                >
                                  <Trash2 />
                                </Button>
                              </div>
                            ) : null}
                          </>
                        )}
                      </li>
                    );
                  })}
                </ul>
                {canEdit ? (
                  adding?.key === topic.key ? (
                    <div className="flex items-start gap-1 px-1.5 py-1">
                      <Textarea
                        autoFocus
                        value={adding.text}
                        disabled={busy}
                        className="min-h-16 flex-1"
                        placeholder={t('addFactPlaceholder')}
                        aria-label={t('addFact')}
                        onChange={event => setAdding({ ...adding, text: event.target.value })}
                        onKeyDown={event => {
                          if (event.key === 'Enter' && !event.shiftKey) {
                            event.preventDefault();
                            void saveAdded(topic);
                          }
                          if (event.key === 'Escape') setAdding(null);
                        }}
                      />
                      <Button
                        size="xs"
                        isIcon
                        aria-label={t('save')}
                        disabled={busy}
                        onClick={() => void saveAdded(topic)}
                      >
                        <Check />
                      </Button>
                      <Button
                        variant="ghost"
                        size="xs"
                        isIcon
                        aria-label={t('cancel')}
                        disabled={busy}
                        onClick={() => setAdding(null)}
                      >
                        <X />
                      </Button>
                    </div>
                  ) : (
                    <Button
                      variant="ghost"
                      size="xs"
                      className="ml-1 text-muted-foreground hover:text-foreground"
                      disabled={busy}
                      onClick={() => setAdding({ key: topic.key, text: '' })}
                    >
                      <Plus className="mr-1 size-3" />
                      {t('addFact')}
                    </Button>
                  )
                ) : null}
              </section>
            );
          })}
        </div>
      )}

      <ConfirmDialog
        open={Boolean(pendingTopic)}
        onOpenChange={next => {
          if (!next) setPendingTopic(null);
        }}
        title={t('deleteTopicTitle', { topic: pendingTopic?.title ?? '' })}
        description={t('deleteTopicDescription')}
        confirmText={t('deleteTopic')}
        cancelText={t('cancel')}
        variant="danger"
        loading={busy}
        onConfirm={() => {
          if (pendingTopic) void deleteTopic(pendingTopic);
        }}
      />

      <Dialog open={addOpen} onOpenChange={setAddOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('addTitle')}</DialogTitle>
            <DialogDescription>{t('addDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            <Textarea
              autoFocus
              value={addText}
              maxLength={2000}
              className="min-h-24"
              placeholder={t('addFactPlaceholder')}
              aria-label={t('addFact')}
              onChange={event => setAddText(event.target.value)}
            />
            <div className="space-y-1.5">
              <div className="text-xs font-medium text-muted-foreground">{t('addTarget')}</div>
              <Select value={addTarget} onValueChange={setAddTarget}>
                <SelectTrigger className="w-full" aria-label={t('addTarget')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {addTargets.map(target => (
                    <SelectItem key={target.value} value={target.value}>
                      {target.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setAddOpen(false)}>
              {t('cancel')}
            </Button>
            <Button
              disabled={busy || !addText.trim() || !addTarget}
              onClick={() => void submitAdd()}
            >
              {busy ? <Loader2 className="mr-1 size-4 animate-spin" /> : null}
              {t('save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

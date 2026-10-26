'use client';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { Check, Loader2, MoreHorizontal, Pencil, Plus, Search, Trash2, X } from 'lucide-react';
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
import { joinFacts, splitFacts } from './memory-topics';

// Long enough to notice a mistake and reach the Undo button.
const undoToastDuration = 10_000;

export interface MemoryTopic {
  key: string;
  title: string;
  content: string;
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
  /** Actions for the whole memory, such as export and clear all. */
  menu?: ReactNode;
  empty?: ReactNode;
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
  menu,
  empty,
}: MemoryBoardProps) {
  const t = useT('common.memoryItems');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [editing, setEditing] = useState<{ key: string; index: number; text: string } | null>(null);
  const [adding, setAdding] = useState<{ key: string; text: string } | null>(null);
  const [pendingTopic, setPendingTopic] = useState<MemoryTopic | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [addTarget, setAddTarget] = useState('');
  const [addText, setAddText] = useState('');
  // Undo runs after the save has refreshed the topics, so it must use the
  // latest onSave, which carries the new revision.
  const onSaveRef = useRef(onSave);
  useEffect(() => {
    onSaveRef.current = onSave;
  }, [onSave]);

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
    if (await writeFacts(topic, facts, t('factSaved'))) setEditing(null);
  };

  const saveAdded = async (topic: MemoryTopic) => {
    const text = adding?.text.trim() ?? '';
    if (!text) {
      setAdding(null);
      return;
    }
    if (await writeFacts(topic, [...splitFacts(topic.content), text], t('factSaved'))) {
      setAdding(null);
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
    setAddTarget(addTargets[0]?.value ?? '');
    setAddText('');
    setAddOpen(true);
  };

  const submitAdd = async () => {
    if (!onAdd || !addTarget || !addText.trim()) return;
    if (await run(() => onAdd(addTarget, addText.trim()))) {
      toast.success(t('factSaved'));
      setAddOpen(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        {topics.length > 3 ? (
          <Input
            value={search}
            onChange={event => setSearch(event.target.value)}
            placeholder={t('search')}
            leftIcon={<Search />}
            className="min-w-48 flex-1"
          />
        ) : (
          <div className="flex-1" />
        )}
        {canEdit && onAdd && addTargets.length > 0 ? (
          <Button size="sm" onClick={openAdd} disabled={busy}>
            <Plus className="mr-1 size-4" />
            {t('add')}
          </Button>
        ) : null}
        {menu}
      </div>

      {topics.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {empty ?? t('empty')}
        </div>
      ) : shown.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('noMatch')}
        </div>
      ) : (
        <div className="space-y-3">
          {shown.map(topic => {
            const facts = splitFacts(topic.content);
            return (
              <section key={topic.key} className="rounded-lg border bg-background">
                <header className="flex items-start justify-between gap-3 border-b px-3 py-2.5">
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-medium">
                      {topic.title}
                      <span className="ml-2 text-xs font-normal text-muted-foreground">
                        {facts.length === 1
                          ? t('factCountOne')
                          : t('factCount', { count: facts.length })}
                      </span>
                    </h3>
                    {topic.meta ? (
                      <p className="mt-0.5 text-xs text-muted-foreground">{topic.meta}</p>
                    ) : null}
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {canEdit && topic.undoAutomatic ? (
                      <Button
                        variant="ghost"
                        size="xs"
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
                <ul className="divide-y">
                  {facts.map((fact, index) => {
                    const isEditing = editing?.key === topic.key && editing.index === index;
                    return (
                      <li
                        key={`${index}-${fact}`}
                        className="group flex items-start gap-2 px-3 py-2"
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
                            <p className="min-w-0 flex-1 whitespace-pre-wrap break-words text-sm">
                              {fact}
                            </p>
                            {canEdit ? (
                              <div className="flex shrink-0 gap-1 opacity-100 sm:opacity-0 sm:transition-opacity sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
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
                    <div className="flex items-start gap-2 border-t px-3 py-2">
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
                    <div className="border-t px-1 py-1">
                      <Button
                        variant="ghost"
                        size="xs"
                        disabled={busy}
                        onClick={() => setAdding({ key: topic.key, text: '' })}
                      >
                        <Plus className="mr-1 size-3" />
                        {t('addFact')}
                      </Button>
                    </div>
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

'use client';

import { useMemo, useState, type ReactNode } from 'react';
import { ChevronDown, ChevronRight, Loader2, RotateCcw, Save, Search, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

export interface MemoryListItem {
  /** Stable identity of the row, such as the item key. */
  id: string;
  label: string;
  /** Short tags shown beside the label, such as the category or path. */
  badges?: string[];
  content: string;
  /** A metadata line shown when the row is open, such as revision and source. */
  meta?: string;
  maxChars?: number;
  undoable?: boolean;
}

interface MemoryItemListProps {
  items: MemoryListItem[];
  /** Whether rows may be edited, deleted, and undone. */
  canEdit: boolean;
  busy?: boolean;
  onSave: (item: MemoryListItem, content: string) => Promise<unknown>;
  onDelete?: (item: MemoryListItem) => void;
  onUndo?: (item: MemoryListItem) => void;
  empty?: ReactNode;
}

// Lists memory items as compact, searchable rows: one line each, opened in
// place to read the whole item and edit it. A long memory stays scannable.
export function MemoryItemList({
  items,
  canEdit,
  busy = false,
  onSave,
  onDelete,
  onUndo,
  empty,
}: MemoryItemListProps) {
  const t = useT('common.memoryItems');
  const [search, setSearch] = useState('');
  const [openId, setOpenId] = useState('');
  const [draft, setDraft] = useState('');
  const shown = useMemo(() => {
    const terms = search.trim().toLowerCase().split(/\s+/).filter(Boolean);
    if (terms.length === 0) return items;
    return items.filter(item => {
      const text = `${item.label} ${item.badges?.join(' ') ?? ''} ${item.content}`.toLowerCase();
      return terms.every(term => text.includes(term));
    });
  }, [items, search]);

  const toggle = (item: MemoryListItem) => {
    if (openId === item.id) {
      setOpenId('');
      return;
    }
    setOpenId(item.id);
    setDraft(item.content);
  };

  if (items.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        {empty ?? t('empty')}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {items.length > 5 ? (
        <Input
          value={search}
          onChange={event => setSearch(event.target.value)}
          placeholder={t('search')}
          leftIcon={<Search />}
        />
      ) : null}
      {shown.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('noMatch')}
        </div>
      ) : (
        <div className="divide-y rounded-lg border">
          {shown.map(item => {
            const open = openId === item.id;
            const changed = draft.trim() !== item.content;
            return (
              <div key={item.id} className={cn(open && 'bg-muted/20')}>
                <button
                  type="button"
                  aria-expanded={open}
                  onClick={() => toggle(item)}
                  className="flex w-full items-center gap-2 px-3 py-2.5 text-left hover:bg-muted/30"
                >
                  {open ? (
                    <ChevronDown className="size-4 shrink-0 text-muted-foreground" />
                  ) : (
                    <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                  )}
                  <span className="shrink-0 text-sm font-medium">{item.label}</span>
                  {item.badges?.map(badge => (
                    <Badge key={badge} variant="outline" className="hidden shrink-0 sm:inline-flex">
                      {badge}
                    </Badge>
                  ))}
                  {!open ? (
                    <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
                      {item.content.split('\n')[0]}
                    </span>
                  ) : null}
                </button>
                {open ? (
                  <div className="space-y-2 px-3 pb-3">
                    {item.meta ? (
                      <div className="text-xs text-muted-foreground">{item.meta}</div>
                    ) : null}
                    {canEdit ? (
                      <Textarea
                        value={draft}
                        maxLength={item.maxChars ?? 2000}
                        disabled={busy}
                        onChange={event => setDraft(event.target.value)}
                        className="min-h-24"
                      />
                    ) : (
                      <p className="whitespace-pre-wrap break-words text-sm">{item.content}</p>
                    )}
                    {canEdit ? (
                      <div className="flex flex-wrap justify-end gap-2">
                        {item.undoable && onUndo ? (
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy}
                            onClick={() => onUndo(item)}
                          >
                            <RotateCcw className="mr-1 size-4" />
                            {t('undo')}
                          </Button>
                        ) : null}
                        {onDelete ? (
                          <Button
                            variant="outline"
                            size="sm"
                            className="text-destructive"
                            disabled={busy}
                            onClick={() => onDelete(item)}
                          >
                            <Trash2 className="mr-1 size-4" />
                            {t('delete')}
                          </Button>
                        ) : null}
                        <Button
                          size="sm"
                          disabled={busy || !changed || !draft.trim()}
                          onClick={() => {
                            onSave(item, draft.trim()).then(
                              () => setOpenId(''),
                              // The caller reports the error; the row stays open.
                              () => undefined
                            );
                          }}
                        >
                          {busy ? (
                            <Loader2 className="mr-1 size-4 animate-spin" />
                          ) : (
                            <Save className="mr-1 size-4" />
                          )}
                          {t('save')}
                        </Button>
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

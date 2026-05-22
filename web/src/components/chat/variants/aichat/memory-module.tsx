'use client';

import { useEffect, useMemo, useState } from 'react';
import { Brain, Check, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import {
  useAccountMemory,
  useCreateAccountMemoryEntry,
  useDeleteAccountMemoryEntry,
  useUpdateAccountMemoryEntry,
  useUpdateAccountMemorySettings,
} from '@/hooks/use-account-memory';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AccountMemoryCategory, AccountMemoryEntry } from '@/services/types/memory';

const categoryOptions: AccountMemoryCategory[] = [
  'preference',
  'profile',
  'instruction',
  'fact',
  'other',
];

interface AIChatMemoryModuleProps {
  disabled?: boolean;
  onEnabledChange?: (enabled: boolean) => void;
}

export function AIChatMemoryModule({ disabled, onEnabledChange }: AIChatMemoryModuleProps) {
  const t = useT('webapp');
  const { data, isLoading } = useAccountMemory();
  const updateSettings = useUpdateAccountMemorySettings();
  const createEntry = useCreateAccountMemoryEntry();
  const [draft, setDraft] = useState('');
  const [category, setCategory] = useState<AccountMemoryCategory>('preference');
  const enabled = Boolean(data?.enabled);
  const entries = useMemo(() => data?.entries ?? [], [data?.entries]);

  useEffect(() => {
    onEnabledChange?.(enabled);
  }, [enabled, onEnabledChange]);

  const handleToggle = (checked: boolean) => {
    onEnabledChange?.(checked);
    updateSettings.mutate({ enabled: checked });
  };

  const handleCreate = () => {
    const content = draft.trim();
    if (!content) return;
    createEntry.mutate(
      { content, category },
      {
        onSuccess: () => {
          setDraft('');
          toast.success(t('consoleChat.memory.saved'));
        },
      }
    );
  };

  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          isIcon
          variant={enabled ? 'secondary' : 'ghost'}
          className={cn('size-8 rounded-full', enabled ? 'text-primary' : '')}
          disabled={disabled}
          title={t('consoleChat.memory.title')}
        >
          <Brain className="size-4" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" side="top" className="w-[min(92vw,420px)] p-0">
        <PopoverHeader className="border-b px-3 py-2">
          <div className="flex items-center justify-between gap-3">
            <PopoverTitle className="text-sm">{t('consoleChat.memory.title')}</PopoverTitle>
            <Switch
              checked={enabled}
              disabled={isLoading || updateSettings.isPending}
              onCheckedChange={handleToggle}
            />
          </div>
        </PopoverHeader>

        <div className="max-h-[420px] space-y-3 overflow-y-auto p-3">
          <div className="space-y-2">
            <Textarea
              value={draft}
              onChange={event => setDraft(event.target.value)}
              placeholder={t('consoleChat.memory.placeholder')}
              className="min-h-16 resize-none text-xs"
            />
            <div className="flex items-center gap-2">
              <CategorySelect value={category} onChange={setCategory} />
              <Button
                isIcon
                size="sm"
                className="size-8 shrink-0"
                disabled={!draft.trim() || createEntry.isPending}
                onClick={handleCreate}
                title={t('consoleChat.memory.add')}
              >
                <Plus className="size-4" />
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            {entries.length === 0 ? (
              <div className="rounded-md border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
                {t('consoleChat.memory.empty')}
              </div>
            ) : (
              entries.map(entry => <MemoryEntryEditor key={entry.id} entry={entry} />)
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

function MemoryEntryEditor({ entry }: { entry: AccountMemoryEntry }) {
  const t = useT('webapp');
  const updateEntry = useUpdateAccountMemoryEntry();
  const deleteEntry = useDeleteAccountMemoryEntry();
  const [content, setContent] = useState(entry.content);
  const [category, setCategory] = useState<AccountMemoryCategory>(entry.category);
  const [enabled, setEnabled] = useState(entry.enabled);

  useEffect(() => {
    setContent(entry.content);
    setCategory(entry.category);
    setEnabled(entry.enabled);
  }, [entry]);

  const dirty = content !== entry.content || category !== entry.category || enabled !== entry.enabled;

  const handleSave = () => {
    const nextContent = content.trim();
    if (!nextContent) return;
    updateEntry.mutate(
      {
        id: entry.id,
        payload: {
          content: nextContent,
          category,
          enabled,
        },
      },
      {
        onSuccess: () => toast.success(t('consoleChat.memory.saved')),
      }
    );
  };

  return (
    <div className="rounded-md border p-2">
      <Textarea
        value={content}
        onChange={event => setContent(event.target.value)}
        className="min-h-14 resize-none border-0 p-1 text-xs shadow-none focus-visible:ring-0"
      />
      <div className="mt-2 flex items-center gap-2">
        <CategorySelect value={category} onChange={setCategory} />
        <Switch checked={enabled} onCheckedChange={setEnabled} />
        <Button
          isIcon
          variant="ghost"
          size="sm"
          className="ml-auto size-7"
          disabled={!dirty || updateEntry.isPending || !content.trim()}
          onClick={handleSave}
          title={t('consoleChat.memory.save')}
        >
          <Check className="size-4" />
        </Button>
        <Button
          isIcon
          variant="ghost"
          size="sm"
          className="size-7 text-destructive hover:text-destructive"
          disabled={deleteEntry.isPending}
          onClick={() => deleteEntry.mutate(entry.id)}
          title={t('consoleChat.memory.delete')}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}

function CategorySelect({
  value,
  onChange,
}: {
  value: AccountMemoryCategory;
  onChange: (value: AccountMemoryCategory) => void;
}) {
  const t = useT('webapp');
  return (
    <select
      value={value}
      onChange={event => onChange(event.target.value as AccountMemoryCategory)}
      className="h-8 min-w-0 flex-1 rounded-md border bg-background px-2 text-xs"
    >
      {categoryOptions.map(option => (
        <option key={option} value={option}>
          {t(`consoleChat.memory.categories.${option}`)}
        </option>
      ))}
    </select>
  );
}

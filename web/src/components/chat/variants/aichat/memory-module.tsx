'use client';

import { useEffect, useMemo, useState } from 'react';
import { Brain, CalendarClock, Check, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
import type {
  AccountMemoryCategory,
  AccountMemoryEntry,
  AccountMemoryType,
} from '@/services/types/memory';
import { formatDate } from '@/utils/format';

const categoryOptions: AccountMemoryCategory[] = [
  'preference',
  'profile',
  'instruction',
  'fact',
  'other',
];

const memoryTypeOptions: AccountMemoryType[] = ['long_term', 'temporary'];

interface AIChatMemoryModuleProps {
  disabled?: boolean;
  onEnabledChange?: (enabled: boolean) => void;
}

export function AIChatMemoryModule({ disabled, onEnabledChange }: AIChatMemoryModuleProps) {
  const t = useT('webapp');
  const { data, isLoading } = useAccountMemory();
  const updateSettings = useUpdateAccountMemorySettings();
  const [managerOpen, setManagerOpen] = useState(false);
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    setEnabled(Boolean(data?.enabled));
  }, [data?.enabled]);

  useEffect(() => {
    onEnabledChange?.(enabled);
  }, [enabled, onEnabledChange]);

  const handleToggle = (checked: boolean) => {
    const previous = enabled;
    setEnabled(checked);
    onEnabledChange?.(checked);
    updateSettings.mutate(
      { enabled: checked },
      {
        onError: () => {
          setEnabled(previous);
          onEnabledChange?.(previous);
        },
      }
    );
  };

  return (
    <>
      <div
        className={cn(
          'flex h-8 shrink-0 items-center gap-0.5 rounded-full border border-border/70 bg-background/90 px-0.5 pr-1 shadow-xs transition-colors',
          enabled ? 'border-primary/30 bg-primary/5' : 'hover:border-border-strong hover:bg-muted/30',
          disabled ? 'opacity-60' : ''
        )}
      >
        <Button
          isIcon
          variant="ghost"
          className={cn(
            'size-7 rounded-full bg-transparent hover:bg-background/80',
            enabled ? 'text-primary hover:text-primary' : 'text-muted-foreground hover:text-foreground'
          )}
          disabled={disabled}
          onClick={() => setManagerOpen(true)}
          title={t('consoleChat.memory.title')}
        >
          <Brain className="size-4" />
        </Button>
        <Switch
          checked={enabled}
          disabled={disabled || isLoading || updateSettings.isPending}
          className="scale-90"
          onCheckedChange={handleToggle}
          aria-label={t('consoleChat.memory.title')}
        />
      </div>
      <AccountMemoryManagerDialog open={managerOpen} onOpenChange={setManagerOpen} />
    </>
  );
}

interface AccountMemoryManagerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function AccountMemoryManagerDialog({
  open,
  onOpenChange,
}: AccountMemoryManagerDialogProps) {
  const t = useT('webapp');
  const { data } = useAccountMemory();
  const createEntry = useCreateAccountMemoryEntry();
  const [draft, setDraft] = useState('');
  const [category, setCategory] = useState<AccountMemoryCategory>('preference');
  const [memoryType, setMemoryType] = useState<AccountMemoryType>('long_term');
  const [expiresAt, setExpiresAt] = useState('');
  const entries = useMemo(
    () => [...(data?.entries ?? [])].sort((left, right) => right.updated_at - left.updated_at),
    [data?.entries]
  );

  const handleCreate = () => {
    const content = draft.trim();
    if (!content) return;
    const expiresAtISO = memoryType === 'temporary' ? datetimeLocalToISO(expiresAt) : undefined;
    if (memoryType === 'temporary' && !expiresAtISO) return;
    createEntry.mutate(
      { content, category, memory_type: memoryType, expires_at: expiresAtISO },
      {
        onSuccess: () => {
          setDraft('');
          setMemoryType('long_term');
          setExpiresAt('');
          toast.success(t('consoleChat.memory.saved'));
        },
      }
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="w-[calc(100vw-2rem)] p-0">
        <DialogHeader className="border-b p-5 pb-4">
          <DialogTitle>{t('consoleChat.memory.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="max-h-[calc(100vh-9rem)] space-y-4 p-5">
          <Card variant="subtle" className="border border-border/70">
            <CardContent className="space-y-3 p-4">
              <Textarea
                value={draft}
                onChange={event => setDraft(event.target.value)}
                placeholder={t('consoleChat.memory.placeholder')}
                className="min-h-24 resize-none bg-background text-sm"
              />
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                <CategorySelect value={category} onChange={setCategory} />
                <MemoryTypeSelect value={memoryType} onChange={setMemoryType} />
                {memoryType === 'temporary' ? (
                  <Input
                    type="datetime-local"
                    value={expiresAt}
                    onChange={event => setExpiresAt(event.target.value)}
                    className="h-8 text-xs sm:max-w-56"
                    aria-label={t('consoleChat.memory.expiresAt')}
                  />
                ) : null}
                <Button
                  size="sm"
                  className="w-full sm:w-auto"
                  disabled={
                    !draft.trim() ||
                    createEntry.isPending ||
                    (memoryType === 'temporary' && !datetimeLocalToISO(expiresAt))
                  }
                  onClick={handleCreate}
                  title={t('consoleChat.memory.add')}
                >
                  <Plus className="size-4" />
                  {t('consoleChat.memory.add')}
                </Button>
              </div>
            </CardContent>
          </Card>

          <div className="space-y-2">
            {entries.length === 0 ? (
              <div className="rounded-md border border-dashed px-3 py-8 text-center text-sm text-muted-foreground">
                {t('consoleChat.memory.empty')}
              </div>
            ) : (
              entries.map(entry => <MemoryEntryEditor key={entry.id} entry={entry} />)
            )}
          </div>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function MemoryEntryEditor({ entry }: { entry: AccountMemoryEntry }) {
  const t = useT('webapp');
  const updateEntry = useUpdateAccountMemoryEntry();
  const deleteEntry = useDeleteAccountMemoryEntry();
  const [content, setContent] = useState(entry.content);
  const [category, setCategory] = useState<AccountMemoryCategory>(entry.category);
  const [memoryType, setMemoryType] = useState<AccountMemoryType>(entry.memory_type);
  const [expiresAt, setExpiresAt] = useState(timestampToDatetimeLocal(entry.expires_at));
  const [enabled, setEnabled] = useState(entry.enabled);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

  useEffect(() => {
    setContent(entry.content);
    setCategory(entry.category);
    setMemoryType(entry.memory_type);
    setExpiresAt(timestampToDatetimeLocal(entry.expires_at));
    setEnabled(entry.enabled);
  }, [entry]);

  const dirty =
    content !== entry.content ||
    category !== entry.category ||
    memoryType !== entry.memory_type ||
    expiresAt !== timestampToDatetimeLocal(entry.expires_at) ||
    enabled !== entry.enabled;

  const handleSave = () => {
    const nextContent = content.trim();
    if (!nextContent) return;
    const expiresAtISO = memoryType === 'temporary' ? datetimeLocalToISO(expiresAt) : undefined;
    if (memoryType === 'temporary' && !expiresAtISO) return;
    updateEntry.mutate(
      {
        id: entry.id,
        payload: {
          content: nextContent,
          category,
          memory_type: memoryType,
          expires_at: expiresAtISO,
          enabled,
        },
      },
      {
        onSuccess: () => toast.success(t('consoleChat.memory.saved')),
      }
    );
  };

  const handleDelete = () => {
    deleteEntry.mutate(entry.id);
  };

  return (
    <>
      <Card className={cn('overflow-hidden', enabled ? '' : 'bg-muted/20')}>
        <CardContent className="space-y-3 p-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <CategorySelect value={category} onChange={setCategory} />
            <MemoryTypeSelect value={memoryType} onChange={setMemoryType} />
            {memoryType === 'temporary' ? (
              <Input
                type="datetime-local"
                value={expiresAt}
                onChange={event => setExpiresAt(event.target.value)}
                className="h-8 text-xs sm:max-w-56"
                aria-label={t('consoleChat.memory.expiresAt')}
              />
            ) : null}
            <Switch
              checked={enabled}
              onCheckedChange={setEnabled}
              aria-label={t('consoleChat.memory.title')}
            />
            <div className="flex items-center gap-1 sm:ml-auto">
              <Button
                isIcon
                variant="ghost"
                size="sm"
                className="size-7"
                disabled={
                  !dirty ||
                  updateEntry.isPending ||
                  !content.trim() ||
                  (memoryType === 'temporary' && !datetimeLocalToISO(expiresAt))
                }
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
                onClick={() => setDeleteConfirmOpen(true)}
                title={t('consoleChat.memory.delete')}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          </div>
          <Textarea
            value={content}
            onChange={event => setContent(event.target.value)}
            className="min-h-24 resize-none bg-background text-sm"
          />
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
            <span>{t('consoleChat.memory.updatedAt', { time: formatMemoryTime(entry.updated_at) })}</span>
            {entry.memory_type === 'temporary' && entry.expires_at ? (
              <span className="inline-flex items-center gap-1">
                <CalendarClock className="size-3" />
                {entry.status === 'expired'
                  ? t('consoleChat.memory.expiredAt', {
                      time: formatMemoryTime(entry.expires_at),
                    })
                  : t('consoleChat.memory.validUntil', {
                      time: formatMemoryTime(entry.expires_at),
                    })}
              </span>
            ) : null}
          </div>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('consoleChat.memory.deleteConfirmTitle')}
        description={t('consoleChat.memory.deleteConfirmDescription')}
        confirmText={t('consoleChat.memory.delete')}
        cancelText={t('consoleChat.memory.cancel')}
        variant="warning"
        loading={deleteEntry.isPending}
        onConfirm={handleDelete}
      />
    </>
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
    <Select value={value} onValueChange={next => onChange(next as AccountMemoryCategory)}>
      <SelectTrigger className="h-8 min-w-0 flex-1 rounded-md text-xs sm:max-w-48">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {categoryOptions.map(option => (
          <SelectItem key={option} value={option}>
            {t(`consoleChat.memory.categories.${option}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function MemoryTypeSelect({
  value,
  onChange,
}: {
  value: AccountMemoryType;
  onChange: (value: AccountMemoryType) => void;
}) {
  const t = useT('webapp');
  return (
    <Select value={value} onValueChange={next => onChange(next as AccountMemoryType)}>
      <SelectTrigger className="h-8 min-w-0 flex-1 rounded-md text-xs sm:max-w-44">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {memoryTypeOptions.map(option => (
          <SelectItem key={option} value={option}>
            {t(`consoleChat.memory.types.${option}`)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function formatMemoryTime(value?: number | null) {
  if (!value) return '-';
  return formatDate(value, 'YYYY-MM-DD HH:mm');
}

function timestampToDatetimeLocal(value?: number | null) {
  if (!value) return '';
  const date = new Date(value < 1e12 ? value * 1000 : value);
  if (Number.isNaN(date.getTime())) return '';
  const pad = (input: number) => input.toString().padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours()
  )}:${pad(date.getMinutes())}`;
}

function datetimeLocalToISO(value: string) {
  if (!value) return undefined;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return undefined;
  return date.toISOString();
}

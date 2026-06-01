'use client';

import { useMemo, useState } from 'react';
import {
  AlertCircle,
  ChevronDown,
  ChevronUp,
  Edit3,
  Eye,
  Layers3,
  Loader2,
  Plus,
  RefreshCw,
  Save,
  Search,
  SearchX,
  Trash2,
} from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type { FileDocumentChunk } from '@/services/types/file';
import { useFileChunks, useUpdateFileChunk } from '@/hooks/file/use-file-chunks';
import { cn } from '@/lib/utils';

interface FileChunksPanelProps {
  fileId: string;
  enabled: boolean;
}

type ChunkFilter = 'all' | 'enabled' | 'disabled';

function ChunkSkeleton() {
  return (
    <div className="space-y-4">
      {Array.from({ length: 3 }).map((_, index) => (
        <div key={index} className="rounded-lg border border-border bg-background p-5">
          <Skeleton className="h-6 w-48" />
          <Skeleton className="mt-5 h-16 w-full" />
          <Skeleton className="mt-4 h-10 w-full" />
        </div>
      ))}
    </div>
  );
}

export function FileChunksPanel({ fileId, enabled }: FileChunksPanelProps) {
  const t = useT('files');
  const [draftByChunk, setDraftByChunk] = useState<Record<string, string>>({});
  const [expandedIds, setExpandedIds] = useState<Record<string, boolean>>({});
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<ChunkFilter>('all');
  const { data, isLoading, error } = useFileChunks(fileId, { limit: 500 }, { enabled });
  const updateChunk = useUpdateFileChunk(fileId);
  const response = data?.data;
  const primaryChunks = useMemo(
    () => (response?.tree && response.tree.length > 0 ? response.tree : response?.items ?? []),
    [response?.items, response?.tree]
  );
  const total = response?.primary_chunk_count ?? response?.total ?? primaryChunks.length;
  const secondaryTotal = response?.secondary_chunk_count ?? countSecondaryChunks(primaryChunks);

  const visibleChunks = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return primaryChunks.filter(chunk => {
      if (filter === 'enabled' && !chunk.enabled) return false;
      if (filter === 'disabled' && chunk.enabled) return false;
      if (!keyword) return true;
      return (
        chunk.content.toLowerCase().includes(keyword) ||
        (chunk.children ?? []).some(child => child.content.toLowerCase().includes(keyword))
      );
    });
  }, [filter, primaryChunks, search]);

  const allExpanded = visibleChunks.length > 0 && visibleChunks.every(chunk => expandedIds[chunk.id] ?? true);

  const updateDraft = (chunkId: string, value: string) => {
    setDraftByChunk(current => ({ ...current, [chunkId]: value }));
  };

  const saveChunkContent = async (chunk: FileDocumentChunk) => {
    const content = draftByChunk[chunk.id] ?? chunk.content;
    await updateChunk.mutateAsync({ chunkId: chunk.id, data: { content } });
    setDraftByChunk(current => {
      const next = { ...current };
      delete next[chunk.id];
      return next;
    });
  };

  const toggleChunkEnabled = async (chunk: FileDocumentChunk, checked: boolean) => {
    await updateChunk.mutateAsync({ chunkId: chunk.id, data: { enabled: checked } });
  };

  const toggleExpanded = (chunkId: string) => {
    setExpandedIds(current => ({ ...current, [chunkId]: !(current[chunkId] ?? true) }));
  };

  const setAllExpanded = (expanded: boolean) => {
    const next: Record<string, boolean> = {};
    for (const chunk of visibleChunks) {
      next[chunk.id] = expanded;
    }
    setExpandedIds(current => ({ ...current, ...next }));
  };

  if (!enabled) {
    return (
      <Alert>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.chunks.notReadyTitle')}</AlertTitle>
        <AlertDescription>{t('detail.chunks.notReadyDescription')}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) return <ChunkSkeleton />;

  if (error || !response) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.chunks.loadErrorTitle')}</AlertTitle>
        <AlertDescription>{t('detail.chunks.loadErrorDescription')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-xl font-semibold leading-tight text-foreground">{t('detail.chunks.title')}</h2>
            <Badge variant="outline" className="rounded-full px-3 py-1 text-sm">
              {t('detail.chunks.total', { count: total })}
            </Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">{t('detail.index.description')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-full sm:w-72">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('detail.chunks.searchPlaceholder')}
              className="h-10 rounded-lg pl-9"
            />
          </div>
          <select
            value={filter}
            onChange={event => setFilter(event.target.value as ChunkFilter)}
            className="h-10 rounded-lg border border-input bg-background px-4 text-sm font-medium text-foreground shadow-sm"
            aria-label={t('detail.chunks.filters.all')}
          >
            <option value="all">{t('detail.chunks.filters.all')}</option>
            <option value="enabled">{t('detail.chunks.filters.enabled')}</option>
            <option value="disabled">{t('detail.chunks.filters.disabled')}</option>
          </select>
          <Button variant="outline" className="h-10 gap-2 rounded-lg" onClick={() => setAllExpanded(!allExpanded)}>
            {allExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            {allExpanded ? t('detail.chunks.collapseAll') : t('detail.chunks.expandAll')}
          </Button>
          <Button variant="outline" className="h-10 gap-2 rounded-lg" disabled>
            <RefreshCw className="h-4 w-4" />
            {t('detail.chunks.resegment')}
          </Button>
          <Button className="h-10 gap-2 rounded-lg" disabled>
            <Plus className="h-4 w-4" />
            {t('detail.chunks.add')}
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-background px-4 py-3">
        <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
          <span className="h-5 w-5 rounded-full border-2 border-primary" />
          <span>{t('detail.chunks.selectAll', { count: total })}</span>
          <span className="text-border">|</span>
          <span>{t('detail.chunks.secondaryCount', { count: secondaryTotal })}</span>
          <span className="text-border">|</span>
          <span>{t('detail.chunks.generationNo', { value: response.generation_no })}</span>
        </div>
      </div>

      {visibleChunks.length === 0 ? (
        <div className="flex min-h-[280px] items-center justify-center rounded-lg border border-dashed border-border bg-background p-6 text-center">
          <div>
            <SearchX className="mx-auto h-8 w-8 text-muted-foreground" />
            <div className="mt-3 text-sm font-medium text-foreground">
              {t('detail.chunks.emptyTitle')}
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('detail.chunks.emptyDescription')}
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-4">
          {visibleChunks.map(chunk => (
            <PrimaryChunkCard
              key={chunk.id}
              chunk={chunk}
              expanded={expandedIds[chunk.id] ?? true}
              draftByChunk={draftByChunk}
              disabled={updateChunk.isPending}
              onDraftChange={updateDraft}
              onSave={saveChunkContent}
              onToggleEnabled={toggleChunkEnabled}
              onToggleExpanded={toggleExpanded}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function PrimaryChunkCard({
  chunk,
  expanded,
  draftByChunk,
  disabled,
  onDraftChange,
  onSave,
  onToggleEnabled,
  onToggleExpanded,
}: {
  chunk: FileDocumentChunk;
  expanded: boolean;
  draftByChunk: Record<string, string>;
  disabled: boolean;
  onDraftChange: (chunkId: string, value: string) => void;
  onSave: (chunk: FileDocumentChunk) => Promise<void>;
  onToggleEnabled: (chunk: FileDocumentChunk, checked: boolean) => Promise<void>;
  onToggleExpanded: (chunkId: string) => void;
}) {
  const t = useT('files');
  const children = chunk.children ?? [];

  return (
    <article className="rounded-lg border border-border bg-background p-5 shadow-sm transition-colors focus-within:border-primary/70">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <span className="h-5 w-5 rounded-full border-2 border-primary" />
            <h3 className="text-lg font-semibold text-primary">#{chunk.position + 1}</h3>
            <Badge variant="info" className="rounded-full px-3">
              <Layers3 className="mr-1 h-3.5 w-3.5" />
              {t('detail.chunks.primary')}
            </Badge>
            <span className={cn('text-sm font-medium', chunk.enabled ? 'text-success' : 'text-muted-foreground')}>
              {chunk.enabled ? t('detail.chunks.enabled') : t('detail.chunks.disabled')}
            </span>
            <Badge variant="subtle">{chunk.status}</Badge>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">{t('detail.chunks.enabled')}</span>
          <Switch
            checked={chunk.enabled}
            onCheckedChange={checked => void onToggleEnabled(chunk, checked)}
            disabled={disabled}
          />
          <Button variant="ghost" size="sm" className="h-9 w-9 p-0" disabled>
            <Edit3 className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="sm" className="h-9 w-9 p-0" disabled>
            <Trash2 className="h-4 w-4" />
          </Button>
          <Button variant="ghost" size="sm" className="h-9 w-9 p-0" onClick={() => onToggleExpanded(chunk.id)}>
            {expanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          </Button>
        </div>
      </div>

      <p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-foreground">{chunk.content}</p>

      <div className="mt-5 flex flex-wrap items-center gap-3 text-sm">
        <Badge variant="subtle" className="rounded-full">
          {t('detail.chunks.secondaryCount', { count: children.length })}
        </Badge>
        <Button variant="link" className="h-auto p-0 text-primary" onClick={() => onToggleExpanded(chunk.id)}>
          {t('detail.chunks.manageSecondary')}
        </Button>
        <Button variant="link" className="h-auto gap-1 p-0 text-primary" disabled>
          <Eye className="h-4 w-4" />
          {t('detail.chunks.viewOriginal')}
        </Button>
        <span className="text-muted-foreground">
          {t('detail.chunks.characters', { count: chunk.content.length })}
        </span>
      </div>

      {expanded ? (
        <div className="mt-4 space-y-3">
          {children.map((child, index) => (
            <SecondaryChunkRow
              key={child.id}
              chunk={child}
              index={index}
              draft={draftByChunk[child.id] ?? child.content}
              disabled={disabled}
              onDraftChange={onDraftChange}
              onSave={onSave}
              onToggleEnabled={onToggleEnabled}
            />
          ))}
        </div>
      ) : null}
    </article>
  );
}

function SecondaryChunkRow({
  chunk,
  index,
  draft,
  disabled,
  onDraftChange,
  onSave,
  onToggleEnabled,
}: {
  chunk: FileDocumentChunk;
  index: number;
  draft: string;
  disabled: boolean;
  onDraftChange: (chunkId: string, value: string) => void;
  onSave: (chunk: FileDocumentChunk) => Promise<void>;
  onToggleEnabled: (chunk: FileDocumentChunk, checked: boolean) => Promise<void>;
}) {
  const t = useT('files');
  const dirty = draft !== chunk.content;

  return (
    <div className="rounded-lg border border-border bg-muted/20 p-4">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex flex-wrap items-center gap-3">
          <span className="font-mono text-sm font-semibold text-success">#S-{index + 1}</span>
          <Badge variant="subtle" className="rounded-full">
            {t('detail.chunks.secondary')}
          </Badge>
          <span className="text-sm text-muted-foreground">
            {t('detail.chunks.characters', { count: chunk.content.length })}
          </span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">{t('detail.chunks.enabled')}</span>
          <Switch
            checked={chunk.enabled}
            onCheckedChange={checked => void onToggleEnabled(chunk, checked)}
            disabled={disabled}
          />
        </div>
      </div>
      <Textarea
        value={draft}
        onChange={event => onDraftChange(chunk.id, event.target.value)}
        className="mt-3 min-h-24 resize-y bg-background text-sm leading-6"
        disabled={disabled}
      />
      <div className="mt-3 flex justify-end">
        <Button
          size="sm"
          variant="outline"
          className="gap-2 rounded-lg"
          onClick={() => void onSave(chunk)}
          disabled={disabled || !dirty || draft.trim() === ''}
        >
          {disabled ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
          {t('detail.chunks.save')}
        </Button>
      </div>
    </div>
  );
}

function countSecondaryChunks(chunks: FileDocumentChunk[]): number {
  return chunks.reduce((count, chunk) => count + (chunk.children?.length ?? 0), 0);
}

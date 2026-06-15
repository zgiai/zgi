'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  AlertCircle,
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Edit3,
  Layers3,
  Loader2,
  PanelLeftOpen,
  Save,
  Search,
  SearchX,
} from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type { FileDocumentChunk } from '@/services/types/file';
import { useFileChunks, useUpdateFileChunk } from '@/hooks/file/use-file-chunks';
import { cn } from '@/lib/utils';
import type { FilePreviewLocator } from './file-original-preview-panel';

interface FileChunksPanelProps {
  fileId: string;
  enabled: boolean;
  className?: string;
  originalPreviewHidden?: boolean;
  onToggleOriginalPreview?: () => void;
  onLocateIssue?: (locator: FilePreviewLocator) => void;
}

type ChunkFilter = 'all' | 'issues' | 'enabled' | 'disabled';
const SHOW_CHUNK_QUALITY_ISSUES = false;

interface ChunkQualityIssue {
  id?: string;
  type?: string;
  reason?: string;
  status?: string;
  confidence?: number;
  originalContent?: string;
  contentExcerpt?: string;
  sourceLocator?: FilePreviewLocator;
}
type FilesTranslator = ((key: string, values?: Record<string, unknown>) => string) & {
  has?: (key: string) => boolean;
};

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

export function FileChunksPanel({
  fileId,
  enabled,
  className,
  originalPreviewHidden = false,
  onToggleOriginalPreview,
  onLocateIssue,
}: FileChunksPanelProps) {
  const t = useT('files');
  const [editingPrimaryChunkId, setEditingPrimaryChunkId] = useState<string | null>(null);
  const [primaryDraft, setPrimaryDraft] = useState('');
  const [expandedIds, setExpandedIds] = useState<Record<string, boolean>>({});
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
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

  const visibleChunks = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    return primaryChunks.filter(chunk => {
      if (filter === 'enabled' && !chunk.enabled) return false;
      if (filter === 'disabled' && chunk.enabled) return false;
      if (SHOW_CHUNK_QUALITY_ISSUES && filter === 'issues' && !hasChunkQualityIssues(chunk)) return false;
      if (!keyword) return true;
      return (
        chunk.content.toLowerCase().includes(keyword) ||
        (chunk.children ?? []).some(child => child.content.toLowerCase().includes(keyword))
      );
    });
  }, [filter, primaryChunks, search]);

  const allExpanded = visibleChunks.length > 0 && visibleChunks.every(chunk => expandedIds[chunk.id] ?? false);
  const visibleChunkIds = useMemo(() => visibleChunks.map(chunk => chunk.id), [visibleChunks]);
  const selectedVisibleIds = useMemo(
    () => selectedChunkIds.filter(id => visibleChunkIds.includes(id)),
    [selectedChunkIds, visibleChunkIds]
  );
  const selectedChunks = useMemo(
    () => visibleChunks.filter(chunk => selectedVisibleIds.includes(chunk.id)),
    [selectedVisibleIds, visibleChunks]
  );
  const selectedCount = selectedVisibleIds.length;
  const allVisibleSelected = visibleChunks.length > 0 && selectedCount === visibleChunks.length;
  const someVisibleSelected = selectedCount > 0 && !allVisibleSelected;

  useEffect(() => {
    setSelectedChunkIds(current => current.filter(id => visibleChunkIds.includes(id)));
  }, [visibleChunkIds]);

  useEffect(() => {
    if (!SHOW_CHUNK_QUALITY_ISSUES && filter === 'issues') {
      setFilter('all');
    }
  }, [filter]);

  const startEditPrimary = (chunk: FileDocumentChunk) => {
    setEditingPrimaryChunkId(chunk.id);
    setPrimaryDraft(chunk.content);
  };

  const cancelEditPrimary = () => {
    setEditingPrimaryChunkId(null);
    setPrimaryDraft('');
  };

  const savePrimaryChunkContent = async (chunk: FileDocumentChunk) => {
    await updateChunk.mutateAsync({
      chunkId: chunk.id,
      data: { content: primaryDraft },
    });
    cancelEditPrimary();
  };

  const toggleChunkEnabled = async (chunk: FileDocumentChunk, checked: boolean) => {
    await updateChunk.mutateAsync({ chunkId: chunk.id, data: { enabled: checked } });
  };

  const toggleChunkSelected = (chunkId: string, checked: boolean) => {
    setSelectedChunkIds(current => {
      if (checked) {
        return current.includes(chunkId) ? current : [...current, chunkId];
      }
      return current.filter(id => id !== chunkId);
    });
  };

  const toggleAllVisibleSelected = (checked: boolean) => {
    setSelectedChunkIds(checked ? visibleChunkIds : []);
  };

  const batchSetSelectedEnabled = async (checked: boolean) => {
    await Promise.all(
      selectedChunks
        .filter(chunk => chunk.enabled !== checked)
        .map(chunk => updateChunk.mutateAsync({ chunkId: chunk.id, data: { enabled: checked } }))
    );
  };

  const toggleExpanded = (chunkId: string) => {
    setExpandedIds(current => ({ ...current, [chunkId]: !(current[chunkId] ?? false) }));
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
      <Alert className={className}>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.chunks.notReadyTitle')}</AlertTitle>
        <AlertDescription>{t('detail.chunks.notReadyDescription')}</AlertDescription>
      </Alert>
    );
  }

  if (isLoading) {
    return (
      <div className={cn('h-full min-h-0 overflow-y-auto p-4 sm:p-5', className)}>
        <ChunkSkeleton />
      </div>
    );
  }

  if (error || !response) {
    return (
      <Alert variant="destructive" className={className}>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.chunks.loadErrorTitle')}</AlertTitle>
        <AlertDescription>{t('detail.chunks.loadErrorDescription')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className={cn('flex h-full min-h-0 flex-col overflow-hidden bg-bg-canvas', className)}>
      <div className="shrink-0 border-b bg-background px-4 py-2.5 sm:px-5">
        <div className="flex flex-wrap items-center gap-2">
          <div className="mr-auto flex min-w-[160px] items-center gap-2">
            <h2 className="text-lg font-semibold leading-tight text-foreground">{t('detail.chunks.title')}</h2>
            <Badge variant="outline" className="rounded-full px-2.5 py-0.5 text-xs">
              {t('detail.chunks.total', { count: total })}
            </Badge>
          </div>

          {originalPreviewHidden && onToggleOriginalPreview ? (
            <Button
              type="button"
              variant="outline"
              className="h-8 gap-1.5 rounded-md px-2.5 text-sm"
              onClick={onToggleOriginalPreview}
            >
              <PanelLeftOpen className="h-4 w-4" />
              {t('detail.previewToggle.showOriginal')}
            </Button>
          ) : null}

          <div className="relative min-w-[220px] flex-1 sm:max-w-[360px]">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('detail.chunks.searchPlaceholder')}
              className="h-8 rounded-md pl-8 text-sm"
            />
          </div>
          <select
            value={filter}
            onChange={event => setFilter(event.target.value as ChunkFilter)}
            className="h-8 rounded-md border border-input bg-background px-2.5 text-sm font-medium text-foreground shadow-sm"
            aria-label={t('detail.chunks.filters.all')}
          >
            <option value="all">{t('detail.chunks.filters.all')}</option>
            {SHOW_CHUNK_QUALITY_ISSUES ? (
              <option value="issues">{t('detail.chunks.filters.issues')}</option>
            ) : null}
            <option value="enabled">{t('detail.chunks.filters.enabled')}</option>
            <option value="disabled">{t('detail.chunks.filters.disabled')}</option>
          </select>
          <Button variant="outline" className="h-8 gap-1.5 rounded-md px-2.5 text-sm" onClick={() => setAllExpanded(!allExpanded)}>
            {allExpanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
            {allExpanded ? t('detail.chunks.collapseAll') : t('detail.chunks.expandAll')}
          </Button>
        </div>

        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-muted-foreground">
          <label className="flex items-center gap-2">
            <Checkbox
              checked={allVisibleSelected ? true : someVisibleSelected ? 'indeterminate' : false}
              onCheckedChange={checked => toggleAllVisibleSelected(checked === true)}
              disabled={visibleChunks.length === 0 || updateChunk.isPending}
              className="h-4 w-4 rounded-full"
              aria-label={t('detail.chunks.selectAll', { count: visibleChunks.length })}
            />
            <span>{t('detail.chunks.selectAll', { count: visibleChunks.length })}</span>
          </label>
          {selectedCount > 0 ? (
            <>
              <span className="text-border">|</span>
              <span className="font-medium text-foreground">
                {t('detail.chunks.selectedCount', { count: selectedCount })}
              </span>
            </>
          ) : null}
          <div className="ml-auto flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="h-8 rounded-md px-2.5"
              disabled={selectedCount === 0 || updateChunk.isPending}
              onClick={() => void batchSetSelectedEnabled(true)}
            >
              {t('detail.chunks.batchEnable')}
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-8 rounded-md px-2.5"
              disabled={selectedCount === 0 || updateChunk.isPending}
              onClick={() => void batchSetSelectedEnabled(false)}
            >
              {t('detail.chunks.batchDisable')}
            </Button>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 sm:p-5">
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
                expanded={expandedIds[chunk.id] ?? false}
                selected={selectedVisibleIds.includes(chunk.id)}
                editing={editingPrimaryChunkId === chunk.id}
                draft={editingPrimaryChunkId === chunk.id ? primaryDraft : chunk.content}
                disabled={updateChunk.isPending}
                onEditPrimary={startEditPrimary}
                onCancelEdit={cancelEditPrimary}
                onDraftChange={setPrimaryDraft}
                onSavePrimary={savePrimaryChunkContent}
                onSelect={toggleChunkSelected}
                onToggleEnabled={toggleChunkEnabled}
                onToggleExpanded={toggleExpanded}
                onLocateIssue={onLocateIssue}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function PrimaryChunkCard({
  chunk,
  expanded,
  selected,
  editing,
  draft,
  disabled,
  onEditPrimary,
  onCancelEdit,
  onDraftChange,
  onSavePrimary,
  onSelect,
  onToggleEnabled,
  onToggleExpanded,
  onLocateIssue,
}: {
  chunk: FileDocumentChunk;
  expanded: boolean;
  selected: boolean;
  editing: boolean;
  draft: string;
  disabled: boolean;
  onEditPrimary: (chunk: FileDocumentChunk) => void;
  onCancelEdit: () => void;
  onDraftChange: (value: string) => void;
  onSavePrimary: (chunk: FileDocumentChunk) => Promise<void>;
  onSelect: (chunkId: string, checked: boolean) => void;
  onToggleEnabled: (chunk: FileDocumentChunk, checked: boolean) => Promise<void>;
  onToggleExpanded: (chunkId: string) => void;
  onLocateIssue?: (locator: FilePreviewLocator) => void;
}) {
  const t = useT('files');
  const children = chunk.children ?? [];
  const qualityIssues = SHOW_CHUNK_QUALITY_ISSUES ? chunkQualityIssues(chunk) : [];

  return (
    <article
      className={cn(
        'rounded-lg border bg-background p-5 shadow-sm transition-colors focus-within:border-primary/70',
        selected ? 'border-primary ring-1 ring-primary' : 'border-border'
      )}
    >
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-3">
            <Checkbox
              checked={selected}
              onCheckedChange={checked => onSelect(chunk.id, checked === true)}
              disabled={disabled}
              className="h-5 w-5 rounded-full"
              aria-label={t('detail.chunks.selectChunk', { position: chunk.position + 1 })}
            />
            <h3 className="text-lg font-semibold text-primary">#{chunk.position + 1}</h3>
            <Badge variant="info" className="rounded-full px-3">
              <Layers3 className="mr-1 h-3.5 w-3.5" />
              {t('detail.chunks.primary')}
            </Badge>
            {qualityIssues.length > 0 ? (
              <Badge variant="warning" className="rounded-full px-3">
                <AlertTriangle className="mr-1 h-3.5 w-3.5" />
                {t('detail.chunks.issues.badge', { count: qualityIssues.length })}
              </Badge>
            ) : null}
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
          <Button
            variant="ghost"
            size="sm"
            className="h-9 w-9 p-0"
            onClick={() => onEditPrimary(chunk)}
            disabled={disabled || editing}
            aria-label={t('detail.chunks.edit')}
          >
            <Edit3 className="h-4 w-4" />
          </Button>
          {/* Re-enable after file chunk deletion API is implemented. */}
          {/*
          <Button variant="ghost" size="sm" className="h-9 w-9 p-0">
            <Trash2 className="h-4 w-4" />
          </Button>
          */}
          <Button variant="ghost" size="sm" className="h-9 w-9 p-0" onClick={() => onToggleExpanded(chunk.id)}>
            {expanded ? <ChevronUp className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          </Button>
        </div>
      </div>

      {qualityIssues.length > 0 ? (
        <div className="mt-4 space-y-2 rounded-md border border-warning/25 bg-warning/10 px-3 py-2 text-sm text-warning">
          <div className="font-medium">{t('detail.chunks.issues.title')}</div>
          {qualityIssues.map(issue => (
            <ChunkIssueRow
              key={issue.id || `${issue.reason}-${issue.contentExcerpt}`}
              issue={issue}
              onLocateIssue={onLocateIssue}
            />
          ))}
        </div>
      ) : null}

      {editing ? (
        <div className="mt-4 rounded-lg border border-primary/30 bg-primary/5 p-3">
          <Textarea
            value={draft}
            onChange={event => onDraftChange(event.target.value)}
            className="min-h-44 resize-y bg-background text-sm leading-6"
            disabled={disabled}
            autoFocus
          />
          <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
            <span className="text-xs text-muted-foreground">
              {t('detail.chunks.characters', { count: draft.length })}
            </span>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" onClick={onCancelEdit} disabled={disabled}>
                {t('detail.chunks.cancel')}
              </Button>
              <Button
                size="sm"
                className="gap-1.5"
                onClick={() => void onSavePrimary(chunk)}
                disabled={disabled || draft.trim() === '' || draft === chunk.content}
              >
                {disabled ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Save className="h-4 w-4" />
                )}
                {t('detail.chunks.save')}
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <p className="mt-4 whitespace-pre-wrap text-sm leading-7 text-foreground">
          <HighlightedChunkContent
            content={chunk.content}
            issues={qualityIssues}
            onLocateIssue={onLocateIssue}
          />
        </p>
      )}

      <div className="mt-5 flex flex-wrap items-center gap-3 text-sm">
        <Badge variant="subtle" className="rounded-full">
          {t('detail.chunks.secondaryCount', { count: children.length })}
        </Badge>
        <Button variant="link" className="h-auto p-0 text-primary" onClick={() => onToggleExpanded(chunk.id)}>
          {expanded ? t('detail.chunks.collapseSecondary') : t('detail.chunks.viewSecondary')}
        </Button>
        {/* Re-enable after source-locator preview is implemented for chunks. */}
        {/*
        <Button variant="link" className="h-auto gap-1 p-0 text-primary">
          <Eye className="h-4 w-4" />
          {t('detail.chunks.viewOriginal')}
        </Button>
        */}
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
            />
          ))}
        </div>
      ) : null}
    </article>
  );
}

function hasChunkQualityIssues(chunk: FileDocumentChunk) {
  if (chunkQualityIssues(chunk).length > 0) {
    return true;
  }
  return (chunk.children ?? []).some(child => chunkQualityIssues(child).length > 0);
}

function chunkQualityIssues(chunk: FileDocumentChunk): ChunkQualityIssue[] {
  const raw = chunk.metadata_json?.quality_issues;
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .filter((item): item is Record<string, unknown> => item !== null && typeof item === 'object' && !Array.isArray(item))
    .map(item => ({
      id: typeof item.id === 'string' ? item.id : undefined,
      type: typeof item.type === 'string' ? item.type : undefined,
      reason: typeof item.reason === 'string' ? item.reason : undefined,
      status: typeof item.status === 'string' ? item.status : undefined,
      confidence: typeof item.confidence === 'number' ? item.confidence : undefined,
      originalContent: typeof item.original_content === 'string' ? item.original_content : undefined,
      contentExcerpt: typeof item.content_excerpt === 'string' ? item.content_excerpt : undefined,
      sourceLocator: parseIssueSourceLocator(item),
    }));
}

function ChunkIssueRow({
  issue,
  onLocateIssue,
}: {
  issue: ChunkQualityIssue;
  onLocateIssue?: (locator: FilePreviewLocator) => void;
}) {
  const t = useT('files');
  const locator = issue.sourceLocator;
  const canLocate = Boolean(locator?.bbox && Number.isFinite(locator.page));
  const page = locator?.page;
  const excerpt = issue.contentExcerpt || issue.originalContent;

  return (
    <div className="rounded border border-warning/20 bg-background/70 px-2.5 py-2 text-warning">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{qualityIssueText(issue, t as FilesTranslator)}</span>
        {Number.isFinite(page) ? (
          <Badge variant="outline" className="border-warning/30 text-warning">
            {t('detail.chunks.issues.page', { page })}
          </Badge>
        ) : null}
        {canLocate && onLocateIssue ? (
          <Button
            type="button"
            variant="link"
            className="h-auto p-0 text-warning"
            onClick={() => onLocateIssue(locator as FilePreviewLocator)}
          >
            {t('detail.chunks.issues.locate')}
          </Button>
        ) : (
          <span className="text-xs text-warning/80">{t('detail.chunks.issues.impreciseLocation')}</span>
        )}
      </div>
      {excerpt ? (
        <div className="mt-1 line-clamp-2 text-xs leading-5 text-warning/85">
          {t('detail.chunks.issues.excerpt', { text: excerpt })}
        </div>
      ) : null}
    </div>
  );
}

function HighlightedChunkContent({
  content,
  issues,
  onLocateIssue,
}: {
  content: string;
  issues: ChunkQualityIssue[];
  onLocateIssue?: (locator: FilePreviewLocator) => void;
}) {
  const ranges = issueHighlightRanges(content, issues);
  if (ranges.length === 0) {
    return <>{content}</>;
  }
  const parts: Array<{ text: string; highlighted: boolean; locator?: FilePreviewLocator }> = [];
  let cursor = 0;
  for (const range of ranges) {
    if (range.start > cursor) {
      parts.push({ text: content.slice(cursor, range.start), highlighted: false });
    }
    parts.push({ text: content.slice(range.start, range.end), highlighted: true, locator: range.locator });
    cursor = range.end;
  }
  if (cursor < content.length) {
    parts.push({ text: content.slice(cursor), highlighted: false });
  }
  return (
    <>
      {parts.map((part, index) =>
        part.highlighted ? (
          <mark
            key={index}
            role={part.locator && onLocateIssue ? 'button' : undefined}
            tabIndex={part.locator && onLocateIssue ? 0 : undefined}
            className={cn(
              'rounded bg-warning/20 px-0.5 text-warning',
              part.locator && onLocateIssue && 'cursor-pointer ring-warning/30 hover:ring-2'
            )}
            onClick={() => {
              if (part.locator && onLocateIssue) {
                onLocateIssue(part.locator);
              }
            }}
            onKeyDown={event => {
              if (!part.locator || !onLocateIssue) return;
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault();
                onLocateIssue(part.locator);
              }
            }}
          >
            {part.text}
          </mark>
        ) : (
          <span key={index}>{part.text}</span>
        )
      )}
    </>
  );
}

function issueHighlightRanges(content: string, issues: ChunkQualityIssue[]) {
  const ranges: Array<{ start: number; end: number; locator?: FilePreviewLocator }> = [];
  for (const issue of issues) {
    const candidates = [issue.originalContent, issue.contentExcerpt]
      .map(value => normalizeHighlightCandidate(value))
      .filter((value): value is string => Boolean(value));
    for (const candidate of candidates) {
      const start = content.indexOf(candidate);
      if (start >= 0) {
        ranges.push({ start, end: start + candidate.length, locator: issue.sourceLocator });
        break;
      }
    }
  }
  return mergeRanges(ranges);
}

function normalizeHighlightCandidate(value?: string) {
  const trimmed = value?.trim();
  if (!trimmed) {
    return undefined;
  }
  return trimmed.endsWith('...') ? trimmed.slice(0, -3).trim() : trimmed;
}

function mergeRanges(ranges: Array<{ start: number; end: number; locator?: FilePreviewLocator }>) {
  if (ranges.length <= 1) {
    return ranges;
  }
  const sorted = [...ranges].sort((a, b) => a.start - b.start);
  const merged: Array<{ start: number; end: number; locator?: FilePreviewLocator }> = [];
  for (const range of sorted) {
    const last = merged[merged.length - 1];
    if (last && range.start <= last.end) {
      last.end = Math.max(last.end, range.end);
      last.locator = last.locator || range.locator;
      continue;
    }
    merged.push({ ...range });
  }
  return merged;
}

function parseIssueSourceLocator(item: Record<string, unknown>): FilePreviewLocator | undefined {
  const raw = item.source_locator;
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return undefined;
  }
  const locator = raw as Record<string, unknown>;
  const page = numberValue(locator.page);
  const bbox = parseIssueBBox(locator.bbox);
  if (!bbox && page === undefined) {
    return undefined;
  }
  return {
    id: typeof item.id === 'string' ? item.id : undefined,
    page,
    bbox,
    label: typeof item.reason === 'string' ? item.reason : undefined,
  };
}

function parseIssueBBox(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return undefined;
  }
  const box = value as Record<string, unknown>;
  const left = numberValue(box.left);
  const top = numberValue(box.top);
  const right = numberValue(box.right);
  const bottom = numberValue(box.bottom);
  if (left === undefined || top === undefined || right === undefined || bottom === undefined) {
    return undefined;
  }
  return { left, top, right, bottom };
}

function numberValue(value: unknown) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function qualityIssueText(issue: ChunkQualityIssue, t: FilesTranslator) {
  const reasons = (issue.reason ?? '')
    .split(',')
    .map(reason => reason.trim())
    .filter(Boolean)
    .map(reason => qualityIssueReasonText(reason, t));
  if (reasons.length > 0) {
    return reasons.join('、');
  }
  return issue.type || t('detail.chunks.issues.fallback');
}

function qualityIssueReasonText(reason: string, t: FilesTranslator) {
  switch (reason) {
    case 'low_confidence_text':
      return t('detail.parseReview.reviewReasons.lowConfidenceText');
    case 'low_confidence_table':
      return t('detail.parseReview.reviewReasons.lowConfidenceTable');
    case 'low_confidence_image_ocr':
      return t('detail.parseReview.reviewReasons.lowConfidenceImageOcr');
    case 'review_required':
      return t('detail.parseReview.reviewReasons.reviewRequired');
    case 'ocr_fallback':
      return t('detail.parseReview.reviewReasons.ocrFallback');
    case 'local_vlm_fallback':
      return t('detail.parseReview.reviewReasons.vlmFallback');
    case 'table_structure_risk':
      return t('detail.parseReview.reviewReasons.tableStructureRisk');
    default:
      return reason;
  }
}

function SecondaryChunkRow({
  chunk,
  index,
}: {
  chunk: FileDocumentChunk;
  index: number;
}) {
  const t = useT('files');

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
      </div>
      <div className="mt-3 rounded-lg border border-border bg-background p-3 shadow-sm">
        <p className="max-h-24 min-w-0 flex-1 overflow-hidden whitespace-pre-wrap text-sm leading-6 text-foreground">
          {chunk.content}
        </p>
      </div>
    </div>
  );
}

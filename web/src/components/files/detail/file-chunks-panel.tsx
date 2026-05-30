'use client';

import { useState } from 'react';
import { AlertCircle, FileText, Loader2, Save, SearchX } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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

function isLeafChunk(chunk: FileDocumentChunk) {
  return !chunk.children || chunk.children.length === 0;
}

function flattenChunkCount(chunks: FileDocumentChunk[]): number {
  return chunks.reduce(
    (count, chunk) => count + 1 + flattenChunkCount(chunk.children ?? []),
    0
  );
}

function ChunkSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 4 }).map((_, index) => (
        <div key={index} className="rounded-md border border-border bg-background p-4">
          <Skeleton className="h-5 w-40" />
          <Skeleton className="mt-3 h-24 w-full" />
        </div>
      ))}
    </div>
  );
}

export function FileChunksPanel({ fileId, enabled }: FileChunksPanelProps) {
  const t = useT('files');
  const [draftByChunk, setDraftByChunk] = useState<Record<string, string>>({});
  const { data, isLoading, error } = useFileChunks(
    fileId,
    { include_tree: true, limit: 500 },
    { enabled }
  );
  const updateChunk = useUpdateFileChunk(fileId);
  const response = data?.data;
  const chunks = response?.tree && response.tree.length > 0 ? response.tree : response?.items ?? [];
  const total = response?.total ?? flattenChunkCount(chunks);

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
      <div className="rounded-md border border-border bg-background p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-foreground">{t('detail.chunks.title')}</h2>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge variant="outline">{t('detail.chunks.total', { count: total })}</Badge>
              <Badge variant="subtle">
                {t('detail.chunks.generationNo', { value: response.generation_no })}
              </Badge>
            </div>
          </div>
        </div>
      </div>

      {chunks.length === 0 ? (
        <div className="flex min-h-[280px] items-center justify-center rounded-md border border-dashed border-border bg-background p-6 text-center">
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
        <div className="space-y-3">
          {chunks.map(chunk => (
            <ChunkNode
              key={chunk.id}
              chunk={chunk}
              level={0}
              draftByChunk={draftByChunk}
              disabled={updateChunk.isPending}
              onDraftChange={updateDraft}
              onSave={saveChunkContent}
              onToggleEnabled={toggleChunkEnabled}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function ChunkNode({
  chunk,
  level,
  draftByChunk,
  disabled,
  onDraftChange,
  onSave,
  onToggleEnabled,
}: {
  chunk: FileDocumentChunk;
  level: number;
  draftByChunk: Record<string, string>;
  disabled: boolean;
  onDraftChange: (chunkId: string, value: string) => void;
  onSave: (chunk: FileDocumentChunk) => Promise<void>;
  onToggleEnabled: (chunk: FileDocumentChunk, checked: boolean) => Promise<void>;
}) {
  const t = useT('files');
  const leaf = isLeafChunk(chunk);
  const draft = draftByChunk[chunk.id] ?? chunk.content;
  const dirty = draft !== chunk.content;

  return (
    <article
      className={cn(
        'rounded-md border bg-background p-4',
        !chunk.enabled && 'opacity-70',
        level > 0 && 'ml-4 border-l-4'
      )}
    >
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <FileText className="h-4 w-4 text-muted-foreground" />
            <h3 className="text-sm font-semibold text-foreground">
              {t('detail.chunks.chunkTitle', { position: chunk.position + 1 })}
            </h3>
            <Badge variant={leaf ? 'info' : 'outline'}>
              {leaf ? t('detail.chunks.leaf') : t('detail.chunks.parent')}
            </Badge>
            <Badge variant={chunk.enabled ? 'success' : 'subtle'}>
              {chunk.enabled ? t('detail.chunks.enabled') : t('detail.chunks.disabled')}
            </Badge>
            <Badge variant="subtle">{chunk.status}</Badge>
          </div>
        </div>
        {leaf ? (
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">{t('detail.chunks.enabled')}</span>
            <Switch
              checked={chunk.enabled}
              onCheckedChange={checked => void onToggleEnabled(chunk, checked)}
              disabled={disabled}
            />
          </div>
        ) : null}
      </div>

      {leaf ? (
        <div className="mt-3">
          <Textarea
            value={draft}
            onChange={event => onDraftChange(chunk.id, event.target.value)}
            className="min-h-32 bg-bg-canvas/40"
            disabled={disabled}
          />
          <div className="mt-3 flex justify-end">
            <Button
              size="sm"
              className="gap-2"
              onClick={() => void onSave(chunk)}
              disabled={disabled || !dirty || draft.trim() === ''}
            >
              {disabled ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              {t('detail.chunks.save')}
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-3 whitespace-pre-wrap rounded-md bg-muted/40 p-3 text-sm leading-6 text-muted-foreground">
          {chunk.content}
        </div>
      )}

      {chunk.children && chunk.children.length > 0 ? (
        <div className="mt-3 space-y-3">
          {chunk.children.map(child => (
            <ChunkNode
              key={child.id}
              chunk={child}
              level={level + 1}
              draftByChunk={draftByChunk}
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

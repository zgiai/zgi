'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { Loader2, RotateCcw } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useT } from '@/i18n';
import type {
  MemoryHistoryCommit,
  MemoryHistoryPage,
  MemoryHistoryVersion,
} from '@/services/types/memory-history';
import { getErrorMessage } from '@/utils/error-notifications';

interface MemoryHistoryPanelProps {
  /** Loads one page; `before` continues from the previous page. */
  load: (before?: string) => Promise<MemoryHistoryPage>;
  revert: (versionId: string) => Promise<unknown>;
  /** Display name of a memory file, by key. */
  labelFor?: (key: string) => string;
  canRevert?: boolean;
  /** Changes when the memory was edited elsewhere, so the history reloads. */
  refreshKey?: unknown;
  onReverted?: () => void | Promise<void>;
}

const knownActions = ['create', 'update', 'delete', 'clear', 'undo'] as const;
const knownSources = ['explicit', 'automatic', 'manager', 'legacy'] as const;

// Shows a user's memory history (ADR 0013) newest first, and lets them revert
// the latest change to a file.
export function MemoryHistoryPanel({
  load,
  revert,
  labelFor,
  canRevert = true,
  refreshKey,
  onReverted,
}: MemoryHistoryPanelProps) {
  const t = useT('common.memoryHistory');
  const [commits, setCommits] = useState<MemoryHistoryCommit[]>([]);
  const [nextBefore, setNextBefore] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [pending, setPending] = useState<MemoryHistoryVersion | null>(null);
  const [reverting, setReverting] = useState(false);
  const loadRef = useRef(load);

  useEffect(() => {
    loadRef.current = load;
  }, [load]);

  const reload = useCallback(async () => {
    setLoading(true);
    setFailed(false);
    try {
      const page = await loadRef.current();
      setCommits(page.commits ?? []);
      setNextBefore(page.next_before);
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload, refreshKey]);

  const loadMore = async () => {
    if (!nextBefore) return;
    setLoading(true);
    try {
      const page = await loadRef.current(nextBefore);
      setCommits(current => [...current, ...(page.commits ?? [])]);
      setNextBefore(page.next_before);
    } catch (error) {
      toast.error(getErrorMessage(error) || t('loadFailed'));
    } finally {
      setLoading(false);
    }
  };

  const confirmRevert = async () => {
    if (!pending) return;
    setReverting(true);
    try {
      await revert(pending.id);
      toast.success(t('reverted'));
      setPending(null);
      await reload();
      await onReverted?.();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('revertFailed'));
      await reload();
    } finally {
      setReverting(false);
    }
  };

  const actionLabel = (action: string) =>
    (knownActions as readonly string[]).includes(action)
      ? t(`actions.${action as (typeof knownActions)[number]}`)
      : action;
  const sourceLabel = (source: string) =>
    (knownSources as readonly string[]).includes(source)
      ? t(`sources.${source as (typeof knownSources)[number]}`)
      : source;

  if (failed && commits.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t('unavailable')}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="text-xs text-muted-foreground">{t('description')}</div>
      {loading && commits.length === 0 ? (
        <div className="flex min-h-24 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : commits.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('empty')}
        </div>
      ) : (
        commits.map(commit => (
          <section key={commit.id} className="space-y-2 rounded-lg border p-3">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <Badge variant="secondary">{sourceLabel(commit.source_kind)}</Badge>
              <span>{new Date(commit.created_at).toLocaleString()}</span>
            </div>
            {(commit.versions ?? []).map(version => (
              <div key={version.id} className="space-y-1.5 rounded-md bg-muted/30 p-2.5 text-sm">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <Badge variant="outline">{actionLabel(version.action)}</Badge>
                    <span className="truncate font-medium">
                      {labelFor?.(version.key) || version.key}
                    </span>
                  </div>
                  {canRevert && version.revertible ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      onClick={() => setPending(version)}
                    >
                      <RotateCcw className="mr-1 size-3" />
                      {t('revert')}
                    </Button>
                  ) : null}
                </div>
                {version.content_purged ? (
                  <div className="text-xs text-muted-foreground">{t('purged')}</div>
                ) : (
                  <div className="grid gap-2 text-xs sm:grid-cols-2">
                    <div className="min-w-0">
                      <div className="mb-0.5 text-muted-foreground">{t('before')}</div>
                      <div className="whitespace-pre-wrap break-words text-muted-foreground">
                        {version.before || t('emptyValue')}
                      </div>
                    </div>
                    <div className="min-w-0">
                      <div className="mb-0.5 text-muted-foreground">{t('after')}</div>
                      <div className="whitespace-pre-wrap break-words">
                        {version.after || t('emptyValue')}
                      </div>
                    </div>
                  </div>
                )}
                {version.evidence ? (
                  <div className="text-xs text-muted-foreground">
                    {t('evidence', { evidence: version.evidence })}
                  </div>
                ) : null}
                {version.reason ? (
                  <div className="text-xs text-muted-foreground">
                    {t('reason', { reason: version.reason })}
                  </div>
                ) : null}
              </div>
            ))}
          </section>
        ))
      )}
      {nextBefore ? (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={loading}
            onClick={() => void loadMore()}
          >
            {loading ? <Loader2 className="mr-1.5 size-3.5 animate-spin" /> : null}
            {t('loadMore')}
          </Button>
        </div>
      ) : null}
      <ConfirmDialog
        open={Boolean(pending)}
        onOpenChange={next => !next && setPending(null)}
        title={t('revertConfirmTitle')}
        description={t('revertConfirmDescription')}
        confirmText={t('revert')}
        cancelText={t('cancel')}
        variant="warning"
        loading={reverting}
        onConfirm={() => void confirmRevert()}
      />
    </div>
  );
}

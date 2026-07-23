'use client';

import { useEffect, useState } from 'react';
import { CheckCircle2, History, Loader2, RotateCcw, X } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover';
import { Skeleton } from '@/components/ui/skeleton';
import { Checkbox } from '@/components/ui/checkbox';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentPublishedVersionListItem } from './types';
import type { AgentPublishedVersionRollbackPreview } from '@/services/types/agent';

interface AgentRuntimeVersionPopoverProps {
  open: boolean;
  isLoading: boolean;
  isRollingBack: boolean;
  isLoadingPreview: boolean;
  isPreviewing: boolean;
  canOpen?: boolean;
  canRollback?: boolean;
  versions: AgentPublishedVersionListItem[];
  selectedVersionId: string;
  rollbackPreview?: AgentPublishedVersionRollbackPreview;
  onOpenChange: (open: boolean) => void;
  onSelectVersion: (versionId: string) => void;
  onCancelPreview: () => void;
  onConfirmRollback: () => void;
}

function formatVersionTime(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleString(undefined, { hour12: false });
}

export function AgentRuntimeVersionPopover({
  open,
  isLoading,
  isRollingBack,
  isLoadingPreview,
  isPreviewing,
  canOpen = true,
  canRollback = true,
  versions,
  selectedVersionId,
  rollbackPreview,
  onOpenChange,
  onSelectVersion,
  onCancelPreview,
  onConfirmRollback,
}: AgentRuntimeVersionPopoverProps) {
  const t = useT('agents.agentRuntime');
  const selectedVersion = versions.find(version => version.id === selectedVersionId);
  const [cleanupConfirmed, setCleanupConfirmed] = useState(false);
  const removedBindings = rollbackPreview?.removed_bindings ?? [];

  useEffect(() => {
    setCleanupConfirmed(false);
  }, [open, selectedVersionId, rollbackPreview?.impact_token]);

  return (
    <Popover open={open && canOpen} onOpenChange={nextOpen => canOpen && onOpenChange(nextOpen)}>
      <Tooltip>
        <PopoverTrigger asChild>
          <TooltipTrigger asChild>
            <Button
              isIcon
              variant="ghost"
              size="sm"
              interactive="subtle"
              aria-label={t('header.versions')}
              disabled={!canOpen}
            >
              <History className="size-[18px]" />
            </Button>
          </TooltipTrigger>
        </PopoverTrigger>
        <TooltipContent>{t('header.versions')}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" sideOffset={10} className="w-[360px] p-0">
        <PopoverHeader className="border-b px-4 py-3">
          <PopoverTitle>{t('publishedVersions.title')}</PopoverTitle>
          <PopoverDescription>{t('publishedVersions.popoverDescription')}</PopoverDescription>
        </PopoverHeader>
        <div className="max-h-[min(420px,calc(100vh-180px))] overflow-y-auto">
          <div className="space-y-3 p-4">
            {isLoading ? (
              <>
                <Skeleton className="h-20 w-full" />
                <Skeleton className="h-20 w-full" />
              </>
            ) : versions.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('publishedVersions.empty')}
              </div>
            ) : (
              versions.map((version, index) => {
                const selected = selectedVersionId === version.id;
                const publishedAt = formatVersionTime(version.created_at);
                const displayName =
                  version.name?.trim() || t('publishedVersions.fallbackName', { time: publishedAt });
                return (
                  <button
                    key={version.id}
                    type="button"
                    className={cn(
                      'relative w-full rounded-lg border bg-background p-3 text-left transition-colors hover:border-primary/50',
                      selected ? 'border-primary bg-primary/5' : ''
                    )}
                    onClick={() => onSelectVersion(version.id)}
                  >
                    <div className="absolute -left-4 top-4 h-2 w-2 rounded-full bg-border" />
                    {index < versions.length - 1 ? (
                      <div className="absolute -left-[13px] top-6 h-[calc(100%+12px)] w-px bg-border" />
                    ) : null}
                    <div className="flex items-center justify-between gap-2">
                      <div className="truncate text-sm font-medium" title={displayName}>
                        {displayName}
                      </div>
                      <div className="flex shrink-0 items-center gap-1">
                        {version.is_current ? (
                          <Badge variant="subtle" className="gap-1">
                            <CheckCircle2 className="size-3" />
                            {t('publishedVersions.current')}
                          </Badge>
                        ) : null}
                        {selected ? (
                          <Badge variant="outline">{t('publishedVersions.previewing')}</Badge>
                        ) : null}
                      </div>
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {t('publishedVersions.publishedAt', { time: publishedAt })}
                    </div>
                  </button>
                );
              })
            )}
          </div>
          {selectedVersionId ? (
            <div className="border-t px-4 py-3">
              {selectedVersion ? (
                <div>
                  <div className="text-xs font-medium text-foreground">
                    {t('publishedVersions.descriptionTitle')}
                  </div>
                  <p className="mt-1 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">
                    {selectedVersion.description?.trim() ||
                      t('publishedVersions.noDescription')}
                  </p>
                </div>
              ) : null}
              <div className="mt-3 border-t pt-3">
                {isLoadingPreview ? (
                  <div className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" />
                    {t('publishedVersions.loadingImpact')}
                  </div>
                ) : rollbackPreview ? (
                  <div className="space-y-2">
                    <div className="text-sm font-medium">{t('publishedVersions.impactTitle')}</div>
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('publishedVersions.impactSummary', { count: removedBindings.length })}
                    </p>
                    {removedBindings.length > 0 ? (
                      <>
                        <div className="max-h-28 space-y-1 overflow-y-auto rounded-md border p-2">
                          {removedBindings.map((item, index) => (
                            <div
                              key={`${item.binding_type}:${item.parent_resource_id ?? ''}:${item.resource_id}:${index}`}
                              className="truncate text-xs text-muted-foreground"
                            >
                              {item.display_name || item.resource_id}
                            </div>
                          ))}
                        </div>
                        <label className="flex cursor-pointer items-start gap-2 text-xs leading-5">
                          <Checkbox
                            checked={cleanupConfirmed}
                            onCheckedChange={checked => setCleanupConfirmed(checked === true)}
                          />
                          <span>{t('publishedVersions.confirmCleanup')}</span>
                        </label>
                      </>
                    ) : (
                      <p className="text-xs text-muted-foreground">
                        {t('publishedVersions.noBindingsRemoved')}
                      </p>
                    )}
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}
        </div>
        <div className="flex items-center justify-between gap-2 border-t p-3">
          <Button
            variant="outline"
            size="sm"
            onClick={onCancelPreview}
            disabled={!isPreviewing || isRollingBack}
          >
            <X className="mr-1.5 size-4" />
            {t('publishedVersions.cancelPreview')}
          </Button>
          <Button
            size="sm"
            onClick={onConfirmRollback}
            disabled={
              !canRollback ||
              !selectedVersion ||
              !isPreviewing ||
              !rollbackPreview?.impact_token ||
              isLoadingPreview ||
              isRollingBack ||
              (removedBindings.length > 0 && !cleanupConfirmed)
            }
          >
            {isRollingBack ? (
              <Loader2 className="mr-1.5 size-4 animate-spin" />
            ) : (
              <RotateCcw className="mr-1.5 size-4" />
            )}
            {t('publishedVersions.rollback')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

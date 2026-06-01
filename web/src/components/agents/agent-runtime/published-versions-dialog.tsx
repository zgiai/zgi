'use client';

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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentPublishedVersionListItem } from './types';

interface AgentRuntimeVersionPopoverProps {
  open: boolean;
  isLoading: boolean;
  isRollingBack: boolean;
  isPreviewing: boolean;
  versions: AgentPublishedVersionListItem[];
  selectedVersionId: string;
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
  isPreviewing,
  versions,
  selectedVersionId,
  onOpenChange,
  onSelectVersion,
  onCancelPreview,
  onConfirmRollback,
}: AgentRuntimeVersionPopoverProps) {
  const t = useT('agents.agentRuntime');
  const selectedVersion = versions.find(version => version.id === selectedVersionId);

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <Tooltip>
        <PopoverTrigger asChild>
          <TooltipTrigger asChild>
            <Button
              isIcon
              variant="ghost"
              size="sm"
              interactive="subtle"
              aria-label={t('header.versions')}
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
                      <div className="truncate text-sm font-medium">{version.version}</div>
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
                      {formatVersionTime(version.created_at)}
                    </div>
                    <div className="mt-2 truncate text-[11px] text-muted-foreground/70">
                      {version.version_uuid}
                    </div>
                  </button>
                );
              })
            )}
          </div>
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
            disabled={!selectedVersion || !isPreviewing || isRollingBack}
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

'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import {
  Bot,
  CheckCircle2,
  Copy,
  ExternalLink,
  History,
  Loader2,
  MoreHorizontal,
  Play,
  Save,
  Upload,
  X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentRuntimeAgent, AgentRuntimeSaveState } from './types';
import { pickAgentInitials } from './utils';

interface AgentRuntimeHeaderProps {
  agentId: string;
  agent: AgentRuntimeAgent;
  saveState: AgentRuntimeSaveState;
  saveText: string;
  isDirty: boolean;
  isPublishing: boolean;
  disablePrimaryActions?: boolean;
  webAppUrl: string;
  versionControl?: ReactNode;
  showPreviewAction?: boolean;
  isPreviewOpen?: boolean;
  onSave: () => void;
  onPublish: () => void;
  onCopyWebAppUrl: () => void;
  onTogglePreview?: () => void;
  onOpenPublishedVersions: () => void;
}

export function AgentRuntimeHeader({
  agentId,
  agent,
  saveState,
  saveText,
  isDirty,
  isPublishing,
  disablePrimaryActions = false,
  webAppUrl,
  versionControl,
  showPreviewAction = false,
  isPreviewOpen = false,
  onSave,
  onPublish,
  onCopyWebAppUrl,
  onTogglePreview,
  onOpenPublishedVersions,
}: AgentRuntimeHeaderProps) {
  const t = useT('agents.agentRuntime');
  const saveDotClassName =
    saveState === 'error'
      ? 'bg-destructive'
      : isDirty
        ? 'bg-yellow-400'
        : saveState === 'saved'
          ? 'bg-emerald-500'
          : 'bg-muted-foreground/40';

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b bg-background px-4">
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-sm font-semibold text-primary">
          {agent?.icon_type === 'image' && agent.icon_url ? (
            <img src={agent.icon_url} alt="" className="size-full rounded-lg object-cover" />
          ) : (
            pickAgentInitials(agent?.name)
          )}
        </div>
        <div className="min-w-0">
          <div className="flex min-w-0 items-center gap-2">
            <h1 className="truncate text-sm font-semibold">{agent?.name || t('fallbackName')}</h1>
            <Badge variant="outline" className="hidden h-6 gap-1 rounded-md px-2 text-[11px] sm:inline-flex">
              <Bot className="size-3" />
              {t('fallbackName')}
            </Badge>
          </div>
          <div className="hidden truncate text-xs text-muted-foreground lg:block">
            {agent?.description || t('defaultModeDescription')}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        <div
          className={cn(
            'hidden items-center gap-1.5 text-xs text-muted-foreground xl:flex',
            saveState === 'error' ? 'text-destructive' : ''
          )}
        >
          {saveState === 'saving' ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : saveState === 'saved' ? (
            <CheckCircle2 className="size-3.5 text-success" />
          ) : null}
          {saveText}
        </div>

        <Tooltip>
          <TooltipTrigger asChild>
            <div className="relative">
              <Button
                variant="ghost"
                size="sm"
                isIcon
                interactive="subtle"
                onClick={onSave}
                disabled={disablePrimaryActions || saveState === 'saving'}
                aria-label={t('header.save')}
              >
                {saveState === 'saving' ? (
                  <Loader2 className="size-[18px] animate-spin" />
                ) : (
                  <Save className="size-[18px]" />
                )}
                <span
                  className={cn(
                    'absolute right-1 top-1 size-2 rounded-full border-2 border-background',
                    saveDotClassName
                  )}
                />
              </Button>
            </div>
          </TooltipTrigger>
          <TooltipContent>{t('header.save')}</TooltipContent>
        </Tooltip>

        {versionControl ?? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                isIcon
                variant="ghost"
                size="sm"
                interactive="subtle"
                onClick={onOpenPublishedVersions}
                aria-label={t('header.versions')}
              >
                <History className="size-[18px]" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('header.versions')}</TooltipContent>
          </Tooltip>
        )}

        {showPreviewAction ? (
          <Button
            size="sm"
            aria-pressed={isPreviewOpen}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md border px-2 text-white shadow-none transition-colors focus-visible:ring-emerald-500/30 focus-visible:ring-offset-1 active:border-emerald-700 active:bg-emerald-700 sm:px-3.5 2xl:hidden',
              isPreviewOpen
                ? 'border-emerald-600 bg-emerald-600 ring-2 ring-emerald-400/25 hover:border-emerald-700 hover:bg-emerald-700'
                : 'border-emerald-600/30 bg-emerald-600 hover:border-emerald-700 hover:bg-emerald-700'
            )}
            onClick={onTogglePreview}
            aria-label={isPreviewOpen ? t('header.closeDebug') : t('header.debug')}
          >
            {isPreviewOpen ? <X className="size-4" /> : <Play className="size-4" fill="currentColor" />}
            <span className="hidden font-semibold lg:inline">
              {isPreviewOpen ? t('header.closeDebug') : t('header.debug')}
            </span>
          </Button>
        ) : null}

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="sm" className="gap-1.5 px-3">
              {isPublishing ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
              <span className="hidden font-semibold sm:inline">{t('header.publish')}</span>
              <MoreHorizontal className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <div className="px-2 py-1">
              <Button
                className="w-full"
                onClick={onPublish}
                disabled={disablePrimaryActions || isPublishing || saveState === 'saving'}
              >
                {isPublishing ? <Loader2 className="size-4 animate-spin" /> : <Upload className="size-4" />}
                {t('header.publish')}
              </Button>
            </div>
            <DropdownMenuItem
              disabled={!webAppUrl}
              onSelect={() => webAppUrl && window.open(webAppUrl, '_blank')}
            >
              <ExternalLink className="size-4" />
              {t('header.openWebApp')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={!webAppUrl} onSelect={onCopyWebAppUrl}>
              <Copy className="size-4" />
              {t('header.copyWebAppLink')}
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link href={`/console/agents/${agentId}/logs`}>
                <History className="size-4" />
                {t('header.runtimeLogs')}
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}

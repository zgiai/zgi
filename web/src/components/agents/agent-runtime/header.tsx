'use client';

import Link from 'next/link';
import type { ReactNode } from 'react';
import {
  ArrowLeft,
  Bot,
  CheckCircle2,
  Copy,
  ExternalLink,
  History,
  Loader2,
  MoreHorizontal,
  Save,
  Upload,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
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
  onBack: () => void;
  onSave: () => void;
  onPublish: () => void;
  onCopyWebAppUrl: () => void;
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
  onBack,
  onSave,
  onPublish,
  onCopyWebAppUrl,
  onOpenPublishedVersions,
}: AgentRuntimeHeaderProps) {
  const t = useT('agents.agentRuntime');

  return (
    <header className="flex h-14 shrink-0 items-center justify-between gap-3 border-b bg-background px-4">
      <div className="flex min-w-0 items-center gap-3">
        <Button isIcon variant="ghost" className="size-8" onClick={onBack}>
          <ArrowLeft className="size-4" />
        </Button>
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
            <Badge variant="outline" className="h-6 gap-1 rounded-md px-2 text-[11px]">
              <Bot className="size-3" />
              {t('fallbackName')}
            </Badge>
          </div>
          <div className="truncate text-xs text-muted-foreground">
            {agent?.description || t('defaultModeDescription')}
          </div>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        <div
          className={cn(
            'hidden items-center gap-1.5 text-xs text-muted-foreground md:flex',
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
        <Button
          variant="outline"
          onClick={onSave}
          disabled={disablePrimaryActions || saveState === 'saving' || !isDirty}
        >
          <Save className="mr-2 size-4" />
          {t('header.save')}
        </Button>
        <Button onClick={onPublish} disabled={disablePrimaryActions || isPublishing || saveState === 'saving'}>
          <Upload className="mr-2 size-4" />
          {t('header.publish')}
        </Button>
        {versionControl ?? (
          <Button variant="outline" onClick={onOpenPublishedVersions} className="gap-2">
            <History className="size-4" />
            {t('header.versions')}
          </Button>
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button isIcon variant="ghost" className="size-8" aria-label={t('header.more')}>
              <MoreHorizontal className="size-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
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

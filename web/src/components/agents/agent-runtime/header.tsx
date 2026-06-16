'use client';

import { useState, type ReactNode } from 'react';
import {
  Bot,
  CheckCircle2,
  Cloud,
  CloudOff,
  Copy,
  ExternalLink,
  History,
  Loader2,
  Play,
  Save,
  UploadCloud,
  X,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { useUpdateWebAppStatus } from '@/hooks/agent/use-agents';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { WebAppStatus } from '@/services/types/agent';
import type { AgentRuntimeAgent, AgentRuntimeSaveState } from './types';
import { pickAgentInitials } from './utils';

const WEB_APP_OFFLINE_REASON_MAX_LENGTH = 500;

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
  const webAppStatusMutation = useUpdateWebAppStatus();
  const [webAppStatusDialogOpen, setWebAppStatusDialogOpen] = useState(false);
  const [offlineReason, setOfflineReason] = useState('');
  const saveDotClassName =
    saveState === 'error'
      ? 'bg-destructive'
      : isDirty
        ? 'bg-yellow-400'
        : saveState === 'saved'
          ? 'bg-emerald-500'
          : 'bg-muted-foreground/40';
  const isPublished = Boolean(agent?.is_published);
  const isWebAppOffline = agent?.web_app_status === 'inactive';
  const isWebAppOnline = !isWebAppOffline;
  const publishLabel = isPublished ? t('header.update') : t('header.publish');
  const publishingLabel = isPublished ? t('header.updating') : t('header.publishing');
  const nextWebAppStatus: WebAppStatus = isWebAppOffline ? 'active' : 'inactive';
  const webAppStatusActionLabel = isWebAppOffline
    ? t('header.bringOnline')
    : t('header.takeOffline');
  const offlineReasonLength = Array.from(offlineReason).length;
  const isOfflineReasonTooLong = offlineReasonLength > WEB_APP_OFFLINE_REASON_MAX_LENGTH;

  const handleOpenWebApp = () => {
    if (!webAppUrl || isWebAppOffline) return;

    const opened = window.open('', '_blank');
    if (opened) {
      opened.opener = null;
      opened.location.href = webAppUrl;
    } else {
      window.location.href = webAppUrl;
    }
  };

  const handleWebAppStatusConfirm = () => {
    if (disablePrimaryActions) {
      return;
    }
    if (nextWebAppStatus === 'inactive' && isOfflineReasonTooLong) {
      return;
    }

    webAppStatusMutation.mutate(
      {
        agentId,
        data: {
          status: nextWebAppStatus,
          reason:
            nextWebAppStatus === 'inactive' && offlineReason.trim()
              ? offlineReason.trim()
              : undefined,
        },
      },
      {
        onSuccess: () => {
          setWebAppStatusDialogOpen(false);
          setOfflineReason('');
        },
      }
    );
  };

  return (
    <>
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
              <Badge
                variant="outline"
                className="hidden h-6 gap-1 rounded-md px-2 text-[11px] sm:inline-flex"
              >
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
                'inline-flex items-center gap-1.5 rounded-md border px-2 text-white shadow-none transition-colors focus-visible:ring-emerald-500/30 focus-visible:ring-offset-1 active:border-emerald-700 active:bg-emerald-700 sm:px-3.5',
                isPreviewOpen
                  ? 'border-emerald-600 bg-emerald-600 ring-2 ring-emerald-400/25 hover:border-emerald-700 hover:bg-emerald-700'
                  : 'border-emerald-600/30 bg-emerald-600 hover:border-emerald-700 hover:bg-emerald-700'
              )}
              onClick={onTogglePreview}
              aria-label={isPreviewOpen ? t('header.closeDebug') : t('header.debug')}
            >
              {isPreviewOpen ? (
                <X className="size-4" />
              ) : (
                <Play className="size-4" fill="currentColor" />
              )}
              <span className="hidden font-semibold lg:inline">
                {isPreviewOpen ? t('header.closeDebug') : t('header.debug')}
              </span>
            </Button>
          ) : null}

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="sm"
                className="flex items-center gap-1.5 rounded-md border border-primary/25 bg-primary/10 px-3.5 text-primary shadow-none transition-colors hover:border-primary/35 hover:bg-primary/15"
                aria-label={isPublishing ? publishingLabel : publishLabel}
                disabled={disablePrimaryActions || isPublishing || saveState === 'saving'}
              >
                {isPublishing ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <UploadCloud className="size-4" />
                )}
                <span className="hidden font-semibold sm:inline">{publishLabel}</span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <div className="px-2 py-1">
                <Button
                  className="w-full rounded-md border border-primary/25 bg-primary/10 text-primary shadow-none hover:border-primary/35 hover:bg-primary/15"
                  onClick={onPublish}
                  disabled={disablePrimaryActions || isPublishing || saveState === 'saving'}
                >
                  {isPublishing ? (
                    <Loader2 className="size-5 animate-spin" />
                  ) : (
                    <UploadCloud className="size-5" />
                  )}
                  {isPublishing ? publishingLabel : publishLabel}
                </Button>
              </div>
              {isPublished ? (
                <>
                  <div className="my-1 h-px w-full bg-border" />
                  <div className="flex items-center justify-between gap-3 px-2 py-1.5 text-xs text-muted-foreground">
                    <span>{t('header.webAppStatus')}</span>
                    <Badge
                      variant="outline"
                      className={
                        isWebAppOnline
                          ? 'border-primary/40 bg-primary/10 text-primary'
                          : 'border-destructive/40 bg-destructive/10 text-destructive'
                      }
                    >
                      {isWebAppOnline ? t('header.online') : t('header.offline')}
                    </Badge>
                  </div>
                </>
              ) : null}
              {isPublished ? (
                <DropdownMenuItem
                  disabled={disablePrimaryActions}
                  onSelect={() => {
                    setWebAppStatusDialogOpen(true);
                  }}
                >
                  {isWebAppOffline ? <Cloud className="size-4" /> : <CloudOff className="size-4" />}
                  {webAppStatusActionLabel}
                </DropdownMenuItem>
              ) : null}
              <DropdownMenuItem
                disabled={!webAppUrl || isWebAppOffline}
                onSelect={event => {
                  event.preventDefault();
                  handleOpenWebApp();
                }}
              >
                <ExternalLink className="size-4" />
                {t('header.openWebApp')}
              </DropdownMenuItem>
              <DropdownMenuItem disabled={!webAppUrl} onSelect={onCopyWebAppUrl}>
                <Copy className="size-4" />
                {t('header.copyWebAppLink')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      <Dialog open={webAppStatusDialogOpen} onOpenChange={setWebAppStatusDialogOpen}>
        <DialogContent size="md" className="p-0 text-left">
          <DialogHeader>
            <DialogTitle>
              {isWebAppOffline ? t('header.onlineTitle') : t('header.offlineTitle')}
            </DialogTitle>
            <DialogDescription>
              {isWebAppOffline ? t('header.onlineDescription') : t('header.offlineDescription')}
            </DialogDescription>
          </DialogHeader>

          {!isWebAppOffline ? (
            <DialogBody className="space-y-2">
              <Label htmlFor="agent-webapp-offline-reason">{t('header.reasonLabel')}</Label>
              <Textarea
                id="agent-webapp-offline-reason"
                value={offlineReason}
                placeholder={t('header.reasonPlaceholder')}
                onChange={event => setOfflineReason(event.target.value)}
                aria-invalid={isOfflineReasonTooLong}
                className="min-h-28"
              />
              <div
                className={cn(
                  'text-xs',
                  isOfflineReasonTooLong ? 'text-destructive' : 'text-muted-foreground'
                )}
              >
                {t('header.reasonCount', {
                  count: offlineReasonLength,
                  max: WEB_APP_OFFLINE_REASON_MAX_LENGTH,
                })}
              </div>
              {isOfflineReasonTooLong ? (
                <p className="text-xs text-destructive">{t('header.reasonTooLong')}</p>
              ) : null}
            </DialogBody>
          ) : null}

          <DialogFooter>
            <Button
              variant="ghost"
              onClick={() => {
                setWebAppStatusDialogOpen(false);
              }}
            >
              {t('header.cancel')}
            </Button>
            <Button
              variant={isWebAppOffline ? 'default' : 'destructive'}
              onClick={handleWebAppStatusConfirm}
              disabled={
                disablePrimaryActions || webAppStatusMutation.isPending || isOfflineReasonTooLong
              }
            >
              {webAppStatusMutation.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : isWebAppOffline ? (
                <Cloud className="size-4" />
              ) : (
                <CloudOff className="size-4" />
              )}
              {webAppStatusActionLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

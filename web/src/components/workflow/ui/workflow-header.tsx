'use client';

import React from 'react';
import { useT } from '@/i18n';
import WorkflowToolbar from './workflow-toolbar';
import WorkflowRunsDropdown from './workflow-runs-dropdown';
import {
  AppWindowIcon,
  Cloud,
  CloudOff,
  History,
  Loader2,
  Play,
  SaveIcon,
  Settings2,
  UploadCloud,
  KeySquare,
  MessageCircleCode,
  X,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { AgentType, type WebAppStatus } from '@/services/types/agent';
import Link from 'next/link';
import { useWorkflowStore } from '../store';
import { useActivePanel } from '../hooks/use-active-panel';
import { formatDate } from '@/utils/format';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useLatestWorkflowVersion } from '@/hooks/workflow/use-workflow';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { formatMs } from '@/utils/format';
import useWorkflowValidation from '../hooks/use-workflow-validation';
import { isBannerHidden, hideBanner, BannerKey } from '@/utils/ui-local';
import { useUpdateWebAppStatus } from '@/hooks/agent/use-agents';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { ICON_BG, ICON_TEXT, WORKFLOW_AUTOSAVE_INTERVAL_MS } from '@/lib/config';
import type { IconType } from '@/utils/icon-helpers';

interface WorkflowHeaderProps {
  // Basic info
  agentId: string;
  agentName: string;
  agentIconType?: IconType;
  agentIcon?: string;
  agentIconUrl?: string;
  agentType: string;
  webAppStatus?: WebAppStatus;
  // Dirty/save state
  isDirty: boolean;
  isSaving: boolean;
  // Optional publish state for new publish action
  isPublishing?: boolean;
  // Control whether publish is allowed (e.g., validation passed)
  canPublish?: boolean;
  // Actions
  onSave: () => Promise<void> | void;
  onPublish: ({
    silent,
    saveToast,
  }: {
    silent?: boolean;
    saveToast?: string;
  }) => Promise<void> | void;
  // Read-only states
  isReadOnly: boolean;
  /** True when viewing run history */
  isHistoryMode?: boolean;
  /** True when user lacks edit permission */
  isPermissionReadOnly?: boolean;
  // History state & actions
  selectedRunId: string | null;
  onSelectRunHistory: (runId: string) => void;
  onExitHistory: () => void;
}

/**
 * WorkflowHeader - top header of workflow editor
 * - Shows agent id, toolbar, run history dropdown, run and save buttons
 * - Shows read-only banner when in history mode
 */
const WorkflowHeader: React.FC<WorkflowHeaderProps> = ({
  agentId,
  agentName,
  agentIconType,
  agentIcon,
  agentIconUrl,
  agentType,
  webAppStatus,
  isDirty,
  isSaving,
  isPublishing,
  canPublish,
  onSave,
  onPublish,
  isReadOnly,
  isHistoryMode = false,
  isPermissionReadOnly = false,
  selectedRunId,
  onSelectRunHistory,
  onExitHistory,
}) => {
  const t = useT('agents');
  const debuggerRunsQuery = React.useMemo(() => ({ triggered_from: 'debugging' as const }), []);
  // const tNodesToolbar = useTranslations('nodes.workflow.toolbar');

  const { data: latest } = useLatestWorkflowVersion(agentId);
  const webAppStatusMutation = useUpdateWebAppStatus();
  const lastSavedAt = useWorkflowStore.use.lastSavedAt();
  const activePanel = useActivePanel(state => state.active);
  const { errors } = useWorkflowValidation();
  const [runWarnOpen, setRunWarnOpen] = React.useState(false);
  const [webAppStatusDialogOpen, setWebAppStatusDialogOpen] = React.useState(false);
  const [offlineReason, setOfflineReason] = React.useState('');
  const [dontWarnAgain, setDontWarnAgain] = React.useState(false);
  const setOpenValidationIssues = useWorkflowStore.use.setOpenValidationIssues();
  const openIssues = React.useCallback(
    () => setOpenValidationIssues(true),
    [setOpenValidationIssues]
  );
  const lastSavedLabel = React.useMemo(() => {
    const ts = typeof lastSavedAt === 'number' ? lastSavedAt : null;
    const timeStr = ts ? formatDate(ts, 'HH:mm:ss') : '--';
    return t('workflow.lastAutoSaved', { time: timeStr });
  }, [lastSavedAt, t]);
  const agentIconData = React.useMemo(() => {
    let textIcon = agentName?.slice(0, 2).toUpperCase() || ICON_TEXT;
    let iconBackground = ICON_BG;

    if (agentIconType === 'text' && agentIcon) {
      try {
        const parsed = JSON.parse(agentIcon);
        textIcon = parsed?.icon || textIcon;
        iconBackground = parsed?.icon_background || iconBackground;
      } catch {
        textIcon = agentIcon || textIcon;
      }
    }

    return { textIcon, iconBackground };
  }, [agentIcon, agentIconType, agentName]);
  const autoSaveLabel = t('workflow.autoSaveTips', {
    interval: formatMs(WORKFLOW_AUTOSAVE_INTERVAL_MS),
  });

  const hasPubilshed = latest?.data?.workflow_id;
  const isWebAppOffline = webAppStatus === 'inactive';
  const webAppUrl = latest?.data?.web_app_id
    ? `/webapp/${latest.data.web_app_id}/${agentType === AgentType.CONVERSATIONAL_AGENT ? 'chat' : 'run'}`
    : '';
  const nextWebAppStatus: WebAppStatus = isWebAppOffline ? 'active' : 'inactive';
  const webAppStatusLabel = isWebAppOffline
    ? t('workflow.webappStatus.offline')
    : t('workflow.webappStatus.online');
  const webAppStatusActionLabel = isWebAppOffline
    ? t('workflow.webappStatus.bringOnline')
    : t('workflow.webappStatus.takeOffline');
  const offlineReasonLength = Array.from(offlineReason).length;
  const isOfflineReasonTooLong = offlineReasonLength > 500;
  const openConversationPanel = () => {
    const setActive = useActivePanel.getState().setActive;
    const win = window as Window & {
      __workflowConversationPanelOpen?: boolean;
      __workflowConversationPanelShake?: () => void;
    };
    if (win.__workflowConversationPanelOpen) {
      toast(t('workflow.panelAlreadyOpen', { name: t('workflow.conversationVariables.title') }));
      win.__workflowConversationPanelShake?.();
    }
    setActive('conversation-variables');
  };

  const openFeaturesPanel = () => {
    const setActive = useActivePanel.getState().setActive;
    const win = window as Window & {
      __workflowFeaturesPanelOpen?: boolean;
      __workflowFeaturesPanelShake?: () => void;
    };
    if (win.__workflowFeaturesPanelOpen) {
      toast(t('workflow.panelAlreadyOpen', { name: t('workflow.features.title') }));
      win.__workflowFeaturesPanelShake?.();
    }
    setActive('features');
  };

  const openEnvironmentPanel = () => {
    const setActive = useActivePanel.getState().setActive;
    const win = window as Window & {
      __workflowEnvironmentPanelOpen?: boolean;
      __workflowEnvironmentPanelShake?: () => void;
    };
    if (win.__workflowEnvironmentPanelOpen) {
      toast(t('workflow.panelAlreadyOpen', { name: t('workflow.environmentVariables.title') }));
      win.__workflowEnvironmentPanelShake?.();
    }
    setActive('environment-variables');
  };

  const handleRunClickProceed = () => {
    const win = window as Window & {
      __workflowRunPanelOpen?: boolean;
      __workflowChatPanelOpen?: boolean;
      __workflowRunPanelShake?: () => void;
      __workflowChatPanelShake?: () => void;
    };
    const setActive = useActivePanel.getState().setActive;
    if (agentType === AgentType.CONVERSATIONAL_AGENT) {
      if (win.__workflowChatPanelOpen) {
        toast(t('workflow.panelAlreadyOpen', { name: t('workflow.chat.title') }));
        win.__workflowChatPanelShake?.();
      }
      setActive('chat');
    } else {
      if (win.__workflowRunPanelOpen) {
        toast(t('workflow.panelAlreadyOpen', { name: t('workflow.runDraft') }));
        win.__workflowRunPanelShake?.();
      }
      setActive('run');
    }
  };

  const isDebugPanelOpen =
    agentType === AgentType.CONVERSATIONAL_AGENT ? activePanel === 'chat' : activePanel === 'run';

  const handleRunClick = async () => {
    if (isDebugPanelOpen) {
      useActivePanel.getState().setActive(null);
      return;
    }

    if (errors.length > 0 && !isBannerHidden(BannerKey.WorkflowRunErrorsWarning)) {
      setRunWarnOpen(true);
      return;
    }
    handleRunClickProceed();
  };

  const handlePublishClick = async () => {
    if (canPublish === false) {
      toast.error(t('workflow.fixErrorsBeforePublishing'));
      return;
    }

    await onPublish({
      silent: false,
      saveToast: hasPubilshed
        ? t('workflow.workflowUpdatedSuccessfully')
        : t('workflow.workflowPublishedSuccessfully'),
    });
  };

  const handleWebAppStatusConfirm = () => {
    if (nextWebAppStatus === 'inactive' && isOfflineReasonTooLong) {
      toast.error(t('workflow.webappStatus.reasonTooLong'));
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
  const openWebAppLandingPage = React.useCallback(() => {
    if (!webAppUrl || isWebAppOffline) return;

    const opened = window.open('', '_blank');
    if (opened) {
      opened.opener = null;
      opened.location.href = webAppUrl;
    } else {
      window.location.href = webAppUrl;
    }
  }, [isWebAppOffline, webAppUrl]);

  return (
    <div className="flex-shrink-0 bg-gradient-to-b from-background to-transparent absolute inset-x-0 top-0 z-10 h-14">
      <div className="flex items-center justify-between h-full px-4">
        <div className="flex min-w-0 max-w-[250px] shrink items-center gap-2.5 xl:max-w-[340px]">
          <IconPreview
            iconType={agentIconType === 'image' ? 'image' : 'text'}
            icon={agentIconData.textIcon}
            iconBackground={agentIconData.iconBackground}
            src={agentIconType === 'image' ? agentIconUrl || '' : ''}
            alt={agentName || t('noName')}
            size="xs"
            editable={false}
            showUpload={false}
            showRemove={false}
            className="shrink-0 rounded-lg"
          />
          <div className="flex min-w-0 flex-col overflow-hidden">
            <div
              className="flex min-w-0 items-center gap-1.5 text-[13px] font-semibold leading-none text-foreground"
              title={agentName || t('noName')}
            >
              <span className="truncate">{agentName || t('noName')}</span>
              {isSaving && <Loader2 size={12} className="shrink-0 animate-spin text-primary" />}
            </div>
            <div
              className="mt-1 truncate text-[11px] leading-none text-muted-foreground"
              title={`${lastSavedLabel} · ${autoSaveLabel}`}
            >
              {lastSavedLabel}
            </div>
          </div>
        </div>

        {isReadOnly ? (
          <div className="flex items-center gap-2">
            {isHistoryMode ? (
              // History mode: show run history info with exit button
              <>
                <div className="text-sm text-amber-600">
                  {selectedRunId
                    ? t('workflow.viewingRunHistoryWithId', { id: selectedRunId })
                    : t('workflow.viewingRunHistory')}
                </div>
                <WorkflowRunsDropdown
                  agentId={agentId}
                  query={debuggerRunsQuery}
                  icon={
                    agentType === AgentType.CONVERSATIONAL_AGENT ? <History size={20} /> : undefined
                  }
                  tooltipLabel={
                    agentType === AgentType.CONVERSATIONAL_AGENT
                      ? t('workflow.conversationHistory.title')
                      : t('workflow.recentRuns')
                  }
                  dropdownLabel={t('workflow.recentRuns')}
                  itemFilter={
                    agentType === AgentType.CONVERSATIONAL_AGENT
                      ? item => Boolean(item.conversation_id)
                      : undefined
                  }
                  onSelect={(runId: string) => {
                    onSelectRunHistory(runId);
                  }}
                />
                <button
                  onClick={onExitHistory}
                  className="px-3 py-1.5 bg-amber-600 text-white rounded hover:bg-amber-700 text-sm"
                >
                  {t('workflow.returnToEdit')}
                </button>
              </>
            ) : isPermissionReadOnly ? (
              // Permission-based read-only: show read-only notice without exit button
              <div className="text-sm text-muted-foreground bg-muted px-3 py-1.5 rounded">
                {t('workflow.readOnlyMode')}
              </div>
            ) : null}
          </div>
        ) : (
          <>
            <WorkflowToolbar />
            <div className="flex items-center gap-2">
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="relative">
                    <Button
                      onClick={() => onSave()}
                      variant="ghost"
                      size="sm"
                      isIcon
                      interactive="subtle"
                      aria-label={t('workflow.save')}
                      disabled={isSaving}
                    >
                      <SaveIcon size={18} />
                      <div
                        className={cn(
                          'w-2 h-2 rounded-full absolute top-0.5 right-0.5 border-2 border-background',
                          isDirty ? 'bg-yellow-400' : 'bg-emerald-500'
                        )}
                      />
                    </Button>
                  </div>
                </TooltipTrigger>
                <TooltipContent>{t('workflow.save')}</TooltipContent>
              </Tooltip>

              {agentType === AgentType.CONVERSATIONAL_AGENT && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      onClick={openFeaturesPanel}
                      variant="ghost"
                      size="sm"
                      isIcon
                      interactive="subtle"
                      aria-label={t('workflow.features.title')}
                    >
                      <Settings2 size={18} />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('workflow.features.title')}</TooltipContent>
                </Tooltip>
              )}
              {agentType !== AgentType.CONVERSATIONAL_AGENT && (
                <WorkflowRunsDropdown
                  agentId={agentId}
                  query={debuggerRunsQuery}
                  onSelect={(runId: string) => {
                    onSelectRunHistory(runId);
                  }}
                />
              )}
              {agentType === AgentType.CONVERSATIONAL_AGENT && (
                <WorkflowRunsDropdown
                  agentId={agentId}
                  query={debuggerRunsQuery}
                  icon={<History size={20} />}
                  tooltipLabel={t('workflow.conversationHistory.title')}
                  dropdownLabel={t('workflow.recentRuns')}
                  itemFilter={item => Boolean(item.conversation_id)}
                  onSelect={(runId: string) => {
                    onSelectRunHistory(runId);
                  }}
                />
              )}
              {agentType === AgentType.CONVERSATIONAL_AGENT && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      onClick={openConversationPanel}
                      variant="ghost"
                      size="sm"
                      isIcon
                      interactive="subtle"
                      aria-label={t('workflow.conversationVariables.title')}
                    >
                      <MessageCircleCode size={18} />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('workflow.conversationVariables.title')}</TooltipContent>
                </Tooltip>
              )}

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    onClick={openEnvironmentPanel}
                    variant="ghost"
                    size="sm"
                    isIcon
                    interactive="subtle"
                    aria-label={t('workflow.environmentVariables.title')}
                  >
                    <KeySquare size={18} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('workflow.environmentVariables.title')}</TooltipContent>
              </Tooltip>

              <Button
                onClick={handleRunClick}
                size="sm"
                aria-pressed={isDebugPanelOpen}
                className={cn(
                  'flex items-center gap-1.5 rounded-md border px-3.5 text-white shadow-none transition-colors focus-visible:ring-emerald-500/30 focus-visible:ring-offset-1 active:border-emerald-700 active:bg-emerald-700',
                  isDebugPanelOpen
                    ? 'border-emerald-600 bg-emerald-600 ring-2 ring-emerald-400/25 hover:border-emerald-700 hover:bg-emerald-700'
                    : 'border-emerald-600/30 bg-emerald-600 hover:border-emerald-700 hover:bg-emerald-700'
                )}
              >
                {isDebugPanelOpen ? <X size={16} /> : <Play size={16} fill="currentColor" />}
                <span className="font-semibold">
                  {isDebugPanelOpen ? t('workflow.closeDebug') : t('workflow.run')}
                </span>
              </Button>
              <Dialog open={runWarnOpen} onOpenChange={setRunWarnOpen}>
                <DialogContent className="max-w-[440px] p-0 overflow-hidden text-left">
                  <DialogHeader className="pb-2">
                    <DialogTitle className="text-xl font-black tracking-tight flex items-center gap-3">
                      <div className="h-8 w-8 bg-amber-100 text-amber-500 flex items-center justify-center rounded-lg">
                        <span className="text-lg font-black">!</span>
                      </div>
                      {t('workflow.runErrorsDialog.title')}
                    </DialogTitle>
                  </DialogHeader>

                  <DialogBody className="py-6 space-y-6">
                    <div className="bg-amber-50/50 p-4 rounded-2xl border border-amber-100 text-sm font-medium leading-relaxed text-neutral-600">
                      {t('workflow.runErrorsDialog.description')}
                    </div>

                    <div
                      className="flex items-center gap-3 px-1 group cursor-pointer"
                      onClick={() => setDontWarnAgain(!dontWarnAgain)}
                    >
                      <Checkbox
                        id="wf-run-warn-hide"
                        checked={dontWarnAgain}
                        onCheckedChange={v => setDontWarnAgain(Boolean(v))}
                        className="w-5 h-5"
                      />
                      <Label
                        htmlFor="wf-run-warn-hide"
                        className="text-sm font-bold text-neutral-500 group-hover:text-primary transition-colors cursor-pointer"
                      >
                        {t('workflow.runErrorsDialog.dontShowAgain')}
                      </Label>
                    </div>
                  </DialogBody>

                  <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
                    <Button
                      variant="ghost"
                      className="font-semibold"
                      onClick={() => {
                        setRunWarnOpen(false);
                        openIssues();
                      }}
                    >
                      {t('workflow.runErrorsDialog.viewErrors')}
                    </Button>
                    <Button
                      size="lg"
                      className="px-10 font-bold shadow-sm"
                      onClick={() => {
                        if (dontWarnAgain) hideBanner(BannerKey.WorkflowRunErrorsWarning);
                        setRunWarnOpen(false);
                        handleRunClickProceed();
                      }}
                    >
                      {t('workflow.runErrorsDialog.continueRun')}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="sm"
                    className="flex items-center gap-1.5 rounded-md border border-primary/25 bg-primary/10 px-3.5 text-primary shadow-none transition-colors hover:border-primary/35 hover:bg-primary/15"
                  >
                    <UploadCloud size={16} />
                    <span className="font-semibold">{t('workflow.publish')}</span>
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-56">
                  <div className="w-full">
                    <div className="px-2 py-1">
                      <Button
                        onClick={handlePublishClick}
                        disabled={Boolean(isPublishing) || isSaving}
                        title={canPublish === false ? t('workflow.fixErrorsBeforePublishing') : ''}
                        className="w-full rounded-md border border-primary/25 bg-primary/10 text-primary shadow-none hover:border-primary/35 hover:bg-primary/15"
                      >
                        {isPublishing ? (
                          <Loader2 size={20} className="animate-spin" />
                        ) : (
                          <UploadCloud size={20} />
                        )}
                        {hasPubilshed
                          ? isPublishing
                            ? t('workflow.updating')
                            : t('workflow.update')
                          : isPublishing
                            ? t('workflow.publishing')
                            : t('workflow.publish')}
                      </Button>
                    </div>
                  </div>

                  {hasPubilshed && (
                    <>
                      <div className="w-full h-px bg-border my-1" />
                      <div className="flex items-center justify-between gap-3 px-2 py-1.5 text-xs text-muted-foreground">
                        <span>{t('workflow.webappStatus.label')}</span>
                        <Badge
                          variant="outline"
                          className={
                            isWebAppOffline
                              ? 'bg-destructive/10 text-destructive border-destructive/40'
                              : 'bg-primary/10 text-primary border-primary/40'
                          }
                        >
                          {webAppStatusLabel}
                        </Badge>
                      </div>
                      <DropdownMenuItem
                        onSelect={() => {
                          setWebAppStatusDialogOpen(true);
                        }}
                      >
                        {isWebAppOffline ? (
                          <Cloud className="text-highlight" />
                        ) : (
                          <CloudOff className="text-highlight" />
                        )}
                        {webAppStatusActionLabel}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={isWebAppOffline || !webAppUrl}
                        onSelect={event => {
                          event.preventDefault();
                          openWebAppLandingPage();
                        }}
                      >
                        <AppWindowIcon className="text-highlight" />
                        {t('workflow.webapp')}
                      </DropdownMenuItem>
                      <DropdownMenuItem asChild>
                        <Link href={`/console/agents/${agentId}/logs`}>
                          <History className="text-highlight" />
                          {t('workflow.webappLogs')}
                        </Link>
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
              <Dialog open={webAppStatusDialogOpen} onOpenChange={setWebAppStatusDialogOpen}>
                <DialogContent size="md" className="p-0 overflow-hidden text-left">
                  <DialogHeader>
                    <DialogTitle>
                      {isWebAppOffline
                        ? t('workflow.webappStatus.onlineTitle')
                        : t('workflow.webappStatus.offlineTitle')}
                    </DialogTitle>
                    <DialogDescription>
                      {isWebAppOffline
                        ? t('workflow.webappStatus.onlineDescription')
                        : t('workflow.webappStatus.offlineDescription')}
                    </DialogDescription>
                  </DialogHeader>

                  {!isWebAppOffline ? (
                    <DialogBody className="space-y-2">
                      <Label htmlFor="webapp-offline-reason">
                        {t('workflow.webappStatus.reasonLabel')}
                      </Label>
                      <Textarea
                        id="webapp-offline-reason"
                        value={offlineReason}
                        placeholder={t('workflow.webappStatus.reasonPlaceholder')}
                        onChange={event => setOfflineReason(event.target.value)}
                        aria-invalid={isOfflineReasonTooLong}
                        className="min-h-28"
                      />
                      <div
                        className={`text-xs ${
                          isOfflineReasonTooLong ? 'text-destructive' : 'text-muted-foreground'
                        }`}
                      >
                        {t('workflow.webappStatus.reasonCount', {
                          count: offlineReasonLength,
                          max: 500,
                        })}
                      </div>
                    </DialogBody>
                  ) : null}

                  <DialogFooter>
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setWebAppStatusDialogOpen(false);
                      }}
                    >
                      {t('workflow.webappStatus.cancel')}
                    </Button>
                    <Button
                      variant={isWebAppOffline ? 'default' : 'destructive'}
                      onClick={handleWebAppStatusConfirm}
                      disabled={webAppStatusMutation.isPending || isOfflineReasonTooLong}
                    >
                      {webAppStatusMutation.isPending ? (
                        <Loader2 size={16} className="animate-spin" />
                      ) : isWebAppOffline ? (
                        <Cloud size={16} />
                      ) : (
                        <CloudOff size={16} />
                      )}
                      {webAppStatusActionLabel}
                    </Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default WorkflowHeader;

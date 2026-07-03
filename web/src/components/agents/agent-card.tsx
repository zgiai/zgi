'use client';

import React, { memo } from 'react';
import Link from 'next/link';
import { Card, CardContent } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

import {
  Workflow,
  MoreHorizontal,
  Trash2,
  Edit,
  Download,
  MessageSquareText,
  MoveRight,
  Bot,
} from 'lucide-react';
import { useT } from '@/i18n';
import { useDeleteAgent } from '@/hooks/agent/use-agents';
import { AgentType, type Agent } from '@/services/types/agent';
import { useState } from 'react';
import { IconPreview } from '../common/icon-input/icon-preview';
import AgentDialog from './agent-dialog';
import { Badge } from '../ui/badge';
import { useQueryClient } from '@tanstack/react-query';
import { agentService } from '@/services';
import { useExportWorkflow } from '@/hooks/workflow/use-workflow-import-export';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { getAgentDetailEditHref } from '@/utils/agent-detail-routes';

interface AgentCardProps {
  agent: Agent;
  /** The page index where this agent resides in the paged list */
  pageIndex: number;
  /** Callback before navigating from the list into the agent detail page. */
  onNavigate?: () => void;
  /** Callback when agent is deleted; provides id and page index for incremental refetch */
  onDeleted?: (id: string, pageIndex: number) => void;
}

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

function AgentCard({ agent, onDeleted, onNavigate, pageIndex }: AgentCardProps) {
  const t = useT('agents');
  const tCommon = useT('common');
  const deleteMutation = useDeleteAgent();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [exportConfirmOpen, setExportConfirmOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const queryClient = useQueryClient();
  const { exportWorkflow, isExporting } = useExportWorkflow();
  const { currentOrganization } = useOrganizations();

  // Permissions
  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('agent.manage');
  const canMoveAssets = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');
  const agentHref = getAgentDetailEditHref(agent.id, agent.agent_type);
  const modeText =
    agent.agent_type === AgentType.AGENT
      ? t('modes.agent')
      : agent.agent_type === AgentType.WORKFLOW
        ? t('modes.workflow')
        : t('modes.conversational');
  const ModeIcon =
    agent.agent_type === AgentType.AGENT
      ? Bot
      : agent.agent_type === AgentType.WORKFLOW
        ? Workflow
        : MessageSquareText;
  const isWebAppOffline = agent.is_published && agent.web_app_status === 'inactive';
  const isPublishedOnline = agent.is_published && agent.web_app_status === 'active';
  const statusText = isWebAppOffline
    ? t('status.webappOffline')
    : isPublishedOnline
      ? t('status.published')
      : t('status.draft');
  const statusClassName = isWebAppOffline
    ? 'bg-destructive/10 text-destructive font-normal border-destructive/40'
    : isPublishedOnline
      ? 'bg-success/10 text-success font-normal border-success'
      : 'bg-muted text-muted-foreground font-normal';

  return (
    <div className="relative h-48">
      <Link href={agentHref} className="block h-full" onClick={onNavigate}>
        <Card className="flex h-full shrink-0 flex-col border border-border/80 shadow-sm transition-colors hover:border-border hover:bg-muted/20 hover:shadow-sm">
          <CardContent className="flex h-full min-w-0 flex-1 flex-col p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex h-10 w-10 shrink-0 items-center justify-center">
                <IconPreview
                  icon={
                    (() => {
                      try {
                        const iconData = JSON.parse(agent.icon || '{}');
                        return iconData?.icon || '';
                      } catch (_e) {
                        return agent.name.slice(0, 2).toUpperCase() || ICON_TEXT;
                      }
                    })() ||
                    agent.name.slice(0, 2).toUpperCase() ||
                    ICON_TEXT
                  }
                  iconType={agent.icon_type}
                  src={agent.icon_type === 'image' ? agent.icon_url || '' : ''}
                  iconBackground={
                    agent.icon_type === 'image'
                      ? ''
                      : (() => {
                          try {
                            const iconData = JSON.parse(agent.icon || '{}');
                            return iconData?.icon_background || ICON_BG;
                          } catch (_e) {
                            return '';
                          }
                        })()
                  }
                  editable={false}
                  size="sm"
                />
              </div>
              <div className="shrink-0">
                <Badge variant="outline" className={`text-[11px] ${statusClassName}`}>
                  {statusText}
                </Badge>
              </div>
            </div>
            <h3
              className="mt-3 line-clamp-2 min-h-10 break-all text-sm font-semibold leading-5 text-foreground"
              title={agent.name}
            >
              {agent.name || t('noName')}
            </h3>
            <p className="mt-3 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
              {agent.description || t('noDescription')}
            </p>
            <div className="mt-auto flex items-center gap-2 pr-8 text-xs text-muted-foreground">
              <ModeIcon className="h-4 w-4 shrink-0" />
              <span className="truncate">{modeText}</span>
            </div>
          </CardContent>
        </Card>
      </Link>
      {/* Show actions available to workspace managers or organization admins. */}
      {(canManage || canMoveAssets) && (
        <div className="absolute bottom-2 right-2">
          <DropdownMenu
            onOpenChange={open => {
              if (open && canManage) {
                // Prefetch agent detail when actions menu opens
                queryClient.prefetchQuery({
                  queryKey: ['agents', 'detail', agent.id],
                  queryFn: () => agentService.getAgent(agent.id),
                  staleTime: 5 * 60 * 1000,
                  gcTime: 10 * 60 * 1000,
                });
              }
            }}
          >
            <DropdownMenuTrigger asChild>
              <button className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground">
                <MoreHorizontal className="h-4 w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canManage && (
                <>
                  <DropdownMenuItem inset onSelect={() => setEditOpen(true)}>
                    <Edit className="h-4 w-4" />
                    {t('actions.edit')}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    inset
                    disabled={isExporting}
                    onSelect={() => setExportConfirmOpen(true)}
                  >
                    <Download className="h-4 w-4" />
                    {t('actions.exportYaml')}
                  </DropdownMenuItem>
                </>
              )}
              {canMoveAssets && (
                <DropdownMenuItem inset onSelect={() => setMoveOpen(true)}>
                  <MoveRight className="h-4 w-4" />
                  {tCommon('assetMove.title')}
                </DropdownMenuItem>
              )}
              {canManage && (
                <DropdownMenuItem variant="destructive" inset onSelect={() => setConfirmOpen(true)}>
                  <Trash2 className="h-4 w-4" />
                  {t('actions.delete')}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
      {/* Unified create/edit dialog in edit mode */}
      <AgentDialog open={editOpen} mode="edit" agentId={agent.id} onOpenChange={setEditOpen} />
      <WorkspaceAssetMoveDialog
        open={moveOpen}
        onOpenChange={setMoveOpen}
        assetType="agent"
        assetId={agent.id}
        assetName={agent.name}
      />
      <ConfirmDialog
        open={exportConfirmOpen}
        onOpenChange={setExportConfirmOpen}
        title={t('exportConfirmTitle', { name: agent.name })}
        description={t('exportConfirmDescription')}
        confirmText={t('actions.exportYaml')}
        cancelText={tCommon('close')}
        onConfirm={() => {
          void exportWorkflow({
            agentId: agent.id,
            version: agent.is_published ? 'published' : 'draft',
          });
          setExportConfirmOpen(false);
        }}
        loading={isExporting}
      />
      {/* Delete confirmation dialog outside dropdown */}
      <ConfirmDialog
        variant="danger"
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('deleteConfirmTitle', { name: agent.name })}
        description={t('deleteConfirmDescription')}
        confirmText={tCommon('confirm')}
        cancelText={tCommon('close')}
        onConfirm={() =>
          deleteMutation.mutate(agent.id, {
            onSuccess: () => {
              setConfirmOpen(false);
              onDeleted?.(agent.id, pageIndex);
            },
          })
        }
        loading={deleteMutation.isPending}
      />
    </div>
  );
}

AgentCard.displayName = 'AgentCard';

export default memo(AgentCard);

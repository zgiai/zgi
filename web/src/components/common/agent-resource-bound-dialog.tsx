'use client';

import Link from 'next/link';
import { AlertTriangle, ExternalLink } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { AgentResourceBoundImpact, AgentResourceImpactAgent } from '@/services/types/common';

function getAgentIcon(agent: AgentResourceImpactAgent): {
  icon: string;
  iconBackground: string;
  iconType: 'text' | 'image';
  src?: string;
} {
  const fallback = agent.name?.slice(0, 2).toUpperCase() || ICON_TEXT;
  if (agent.icon_type === 'text' && agent.icon) {
    try {
      const parsed = JSON.parse(agent.icon) as { icon?: string; icon_background?: string };
      return {
        icon: parsed.icon || fallback,
        iconBackground: parsed.icon_background || ICON_BG,
        iconType: 'text' as const,
      };
    } catch {
      return { icon: agent.icon || fallback, iconBackground: ICON_BG, iconType: 'text' as const };
    }
  }

  const imageSrc =
    agent.icon_type === 'image' &&
    agent.icon &&
    (/^(?:https?:|data:|\/)/.test(agent.icon) || agent.icon.startsWith('blob:'))
      ? agent.icon
      : undefined;
  return imageSrc
    ? { icon: fallback, iconBackground: ICON_BG, iconType: 'image' as const, src: imageSrc }
    : { icon: fallback, iconBackground: ICON_BG, iconType: 'text' as const };
}

export interface AgentResourceBoundDialogProps {
  open: boolean;
  impact?: AgentResourceBoundImpact | null;
  agents?: AgentResourceImpactAgent[];
  loading?: boolean;
  actionLabel?: string;
  actionVariant?: 'default' | 'destructive';
  description?: string;
  warningTitle?: string;
  warningDescription?: string;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

export function AgentResourceBoundDialog({
  open,
  impact,
  agents: suppliedAgents,
  loading = false,
  actionLabel,
  actionVariant = 'destructive',
  description,
  warningTitle,
  warningDescription,
  onOpenChange,
  onConfirm,
}: AgentResourceBoundDialogProps) {
  const t = useT('common');
  const agents = impact?.agents ?? suppliedAgents ?? [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('agentResourceBound.title')}</DialogTitle>
          <DialogDescription>
            {description || t('agentResourceBound.description', { count: agents.length })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[460px] space-y-3">
          <Alert className="border-amber-300/70 bg-amber-50/70 text-amber-950 dark:bg-amber-950/20 dark:text-amber-100">
            <AlertTriangle className="size-4" />
            <AlertTitle>{warningTitle || t('agentResourceBound.warningTitle')}</AlertTitle>
            <AlertDescription>
              {warningDescription || t('agentResourceBound.warningDescription')}
            </AlertDescription>
          </Alert>
          <div className="space-y-2">
            {agents.map(agent => {
              const icon = getAgentIcon(agent);
              return (
                <div
                  key={agent.agent_id}
                  className="flex items-center gap-3 rounded-md border bg-background p-3"
                >
                  <IconPreview
                    icon={icon.icon}
                    iconType={icon.iconType}
                    iconBackground={icon.iconBackground}
                    src={icon.src}
                    alt={agent.name || t('agentResourceBound.unavailableAgent')}
                    editable={false}
                    size="sidebarExpanded"
                  />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">
                      {agent.name || t('agentResourceBound.unavailableAgent')}
                    </div>
                    <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                      {agent.description || t('agentResourceBound.noDescription')}
                    </div>
                  </div>
                  <Button asChild type="button" variant="ghost" size="sm" className="shrink-0">
                    <Link
                      href={`/console/agents/${agent.agent_id}`}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {t('agentResourceBound.viewDetails')}
                      <ExternalLink className="size-3.5" />
                    </Link>
                  </Button>
                </div>
              );
            })}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button type="button" variant={actionVariant} loading={loading} onClick={onConfirm}>
            {actionLabel || t('agentResourceBound.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

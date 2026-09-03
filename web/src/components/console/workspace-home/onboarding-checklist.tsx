'use client';

import Link from 'next/link';
import { ArrowRight, Check, Circle, CircleAlert, LoaderCircle, LockKeyhole } from 'lucide-react';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import type { WorkspaceHomeAccessState } from '@/hooks/console/use-workspace-home';
import { getZGIConsoleNavigationTarget } from '@/routes/console-navigation';

interface ChecklistState {
  workspace: boolean;
  model: boolean;
  chat: boolean;
  agent: boolean;
  knowledge: boolean;
  completed: number;
  total: number;
}

interface ChecklistItem {
  key: keyof Pick<ChecklistState, 'workspace' | 'model' | 'chat' | 'agent' | 'knowledge'>;
  title: string;
  description: string;
  href?: string;
  action?: string;
  accessState: WorkspaceHomeAccessState;
}

export function OnboardingChecklist({
  checklist,
  canManageModelConfig,
  access,
  isLoading,
  hasError,
}: {
  checklist: ChecklistState;
  canManageModelConfig: boolean;
  access: {
    workspace: WorkspaceHomeAccessState;
    modelConfig: WorkspaceHomeAccessState;
    chat: WorkspaceHomeAccessState;
    agentCreate: WorkspaceHomeAccessState;
    knowledgeCreate: WorkspaceHomeAccessState;
  };
  isLoading: boolean;
  hasError: boolean;
}) {
  const t = useT('dashboard');
  const tNavigation = useT('navigation');
  const progress = Math.round((checklist.completed / checklist.total) * 100);
  const items: ChecklistItem[] = [
    {
      key: 'workspace',
      title: t('stats.consoleHome.onboarding.workspaceTitle'),
      description: t('stats.consoleHome.onboarding.workspaceDescription'),
      href: '/console/workspace',
      action: t('stats.consoleHome.actions.manageWorkspace'),
      accessState: access.workspace,
    },
    {
      key: 'model',
      title: t('stats.consoleHome.onboarding.modelTitle'),
      description:
        isLoading || access.modelConfig === 'loading'
          ? t('stats.consoleHome.checking')
          : canManageModelConfig
            ? t('stats.consoleHome.onboarding.modelDescription')
            : t('stats.consoleHome.modelConfigManagedByAdmin'),
      href: canManageModelConfig ? '/dashboard/provider' : undefined,
      action: canManageModelConfig ? t('stats.consoleHome.actions.configureModels') : undefined,
      accessState: access.modelConfig,
    },
    {
      key: 'chat',
      title: t('stats.consoleHome.onboarding.chatTitle'),
      description: t('stats.consoleHome.onboarding.chatDescription'),
      href: '/console/work/chat',
      action: t('stats.consoleHome.actions.openChat'),
      accessState: access.chat,
    },
    {
      key: 'agent',
      title: t('stats.consoleHome.onboarding.agentTitle'),
      description: t('stats.consoleHome.onboarding.agentDescription'),
      href: '/console/agents',
      action: t('stats.consoleHome.actions.createAgent'),
      accessState: access.agentCreate,
    },
    {
      key: 'knowledge',
      title: t('stats.consoleHome.onboarding.knowledgeTitle'),
      description: t('stats.consoleHome.onboarding.knowledgeDescription'),
      href: '/console/dataset',
      action: t('stats.consoleHome.actions.createKnowledge'),
      accessState: access.knowledgeCreate,
    },
  ];

  return (
    <Card className="border-border/80 shadow-sm">
      <CardHeader className="pb-4">
        <div className="flex items-start justify-between gap-4">
          <div>
            <CardTitle className="text-lg">{t('stats.consoleHome.onboarding.title')}</CardTitle>
            <CardDescription className="mt-1.5 leading-6">
              {t('stats.consoleHome.onboarding.description')}
            </CardDescription>
          </div>
          <Badge
            variant={
              !isLoading && !hasError && checklist.completed === checklist.total
                ? 'success'
                : 'subtle'
            }
          >
            {isLoading
              ? t('stats.consoleHome.checking')
              : hasError
                ? t('stats.consoleHome.unavailable')
                : t('stats.consoleHome.onboarding.progress', {
                    completed: checklist.completed,
                    total: checklist.total,
                  })}
          </Badge>
        </div>
        <div
          className="mt-4 h-1.5 overflow-hidden rounded-full bg-muted"
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={isLoading || hasError ? undefined : progress}
          aria-valuetext={
            isLoading
              ? t('stats.consoleHome.checking')
              : hasError
                ? t('stats.consoleHome.unavailable')
                : undefined
          }
        >
          <div
            className={cn(
              'h-full rounded-full bg-primary transition-[width] duration-300',
              isLoading && 'w-1/3 animate-pulse'
            )}
            style={isLoading || hasError ? undefined : { width: `${progress}%` }}
          />
        </div>
      </CardHeader>
      {hasError && !isLoading ? (
        <CardContent>
          <div className="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 px-3 py-3 text-xs leading-5 text-warning">
            <CircleAlert className="mt-0.5 size-3.5 shrink-0" />
            <span>{t('stats.consoleHome.onboarding.loadError')}</span>
          </div>
        </CardContent>
      ) : (
        <CardContent className="space-y-1">
          {items.map(item => {
            const isComplete = !isLoading && checklist[item.key];
            const isAccessLoading = isLoading || item.accessState === 'loading';
            const hasAccessError = !isLoading && item.accessState === 'error';
            const isBlocked = item.accessState === 'forbidden' || isAccessLoading || hasAccessError;
            const navigationTarget = item.href
              ? getZGIConsoleNavigationTarget(item.href, item.accessState)
              : undefined;
            return (
              <div
                key={item.key}
                className="flex items-start gap-3 rounded-lg px-2 py-3 transition-colors hover:bg-muted/40"
              >
                <span
                  className={cn(
                    'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full border',
                    isComplete
                      ? 'border-success/30 bg-success/15 text-success'
                      : 'border-border text-muted-foreground'
                  )}
                >
                  {isAccessLoading ? (
                    <LoaderCircle className="size-3 animate-spin" />
                  ) : isComplete ? (
                    <Check className="size-3.5" />
                  ) : (
                    <Circle className="size-2.5" />
                  )}
                </span>
                <div className="min-w-0 flex-1">
                  <div className={cn('text-sm font-medium', isComplete && 'text-muted-foreground')}>
                    {item.title}
                  </div>
                  <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                    {item.description}
                  </p>
                </div>
                {!isComplete && item.accessState === 'forbidden' ? (
                  <span
                    className="mt-0.5 flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground"
                    title={tNavigation('permissionRequired')}
                  >
                    <LockKeyhole className="size-3" />
                    {tNavigation('permissionRequired')}
                  </span>
                ) : null}
                {!isComplete && isAccessLoading ? (
                  <span className="mt-0.5 flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
                    {tNavigation('checkingAccess')}
                  </span>
                ) : null}
                {!isComplete && hasAccessError ? (
                  <span className="mt-0.5 flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
                    <CircleAlert className="size-3" />
                    {tNavigation('accessCheckFailed')}
                  </span>
                ) : null}
                {!isComplete && !isBlocked && navigationTarget && item.action ? (
                  <Button asChild variant="ghost" size="xs" className="mt-0.5">
                    <Link
                      href={navigationTarget}
                      title={
                        item.accessState === 'setup_required'
                          ? tNavigation('setupRequired')
                          : undefined
                      }
                    >
                      {item.accessState === 'setup_required'
                        ? tNavigation('switchWorkspace')
                        : item.action}
                      <ArrowRight className="size-3.5" />
                    </Link>
                  </Button>
                ) : null}
              </div>
            );
          })}
        </CardContent>
      )}
    </Card>
  );
}

'use client';

import Link from 'next/link';
import {
  ArrowUpRight,
  Atom,
  BookOpen,
  LayoutGrid,
  CircleAlert,
  LoaderCircle,
  LockKeyhole,
  MessageSquare,
  Workflow,
} from 'lucide-react';
import { useT } from '@/i18n';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import type { WorkspaceHomeAccessState } from '@/hooks/console/use-workspace-home';
import { cn } from '@/lib/utils';
import { getZGIConsoleNavigationTarget } from '@/routes/console-navigation';

export function WorkspaceQuickActions({
  access,
}: {
  access: {
    chat: WorkspaceHomeAccessState;
    agent: WorkspaceHomeAccessState;
    knowledge: WorkspaceHomeAccessState;
    workflow: WorkspaceHomeAccessState;
    apps: WorkspaceHomeAccessState;
  };
}) {
  const t = useT('dashboard');
  const tNavigation = useT('navigation');
  const actions = [
    {
      title: tNavigation('chat'),
      description: t('stats.consoleHome.nextActions.startChatDescription'),
      href: '/console/work/chat',
      icon: MessageSquare,
      accessState: access.chat,
    },
    {
      title: tNavigation('agents'),
      description: t('stats.consoleHome.nextActions.createAgentDescription'),
      href: '/console/agents',
      icon: Atom,
      accessState: access.agent,
    },
    {
      title: tNavigation('datasets'),
      description: t('stats.consoleHome.nextActions.createKnowledgeDescription'),
      href: '/console/dataset',
      icon: BookOpen,
      accessState: access.knowledge,
    },
    {
      title: tNavigation('workflowAgents'),
      description: t('stats.consoleHome.onboarding.workflowDescription'),
      href: '/console/workflows',
      icon: Workflow,
      accessState: access.workflow,
    },
    {
      title: tNavigation('app'),
      description: t('stats.consoleHome.productEntries.appCenterDescription'),
      href: '/console/work/app',
      icon: LayoutGrid,
      accessState: access.apps,
    },
  ];

  return (
    <Card className="border-border/80 shadow-sm">
      <CardHeader className="pb-4">
        <CardTitle className="text-lg">{t('stats.consoleHome.quickStart.title')}</CardTitle>
        <CardDescription className="leading-6">
          {t('stats.consoleHome.quickStart.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-2 sm:grid-cols-2 xl:grid-cols-1">
        {actions.map(action => {
          const Icon = action.icon;
          const isForbidden = action.accessState === 'forbidden';
          const isLoading = action.accessState === 'loading';
          const hasAccessError = action.accessState === 'error';
          const isBlocked = isForbidden || isLoading || hasAccessError;
          const needsSetup = action.accessState === 'setup_required';
          const navigationTarget = getZGIConsoleNavigationTarget(
            action.href,
            action.accessState
          );
          const blockedReason = isForbidden
            ? tNavigation('permissionRequired')
            : isLoading
              ? tNavigation('checkingAccess')
              : tNavigation('accessCheckFailed');
          const content = (
            <>
              <span
                className={cn(
                  'flex size-9 shrink-0 items-center justify-center rounded-md',
                  isBlocked ? 'bg-muted text-muted-foreground/50' : 'bg-primary/10 text-primary'
                )}
              >
                <Icon className="size-4.5" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-1.5 text-sm font-medium text-foreground">
                  {action.title}
                  {isForbidden ? (
                    <LockKeyhole className="size-3.5 text-muted-foreground" />
                  ) : isLoading ? (
                    <LoaderCircle className="size-3.5 animate-spin text-muted-foreground" />
                  ) : hasAccessError ? (
                    <CircleAlert className="size-3.5 text-muted-foreground" />
                  ) : (
                    <ArrowUpRight className="size-3.5 text-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
                  )}
                  {needsSetup ? (
                    <span className="ml-auto text-[10px] font-medium text-warning">
                      {tNavigation('setupRequired')}
                    </span>
                  ) : null}
                  {isBlocked ? (
                    <span className="ml-auto text-[10px] font-medium text-muted-foreground">
                      {blockedReason}
                    </span>
                  ) : null}
                </span>
                <span className="mt-0.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {action.description}
                </span>
              </span>
            </>
          );
          const className = cn(
            'group flex items-start gap-3 rounded-lg border border-border/70 p-3 transition-colors',
            isBlocked
              ? 'cursor-not-allowed opacity-60'
              : 'hover:border-border-strong hover:bg-muted/40'
          );

          return isBlocked ? (
            <div
              key={action.href}
              aria-disabled="true"
              title={blockedReason}
              aria-label={`${action.title}: ${blockedReason}`}
              className={className}
            >
              {content}
            </div>
          ) : (
            <Link
              key={action.href}
              href={navigationTarget}
              className={className}
              title={needsSetup ? tNavigation('setupRequired') : undefined}
            >
              {content}
            </Link>
          );
        })}
      </CardContent>
    </Card>
  );
}

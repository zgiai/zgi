'use client';

import { Menu } from 'lucide-react';
import { useMemo } from 'react';
import { usePathname } from 'next/navigation';
// import { QuickThemeToggle } from '@/components/theme-switcher';
import { useT } from '@/i18n';
import { UserMenu } from './user-menu';
import { Logo } from '../logo';
import { Button } from '@/components/ui/button';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { useOrganizationStore } from '@/store/organization-store';
import { cn } from '@/lib/utils';

interface ConsoleHeaderProps {
  hidden?: boolean;
  onToggleMobileSidebar?: () => void;
}

export function ConsoleHeader({ hidden, onToggleMobileSidebar }: ConsoleHeaderProps) {
  const pathname = usePathname();
  const tNav = useT('navigation');
  const tDash = useT('dashboard');
  const currentWorkspace = useCurrentWorkspace();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const isDashboardRoute = pathname.startsWith('/dashboard');

  const pageTitle = useMemo(() => {
    const routeTitles: Array<{ match: (path: string) => boolean; title: string }> = [
      { match: path => path === '/console', title: tNav('home') },
      { match: path => path.startsWith('/console/agents'), title: tNav('agents') },
      { match: path => path.startsWith('/console/dataset'), title: tNav('datasets') },
      { match: path => path.startsWith('/console/files'), title: tNav('files') },
      { match: path => path.startsWith('/console/db'), title: tNav('dbs') },
      {
        match: path => path.startsWith('/console/developer/content-parse'),
        title: tNav('fileRecognition'),
      },
      { match: path => path.startsWith('/console/settings'), title: tNav('systemSettings') },
      { match: path => path.startsWith('/console/work/chat'), title: tNav('chat') },
      { match: path => path.startsWith('/console/work/image'), title: tNav('image') },
      { match: path => path.startsWith('/console/work/app'), title: tNav('app') },
      { match: path => path.startsWith('/console/work/task'), title: tNav('task') },
      { match: path => path.startsWith('/console/workspace'), title: tNav('workspaceManagement') },
      { match: path => path === '/dashboard', title: tDash('usage.title') },
      {
        match: path => path.startsWith('/dashboard/organization/workspaces'),
        title: tDash('items.workspaces'),
      },
      {
        match: path => path.startsWith('/dashboard/organization/contacts'),
        title: tDash('items.contacts'),
      },
      {
        match: path => path.startsWith('/dashboard/organization/permissions'),
        title: tDash('items.permissions'),
      },
      {
        match: path => path.startsWith('/dashboard/organization/aichat-skills'),
        title: tDash('items.aichatSkills'),
      },
      {
        match: path => path.startsWith('/dashboard/cost-center/overview'),
        title: tDash('items.billingOverview'),
      },
      {
        match: path => path.startsWith('/dashboard/cost-center/bills'),
        title: tDash('items.billingBills'),
      },
      {
        match: path => path.startsWith('/dashboard/provider'),
        title: tDash('items.llmProviders'),
      },
      {
        match: path => path.startsWith('/dashboard/channel'),
        title: tDash('items.channel'),
      },
      {
        match: path => path.startsWith('/dashboard/api-keys'),
        title: tDash('items.apiKeys'),
      },
      {
        match: path => path.startsWith('/dashboard/settings/model'),
        title: tDash('items.modelSettings'),
      },
      {
        match: path => path.startsWith('/dashboard/market'),
        title: tDash('items.marketplace'),
      },
    ];

    return (
      routeTitles.find(route => route.match(pathname))?.title ??
      (isDashboardRoute ? tDash('items.dashboard') : tNav('console'))
    );
  }, [isDashboardRoute, pathname, tDash, tNav]);

  const workspaceLabel = currentWorkspace?.name || tNav('switchWorkspace');
  const sectionLabel = isDashboardRoute ? tNav('dashboard') : tNav('console');
  const contextLabel = isDashboardRoute ? currentOrganization?.name || null : workspaceLabel;
  const contextPrefix = isDashboardRoute
    ? tNav('organizations')
    : `${tNav('current')} ${tNav('workspace')}`;

  if (hidden) {
    return null;
  }
  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-4 border-b border-border bg-background px-4 md:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <Button
          type="button"
          variant="outline"
          size="default"
          isIcon
          className="md:hidden"
          onClick={onToggleMobileSidebar}
        >
          <Menu className="size-4" />
          <span className="sr-only">Toggle sidebar</span>
        </Button>

        <div className="max-w-32 shrink-0">
          <Logo showName={false} routerToHome />
        </div>

        <div className="hidden h-6 w-px shrink-0 bg-border/70 md:block" />

        <div className="hidden min-w-0 md:flex md:flex-col">
          <div className="truncate text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {sectionLabel}
          </div>
          <div className="truncate text-sm font-semibold text-foreground">{pageTitle}</div>
        </div>
      </div>

      {contextLabel ? (
        <div
          className={cn(
            'hidden max-w-[240px] items-center rounded-full border border-border/70 bg-muted/30 px-2.5 py-1 text-[11px] text-muted-foreground lg:flex',
            'truncate'
          )}
        >
          <span className="truncate">
            {contextPrefix}: <span className="font-medium text-foreground">{contextLabel}</span>
          </span>
        </div>
      ) : null}

      <div className="flex items-center gap-4 md:ml-auto">
        {/* <QuickThemeToggle /> */}
        <UserMenu />
      </div>
    </header>
  );
}
